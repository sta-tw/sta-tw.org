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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"sta-backend/internal/admissions"
	"sta-backend/internal/jobs"
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
