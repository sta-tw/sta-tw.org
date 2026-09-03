package results

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"sta-backend/internal/auth"
	"sta-backend/internal/jobs"
)

type PostgresRepository struct {
	pool      *pgxpool.Pool
	lookupKey []byte
}

func NewPostgresRepository(pool *pgxpool.Pool, lookupKeys ...[]byte) (*PostgresRepository, error) {
	if pool == nil {
		return nil, errors.New("postgres pool is nil")
	}
	var lookupKey []byte
	if len(lookupKeys) > 0 {
		lookupKey = append([]byte(nil), lookupKeys[0]...)
	}
	return &PostgresRepository{pool: pool, lookupKey: lookupKey}, nil
}

func (r *PostgresRepository) ImportOfficialBatch(ctx context.Context, adminID uuid.UUID, input ImportBatchInput) (uuid.UUID, error) {
	isAdmin, err := r.IsAdmin(ctx, adminID)
	if err != nil {
		return uuid.Nil, err
	}
	if !isAdmin {
		return uuid.Nil, ErrAdminRequired
	}
	if len(r.lookupKey) != 32 {
		return uuid.Nil, ErrInvalidInput
	}
	if err := input.Validate(); err != nil {
		return uuid.Nil, err
	}
	input.SourceSHA256 = strings.ToLower(strings.TrimSpace(input.SourceSHA256))
	input.SourceURL = strings.TrimSpace(input.SourceURL)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("begin official result import: %w", err)
	}
	defer tx.Rollback(ctx)
	var batchIDText string
	err = tx.QueryRow(ctx, `
		INSERT INTO official_result_batches (academic_year, school_code, source_url, source_sha256_hex, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text
	`, input.AcademicYear, input.SchoolCode, emptyDash(input.SourceURL), input.SourceSHA256, ResultBatchStatusPendingReview).Scan(&batchIDText)
	if err != nil {
		return uuid.Nil, mapResultError(err)
	}
	batchID, err := uuid.Parse(batchIDText)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse official batch id: %w", err)
	}
	for _, row := range input.Rows {
		if row.AcademicYear != input.AcademicYear || row.SchoolCode != input.SchoolCode || row.ProgramCode == "" {
			return uuid.Nil, ErrInvalidInput
		}
		candidate, err := NormalizeCandidateNumber(row.CandidateNumber)
		if err != nil {
			return uuid.Nil, ErrInvalidInput
		}
		hash, err := auth.LookupHash(r.lookupKey, candidate)
		if err != nil {
			return uuid.Nil, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO official_results
				(batch_id, academic_year, school_code, program_code, candidate_number_lookup_hash,
				 candidate_number_last4, masked_name, result_status, official_rank, quota, source_page)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11, 0))
		`, batchID, row.AcademicYear, row.SchoolCode, row.ProgramCode, hash, LastFour(candidate), emptyDash(row.MaskedName), row.ResultStatus, row.OfficialRank, row.Quota, row.SourcePage); err != nil {
			return uuid.Nil, mapResultError(err)
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE official_results r
		SET application_id = a.id
		FROM applications a
		WHERE r.batch_id = $1
		  AND a.academic_year = r.academic_year
		  AND a.school_code = r.school_code
		  AND a.program_code = r.program_code
		  AND a.status = 'confirmed'
		  AND a.candidate_number_lookup_hash = r.candidate_number_lookup_hash
	`, batchID); err != nil {
		return uuid.Nil, fmt.Errorf("match official results to applications: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("commit official result import: %w", err)
	}
	return batchID, nil
}

