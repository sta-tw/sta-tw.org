package ingestion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"sta-backend/internal/admissions"
	"sta-backend/internal/auth"
	"sta-backend/internal/jobs"
	"sta-backend/internal/obs"
)

type PostgresRepository struct {
	pool                 *pgxpool.Pool
	admissionsRepository *admissions.PostgresRepository
}

func NewPostgresRepository(pool *pgxpool.Pool) (*PostgresRepository, error) {
	if pool == nil {
		return nil, errors.New("ingestion postgres pool is nil")
	}
	admissionsRepository, err := admissions.NewPostgresRepository(pool)
	if err != nil {
		return nil, err
	}
	return &PostgresRepository{pool: pool, admissionsRepository: admissionsRepository}, nil
}

func (r *PostgresRepository) IsAdmin(ctx context.Context, accountID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM account_roles WHERE account_id = $1 AND role = 'admin')`, accountID).Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) queueBrochureJob(ctx context.Context, adminID *uuid.UUID, academicYear int, schoolCode, storageKey, sha256Hex, processor string, now time.Time) (brochureJobRecord, error) {
	return r.queueDocumentJob(ctx, adminID, academicYear, schoolCode, storageKey, sha256Hex, processor, jobs.SourceTypeBrochure, "", "", now)
}

func (r *PostgresRepository) queueCandidateListJob(ctx context.Context, adminID *uuid.UUID, academicYear int, schoolCode, storageKey, sha256Hex, processor, sourceURL, programCode string, now time.Time) (brochureJobRecord, error) {
	return r.queueDocumentJob(ctx, adminID, academicYear, schoolCode, storageKey, sha256Hex, processor, jobs.SourceTypeCandidateList, sourceURL, programCode, now)
}

func (r *PostgresRepository) queueDocumentJob(ctx context.Context, adminID *uuid.UUID, academicYear int, schoolCode, storageKey, sha256Hex, processor, sourceType, sourceURL, programCode string, now time.Time) (brochureJobRecord, error) {
	if academicYear < 100 || academicYear > 999 || len(schoolCode) != 3 || len(sha256Hex) != 64 || strings.TrimSpace(storageKey) == "" || strings.TrimSpace(processor) == "" {
		return brochureJobRecord{}, ErrInvalid
	}
	if sourceType != jobs.SourceTypeBrochure && sourceType != jobs.SourceTypeCandidateList {
		return brochureJobRecord{}, ErrInvalid
	}
	if adminID != nil {
		isAdmin, err := r.IsAdmin(ctx, *adminID)
		if err != nil {
			return brochureJobRecord{}, err
		}
		if !isAdmin {
			return brochureJobRecord{}, ErrAdminRequired
		}
	}

	jobID := uuid.New()
	job := jobs.BrochureExtractJob{
		JobID:         jobID,
		AcademicYear:  academicYear,
		SchoolCode:    schoolCode,
		StorageKey:    storageKey,
		SHA256Hex:     sha256Hex,
		RequestedAt:   now.UTC(),
		ProcessorHint: processor,
		SourceType:    sourceType,
		SourceURL:     strings.TrimSpace(sourceURL),
		ProgramCode:   strings.TrimSpace(programCode),
		Traceparent:   obs.TraceparentFromContext(ctx),
	}
	if err := job.Validate(); err != nil {
		return brochureJobRecord{}, ErrInvalid
	}
	payload, err := json.Marshal(job)
	if err != nil {
		return brochureJobRecord{}, fmt.Errorf("marshal brochure job: %w", err)
	}
	storageKeyDigest := sha256.Sum256([]byte(storageKey))
	idempotencyKey := fmt.Sprintf("%s:%03d:%s:%s:%s:%s:%s", sourceType, academicYear, schoolCode, programCode, sha256Hex, processor, hex.EncodeToString(storageKeyDigest[:]))
	jobType := BrochureJobType
	if sourceType == jobs.SourceTypeCandidateList {
		jobType = CandidateListJobType
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return brochureJobRecord{}, fmt.Errorf("begin ingestion transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	var storedID uuid.UUID
	var status string
	var storedPayload []byte
	err = tx.QueryRow(ctx, `
		INSERT INTO ingestion_jobs (id, job_type, idempotency_key, payload, status, next_attempt_at)
		VALUES ($1, $2, $3, $4::jsonb, 'queued', NULL)
		ON CONFLICT (idempotency_key) DO UPDATE SET
			payload = CASE WHEN ingestion_jobs.status IN ('succeeded', 'running') THEN ingestion_jobs.payload ELSE EXCLUDED.payload END,
			status = CASE WHEN ingestion_jobs.status IN ('succeeded', 'running') THEN ingestion_jobs.status ELSE 'queued' END,
			next_attempt_at = NULL,
			last_error_code = NULL,
			last_error_message = NULL,
			updated_at = CURRENT_TIMESTAMP
		RETURNING id, status, payload
	`, jobID, jobType, idempotencyKey, string(payload)).Scan(&storedID, &status, &storedPayload)
	if err != nil {
		return brochureJobRecord{}, mapRepositoryError(err)
	}
	if err := json.Unmarshal(storedPayload, &job); err != nil {
		return brochureJobRecord{}, fmt.Errorf("decode stored brochure job: %w", err)
	}
	if err := job.Validate(); err != nil {
		return brochureJobRecord{}, fmt.Errorf("stored brochure job is invalid: %w", err)
	}
	if sourceType == jobs.SourceTypeBrochure {
		var runID uuid.UUID
		err = tx.QueryRow(ctx, `
			INSERT INTO brochure_extraction_runs
				(ingestion_job_id, academic_year, school_code, source_sha256_hex, processor_version, status)
			VALUES ($1, $2, $3, $4, $5, 'processing')
			ON CONFLICT (academic_year, school_code, source_sha256_hex, processor_version) DO UPDATE SET
				ingestion_job_id = EXCLUDED.ingestion_job_id,
				status = CASE WHEN brochure_extraction_runs.status IN ('pending_review', 'approved', 'rejected') THEN brochure_extraction_runs.status ELSE 'processing' END,
				error_code = CASE WHEN brochure_extraction_runs.status IN ('pending_review', 'approved', 'rejected') THEN brochure_extraction_runs.error_code ELSE NULL END,
				error_message = CASE WHEN brochure_extraction_runs.status IN ('pending_review', 'approved', 'rejected') THEN brochure_extraction_runs.error_message ELSE NULL END,
				updated_at = CURRENT_TIMESTAMP
			RETURNING id
		`, storedID, academicYear, schoolCode, sha256Hex, processor).Scan(&runID)
		if err != nil {
			return brochureJobRecord{}, mapRepositoryError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return brochureJobRecord{}, fmt.Errorf("commit ingestion transaction: %w", err)
	}
	return brochureJobRecord{Job: job, Status: status, ShouldPublish: status == "queued" || status == "retrying"}, nil
}

func (r *PostgresRepository) queueUploadedBrochureJob(ctx context.Context, createdBy *uuid.UUID, input admissions.BrochureDocumentInput, processor string, now time.Time, intakeChannel string) (brochureJobRecord, error) {
	if strings.TrimSpace(input.StorageKey) == "" || strings.TrimSpace(input.OriginalFileName) == "" ||
		input.FileSizeBytes <= 0 || len(input.SHA256) != 64 || strings.TrimSpace(processor) == "" ||
		(input.MIMEType != "" && input.MIMEType != "application/pdf") || admissions.ValidateOfficialURL(input.SourceURL) != nil ||
		(intakeChannel != UploadChannelAdmin && intakeChannel != UploadChannelExternal && intakeChannel != UploadChannelAISystem) {
		return brochureJobRecord{}, ErrInvalid
	}
	if createdBy != nil && intakeChannel == UploadChannelAdmin {
		isAdmin, err := r.IsAdmin(ctx, *createdBy)
		if err != nil {
			return brochureJobRecord{}, err
		}
		if !isAdmin {
			return brochureJobRecord{}, ErrAdminRequired
		}
	}
	if intakeChannel == UploadChannelExternal && (input.AcademicYear < 100 || input.AcademicYear > 999 || !threeDigitCode(input.SchoolCode) || input.SchoolCode == "000") {
		return brochureJobRecord{}, ErrInvalid
	}
	var createdByValue any
	if createdBy != nil {
		createdByValue = *createdBy
	}
	jobID := uuid.New()
	job := jobs.BrochureExtractJob{
		JobID:         jobID,
		UploadID:      jobID,
		InferIdentity: true,
		AcademicYear:  input.AcademicYear,
		SchoolCode:    strings.TrimSpace(input.SchoolCode),
		StorageKey:    input.StorageKey,
		SHA256Hex:     strings.ToLower(input.SHA256),
		RequestedAt:   now.UTC(),
		ProcessorHint: processor,
		SourceType:    jobs.SourceTypeBrochure,
		SourceURL:     strings.TrimSpace(input.SourceURL),
		Traceparent:   obs.TraceparentFromContext(ctx),
	}
	if err := job.Validate(); err != nil {
		return brochureJobRecord{}, ErrInvalid
	}
	payload, err := json.Marshal(job)
	if err != nil {
		return brochureJobRecord{}, fmt.Errorf("marshal upload-only brochure job: %w", err)
	}
	storageKeyDigest := sha256.Sum256([]byte(input.StorageKey))
	idempotencyKey := fmt.Sprintf("brochure-upload:%s:%s", job.SHA256Hex, hex.EncodeToString(storageKeyDigest[:]))
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return brochureJobRecord{}, fmt.Errorf("begin upload-only ingestion transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	var storedID uuid.UUID
	var status string
	var storedPayload []byte
	if err := tx.QueryRow(ctx, `
		INSERT INTO ingestion_jobs (id, job_type, idempotency_key, payload, status, next_attempt_at)
		VALUES ($1, $2, $3, $4::jsonb, 'queued', NULL)
		RETURNING id, status, payload
	`, jobID, BrochureJobType, idempotencyKey, string(payload)).Scan(&storedID, &status, &storedPayload); err != nil {
		return brochureJobRecord{}, mapRepositoryError(err)
	}
	if err := json.Unmarshal(storedPayload, &job); err != nil {
		return brochureJobRecord{}, fmt.Errorf("decode stored upload-only job: %w", err)
	}
	if err := job.Validate(); err != nil || job.JobID != storedID {
		return brochureJobRecord{}, ErrInvalid
	}
	mimeType := input.MIMEType
	if mimeType == "" {
		mimeType = "application/pdf"
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO brochure_uploads
			(id, ingestion_job_id, storage_key, original_file_name, mime_type, file_size_bytes, sha256_hex, source_url, intake_channel, status, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'queued', $10)
	`, job.UploadID, storedID, input.StorageKey, input.OriginalFileName, mimeType, input.FileSizeBytes, job.SHA256Hex, emptyUploadValue(input.SourceURL), intakeChannel, createdByValue); err != nil {
		return brochureJobRecord{}, mapRepositoryError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return brochureJobRecord{}, fmt.Errorf("commit upload-only ingestion transaction: %w", err)
	}
	return brochureJobRecord{Job: job, Status: status, ShouldPublish: status == "queued" || status == "retrying"}, nil
}

func (r *PostgresRepository) markDispatchStarted(ctx context.Context, jobID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE ingestion_jobs
		SET status = CASE WHEN status = 'succeeded' THEN status ELSE 'running' END,
			attempt_count = CASE WHEN status = 'succeeded' THEN attempt_count ELSE attempt_count + 1 END,
			next_attempt_at = NULL,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, jobID)
	return err
}

func (r *PostgresRepository) markDispatchFailed(ctx context.Context, jobID uuid.UUID, err error) error {
	message := "brochure extraction dispatch failed"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = strings.TrimSpace(err.Error())
	}
	if len(message) > 500 {
		message = message[:500]
	}
	_, updateErr := r.pool.Exec(ctx, `
		UPDATE ingestion_jobs
		SET status = 'retrying', last_error_code = 'dispatch_failed', last_error_message = $2,
			next_attempt_at = CURRENT_TIMESTAMP + INTERVAL '30 seconds', updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND status <> 'succeeded'
	`, jobID, message)
	return updateErr
}

func (r *PostgresRepository) GetJobStatus(ctx context.Context, jobID uuid.UUID) (JobStatus, error) {
	var item JobStatus
	var raw []byte
	var sourceType string
	err := r.pool.QueryRow(ctx, `
		SELECT id, job_type, COALESCE(payload->>'source_type', 'brochure'),
		       COALESCE(payload->>'academic_year', '0')::int, COALESCE(payload->>'school_code', ''),
		       status, attempt_count, COALESCE(last_error_code, ''), COALESCE(last_error_message, ''), created_at, updated_at, payload
		FROM ingestion_jobs
		WHERE id = $1
	`, jobID).Scan(&item.ID, &item.JobType, &sourceType, &item.AcademicYear, &item.SchoolCode,
		&item.Status, &item.AttemptCount, &item.LastErrorCode, &item.LastErrorMessage, &item.CreatedAt, &item.UpdatedAt, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return JobStatus{}, ErrNotFound
	}
	if err != nil {
		return JobStatus{}, fmt.Errorf("get extraction job status: %w", err)
	}
	item.SourceType = sourceType
	return item, nil
}