// ApplyCandidateListExtractionResult converts a Python list extraction into
// the same pending official-result batch used by the manual JSON import API.
// Plain candidate numbers/names are accepted only in this transient call;
// they are hashed/masked before the transaction commits.
func (r *PostgresRepository) ApplyCandidateListExtractionResult(ctx context.Context, result jobs.CandidateListExtractionResult) error {
	result.SHA256Hex = strings.ToLower(strings.TrimSpace(result.SHA256Hex))
	if err := result.Validate(); err != nil || len(r.lookupKey) != 32 {
		return fmt.Errorf("%w: candidate list extraction is invalid", ErrInvalidInput)
	}
	if result.SourceURL != "" && result.SourceURL != "-" && ValidateOfficialSourceURL(result.SourceURL) != nil {
		return ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin candidate list extraction: %w", err)
	}
	defer tx.Rollback(ctx)

	var jobType, jobStatus string
	var payload []byte
	err = tx.QueryRow(ctx, `
		SELECT job_type, status, payload
		FROM ingestion_jobs
		WHERE id = $1
		FOR UPDATE
	`, result.JobID).Scan(&jobType, &jobStatus, &payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("load candidate list extraction job: %w", err)
	}
	if jobType != jobs.CandidateListExtractRoutingKey {
		return ErrInvalidInput
	}
	var job jobs.BrochureExtractJob
	if err := json.Unmarshal(payload, &job); err != nil || job.EffectiveSourceType() != jobs.SourceTypeCandidateList || job.AcademicYear != result.AcademicYear || job.SchoolCode != result.SchoolCode || strings.ToLower(job.SHA256Hex) != result.SHA256Hex {
		return ErrInvalidInput
	}
	if jobStatus == "succeeded" {
		return tx.Commit(ctx)
	}
	sourceURL := strings.TrimSpace(result.SourceURL)
	if sourceURL == "" {
		sourceURL = strings.TrimSpace(job.SourceURL)
	}
	sourceURL = emptyDash(sourceURL)

	var batchID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO official_result_batches (academic_year, school_code, source_url, source_sha256_hex, status)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (academic_year, school_code, source_sha256_hex) DO NOTHING
		RETURNING id
	`, result.AcademicYear, result.SchoolCode, sourceURL, result.SHA256Hex, ResultBatchStatusPendingReview).Scan(&batchID)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.QueryRow(ctx, `SELECT id FROM official_result_batches WHERE academic_year=$1 AND school_code=$2 AND source_sha256_hex=$3 FOR UPDATE`, result.AcademicYear, result.SchoolCode, result.SHA256Hex).Scan(&batchID); err != nil {
			return mapResultError(err)
		}
	} else if err != nil {
		return mapResultError(err)
	}
	if jobStatus == "succeeded" {
		return tx.Commit(ctx)
	}
	seen := make(map[string]struct{}, len(result.Rows))
	for _, row := range result.Rows {
		programCode := strings.TrimSpace(row.ProgramCode)
		if programCode == "" {
			programCode = strings.TrimSpace(job.ProgramCode)
		}
		if !threeDigitCode.MatchString(programCode) {
			return ErrInvalidInput
		}
		candidateNumber, err := NormalizeCandidateNumber(row.CandidateNumber)
		if err != nil {
			return ErrInvalidInput
		}
		hash, err := auth.LookupHash(r.lookupKey, candidateNumber)
		if err != nil {
			return err
		}
		key := programCode + ":" + string(hash)
		if _, exists := seen[key]; exists {
			return ErrConflict
		}
		seen[key] = struct{}{}
		status := strings.TrimSpace(row.ResultStatus)
		if status == "" {
			status = ResultStatusUnknown
		}
		if !validResultStatus(status) || (status == ResultStatusAdmitted || status == ResultStatusWaitlisted) && row.OfficialRank == nil {
			return ErrInvalidInput
		}
		nameForMasking := strings.TrimSpace(row.CandidateName)
		if nameForMasking == "" {
			nameForMasking = strings.TrimSpace(row.MaskedName)
		}
		// Treat both name fields as transient input. Re-mask even a supplied
		// masked value so an external extractor cannot accidentally persist a
		// full name under the masked_name key.
		maskedName := maskCandidateName(nameForMasking)
		if maskedName == "" {
			return ErrInvalidInput
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO official_results
				(batch_id, academic_year, school_code, program_code, candidate_number_lookup_hash,
				 candidate_number_last4, masked_name, result_status, official_rank, quota, source_page)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11, 0))
			ON CONFLICT (batch_id, academic_year, school_code, program_code, candidate_number_lookup_hash) DO NOTHING
		`, batchID, result.AcademicYear, result.SchoolCode, programCode, hash, LastFour(candidateNumber), maskedName, status, row.OfficialRank, row.Quota, row.SourcePage); err != nil {
			return mapResultError(err)
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE official_results r
		SET application_id = a.id
		FROM applications a
		WHERE r.batch_id = $1
		  AND a.academic_year = r.academic_year
		  AND a.school_code = r.school_code
		  AND a.program_code = r.program_code
		  AND a.status = 'confirmed'
		  AND a.candidate_number_lookup_hash = r.candidate_number_lookup_hash
	`, batchID); err != nil {
		return fmt.Errorf("match extracted candidate list to applications: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE ingestion_jobs
		SET status='succeeded', locked_at=NULL, next_attempt_at=NULL,
		    last_error_code=NULL, last_error_message=NULL, updated_at=CURRENT_TIMESTAMP
		WHERE id=$1
	`, result.JobID); err != nil {
		return fmt.Errorf("mark candidate list extraction succeeded: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit candidate list extraction: %w", err)
	}
	return nil
}

func maskCandidateName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "-"
	}
	runes := []rune(name)
	if len(runes) == 1 {
		return string(runes)
	}
	return string(runes[0]) + strings.Repeat("○", len(runes)-1)
}

func (r *PostgresRepository) IsAdmin(ctx context.Context, accountID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM account_roles WHERE account_id = $1 AND role = 'admin')`, accountID).Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) PublishOfficialBatch(ctx context.Context, adminID, batchID uuid.UUID) error {
	isAdmin, err := r.IsAdmin(ctx, adminID)
	if err != nil {
		return err
	}
	if !isAdmin {
		return ErrAdminRequired
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin official result publish: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('sta.official_results.publish'))`); err != nil {
		return fmt.Errorf("lock official result publishing: %w", err)
	}
	var year int
	var school string
	if err := tx.QueryRow(ctx, `SELECT academic_year, school_code FROM official_result_batches WHERE id = $1 AND status = $2 FOR UPDATE`, batchID, ResultBatchStatusPendingReview).Scan(&year, &school); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("lock official result batch: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE official_result_batches SET status = $3 WHERE academic_year = $1 AND school_code = $2 AND status = $4`, year, school, ResultBatchStatusSuperseded, ResultBatchStatusPublished); err != nil {
		return fmt.Errorf("supersede official result batch: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE official_result_batches SET status = $2, reviewed_by = $3, reviewed_at = CURRENT_TIMESTAMP WHERE id = $1`, batchID, ResultBatchStatusPublished, adminID); err != nil {
		return fmt.Errorf("publish official result batch: %w", err)
	}
	// Applications may have been confirmed or had their candidate number
	// entered after import but before review. Reconcile the whole pending batch
	// against the current confirmed applications immediately before inquiries
	// are created, so late registrations are not missed and stale pending
	// matches are not retained.
	if _, err := tx.Exec(ctx, `
		UPDATE official_results
		SET application_id = NULL
		WHERE batch_id = $1
	`, batchID); err != nil {
		return fmt.Errorf("clear pending result matches: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE official_results r
		SET application_id = a.id
		FROM applications a
		WHERE r.batch_id = $1
		  AND r.academic_year = a.academic_year
		  AND r.school_code = a.school_code
		  AND r.program_code = a.program_code
		  AND a.status = 'confirmed'
		  AND a.candidate_number_lookup_hash = r.candidate_number_lookup_hash
	`, batchID); err != nil {
		return fmt.Errorf("refresh official result matches: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE official_results
		SET willingness_index = NULL
		WHERE batch_id = $1
	`, batchID); err != nil {
		return fmt.Errorf("clear official willingness indexes: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		WITH ordered AS (
			SELECT r.id,
			       ((row_number() OVER (
					PARTITION BY r.academic_year, r.school_code, r.program_code
					ORDER BY CASE WHEN r.result_status = 'admitted' THEN 0 ELSE 1 END,
					         r.official_rank, r.id
			       ) - 1)::int) AS willingness_index
			FROM official_results r
			WHERE r.batch_id = $1
			  AND r.result_status IN ('admitted', 'waitlisted')
		)
		UPDATE official_results r
		SET willingness_index = ordered.willingness_index
		FROM ordered
		WHERE r.id = ordered.id
	`, batchID); err != nil {
		return fmt.Errorf("assign official willingness indexes: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE academic_programs p
		SET willingness_values = COALESCE(
			(
				SELECT array_agg(100::smallint ORDER BY r.willingness_index)
				FROM official_results r
				WHERE r.batch_id = $1
				  AND r.academic_year = p.academic_year
				  AND r.school_code = p.school_code
				  AND r.program_code = p.program_code
				  AND r.willingness_index IS NOT NULL
			),
			ARRAY[]::smallint[]
		),
		updated_at = CURRENT_TIMESTAMP
		WHERE p.academic_year = $2 AND p.school_code = $3
	`, batchID, year, school); err != nil {
		return fmt.Errorf("initialize program willingness arrays: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM application_willingness w
		USING official_results r
		WHERE r.batch_id = $1 AND r.application_id = w.application_id
	`, batchID); err != nil {
		return fmt.Errorf("reset willingness values for published batch: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO willingness_inquiries (application_id, official_result_id, result_batch_id, inquiry_round)
		SELECT r.application_id, r.id, r.batch_id, $2
		FROM official_results r
		WHERE r.batch_id = $1
		  AND r.application_id IS NOT NULL
		  AND r.result_status IN ('admitted', 'waitlisted')
		ON CONFLICT (official_result_id, inquiry_round) DO NOTHING
	`, batchID, InquiryRoundResultReleased); err != nil {
		return fmt.Errorf("create result release inquiries: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit official result publish: %w", err)
	}
	return nil
}

func (r *PostgresRepository) CreateAcceptanceDeadlineInquiries(ctx context.Context, adminID, batchID uuid.UUID, deadline time.Time) error {
	isAdmin, err := r.IsAdmin(ctx, adminID)
	if err != nil {
		return err
	}
	if !isAdmin {
		return ErrAdminRequired
	}
	if deadline.Before(time.Now().UTC()) {
		return ErrInvalidInput
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO willingness_inquiries (application_id, official_result_id, result_batch_id, inquiry_round, response_deadline)
		SELECT r.application_id, r.id, r.batch_id, $3, $2
		FROM official_results r
		JOIN official_result_batches b ON b.id = r.batch_id AND b.status = 'published'
		LEFT JOIN application_willingness w ON w.application_id = r.application_id
		WHERE r.batch_id = $1
		  AND r.application_id IS NOT NULL
		  AND r.result_status IN ('admitted', 'waitlisted')
		  AND w.value IS NULL
		ON CONFLICT (official_result_id, inquiry_round) DO UPDATE
		SET response_deadline = EXCLUDED.response_deadline,
		    notification_status = CASE WHEN willingness_inquiries.notification_status = 'enqueued' THEN 'enqueued' ELSE 'pending' END,
		    notification_available_at = CURRENT_TIMESTAMP,
		    notification_error = NULL
	`, batchID, deadline, InquiryRoundAcceptanceDeadline)
	if err != nil {
		return mapResultError(err)
	}
	return nil
}

func (r *PostgresRepository) CorrectOfficialResult(ctx context.Context, adminID, resultID uuid.UUID, input OfficialResultCorrectionInput) error {
	isAdmin, err := r.IsAdmin(ctx, adminID)
	if err != nil {
		return err
	}
	if !isAdmin {
		return ErrAdminRequired
	}
	if input.Validate() != nil {
		return ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin official result correction: %w", err)
	}
	defer tx.Rollback(ctx)
	var before map[string]any
	var applicationID uuid.NullUUID
	var batchID uuid.UUID
	var year int
	var school string
	var previousStatus string
	err = tx.QueryRow(ctx, `
		SELECT r.batch_id, b.academic_year, b.school_code, r.application_id, r.result_status,
		       jsonb_build_object('result_status', r.result_status, 'official_rank', r.official_rank,
		                          'quota', r.quota, 'masked_name', r.masked_name, 'source_page', r.source_page)
		FROM official_results r
		JOIN official_result_batches b ON b.id = r.batch_id AND b.status = $2
		WHERE r.id = $1
		FOR UPDATE
	`, resultID, ResultBatchStatusPublished).Scan(&batchID, &year, &school, &applicationID, &previousStatus, &before)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("load official result correction: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE official_results
		SET result_status = $2, official_rank = $3, quota = $4, masked_name = $5,
		    willingness_index = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, resultID, input.ResultStatus, input.OfficialRank, input.Quota, emptyDash(input.MaskedName)); err != nil {
		return fmt.Errorf("update official result correction: %w", err)
	}
	after := map[string]any{
		"result_status": input.ResultStatus,
		"official_rank": input.OfficialRank,
		"quota":         input.Quota,
		"masked_name":   emptyDash(input.MaskedName),
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_log (actor_account_id, action, entity_type, entity_key, before_data, after_data, reason)
		VALUES ($1, 'correct', 'official_result', $2, $3::jsonb, $4::jsonb, $5)
	`, adminID, resultID.String(), mustJSON(before), mustJSON(after), input.Reason); err != nil {
		return fmt.Errorf("record official result correction: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		WITH ordered AS (
			SELECT r.id,
			       ((row_number() OVER (
					PARTITION BY r.academic_year, r.school_code, r.program_code
					ORDER BY CASE WHEN r.result_status = 'admitted' THEN 0 ELSE 1 END,
					         r.official_rank, r.id
			       ) - 1)::int) AS willingness_index
			FROM official_results r
			WHERE r.batch_id = $1
			  AND r.result_status IN ('admitted', 'waitlisted')
		)
		UPDATE official_results r
		SET willingness_index = ordered.willingness_index
		FROM ordered
		WHERE r.id = ordered.id
	`, batchID); err != nil {
		return fmt.Errorf("rebuild official willingness indexes: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE academic_programs p
		SET willingness_values = COALESCE(
			(
				SELECT array_agg(COALESCE(w.value, 100::smallint) ORDER BY r.willingness_index)
				FROM official_results r
				LEFT JOIN application_willingness w ON w.application_id = r.application_id
				WHERE r.batch_id = $1
				  AND r.academic_year = p.academic_year
				  AND r.school_code = p.school_code
				  AND r.program_code = p.program_code
				  AND r.willingness_index IS NOT NULL
			),
			ARRAY[]::smallint[]
		),
		updated_at = CURRENT_TIMESTAMP
		WHERE p.academic_year = $2 AND p.school_code = $3
	`, batchID, year, school); err != nil {
		return fmt.Errorf("rebuild program willingness arrays: %w", err)
	}
	if isWillingnessResultStatus(input.ResultStatus) && applicationID.Valid {
		if _, err := tx.Exec(ctx, `
			INSERT INTO willingness_inquiries (application_id, official_result_id, result_batch_id, inquiry_round)
			SELECT $1, $2, $3, $4
			WHERE NOT EXISTS (SELECT 1 FROM application_willingness WHERE application_id = $1)
			ON CONFLICT (official_result_id, inquiry_round) DO NOTHING
		`, applicationID.UUID, resultID, batchID, InquiryRoundResultReleased); err != nil {
			return fmt.Errorf("create correction inquiry: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit official result correction: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ListAdminBatches(ctx context.Context, adminID uuid.UUID, query AdminResultBatchQuery) ([]AdminResultBatch, error) {
	if err := r.requireAdmin(ctx, adminID); err != nil {
		return nil, err
	}
	if err := query.Validate(); err != nil {
		return nil, err
	}
	conditions := []string{"TRUE"}
	args := make([]any, 0, 7)
	argIndex := 1
	if query.AcademicYear != 0 {
		conditions = append(conditions, fmt.Sprintf("b.academic_year = $%d", argIndex))
		args = append(args, query.AcademicYear)
		argIndex++
	}
	if query.SchoolCode != "" {
		conditions = append(conditions, fmt.Sprintf("b.school_code = $%d", argIndex))
		args = append(args, query.SchoolCode)
		argIndex++
	}
	if query.Status != "" {
		conditions = append(conditions, fmt.Sprintf("b.status = $%d", argIndex))
		args = append(args, query.Status)
		argIndex++
	}
	limitIndex := argIndex
	offsetIndex := argIndex + 1
	args = append(args, query.Limit, query.Offset)
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT b.id::text, b.academic_year, b.school_code, b.source_url, b.source_sha256_hex,
		       b.status, b.imported_at, b.reviewed_at,
		       COUNT(DISTINCT r.id)::int,
		       COUNT(DISTINCT r.id) FILTER (WHERE r.application_id IS NOT NULL)::int,
		       COUNT(DISTINCT i.id)::int
		FROM official_result_batches b
		LEFT JOIN official_results r ON r.batch_id = b.id
		LEFT JOIN willingness_inquiries i ON i.official_result_id = r.id
		WHERE %s
		GROUP BY b.id
		ORDER BY b.imported_at DESC, b.id DESC
		LIMIT $%d OFFSET $%d
	`, strings.Join(conditions, " AND "), limitIndex, offsetIndex), args...)
	if err != nil {
		return nil, fmt.Errorf("list admin result batches: %w", err)
	}
	defer rows.Close()
	batches := make([]AdminResultBatch, 0)
	for rows.Next() {
		batch, err := scanAdminResultBatch(rows)
		if err != nil {
			return nil, fmt.Errorf("scan admin result batch: %w", err)
		}
		batches = append(batches, batch)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admin result batches: %w", err)
	}
	return batches, nil
}

func (r *PostgresRepository) GetAdminBatch(ctx context.Context, adminID, batchID uuid.UUID) (AdminResultBatchDetail, error) {
	if err := r.requireAdmin(ctx, adminID); err != nil {
		return AdminResultBatchDetail{}, err
	}
	var detail AdminResultBatchDetail
	var idText string
	err := r.pool.QueryRow(ctx, `
		SELECT b.id::text, b.academic_year, b.school_code, b.source_url, b.source_sha256_hex,
		       b.status, b.imported_at, b.reviewed_at,
		       (SELECT COUNT(*)::int FROM official_results r WHERE r.batch_id = b.id),
		       (SELECT COUNT(*)::int FROM official_results r WHERE r.batch_id = b.id AND r.application_id IS NOT NULL),
		       (SELECT COUNT(DISTINCT i.id)::int FROM official_results r JOIN willingness_inquiries i ON i.official_result_id = r.id WHERE r.batch_id = b.id)
		FROM official_result_batches b
		WHERE b.id = $1
	`, batchID).Scan(&idText, &detail.Batch.AcademicYear, &detail.Batch.SchoolCode, &detail.Batch.SourceURL, &detail.Batch.SourceSHA256,
		&detail.Batch.Status, &detail.Batch.ImportedAt, &detail.Batch.ReviewedAt, &detail.Batch.ResultCount, &detail.Batch.MatchedCount, &detail.Batch.InquiryCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminResultBatchDetail{}, ErrNotFound
	}
	if err != nil {
		return AdminResultBatchDetail{}, fmt.Errorf("get admin result batch: %w", err)
	}
	detail.Batch.ID, err = uuid.Parse(idText)
	if err != nil {
		return AdminResultBatchDetail{}, fmt.Errorf("parse admin result batch id: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT r.id::text, r.program_code, r.candidate_number_last4, r.masked_name,
		       r.result_status, r.official_rank, r.quota, r.source_page,
		       r.application_id IS NOT NULL
		FROM official_results r
		WHERE r.batch_id = $1
		ORDER BY r.program_code, r.official_rank NULLS LAST, r.id
	`, batchID)
	if err != nil {
		return AdminResultBatchDetail{}, fmt.Errorf("list admin result rows: %w", err)
	}
	defer rows.Close()
	detail.Rows = make([]AdminResultRow, 0, detail.Batch.ResultCount)
	for rows.Next() {
		var row AdminResultRow
		var rowIDText string
		if err := rows.Scan(&rowIDText, &row.ProgramCode, &row.CandidateNumberLast4, &row.MaskedName, &row.ResultStatus,
			&row.OfficialRank, &row.Quota, &row.SourcePage, &row.ApplicationMatched); err != nil {
			return AdminResultBatchDetail{}, fmt.Errorf("scan admin result row: %w", err)
		}
		row.ID, err = uuid.Parse(rowIDText)
		if err != nil {
			return AdminResultBatchDetail{}, fmt.Errorf("parse admin result row id: %w", err)
		}
		detail.Rows = append(detail.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return AdminResultBatchDetail{}, fmt.Errorf("iterate admin result rows: %w", err)
	}
	return detail, nil
}

func (r *PostgresRepository) requireAdmin(ctx context.Context, accountID uuid.UUID) error {
	isAdmin, err := r.IsAdmin(ctx, accountID)
	if err != nil {
		return err
	}
	if !isAdmin {
		return ErrAdminRequired
	}
	return nil
}

type resultRowScanner interface {
	Scan(dest ...any) error
}

func scanAdminResultBatch(row resultRowScanner) (AdminResultBatch, error) {
	var batch AdminResultBatch
	var idText string
	if err := row.Scan(&idText, &batch.AcademicYear, &batch.SchoolCode, &batch.SourceURL, &batch.SourceSHA256,
		&batch.Status, &batch.ImportedAt, &batch.ReviewedAt, &batch.ResultCount, &batch.MatchedCount, &batch.InquiryCount); err != nil {
		return AdminResultBatch{}, err
	}
	var err error
	batch.ID, err = uuid.Parse(idText)
	if err != nil {
		return AdminResultBatch{}, fmt.Errorf("parse admin result batch id: %w", err)
	}
	return batch, nil
}

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func (r *PostgresRepository) SetCandidateNumber(ctx context.Context, accountID, applicationID uuid.UUID, ciphertext, lookupHash []byte, lastFour string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin candidate number transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	command, err := tx.Exec(ctx, `
		UPDATE applications
		SET candidate_number_ciphertext = $3,
		    candidate_number_lookup_hash = $4,
		    candidate_number_last4 = $5
		WHERE id = $1 AND account_id = $2 AND status = 'confirmed'
	`, applicationID, accountID, ciphertext, lookupHash, lastFour)
	if err != nil {
		return mapResultError(err)
	}
	if command.RowsAffected() != 1 {
		return ErrNotFound
	}
	// A user may correct the candidate number after official data has already
	// been imported. Remove this application's old published matches first;
	// then link only currently published, still-unmatched rows. A result already
	// matched to another application is never reassigned.
	if _, err := tx.Exec(ctx, `
		UPDATE official_results r
		SET application_id = NULL
		FROM official_result_batches b
		WHERE r.application_id = $1
		  AND b.id = r.batch_id
		  AND b.status = 'published'
	`, applicationID); err != nil {
		return fmt.Errorf("clear previous candidate result match: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE official_results r
		SET application_id = a.id
		FROM applications a, official_result_batches b
		WHERE a.id = $1
		  AND a.account_id = $2
		  AND a.status = 'confirmed'
		  AND b.id = r.batch_id
		  AND b.status = 'published'
		  AND a.academic_year = r.academic_year
		  AND a.school_code = r.school_code
		  AND a.program_code = r.program_code
		  AND r.candidate_number_lookup_hash = $3
		  AND r.application_id IS NULL
	`, applicationID, accountID, lookupHash); err != nil {
		return fmt.Errorf("match candidate number to official results: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO willingness_inquiries (application_id, official_result_id, result_batch_id, inquiry_round)
		SELECT r.application_id, r.id, r.batch_id, $2
		FROM official_results r
		JOIN official_result_batches b ON b.id = r.batch_id AND b.status = $3
		LEFT JOIN application_willingness w ON w.application_id = r.application_id
		WHERE r.application_id = $1
		  AND r.result_status IN ('admitted', 'waitlisted')
		  AND w.value IS NULL
		ON CONFLICT (official_result_id, inquiry_round) DO NOTHING
	`, applicationID, InquiryRoundResultReleased, ResultBatchStatusPublished); err != nil {
		return fmt.Errorf("create candidate result inquiry: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_log (actor_account_id, action, entity_type, entity_key, after_data, reason)
		VALUES ($1, 'set_candidate_number', 'application', $2, jsonb_build_object('candidate_number_last4', $3::text), 'user updated candidate number')
	`, accountID, applicationID.String(), lastFour); err != nil {
		return fmt.Errorf("record candidate number change: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit candidate number transaction: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetReport(ctx context.Context, accountID, applicationID uuid.UUID) (Report, error) {
	var report Report
	var applicationIDText string
	var rank, quota, position, probability, currentWillingness float64
	err := r.pool.QueryRow(ctx, `
		WITH target AS (
			SELECT r.batch_id, r.application_id, r.id AS result_id,
			       r.academic_year, r.school_code, r.program_code,
			       r.result_status, r.official_rank, r.quota, r.willingness_index,
			       p.willingness_values, b.imported_at,
			       COALESCE(a.candidate_number_last4, '') AS candidate_number_last4,
			       (a.candidate_number_lookup_hash IS NOT NULL) AS candidate_number_set
			FROM official_results r
			JOIN official_result_batches b ON b.id = r.batch_id AND b.status = $3
			JOIN applications a ON a.id = r.application_id AND a.account_id = $1 AND a.status = 'confirmed'
			JOIN academic_programs p ON p.academic_year = r.academic_year
			  AND p.school_code = r.school_code AND p.program_code = r.program_code
			WHERE r.application_id = $2
			ORDER BY b.imported_at DESC
			LIMIT 1
		), front AS (
			SELECT f.id,
			       t.willingness_values[(f.willingness_index + 1)]::smallint AS value,
			       EXISTS (
					SELECT 1
					FROM willingness_response_events e
					WHERE e.official_result_id = f.id
			       ) AS responded
			FROM official_results f
			JOIN target t ON t.academic_year = f.academic_year
			  AND t.school_code = f.school_code AND t.program_code = f.program_code
			  AND t.batch_id = f.batch_id
			WHERE t.result_status = 'waitlisted'
			  AND f.result_status IN ('admitted', 'waitlisted')
			  AND f.willingness_index IS NOT NULL
			  AND f.willingness_index < t.willingness_index
		)
		SELECT t.application_id::text, t.result_status, t.candidate_number_last4, t.candidate_number_set,
		       COALESCE(t.official_rank, 0), COALESCE(t.quota, -1),
		       (SELECT COUNT(*)::int FROM front),
		       (SELECT COUNT(*)::int FROM front WHERE responded),
		       CASE WHEN t.result_status = 'waitlisted' THEN 1 + (SELECT COUNT(*)::int FROM front WHERE value > 0) ELSE 0 END,
		       CASE
			       WHEN t.result_status = 'admitted' THEN 100::float8
			       WHEN t.result_status = 'waitlisted' THEN COALESCE((SELECT ROUND(AVG(value)::numeric, 2)::float8 FROM front), -1)
			       ELSE -1::float8
		       END,
		       COALESCE(t.willingness_values[(t.willingness_index + 1)]::float8, -1)
		FROM target t
		`, accountID, applicationID, ResultBatchStatusPublished).Scan(
		&applicationIDText, &report.ResultStatus, &report.CandidateNumberLast4, &report.CandidateNumberSet, &rank, &quota,
		&report.FrontCandidateCount, &report.FrontResponseCount,
		&position,
		&probability, &currentWillingness,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Report{}, ErrNotFound
	}
	if err != nil {
		return Report{}, fmt.Errorf("get result report: %w", err)
	}
	report.ApplicationID, err = uuid.Parse(applicationIDText)
	if err != nil {
		return Report{}, fmt.Errorf("parse result application id: %w", err)
	}
	if rank > 0 {
		value := int(rank)
		report.OfficialRank = &value
	}
	if quota >= 0 {
		value := int(quota)
		report.Quota = &value
	}
	if position > 0 {
		value := int(position)
		report.PositionAfterDeclines = &value
	}
	if probability >= 0 {
		value := probability
		report.ReferenceProbability = &value
	}
	if currentWillingness >= 0 {
		value := int16(currentWillingness)
		report.CurrentWillingness = &value
	}
	return report, nil
}

func (r *PostgresRepository) ListInquiries(ctx context.Context, accountID, applicationID uuid.UUID) ([]Inquiry, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM applications WHERE id = $1 AND account_id = $2 AND status = 'confirmed')`, applicationID, accountID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check inquiry application: %w", err)
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := r.pool.Query(ctx, `
		SELECT i.id::text, i.inquiry_round, i.response_deadline, i.created_at,
		       EXISTS (SELECT 1 FROM willingness_response_events e WHERE e.inquiry_id = i.id),
		       COALESCE(p.willingness_values[(r.willingness_index + 1)], w.value, -1)
		FROM willingness_inquiries i
		LEFT JOIN official_results r ON r.id = i.official_result_id
		LEFT JOIN official_result_batches b ON b.id = r.batch_id
		LEFT JOIN academic_programs p ON p.academic_year = r.academic_year
		  AND p.school_code = r.school_code AND p.program_code = r.program_code
		LEFT JOIN application_willingness w ON w.application_id = i.application_id
		WHERE i.application_id = $1
		  AND (
			 i.official_result_id IS NULL
			 OR (
				 b.status = 'published'
				 AND r.id = (
					 SELECT current_result.id
					 FROM official_results current_result
					 JOIN official_result_batches current_batch ON current_batch.id = current_result.batch_id
					 WHERE current_result.application_id = $1
					   AND current_batch.status = 'published'
					 ORDER BY current_batch.imported_at DESC, current_result.updated_at DESC, current_result.id DESC
					 LIMIT 1
				 )
			 )
		  )
		ORDER BY i.created_at DESC
	`, applicationID)
	if err != nil {
		return nil, fmt.Errorf("list willingness inquiries: %w", err)
	}
	defer rows.Close()
	inquiries := make([]Inquiry, 0)
	for rows.Next() {
		var inquiry Inquiry
		var idText string
		var current int16
		if err := rows.Scan(&idText, &inquiry.Round, &inquiry.ResponseDeadline, &inquiry.CreatedAt, &inquiry.Responded, &current); err != nil {
			return nil, fmt.Errorf("scan willingness inquiry: %w", err)
		}
		inquiry.ID, err = uuid.Parse(idText)
		if err != nil {
			return nil, fmt.Errorf("parse willingness inquiry id: %w", err)
		}
		if current >= 0 {
			value := current
			inquiry.CurrentWillingness = &value
		}
		inquiries = append(inquiries, inquiry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate willingness inquiries: %w", err)
	}
	return inquiries, nil
}

func (r *PostgresRepository) SetWillingness(ctx context.Context, accountID, applicationID uuid.UUID, value int16, inquiryID *uuid.UUID) (WillingnessResponse, error) {
	return r.setWillingness(ctx, accountID, applicationID, value, inquiryID, "web", "")
}

// SetWillingnessFromChannel applies a response from an external presentation
// channel through the same canonical willingness transaction used by the
// website. externalEventID is an idempotency key supplied by that channel.
func (r *PostgresRepository) SetWillingnessFromChannel(ctx context.Context, accountID, applicationID uuid.UUID, value int16, inquiryID uuid.UUID, source, externalEventID string) (WillingnessResponse, error) {
	source = strings.ToLower(strings.TrimSpace(source))
	externalEventID = strings.TrimSpace(externalEventID)
	if inquiryID == uuid.Nil || source == "" || source == "web" || externalEventID == "" || len(externalEventID) > 200 {
		return WillingnessResponse{}, ErrInvalidInput
	}
	return r.setWillingness(ctx, accountID, applicationID, value, &inquiryID, source, externalEventID)
}

func (r *PostgresRepository) setWillingness(ctx context.Context, accountID, applicationID uuid.UUID, value int16, inquiryID *uuid.UUID, source, externalEventID string) (WillingnessResponse, error) {
	if err := ValidateWillingness(value); err != nil {
		return WillingnessResponse{}, err
	}
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" || len(source) > 16 {
		return WillingnessResponse{}, ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return WillingnessResponse{}, fmt.Errorf("begin willingness transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM applications WHERE id = $1 AND account_id = $2 AND status = 'confirmed')`, applicationID, accountID).Scan(&exists); err != nil {
		return WillingnessResponse{}, fmt.Errorf("check willingness application: %w", err)
	}
	if !exists {
		return WillingnessResponse{}, ErrNotFound
	}
	if externalEventID != "" {
		// Serialize retries of the same external event before checking the unique
		// key. This avoids turning a concurrent duplicate into a transaction-wide
		// unique-constraint failure.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, source+":"+externalEventID); err != nil {
			return WillingnessResponse{}, fmt.Errorf("lock external willingness event: %w", err)
		}
		var existing WillingnessResponse
		err := tx.QueryRow(ctx, `
			SELECT e.id, r.academic_year, r.school_code, r.program_code,
			       r.result_status, COALESCE(r.official_rank, 0), e.value
			FROM willingness_response_events e
			JOIN official_results r ON r.id = e.official_result_id
			JOIN applications a ON a.id = e.application_id
			WHERE e.response_source = $1
			  AND e.external_event_id = $2
			  AND e.application_id = $3
			  AND a.account_id = $4
		`, source, externalEventID, applicationID, accountID).Scan(
			&existing.ResponseID, &existing.AcademicYear, &existing.SchoolCode,
			&existing.ProgramCode, &existing.ResultStatus, &existing.OfficialRank,
			&existing.Willingness,
		)
		if err == nil {
			if err := tx.Commit(ctx); err != nil {
				return WillingnessResponse{}, fmt.Errorf("commit duplicate willingness transaction: %w", err)
			}
			return existing, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return WillingnessResponse{}, fmt.Errorf("find external willingness event: %w", err)
		}
	}
	var selectedInquiryID, resultID, batchID uuid.UUID
	var academicYear, officialRank, willingnessIndex int
	var schoolCode, programCode, resultStatus string
	if inquiryID != nil && *inquiryID != uuid.Nil {
		if err := tx.QueryRow(ctx, `
			SELECT i.id, r.id, r.batch_id, r.academic_year, r.school_code, r.program_code,
			       r.result_status, COALESCE(r.official_rank, 0), COALESCE(r.willingness_index, -1)
			FROM willingness_inquiries i
			JOIN official_results r ON r.id = i.official_result_id
			JOIN official_result_batches b ON b.id = r.batch_id AND b.status = 'published'
			JOIN applications a ON a.id = i.application_id
			WHERE i.id = $1 AND i.application_id = $2 AND a.account_id = $3 AND a.status = 'confirmed'
			  AND (i.response_deadline IS NULL OR i.response_deadline > CURRENT_TIMESTAMP)
			FOR UPDATE OF i, r
		`, *inquiryID, applicationID, accountID).Scan(&selectedInquiryID, &resultID, &batchID, &academicYear, &schoolCode, &programCode, &resultStatus, &officialRank, &willingnessIndex); errors.Is(err, pgx.ErrNoRows) {
			return WillingnessResponse{}, ErrNotFound
		} else if err != nil {
			return WillingnessResponse{}, fmt.Errorf("check selected willingness inquiry: %w", err)
		}
	} else if err := tx.QueryRow(ctx, `
		SELECT i.id, r.id, r.batch_id, r.academic_year, r.school_code, r.program_code,
		       r.result_status, COALESCE(r.official_rank, 0), COALESCE(r.willingness_index, -1)
		FROM willingness_inquiries i
		JOIN official_results r ON r.id = i.official_result_id
		JOIN official_result_batches b ON b.id = r.batch_id AND b.status = 'published'
			WHERE i.application_id = $1
		  AND (i.response_deadline IS NULL OR i.response_deadline > CURRENT_TIMESTAMP)
			ORDER BY b.imported_at DESC, i.created_at DESC
		LIMIT 1
		FOR UPDATE OF i, r
	`, applicationID).Scan(&selectedInquiryID, &resultID, &batchID, &academicYear, &schoolCode, &programCode, &resultStatus, &officialRank, &willingnessIndex); errors.Is(err, pgx.ErrNoRows) {
		return WillingnessResponse{}, ErrNotFound
	} else if err != nil {
		return WillingnessResponse{}, fmt.Errorf("select latest willingness inquiry: %w", err)
	}
	if selectedInquiryID == uuid.Nil || resultID == uuid.Nil || willingnessIndex < 0 || officialRank < 1 {
		return WillingnessResponse{}, ErrNotFound
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO application_willingness (application_id, value, updated_at)
		VALUES ($1, $2, CURRENT_TIMESTAMP)
		ON CONFLICT (application_id) DO UPDATE SET value = EXCLUDED.value, updated_at = CURRENT_TIMESTAMP
	`, applicationID, value); err != nil {
		return WillingnessResponse{}, mapResultError(err)
	}
	command, err := tx.Exec(ctx, `
		UPDATE academic_programs
		SET willingness_values[($4 + 1)] = $5,
		    updated_at = CURRENT_TIMESTAMP
		WHERE academic_year = $1
		  AND school_code = $2
		  AND program_code = $3
		  AND array_length(willingness_values, 1) > $4
	`, academicYear, schoolCode, programCode, willingnessIndex, value)
	if err != nil {
		return WillingnessResponse{}, fmt.Errorf("update program willingness array: %w", err)
	}
	if command.RowsAffected() != 1 {
		return WillingnessResponse{}, ErrConflict
	}
	var responseID int64
	var responseErr error
	if source == "web" && externalEventID == "" {
		responseErr = tx.QueryRow(ctx, `
			INSERT INTO willingness_response_events
				(application_id, inquiry_id, official_result_id, value)
			VALUES ($1, $2, $3, $4)
			RETURNING id
		`, applicationID, selectedInquiryID, resultID, value).Scan(&responseID)
	} else {
		var externalEvent any
		if externalEventID != "" {
			externalEvent = externalEventID
		}
		responseErr = tx.QueryRow(ctx, `
			INSERT INTO willingness_response_events
				(application_id, inquiry_id, official_result_id, value, response_source, external_event_id)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id
		`, applicationID, selectedInquiryID, resultID, value, source, externalEvent).Scan(&responseID)
	}
	if responseErr != nil {
		return WillingnessResponse{}, fmt.Errorf("record willingness response: %w", responseErr)
	}
	if err := tx.Commit(ctx); err != nil {
		return WillingnessResponse{}, fmt.Errorf("commit willingness transaction: %w", err)
	}
	return WillingnessResponse{
		ResponseID:   responseID,
		AcademicYear: academicYear,
		SchoolCode:   schoolCode,
		ProgramCode:  programCode,
		ResultStatus: resultStatus,
		OfficialRank: officialRank,
		Willingness:  value,
	}, nil
}

func mapResultError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503":
			return ErrNotFound
		case "23505":
			return ErrConflict
		}
	}
	return err
}