// ClaimNextJob is the HTTP transport counterpart of the RabbitMQ consumer.
// Deployments use one transport for a given queue; the claim is protected by
// SKIP LOCKED so multiple Python replicas cannot receive the same job.
func (r *PostgresRepository) ClaimNextJob(ctx context.Context, sourceType string, lease time.Duration) (jobs.BrochureExtractJob, error) {
	if sourceType == "" {
		sourceType = jobs.SourceTypeBrochure
	}
	if sourceType != jobs.SourceTypeBrochure && sourceType != jobs.SourceTypeCandidateList {
		return jobs.BrochureExtractJob{}, ErrInvalid
	}
	if lease <= 0 || lease > time.Hour {
		lease = 15 * time.Minute
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return jobs.BrochureExtractJob{}, fmt.Errorf("begin extraction job claim: %w", err)
	}
	defer tx.Rollback(ctx)
	var jobID uuid.UUID
	var payload []byte
	err = tx.QueryRow(ctx, `
		SELECT id, payload
		FROM ingestion_jobs
		WHERE (
			(status IN ('queued', 'retrying') AND (next_attempt_at IS NULL OR next_attempt_at <= CURRENT_TIMESTAMP))
			OR (status = 'running' AND locked_at IS NOT NULL AND locked_at <= CURRENT_TIMESTAMP AND attempt_count < max_attempts)
		)
		  AND COALESCE(payload->>'source_type', 'brochure') = $1
		ORDER BY created_at, id
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`, sourceType).Scan(&jobID, &payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return jobs.BrochureExtractJob{}, ErrNotFound
	}
	if err != nil {
		return jobs.BrochureExtractJob{}, fmt.Errorf("select extraction job: %w", err)
	}
	var job jobs.BrochureExtractJob
	if err := json.Unmarshal(payload, &job); err != nil {
		return jobs.BrochureExtractJob{}, fmt.Errorf("decode extraction job: %w", err)
	}
	if err := job.Validate(); err != nil || job.JobID != jobID || job.EffectiveSourceType() != sourceType {
		return jobs.BrochureExtractJob{}, ErrInvalid
	}
	if _, err := tx.Exec(ctx, `
		UPDATE ingestion_jobs
		SET status = 'running', attempt_count = attempt_count + 1,
		    locked_at = CURRENT_TIMESTAMP + ($2 * INTERVAL '1 second'),
		    next_attempt_at = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, jobID, lease.Seconds()); err != nil {
		return jobs.BrochureExtractJob{}, fmt.Errorf("lock extraction job: %w", err)
	}
	if job.UploadID != uuid.Nil {
		if _, err := tx.Exec(ctx, `
			UPDATE brochure_uploads
			SET status = 'processing', error_code = NULL, error_message = NULL, updated_at = CURRENT_TIMESTAMP
			WHERE id = $1 AND ingestion_job_id = $2 AND status IN ('queued', 'processing', 'failed')
		`, job.UploadID, jobID); err != nil {
			return jobs.BrochureExtractJob{}, fmt.Errorf("mark brochure upload processing: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return jobs.BrochureExtractJob{}, fmt.Errorf("commit extraction job claim: %w", err)
	}
	return job, nil
}

func (r *PostgresRepository) MarkJobFailure(ctx context.Context, jobID uuid.UUID, input JobFailureInput) error {
	code := strings.TrimSpace(input.Code)
	if code == "" {
		code = "worker_failed"
	}
	if len(code) > 64 || strings.ContainsAny(code, "\x00\r\n") {
		return ErrInvalid
	}
	message := strings.TrimSpace(input.Message)
	if message == "" {
		message = "extraction worker failed"
	}
	if len(message) > 500 {
		message = message[:500]
	}
	var status string
	err := r.pool.QueryRow(ctx, `
		UPDATE ingestion_jobs
		SET status = CASE
				WHEN $2 AND attempt_count < max_attempts THEN 'retrying'
				ELSE 'failed'
			END,
			next_attempt_at = CASE
				WHEN $2 AND attempt_count < max_attempts THEN CURRENT_TIMESTAMP + INTERVAL '30 seconds'
				ELSE NULL
			END,
			locked_at = NULL,
			last_error_code = $3,
			last_error_message = $4,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND status <> 'succeeded'
		RETURNING status
	`, jobID, input.Retryable, code, message).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		var currentStatus string
		if statusErr := r.pool.QueryRow(ctx, `SELECT status FROM ingestion_jobs WHERE id = $1`, jobID).Scan(&currentStatus); errors.Is(statusErr, pgx.ErrNoRows) {
			return ErrNotFound
		} else if statusErr != nil {
			return statusErr
		}
		if currentStatus == "succeeded" {
			return nil
		}
		return ErrInvalidStatus
	}
	if err != nil {
		return fmt.Errorf("mark extraction job failure: %w", err)
	}
	if _, err := r.pool.Exec(ctx, `
		UPDATE brochure_uploads u
		SET status = CASE WHEN $2 AND EXISTS (
			SELECT 1 FROM ingestion_jobs j WHERE j.id = u.ingestion_job_id AND j.status = 'retrying'
		) THEN 'processing' ELSE 'failed' END,
			error_code = $3, error_message = $4, updated_at = CURRENT_TIMESTAMP
		WHERE u.ingestion_job_id = $1
		  AND EXISTS (SELECT 1 FROM ingestion_jobs j WHERE j.id = u.ingestion_job_id AND j.status <> 'succeeded')
	`, jobID, input.Retryable, code, message); err != nil {
		return fmt.Errorf("mark brochure upload failure: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ListRuns(ctx context.Context, adminID uuid.UUID, query RunQuery) ([]Run, error) {
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrAdminRequired
	}
	if query.Limit < 1 || query.Limit > 100 {
		query.Limit = 50
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	conditions := []string{"TRUE"}
	args := make([]any, 0, 4)
	if query.AcademicYear > 0 {
		args = append(args, query.AcademicYear)
		conditions = append(conditions, fmt.Sprintf("r.academic_year = $%d", len(args)))
	}
	if query.Status != "" {
		args = append(args, query.Status)
		conditions = append(conditions, fmt.Sprintf("r.status = $%d", len(args)))
	}
	args = append(args, query.Limit, query.Offset)
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT r.id, r.ingestion_job_id, r.academic_year, r.school_code, r.source_sha256_hex,
		       r.processor_version, r.status, r.raw_extraction, r.error_code, r.error_message,
		       r.reviewed_by, r.reviewed_at, r.created_at, r.updated_at
		FROM brochure_extraction_runs r
		WHERE %s
		ORDER BY r.created_at DESC, r.id DESC
		LIMIT $%d OFFSET $%d
	`, strings.Join(conditions, " AND "), len(args)-1, len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("list brochure extraction runs: %w", err)
	}
	defer rows.Close()
	result := make([]Run, 0)
	for rows.Next() {
		item, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) GetRun(ctx context.Context, adminID, runID uuid.UUID) (Run, error) {
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return Run{}, err
	} else if !ok {
		return Run{}, ErrAdminRequired
	}
	item, err := r.loadRun(ctx, runID)
	if err != nil {
		return Run{}, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, run_id, program_code, extracted_data, source_page, confidence,
		       review_status, reviewed_by, reviewed_at, created_at, updated_at
		FROM brochure_extraction_candidates
		WHERE run_id = $1
		ORDER BY program_code
	`, runID)
	if err != nil {
		return Run{}, fmt.Errorf("list brochure extraction candidates: %w", err)
	}
	defer rows.Close()
	item.Candidates = make([]Candidate, 0)
	for rows.Next() {
		candidate, err := scanCandidate(rows)
		if err != nil {
			return Run{}, err
		}
		item.Candidates = append(item.Candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return Run{}, err
	}
	return item, nil
}

func (r *PostgresRepository) loadRun(ctx context.Context, runID uuid.UUID) (Run, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, ingestion_job_id, academic_year, school_code, source_sha256_hex,
		       processor_version, status, raw_extraction, error_code, error_message,
		       reviewed_by, reviewed_at, created_at, updated_at
		FROM brochure_extraction_runs WHERE id = $1
	`, runID)
	item, err := scanRun(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	return item, err
}

func (r *PostgresRepository) ReviewRun(ctx context.Context, adminID, runID uuid.UUID, input ReviewInput) (Run, error) {
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return Run{}, err
	} else if !ok {
		return Run{}, ErrAdminRequired
	}
	if input.Approved {
		// Every approved candidate must be materialized together with its exact
		// reviewed program form. Bulk approval cannot provide that per-candidate
		// evidence, so approvals deliberately go through ReviewCandidate.
		return Run{}, ErrInvalidStatus
	}
	status := RunStatusRejected
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Run{}, err
	}
	defer tx.Rollback(ctx)
	var currentStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM brochure_extraction_runs WHERE id = $1 FOR UPDATE`, runID).Scan(&currentStatus); errors.Is(err, pgx.ErrNoRows) {
		return Run{}, ErrNotFound
	} else if err != nil {
		return Run{}, err
	} else if currentStatus != RunStatusPending {
		return Run{}, ErrInvalidStatus
	}
	if _, err := tx.Exec(ctx, `
		UPDATE brochure_extraction_runs
		SET status = $2, reviewed_by = $3, reviewed_at = CURRENT_TIMESTAMP,
			error_code = 'admin_rejected', error_message = $4,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, runID, status, adminID, strings.TrimSpace(input.Reason)); err != nil {
		return Run{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE brochure_extraction_candidates SET review_status = $2, reviewed_by = $3, reviewed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE run_id = $1 AND review_status = 'pending'`, runID, mapCandidateStatus(status), adminID); err != nil {
		return Run{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Run{}, err
	}
	return r.loadRun(ctx, runID)
}

func (r *PostgresRepository) ReviewCandidate(ctx context.Context, adminID, candidateID uuid.UUID, input ReviewInput) (Candidate, error) {
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return Candidate{}, err
	} else if !ok {
		return Candidate{}, ErrAdminRequired
	}
	if !input.Approved && strings.TrimSpace(input.Reason) == "" {
		return Candidate{}, ErrInvalid
	}
	status := CandidateRejected
	if input.Approved {
		status = CandidateApproved
		if input.Program == nil {
			return Candidate{}, ErrInvalid
		}
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Candidate{}, err
	}
	defer tx.Rollback(ctx)
	var candidate Candidate
	var runStatus string
	var academicYear int
	var schoolCode string
	err = tx.QueryRow(ctx, `
		SELECT c.id, c.run_id, c.program_code, c.extracted_data, c.source_page, c.confidence,
		       c.review_status, c.reviewed_by, c.reviewed_at, c.created_at, c.updated_at,
		       r.status, r.academic_year, r.school_code
		FROM brochure_extraction_candidates c
		JOIN brochure_extraction_runs r ON r.id = c.run_id
		WHERE c.id = $1
		FOR UPDATE OF c, r
	`, candidateID).Scan(
		&candidate.ID, &candidate.RunID, &candidate.ProgramCode, &candidate.ExtractedData,
		&candidate.SourcePage, &candidate.Confidence, &candidate.ReviewStatus,
		&candidate.ReviewedBy, &candidate.ReviewedAt, &candidate.CreatedAt, &candidate.UpdatedAt,
		&runStatus, &academicYear, &schoolCode,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Candidate{}, ErrNotFound
	}
	if err != nil {
		return Candidate{}, err
	}
	if runStatus != RunStatusPending {
		return Candidate{}, ErrInvalidStatus
	}
	if candidate.ReviewStatus != CandidatePending {
		return Candidate{}, ErrInvalidStatus
	}
	if input.Approved {
		program := *input.Program
		if (program.AcademicYear != 0 && program.AcademicYear != academicYear) ||
			(program.SchoolCode != "" && program.SchoolCode != schoolCode) ||
			(program.ProgramCode != "" && program.ProgramCode != candidate.ProgramCode) {
			return Candidate{}, ErrInvalid
		}
		program.AcademicYear = academicYear
		program.SchoolCode = schoolCode
		program.ProgramCode = candidate.ProgramCode
		reason := strings.TrimSpace(input.Reason)
		if reason == "" {
			reason = "本地擷取候選已人工確認"
		}
		if _, err := r.admissionsRepository.UpsertProgramsInTx(ctx, tx, adminID, admissions.ProgramBatchInput{
			Reason: reason,
			Items:  []admissions.ProgramInput{program},
		}); err != nil {
			if errors.Is(err, admissions.ErrInvalidProgram) || errors.Is(err, admissions.ErrNotFound) {
				return Candidate{}, ErrInvalid
			}
			return Candidate{}, err
		}
	}
	err = tx.QueryRow(ctx, `
		UPDATE brochure_extraction_candidates
		SET review_status = $2, reviewed_by = $3, reviewed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		RETURNING id, run_id, program_code, extracted_data, source_page, confidence,
		          review_status, reviewed_by, reviewed_at, created_at, updated_at
	`, candidateID, status, adminID).Scan(
		&candidate.ID, &candidate.RunID, &candidate.ProgramCode, &candidate.ExtractedData,
		&candidate.SourcePage, &candidate.Confidence, &candidate.ReviewStatus,
		&candidate.ReviewedBy, &candidate.ReviewedAt, &candidate.CreatedAt, &candidate.UpdatedAt,
	)
	if err != nil {
		return Candidate{}, err
	}
	var pendingCount, rejectedCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE review_status = 'pending'),
		       count(*) FILTER (WHERE review_status = 'rejected')
		FROM brochure_extraction_candidates WHERE run_id = $1
	`, candidate.RunID).Scan(&pendingCount, &rejectedCount); err != nil {
		return Candidate{}, err
	}
	if pendingCount == 0 {
		runStatus = RunStatusApproved
		var errorCode, errorMessage *string
		if rejectedCount > 0 {
			runStatus = RunStatusRejected
			code := "candidate_rejected"
			message := strings.TrimSpace(input.Reason)
			errorCode, errorMessage = &code, &message
		}
		if _, err := tx.Exec(ctx, `
			UPDATE brochure_extraction_runs
			SET status = $2, reviewed_by = $3, reviewed_at = CURRENT_TIMESTAMP,
			    error_code = $4, error_message = $5,
			    updated_at = CURRENT_TIMESTAMP
			WHERE id = $1
		`, candidate.RunID, runStatus, adminID, errorCode, errorMessage); err != nil {
			return Candidate{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Candidate{}, err
	}
	return candidate, nil
}

func (r *PostgresRepository) RequeueJob(ctx context.Context, adminID, jobID uuid.UUID) (jobs.BrochureExtractJob, error) {
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return jobs.BrochureExtractJob{}, err
	} else if !ok {
		return jobs.BrochureExtractJob{}, ErrAdminRequired
	}
	var payload []byte
	err := r.pool.QueryRow(ctx, `
		UPDATE ingestion_jobs
		SET status = 'queued', next_attempt_at = NULL, last_error_code = NULL, last_error_message = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND status IN ('queued', 'running', 'retrying', 'failed', 'dead_letter')
		RETURNING payload
	`, jobID).Scan(&payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return jobs.BrochureExtractJob{}, ErrInvalidStatus
	}
	if err != nil {
		return jobs.BrochureExtractJob{}, err
	}
	var job jobs.BrochureExtractJob
	if err := json.Unmarshal(payload, &job); err != nil {
		return jobs.BrochureExtractJob{}, err
	}
	if err := job.Validate(); err != nil {
		return jobs.BrochureExtractJob{}, err
	}
	return job, nil
}

func (r *PostgresRepository) ApplyExtractionResult(ctx context.Context, result jobs.BrochureExtractionResult) error {
	if err := result.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if result.UploadID != uuid.Nil {
		return r.applyUploadedBrochureExtractionResult(ctx, result)
	}
	rawResult, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal brochure extraction result: %w", err)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var jobType, jobStatus string
	var payload []byte
	err = tx.QueryRow(ctx, `SELECT job_type, status, payload FROM ingestion_jobs WHERE id = $1 FOR UPDATE`, result.JobID).Scan(&jobType, &jobStatus, &payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if jobType != BrochureJobType {
		return fmt.Errorf("%w: unexpected job type", ErrInvalid)
	}
	var job jobs.BrochureExtractJob
	if err := json.Unmarshal(payload, &job); err != nil {
		return fmt.Errorf("%w: stored job payload is invalid", ErrInvalid)
	}
	if job.AcademicYear != result.AcademicYear || job.SchoolCode != result.SchoolCode || job.SHA256Hex != result.SHA256Hex || job.ProcessorHint != "" && job.ProcessorHint != result.Processor {
		return fmt.Errorf("%w: extraction result does not match job", ErrInvalid)
	}
	var runID uuid.UUID
	if err := tx.QueryRow(ctx, `
		SELECT id FROM brochure_extraction_runs
		WHERE ingestion_job_id = $1 AND academic_year = $2 AND school_code = $3 AND source_sha256_hex = $4
		FOR UPDATE
	`, result.JobID, result.AcademicYear, result.SchoolCode, result.SHA256Hex).Scan(&runID); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if jobStatus != "succeeded" {
		if _, err := tx.Exec(ctx, `
			UPDATE brochure_extraction_runs
			SET processor_version = $2, status = 'pending_review', raw_extraction = $3::jsonb,
				error_code = NULL, error_message = NULL, updated_at = CURRENT_TIMESTAMP
			WHERE id = $1
		`, runID, result.Processor, string(rawResult)); err != nil {
			return err
		}
		for _, candidate := range result.Candidates {
			candidateData, err := json.Marshal(candidate.Data)
			if err != nil {
				return fmt.Errorf("marshal extraction candidate: %w", err)
			}
			var sourcePage any
			if candidate.SourcePage > 0 {
				sourcePage = candidate.SourcePage
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO brochure_extraction_candidates
					(run_id, program_code, extracted_data, source_page, confidence)
				VALUES ($1, $2, $3::jsonb, $4, $5)
				ON CONFLICT (run_id, program_code) DO UPDATE SET
					extracted_data = EXCLUDED.extracted_data,
					source_page = EXCLUDED.source_page,
					confidence = EXCLUDED.confidence,
					updated_at = CURRENT_TIMESTAMP
			`, runID, candidate.ProgramCode, string(candidateData), sourcePage, candidate.Confidence); err != nil {
				return mapRepositoryError(err)
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE ingestion_jobs
			SET status = 'succeeded', next_attempt_at = NULL, last_error_code = NULL, last_error_message = NULL, updated_at = CURRENT_TIMESTAMP
			WHERE id = $1
		`, result.JobID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) applyUploadedBrochureExtractionResult(ctx context.Context, result jobs.BrochureExtractionResult) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	result, err = r.alignUploadedBrochureSchool(ctx, tx, result)
	if err != nil {
		return err
	}
	rawResult, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal upload-only brochure extraction result: %w", err)
	}
	var jobType, jobStatus string
	var payload []byte
	err = tx.QueryRow(ctx, `
		SELECT job_type, status, payload
		FROM ingestion_jobs WHERE id = $1 FOR UPDATE
	`, result.JobID).Scan(&jobType, &jobStatus, &payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if jobType != BrochureJobType {
		return fmt.Errorf("%w: unexpected upload-only job type", ErrInvalid)
	}
	var job jobs.BrochureExtractJob
	if err := json.Unmarshal(payload, &job); err != nil {
		return fmt.Errorf("%w: stored upload-only job payload is invalid", ErrInvalid)
	}
	if job.UploadID != result.UploadID || job.SHA256Hex != result.SHA256Hex || !job.InferIdentity {
		return fmt.Errorf("%w: upload-only extraction result does not match job", ErrInvalid)
	}
	if jobStatus == "succeeded" {
		return tx.Commit(ctx)
	}
	var uploadID uuid.UUID
	err = tx.QueryRow(ctx, `
		UPDATE brochure_uploads
		SET detected_academic_year = NULLIF($2, 0),
			detected_school_code = NULLIF($3, ''),
			detected_school_name = COALESCE(NULLIF($4, ''), '-'),
			status = 'pending_review', raw_extraction = $5::jsonb,
			error_code = NULL, error_message = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND ingestion_job_id = $6
		RETURNING id
	`, result.UploadID, result.AcademicYear, strings.TrimSpace(result.SchoolCode), strings.TrimSpace(result.SchoolName), string(rawResult), result.JobID).Scan(&uploadID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return mapRepositoryError(err)
	}
	if uploadID != result.UploadID {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `
		UPDATE ingestion_jobs
		SET status = 'succeeded', next_attempt_at = NULL, last_error_code = NULL,
			last_error_message = NULL, locked_at = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, result.JobID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// alignUploadedBrochureSchool replaces an extractor's free-form institution
// label with the canonical school master entry. A brochure often names the
// issuing committee or a college on its cover; the application identity must
// still be the university in the fixed brochure roster.
func (r *PostgresRepository) alignUploadedBrochureSchool(ctx context.Context, tx pgx.Tx, result jobs.BrochureExtractionResult) (jobs.BrochureExtractionResult, error) {
	target := normalizeBrochureSchoolName(result.SchoolName)
	rows, err := tx.Query(ctx, `
		SELECT s.school_code, s.school_name
		FROM schools s
		JOIN brochure_discovery_school_roster roster ON roster.school_code = s.school_code
		WHERE s.is_active = TRUE
		ORDER BY char_length(s.school_name) DESC, s.school_code
	`)
	if err != nil {
		return result, fmt.Errorf("load brochure school roster: %w", err)
	}
	defer rows.Close()

	type schoolEntry struct {
		code string
		name string
	}
	var byCode *schoolEntry
	var best *schoolEntry
	bestScore := 0
	for rows.Next() {
		var entry schoolEntry
		if err := rows.Scan(&entry.code, &entry.name); err != nil {
			return result, fmt.Errorf("scan brochure school roster: %w", err)
		}
		if entry.code == strings.TrimSpace(result.SchoolCode) {
			copy := entry
			byCode = &copy
		}
		if target == "" {
			continue
		}
		score := brochureSchoolMatchScore(target, normalizeBrochureSchoolName(entry.name))
		if score > bestScore {
			copy := entry
			best = &copy
			bestScore = score
		}
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("iterate brochure school roster: %w", err)
	}
	if best == nil {
		best = byCode
	}
	if best == nil {
		return result, nil
	}

	result.SchoolCode = best.code
	result.SchoolName = best.name
	for index := range result.Candidates {
		if result.Candidates[index].Data == nil {
			continue
		}
		result.Candidates[index].Data["school_code"] = best.code
		result.Candidates[index].Data["school_name"] = best.name
	}
	return result, nil
}

func normalizeBrochureSchoolName(value string) string {
	value = strings.ReplaceAll(value, "台", "臺")
	var normalized strings.Builder
	for _, character := range value {
		if unicode.IsSpace(character) || unicode.IsPunct(character) || unicode.IsSymbol(character) {
			continue
		}
		normalized.WriteRune(character)
	}
	return normalized.String()
}

func brochureSchoolMatchScore(target, canonical string) int {
	if target == "" || canonical == "" {
		return 0
	}
	if target == canonical {
		return 100000 + len([]rune(canonical))
	}
	if strings.Contains(target, canonical) {
		return 50000 + len([]rune(canonical))
	}
	if len([]rune(target)) >= 4 && strings.Contains(canonical, target) {
		return 20000 + len([]rune(target))
	}
	return 0
}

func (r *PostgresRepository) ListBrochureUploads(ctx context.Context, adminID uuid.UUID, status string, limit, offset int) ([]BrochureUpload, error) {
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrAdminRequired
	}
	if status != "" && !validUploadStatus(status) {
		return nil, ErrInvalid
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if offset < 0 || offset > 10000 {
		return nil, ErrInvalid
	}
	// This endpoint backs the admin PDF-upload review queue. AI System
	// submissions go through the exact same infer-then-confirm pipeline as an
	// admin's own upload, so they belong in the same queue; the older shared-
	// secret external_api channel still has its own separate review path
	// (brochure-runs / brochure-candidates) and is intentionally excluded.
	conditions := []string{"intake_channel = ANY($1)"}
	args := make([]any, 0, 4)
	args = append(args, []string{UploadChannelAdmin, UploadChannelAISystem})
	if status != "" {
		args = append(args, status)
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
	}
	args = append(args, limit, offset)
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT id, ingestion_job_id, original_file_name, mime_type, file_size_bytes,
		       sha256_hex, source_url, intake_channel, detected_academic_year, detected_school_code,
		       detected_school_name, status, error_code, error_message,
		       created_at, updated_at, reviewed_at, NULL::jsonb
		FROM brochure_uploads
		WHERE %s
		ORDER BY created_at DESC, id DESC
		LIMIT $%d OFFSET $%d
	`, strings.Join(conditions, " AND "), len(args)-1, len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("list brochure uploads: %w", err)
	}
	defer rows.Close()
	result := make([]BrochureUpload, 0)
	for rows.Next() {
		item, err := scanBrochureUpload(rows, false)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// ResolveAISystemToken authenticates a personal API token issued to an
// account holding the ai_system role. The token must be active (not revoked)
// and its owning account must still hold the role and be active itself —
// revoking either independently is enough to cut off access. On success the
// token's last_used_at is refreshed for auditability.
func (r *PostgresRepository) ResolveAISystemToken(ctx context.Context, token string) (uuid.UUID, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return uuid.Nil, ErrNotFound
	}
	hash := auth.HashOpaqueToken(token)
	var accountID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT t.account_id
		FROM account_api_tokens t
		JOIN account_roles r ON r.account_id = t.account_id AND r.role = 'ai_system'
		JOIN accounts a ON a.id = t.account_id AND a.account_status = 'active'
		WHERE t.token_hash = $1 AND t.revoked_at IS NULL
	`, hash).Scan(&accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("resolve ai_system token: %w", err)
	}
	if _, err := r.pool.Exec(ctx, `
		UPDATE account_api_tokens SET last_used_at = CURRENT_TIMESTAMP
		WHERE account_id = $1 AND token_hash = $2 AND revoked_at IS NULL
	`, accountID, hash); err != nil {
		return uuid.Nil, fmt.Errorf("update ai_system token last_used_at: %w", err)
	}
	return accountID, nil
}

func (r *PostgresRepository) GetBrochureUpload(ctx context.Context, adminID, uploadID uuid.UUID) (BrochureUpload, error) {
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return BrochureUpload{}, err
	} else if !ok {
		return BrochureUpload{}, ErrAdminRequired
	}
	return r.loadBrochureUpload(ctx, uploadID, true)
}

// GetBrochureUploadStorageKey is intentionally separate from BrochureUpload's
// JSON model so storage object keys are never exposed in normal admin API
// responses. It is used only to create an authenticated, short-lived download
// URL for manual review.
func (r *PostgresRepository) GetBrochureUploadStorageKey(ctx context.Context, adminID, uploadID uuid.UUID) (string, error) {
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return "", err
	} else if !ok {
		return "", ErrAdminRequired
	}
	var storageKey string
	if err := r.pool.QueryRow(ctx, `SELECT storage_key FROM brochure_uploads WHERE id = $1`, uploadID).Scan(&storageKey); errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	} else if err != nil {
		return "", err
	}
	return storageKey, nil
}

func (r *PostgresRepository) loadBrochureUpload(ctx context.Context, uploadID uuid.UUID, includeCandidates bool) (BrochureUpload, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, ingestion_job_id, original_file_name, mime_type, file_size_bytes,
		       sha256_hex, source_url, intake_channel, detected_academic_year, detected_school_code,
		       detected_school_name, status, error_code, error_message,
		       created_at, updated_at, reviewed_at, raw_extraction
		FROM brochure_uploads WHERE id = $1
	`, uploadID)
	return scanBrochureUpload(row, includeCandidates)
}

func scanBrochureUpload(row interface{ Scan(...any) error }, includeCandidates bool) (BrochureUpload, error) {
	var item BrochureUpload
	var detectedYear *int
	var detectedSchoolCode *string
	var errorCode *string
	var errorMessage *string
	var raw []byte
	if err := row.Scan(
		&item.ID, &item.IngestionJobID, &item.OriginalFileName, &item.MIMEType,
		&item.FileSizeBytes, &item.SHA256, &item.SourceURL, &item.IntakeChannel, &detectedYear,
		&detectedSchoolCode, &item.DetectedSchoolName, &item.Status, &errorCode,
		&errorMessage, &item.CreatedAt, &item.UpdatedAt, &item.ReviewedAt, &raw,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BrochureUpload{}, ErrNotFound
		}
		return BrochureUpload{}, err
	}
	item.DetectedAcademicYear = detectedYear
	if detectedSchoolCode != nil {
		item.DetectedSchoolCode = *detectedSchoolCode
	}
	if errorCode != nil {
		item.ErrorCode = *errorCode
	}
	if errorMessage != nil {
		item.ErrorMessage = *errorMessage
	}
	if includeCandidates && len(raw) > 0 {
		var result jobs.BrochureExtractionResult
		if err := json.Unmarshal(raw, &result); err == nil {
			item.Candidates = make([]UploadCandidate, 0, len(result.Candidates))
			for _, candidate := range result.Candidates {
				data, err := json.Marshal(candidate.Data)
				if err != nil {
					return BrochureUpload{}, err
				}
				item.Candidates = append(item.Candidates, UploadCandidate{
					ProgramCode: candidate.ProgramCode,
					Data:        json.RawMessage(data),
					SourcePage:  candidate.SourcePage,
					Confidence:  candidate.Confidence,
				})
			}
		}
	}
	return item, nil
}

func (r *PostgresRepository) RejectBrochureUpload(ctx context.Context, adminID, uploadID uuid.UUID, reason string) (BrochureUpload, error) {
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return BrochureUpload{}, err
	} else if !ok {
		return BrochureUpload{}, ErrAdminRequired
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || len([]rune(reason)) > 2000 {
		return BrochureUpload{}, ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return BrochureUpload{}, err
	}
	defer tx.Rollback(ctx)
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM brochure_uploads WHERE id = $1 FOR UPDATE`, uploadID).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return BrochureUpload{}, ErrNotFound
	} else if err != nil {
		return BrochureUpload{}, err
	} else if status != "pending_review" {
		return BrochureUpload{}, ErrInvalidStatus
	}
	if _, err := tx.Exec(ctx, `
		UPDATE brochure_uploads
		SET status = 'rejected', error_code = 'admin_rejected', error_message = $2,
			reviewed_by = $3, reviewed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, uploadID, reason, adminID); err != nil {
		return BrochureUpload{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BrochureUpload{}, err
	}
	return r.loadBrochureUpload(ctx, uploadID, true)
}

func (r *PostgresRepository) ConfirmBrochureUpload(ctx context.Context, adminID, uploadID uuid.UUID, input BrochureUploadConfirmInput) (BrochureUploadConfirmResult, error) {
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return BrochureUploadConfirmResult{}, err
	} else if !ok {
		return BrochureUploadConfirmResult{}, ErrAdminRequired
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		reason = "人工確認簡章資料"
	}
	if len([]rune(reason)) > 2000 || input.AcademicYear < 100 || input.AcademicYear > 999 || !threeDigitCode(input.SchoolCode) || input.SchoolCode == "000" || len(input.Programs) == 0 || len(input.Programs) > 500 {
		return BrochureUploadConfirmResult{}, ErrInvalid
	}
	programInputs := make([]admissions.ProgramInput, len(input.Programs))
	copy(programInputs, input.Programs)
	for index := range programInputs {
		programInputs[index].AcademicYear = input.AcademicYear
		programInputs[index].SchoolCode = input.SchoolCode
	}
	batch := admissions.ProgramBatchInput{Reason: reason, Items: programInputs}
	if err := batch.Validate(); err != nil {
		return BrochureUploadConfirmResult{}, ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return BrochureUploadConfirmResult{}, err
	}
	defer tx.Rollback(ctx)
	var upload admissionsBrochureUploadRow
	if err := tx.QueryRow(ctx, `
		SELECT id, ingestion_job_id, storage_key, original_file_name, mime_type,
		       file_size_bytes, sha256_hex, source_url, detected_school_name, status
		FROM brochure_uploads WHERE id = $1 FOR UPDATE
	`, uploadID).Scan(
		&upload.ID, &upload.JobID, &upload.StorageKey, &upload.OriginalFileName,
		&upload.MIMEType, &upload.FileSizeBytes, &upload.SHA256, &upload.SourceURL,
		&upload.DetectedSchoolName, &upload.Status,
	); errors.Is(err, pgx.ErrNoRows) {
		return BrochureUploadConfirmResult{}, ErrNotFound
	} else if err != nil {
		return BrochureUploadConfirmResult{}, err
	} else if upload.Status != "pending_review" {
		return BrochureUploadConfirmResult{}, ErrInvalidStatus
	}
	programs, err := r.admissionsRepository.UpsertProgramsInTx(ctx, tx, adminID, batch)
	if err != nil {
		if errors.Is(err, admissions.ErrInvalidProgram) || errors.Is(err, admissions.ErrNotFound) {
			return BrochureUploadConfirmResult{}, ErrInvalid
		}
		return BrochureUploadConfirmResult{}, err
	}
	programs, err = r.admissionsRepository.PublishProgramsInTx(ctx, tx, adminID, programs, reason)
	if err != nil {
		return BrochureUploadConfirmResult{}, err
	}
	brochure, _, err := r.admissionsRepository.PublishBrochureInTx(ctx, tx, adminID, admissions.BrochureDocumentInput{
		AcademicYear:     input.AcademicYear,
		SchoolCode:       input.SchoolCode,
		OriginalFileName: upload.OriginalFileName,
		StorageKey:       upload.StorageKey,
		MIMEType:         upload.MIMEType,
		FileSizeBytes:    upload.FileSizeBytes,
		SHA256:           upload.SHA256,
		SourceURL:        upload.SourceURL,
	}, reason)
	if err != nil {
		return BrochureUploadConfirmResult{}, err
	}
	schoolName := upload.DetectedSchoolName
	if len(programs) > 0 && strings.TrimSpace(programs[0].SchoolName) != "" {
		schoolName = programs[0].SchoolName
	}
	if _, err := tx.Exec(ctx, `
		UPDATE brochure_uploads
		SET detected_academic_year = $2, detected_school_code = $3,
			detected_school_name = COALESCE(NULLIF($4, ''), '-'), status = 'approved',
			reviewed_by = $5, reviewed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, uploadID, input.AcademicYear, input.SchoolCode, schoolName, adminID); err != nil {
		return BrochureUploadConfirmResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BrochureUploadConfirmResult{}, err
	}
	confirmed, err := r.loadBrochureUpload(ctx, uploadID, true)
	if err != nil {
		return BrochureUploadConfirmResult{}, err
	}
	return BrochureUploadConfirmResult{Upload: confirmed, Brochure: brochure, Programs: programs}, nil
}

type admissionsBrochureUploadRow struct {
	ID                 uuid.UUID
	JobID              uuid.UUID
	StorageKey         string
	OriginalFileName   string
	MIMEType           string
	FileSizeBytes      int64
	SHA256             string
	SourceURL          string
	DetectedSchoolName string
	Status             string
}

func validUploadStatus(value string) bool {
	switch value {
	case "queued", "processing", "pending_review", "approved", "rejected", "failed":
		return true
	default:
		return false
	}
}

func emptyUploadValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}

func scanRun(row interface{ Scan(...any) error }) (Run, error) {
	var item Run
	var raw []byte
	err := row.Scan(&item.ID, &item.IngestionJobID, &item.AcademicYear, &item.SchoolCode,
		&item.SourceSHA256, &item.ProcessorVersion, &item.Status, &raw, &item.ErrorCode,
		&item.ErrorMessage, &item.ReviewedBy, &item.ReviewedAt, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return Run{}, err
	}
	if len(raw) > 0 {
		item.RawExtraction = json.RawMessage(append([]byte(nil), raw...))
	}
	return item, nil
}

func scanCandidate(row interface{ Scan(...any) error }) (Candidate, error) {
	var item Candidate
	var raw []byte
	err := row.Scan(&item.ID, &item.RunID, &item.ProgramCode, &raw, &item.SourcePage,
		&item.Confidence, &item.ReviewStatus, &item.ReviewedBy, &item.ReviewedAt,
		&item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return Candidate{}, err
	}
	item.ExtractedData = json.RawMessage(append([]byte(nil), raw...))
	return item, nil
}

func mapCandidateStatus(runStatus string) string {
	if runStatus == RunStatusApproved {
		return CandidateApproved
	}
	return CandidateRejected
}

func mapRepositoryError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505", "23514", "22001":
			return ErrInvalid
		case "23503":
			return ErrNotFound
		}
	}
	return err
}
