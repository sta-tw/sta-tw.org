package brochurediscovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) (*PostgresRepository, error) {
	if pool == nil {
		return nil, errors.New("brochure discovery postgres pool is nil")
	}
	return &PostgresRepository{pool: pool}, nil
}

func (r *PostgresRepository) IsAdmin(ctx context.Context, accountID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM account_roles WHERE account_id = $1 AND role = 'admin')`, accountID).Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) requireAdmin(ctx context.Context, accountID uuid.UUID) error {
	ok, err := r.IsAdmin(ctx, accountID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrAdminRequired
	}
	return nil
}

func (r *PostgresRepository) CreateCycle(ctx context.Context, adminID uuid.UUID, academicYear int) (Cycle, int64, error) {
	if err := r.requireAdmin(ctx, adminID); err != nil {
		return Cycle{}, 0, err
	}
	if academicYear < 100 || academicYear > 999 {
		return Cycle{}, 0, ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Cycle{}, 0, err
	}
	defer tx.Rollback(ctx)
	var cycle Cycle
	err = tx.QueryRow(ctx, `
		INSERT INTO brochure_discovery_cycles (academic_year, status, created_by)
		VALUES ($1, 'draft', $2)
		ON CONFLICT (academic_year) DO NOTHING
		RETURNING academic_year, status, created_by, started_by, closed_by, started_at, closed_at, created_at, updated_at
	`, academicYear, adminID).Scan(&cycle.AcademicYear, &cycle.Status, &cycle.CreatedBy, &cycle.StartedBy, &cycle.ClosedBy, &cycle.StartedAt, &cycle.ClosedAt, &cycle.CreatedAt, &cycle.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Cycle{}, 0, ErrInvalidStatus
	}
	if err != nil {
		return Cycle{}, 0, err
	}
	result, err := tx.Exec(ctx, `
		INSERT INTO brochure_discovery_tasks (academic_year, school_code, status)
		SELECT $1, school_code, 'pending_search' FROM brochure_discovery_school_roster
		ON CONFLICT (academic_year, school_code) DO NOTHING
	`, academicYear)
	if err != nil {
		return Cycle{}, 0, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO brochure_discovery_cycle_events (academic_year, action, to_status, actor_account_id) VALUES ($1,'created','draft',$2)`, academicYear, adminID); err != nil {
		return Cycle{}, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Cycle{}, 0, err
	}
	return cycle, result.RowsAffected(), nil
}

func (r *PostgresRepository) ListCycles(ctx context.Context, adminID uuid.UUID) ([]Cycle, error) {
	if err := r.requireAdmin(ctx, adminID); err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT academic_year, status, created_by, started_by, closed_by, started_at, closed_at, created_at, updated_at FROM brochure_discovery_cycles ORDER BY academic_year DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Cycle, 0)
	for rows.Next() {
		var item Cycle
		if err := rows.Scan(&item.AcademicYear, &item.Status, &item.CreatedBy, &item.StartedBy, &item.ClosedBy, &item.StartedAt, &item.ClosedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) StartCycle(ctx context.Context, adminID uuid.UUID, academicYear int) (Cycle, error) {
	return r.changeCycleStatus(ctx, adminID, academicYear, CycleDraft, CycleActive)
}

func (r *PostgresRepository) CloseCycle(ctx context.Context, adminID uuid.UUID, academicYear int) (Cycle, error) {
	return r.changeCycleStatus(ctx, adminID, academicYear, CycleActive, CycleClosed)
}

func (r *PostgresRepository) changeCycleStatus(ctx context.Context, adminID uuid.UUID, academicYear int, from, to string) (Cycle, error) {
	if err := r.requireAdmin(ctx, adminID); err != nil {
		return Cycle{}, err
	}
	if academicYear < 100 || academicYear > 999 {
		return Cycle{}, ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Cycle{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('sta.brochure_discovery_cycle'))`); err != nil {
		return Cycle{}, err
	}
	if to == CycleActive {
		var anotherActive bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM brochure_discovery_cycles WHERE status='active' AND academic_year<>$1)`, academicYear).Scan(&anotherActive); err != nil {
			return Cycle{}, err
		}
		if anotherActive {
			return Cycle{}, ErrInvalidStatus
		}
	}
	var cycle Cycle
	err = tx.QueryRow(ctx, `
		UPDATE brochure_discovery_cycles
		SET status=$3::varchar,
		    started_at=CASE WHEN $3::varchar='active' THEN CURRENT_TIMESTAMP ELSE started_at END,
			started_by=CASE WHEN $3::varchar='active' THEN $4::uuid ELSE started_by END,
		    closed_at=CASE WHEN $3::varchar='closed' THEN CURRENT_TIMESTAMP ELSE NULL END,
		    closed_by=CASE WHEN $3::varchar='closed' THEN $4::uuid ELSE NULL END
		WHERE academic_year=$1 AND status=$2
		RETURNING academic_year, status, created_by, started_by, closed_by, started_at, closed_at, created_at, updated_at
	`, academicYear, from, to, adminID).Scan(&cycle.AcademicYear, &cycle.Status, &cycle.CreatedBy, &cycle.StartedBy, &cycle.ClosedBy, &cycle.StartedAt, &cycle.ClosedAt, &cycle.CreatedAt, &cycle.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Cycle{}, ErrInvalidStatus
	}
	if err != nil {
		return Cycle{}, err
	}
	action := "started"
	if to == CycleClosed {
		action = "closed"
	}
	if _, err := tx.Exec(ctx, `INSERT INTO brochure_discovery_cycle_events (academic_year, action, from_status, to_status, actor_account_id) VALUES ($1,$2,$3,$4,$5)`, academicYear, action, from, to, adminID); err != nil {
		return Cycle{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Cycle{}, err
	}
	return cycle, nil
}

func (r *PostgresRepository) List(ctx context.Context, adminID uuid.UUID, academicYear int, query Query) ([]Task, error) {
	if err := r.requireAdmin(ctx, adminID); err != nil {
		return nil, err
	}
	if academicYear < 100 || academicYear > 999 {
		return nil, ErrInvalid
	}
	if query.Limit < 1 || query.Limit > 200 {
		query.Limit = 100
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	condition := "t.academic_year = $1"
	args := []any{academicYear}
	if query.Status != "" {
		args = append(args, query.Status)
		condition += fmt.Sprintf(" AND t.status = $%d", len(args))
	}
	args = append(args, query.Limit, query.Offset)
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT t.academic_year, t.school_code, s.school_name, t.status,
		       COALESCE(t.completion_method, ''), t.attempt_count,
		       COALESCE(t.candidate_source_url, ''), COALESCE(t.candidate_document_url, ''),
		       COALESCE(t.candidate_sha256_hex, ''), t.candidate_confidence,
		       COALESCE(t.candidate_evidence, '{}'::jsonb),
		       COALESCE(t.last_error_code, ''), COALESCE(t.last_error_message, ''),
		       t.last_searched_at, t.next_search_at, t.completed_at, t.completed_by,
		       t.created_at, t.updated_at
		FROM brochure_discovery_tasks t
		JOIN schools s ON s.school_code = t.school_code
		WHERE %s
		ORDER BY t.school_code
		LIMIT $%d OFFSET $%d
	`, condition, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Task, 0, query.Limit)
	for rows.Next() {
		item, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PostgresRepository) ListEvents(ctx context.Context, adminID uuid.UUID, academicYear int, schoolCode string) ([]Event, error) {
	if err := r.requireAdmin(ctx, adminID); err != nil {
		return nil, err
	}
	if academicYear < 100 || academicYear > 999 || !validSchoolCode(schoolCode) {
		return nil, ErrInvalid
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, academic_year, school_code, action, COALESCE(from_status, ''),
		       to_status, actor_account_id, details, created_at
		FROM brochure_discovery_events
		WHERE academic_year=$1 AND school_code=$2
		ORDER BY created_at DESC, id DESC
	`, academicYear, schoolCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Event, 0)
	for rows.Next() {
		var item Event
		var details []byte
		if err := rows.Scan(&item.ID, &item.AcademicYear, &item.SchoolCode, &item.Action,
			&item.FromStatus, &item.ToStatus, &item.ActorAccountID, &details, &item.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(details, &item.Details); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ClaimNext leases one task to an external discovery agent. Expired searching tasks
// are claimable again, so a crashed agent cannot strand a school forever.
func (r *PostgresRepository) ClaimNext(ctx context.Context, adminID uuid.UUID, lease time.Duration) (Task, error) {
	if err := r.requireAdmin(ctx, adminID); err != nil {
		return Task{}, err
	}
	return r.claimNext(ctx, &adminID, lease)
}

func (r *PostgresRepository) ClaimNextSystem(ctx context.Context, lease time.Duration) (Task, error) {
	return r.claimNext(ctx, nil, lease)
}

func (r *PostgresRepository) claimNext(ctx context.Context, actorID *uuid.UUID, lease time.Duration) (Task, error) {
	if lease <= 0 || lease > time.Hour {
		lease = 15 * time.Minute
	}
	leaseSeconds := int(lease / time.Second)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback(ctx)
	var academicYear int
	var code, fromStatus string
	err = tx.QueryRow(ctx, `
		SELECT t.academic_year, t.school_code, t.status
		FROM brochure_discovery_tasks t
		JOIN brochure_discovery_cycles c ON c.academic_year=t.academic_year AND c.status='active'
		WHERE t.status = 'pending_search' OR (t.status = 'searching' AND (t.next_search_at IS NULL OR t.next_search_at <= CURRENT_TIMESTAMP))
		ORDER BY CASE t.status WHEN 'pending_search' THEN 0 ELSE 1 END, t.next_search_at NULLS FIRST, t.school_code
		FOR UPDATE SKIP LOCKED LIMIT 1
	`).Scan(&academicYear, &code, &fromStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE brochure_discovery_tasks
		SET status = 'searching', attempt_count = attempt_count + 1,
		    last_searched_at = CURRENT_TIMESTAMP, next_search_at = CURRENT_TIMESTAMP + make_interval(secs => $3),
		    last_error_code = NULL, last_error_message = NULL
		WHERE academic_year = $1 AND school_code = $2
	`, academicYear, code, leaseSeconds); err != nil {
		return Task{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO brochure_discovery_events (academic_year, school_code, action, from_status, to_status, actor_account_id)
		VALUES ($1, $2, 'claimed', $3, 'searching', $4)
	`, academicYear, code, fromStatus, actorID); err != nil {
		return Task{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Task{}, err
	}
	return r.get(ctx, academicYear, code)
}

func (r *PostgresRepository) SubmitCandidate(ctx context.Context, adminID uuid.UUID, academicYear int, schoolCode string, input CandidateInput) (Task, error) {
	if err := r.requireAdmin(ctx, adminID); err != nil {
		return Task{}, err
	}
	if input.Validate() != nil || input.DetectedAcademicYear != academicYear || academicYear < 100 || academicYear > 999 || !validSchoolCode(schoolCode) {
		return Task{}, ErrInvalid
	}
	evidence, err := json.Marshal(input.Evidence)
	if err != nil {
		return Task{}, ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback(ctx)
	var status string
	if err := tx.QueryRow(ctx, `
		SELECT t.status FROM brochure_discovery_tasks t
		JOIN brochure_discovery_cycles c ON c.academic_year=t.academic_year AND c.status='active'
		WHERE t.academic_year=$1 AND t.school_code=$2 FOR UPDATE OF t
	`, academicYear, schoolCode).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	} else if err != nil {
		return Task{}, err
	} else if status != StatusSearching {
		return Task{}, ErrInvalidStatus
	}
	var documentExists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM brochure_documents
			WHERE academic_year = $1 AND school_code = $2 AND sha256_hex = $3
			  AND source_url IN ($4, $5) AND review_status = 'pending'
		)
	`, academicYear, schoolCode, strings.ToLower(strings.TrimSpace(input.SHA256)), strings.TrimSpace(input.SourceURL), strings.TrimSpace(input.DocumentURL)).Scan(&documentExists); err != nil {
		return Task{}, err
	}
	if !documentExists {
		return Task{}, ErrInvalid
	}
	if _, err := tx.Exec(ctx, `
		UPDATE brochure_discovery_tasks
		SET status = 'under_review', candidate_source_url = $3, candidate_document_url = $4,
		    candidate_sha256_hex = $5, candidate_confidence = $6, candidate_evidence = $7::jsonb,
		    next_search_at = NULL, last_error_code = NULL, last_error_message = NULL
		WHERE academic_year = $1 AND school_code = $2
	`, academicYear, schoolCode, strings.TrimSpace(input.SourceURL), strings.TrimSpace(input.DocumentURL), strings.ToLower(strings.TrimSpace(input.SHA256)), input.Confidence, string(evidence)); err != nil {
		return Task{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO brochure_discovery_events (academic_year, school_code, action, from_status, to_status, actor_account_id, details)
		VALUES ($1, $2, 'candidate_submitted', $3, 'under_review', $4, jsonb_build_object('sha256', $5::text))
	`, academicYear, schoolCode, status, adminID, strings.ToLower(strings.TrimSpace(input.SHA256))); err != nil {
		return Task{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Task{}, err
	}
	return r.get(ctx, academicYear, schoolCode)
}

func (r *PostgresRepository) StoreCandidateSystem(ctx context.Context, academicYear int, schoolCode string, input CandidateInput, document StoredDocumentInput) (Task, string, error) {
	if input.Validate() != nil || input.DetectedAcademicYear != academicYear || academicYear < 100 || academicYear > 999 || !validSchoolCode(schoolCode) ||
		strings.TrimSpace(document.OriginalFileName) == "" || strings.TrimSpace(document.StorageKey) == "" || document.MIMEType != "application/pdf" ||
		document.FileSizeBytes <= 0 || !sha256Pattern.MatchString(strings.ToLower(strings.TrimSpace(document.SHA256))) ||
		!strings.EqualFold(strings.TrimSpace(document.SHA256), strings.TrimSpace(input.SHA256)) {
		return Task{}, "", ErrInvalid
	}
	evidence, err := json.Marshal(input.Evidence)
	if err != nil {
		return Task{}, "", ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Task{}, "", err
	}
	defer tx.Rollback(ctx)
	var taskStatus string
	if err := tx.QueryRow(ctx, `
		SELECT t.status FROM brochure_discovery_tasks t
		JOIN brochure_discovery_cycles c ON c.academic_year=t.academic_year AND c.status='active'
		WHERE t.academic_year=$1 AND t.school_code=$2 FOR UPDATE OF t
	`, academicYear, schoolCode).Scan(&taskStatus); errors.Is(err, pgx.ErrNoRows) {
		return Task{}, "", ErrNotFound
	} else if err != nil {
		return Task{}, "", err
	} else if taskStatus != StatusSearching {
		return Task{}, "", ErrInvalidStatus
	}
	var oldStorageKey, oldDocumentStatus string
	err = tx.QueryRow(ctx, `SELECT storage_key, review_status FROM brochure_documents WHERE academic_year=$1 AND school_code=$2 FOR UPDATE`, academicYear, schoolCode).Scan(&oldStorageKey, &oldDocumentStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		oldStorageKey, oldDocumentStatus = "", "-"
	} else if err != nil {
		return Task{}, "", err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO brochure_documents (academic_year, school_code, storage_key, original_file_name, mime_type, file_size_bytes, sha256_hex, source_url, review_status, published_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'pending',NULL)
		ON CONFLICT (academic_year, school_code) DO UPDATE SET
			storage_key=EXCLUDED.storage_key, original_file_name=EXCLUDED.original_file_name,
			mime_type=EXCLUDED.mime_type, file_size_bytes=EXCLUDED.file_size_bytes,
			sha256_hex=EXCLUDED.sha256_hex, source_url=EXCLUDED.source_url,
			review_status='pending', published_at=NULL, updated_at=CURRENT_TIMESTAMP
	`, academicYear, schoolCode, document.StorageKey, document.OriginalFileName, document.MIMEType, document.FileSizeBytes, strings.ToLower(document.SHA256), strings.TrimSpace(input.DocumentURL))
	if err != nil {
		return Task{}, "", err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO brochure_document_events (academic_year,school_code,storage_key,original_file_name,sha256_hex,action,from_status,to_status,reason)
		VALUES ($1,$2,$3,$4,$5,'discovered',$6,'pending','External discovery candidate')
	`, academicYear, schoolCode, document.StorageKey, document.OriginalFileName, strings.ToLower(document.SHA256), oldDocumentStatus); err != nil {
		return Task{}, "", err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE brochure_discovery_tasks SET status='under_review', candidate_source_url=$3,
		candidate_document_url=$4, candidate_sha256_hex=$5, candidate_confidence=$6,
		candidate_evidence=$7::jsonb, next_search_at=NULL, last_error_code=NULL, last_error_message=NULL
		WHERE academic_year=$1 AND school_code=$2
	`, academicYear, schoolCode, strings.TrimSpace(input.SourceURL), strings.TrimSpace(input.DocumentURL), strings.ToLower(document.SHA256), input.Confidence, string(evidence)); err != nil {
		return Task{}, "", err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO brochure_discovery_events (academic_year,school_code,action,from_status,to_status,details)
		VALUES ($1,$2,'candidate_stored',$3,'under_review',jsonb_build_object('sha256',$4::text))
	`, academicYear, schoolCode, taskStatus, strings.ToLower(document.SHA256)); err != nil {
		return Task{}, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return Task{}, "", err
	}
	item, err := r.get(ctx, academicYear, schoolCode)
	return item, oldStorageKey, err
}

func (r *PostgresRepository) MarkFailure(ctx context.Context, adminID uuid.UUID, academicYear int, schoolCode string, input FailureInput) (Task, error) {
	if err := r.requireAdmin(ctx, adminID); err != nil {
		return Task{}, err
	}
	return r.markFailure(ctx, &adminID, academicYear, schoolCode, input)
}

func (r *PostgresRepository) MarkFailureSystem(ctx context.Context, academicYear int, schoolCode string, input FailureInput) (Task, error) {
	return r.markFailure(ctx, nil, academicYear, schoolCode, input)
}

func (r *PostgresRepository) ReportNoMatchSystem(ctx context.Context, academicYear int, schoolCode string) (Task, error) {
	if academicYear < 100 || academicYear > 999 || !validSchoolCode(schoolCode) {
		return Task{}, ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `
		UPDATE brochure_discovery_tasks t
		SET next_search_at=CURRENT_TIMESTAMP + (INTERVAL '1 day' * LEAST(t.attempt_count, 7))
		FROM brochure_discovery_cycles c
		WHERE t.academic_year=$1 AND t.school_code=$2 AND t.status='searching'
		  AND c.academic_year=t.academic_year AND c.status='active'
	`, academicYear, schoolCode)
	if err != nil {
		return Task{}, err
	}
	if result.RowsAffected() != 1 {
		return Task{}, ErrInvalidStatus
	}
	if _, err := tx.Exec(ctx, `INSERT INTO brochure_discovery_events (academic_year,school_code,action,from_status,to_status,details) VALUES ($1,$2,'no_match','searching','searching',jsonb_build_object('retry','backoff'))`, academicYear, schoolCode); err != nil {
		return Task{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Task{}, err
	}
	return r.get(ctx, academicYear, schoolCode)
}

func (r *PostgresRepository) markFailure(ctx context.Context, actorID *uuid.UUID, academicYear int, schoolCode string, input FailureInput) (Task, error) {
	if academicYear < 100 || academicYear > 999 || !validSchoolCode(schoolCode) || input.Validate() != nil {
		return Task{}, ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `
		UPDATE brochure_discovery_tasks t
		SET status = 'needs_attention', last_error_code = $3::varchar, last_error_message = $4::text, next_search_at = NULL
		FROM brochure_discovery_cycles c
		WHERE t.academic_year = $1 AND t.school_code = $2 AND t.status = 'searching'
		  AND c.academic_year=t.academic_year AND c.status='active'
	`, academicYear, schoolCode, strings.TrimSpace(input.Code), strings.TrimSpace(input.Message))
	if err != nil {
		return Task{}, err
	}
	if result.RowsAffected() != 1 {
		return Task{}, ErrInvalidStatus
	}
	if _, err := tx.Exec(ctx, `INSERT INTO brochure_discovery_events (academic_year, school_code, action, from_status, to_status, actor_account_id, details) VALUES ($1,$2,'technical_failure','searching','needs_attention',$3::uuid,jsonb_build_object('code',$4::text))`, academicYear, schoolCode, actorID, strings.TrimSpace(input.Code)); err != nil {
		return Task{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Task{}, err
	}
	return r.get(ctx, academicYear, schoolCode)
}

func (r *PostgresRepository) Retry(ctx context.Context, adminID uuid.UUID, academicYear int, schoolCode string) (Task, error) {
	if err := r.requireAdmin(ctx, adminID); err != nil {
		return Task{}, err
	}
	if academicYear < 100 || academicYear > 999 || !validSchoolCode(schoolCode) {
		return Task{}, ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `UPDATE brochure_discovery_tasks t SET status='searching', next_search_at=CURRENT_TIMESTAMP, last_error_code=NULL, last_error_message=NULL FROM brochure_discovery_cycles c WHERE t.academic_year=$1 AND t.school_code=$2 AND t.status='needs_attention' AND c.academic_year=t.academic_year AND c.status='active'`, academicYear, schoolCode)
	if err != nil {
		return Task{}, err
	}
	if result.RowsAffected() != 1 {
		return Task{}, ErrInvalidStatus
	}
	if _, err := tx.Exec(ctx, `INSERT INTO brochure_discovery_events (academic_year, school_code, action, from_status, to_status, actor_account_id) VALUES ($1,$2,'retry_requested','needs_attention','searching',$3)`, academicYear, schoolCode, adminID); err != nil {
		return Task{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Task{}, err
	}
	return r.get(ctx, academicYear, schoolCode)
}

func (r *PostgresRepository) Review(ctx context.Context, adminID uuid.UUID, academicYear int, schoolCode string, input ReviewInput) (Task, error) {
	if err := r.requireAdmin(ctx, adminID); err != nil {
		return Task{}, err
	}
	if academicYear < 100 || academicYear > 999 || !validSchoolCode(schoolCode) || input.Validate() != nil {
		return Task{}, ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback(ctx)
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM brochure_discovery_tasks WHERE academic_year=$1 AND school_code=$2 FOR UPDATE`, academicYear, schoolCode).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	} else if err != nil {
		return Task{}, err
	} else if status != StatusUnderReview {
		return Task{}, ErrInvalidStatus
	}
	documentStatus := "rejected"
	taskStatus := StatusSearching
	action := "candidate_rejected"
	completionMethod := any(nil)
	if input.Approved {
		documentStatus = "published"
		taskStatus = StatusCompleted
		action = "candidate_confirmed"
		completionMethod = CompletionExternalConfirmed
	}
	documentAction := "rejected"
	if input.Approved {
		documentAction = "published"
	}
	result, err := tx.Exec(ctx, `UPDATE brochure_documents SET review_status=$3, published_at=CASE WHEN $3='published' THEN CURRENT_TIMESTAMP ELSE NULL END, updated_at=CURRENT_TIMESTAMP WHERE academic_year=$1 AND school_code=$2 AND review_status='pending'`, academicYear, schoolCode, documentStatus)
	if err != nil {
		return Task{}, err
	}
	if result.RowsAffected() != 1 {
		return Task{}, ErrInvalidStatus
	}
	if _, err := tx.Exec(ctx, `
		UPDATE brochure_discovery_tasks SET status=$3, completion_method=$4,
		completed_at=CASE WHEN $3='completed' THEN CURRENT_TIMESTAMP ELSE NULL END,
		completed_by=CASE WHEN $3='completed' THEN $5 ELSE NULL END,
		next_search_at=CASE WHEN $3='searching' THEN CURRENT_TIMESTAMP ELSE NULL END
		WHERE academic_year=$1 AND school_code=$2
	`, academicYear, schoolCode, taskStatus, completionMethod, adminID); err != nil {
		return Task{}, err
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		reason = "-"
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO brochure_document_events (academic_year, school_code, storage_key, original_file_name, sha256_hex, action, from_status, to_status, actor_account_id, reason)
		SELECT academic_year, school_code, storage_key, original_file_name, sha256_hex, $3, 'pending', $4, $5, $6
		FROM brochure_documents WHERE academic_year=$1 AND school_code=$2
	`, academicYear, schoolCode, documentAction, documentStatus, adminID, reason); err != nil {
		return Task{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO brochure_discovery_events (academic_year, school_code, action, from_status, to_status, actor_account_id, details) VALUES ($1,$2,$3,'under_review',$4,$5,jsonb_build_object('reason',$6::text))`, academicYear, schoolCode, action, taskStatus, adminID, reason); err != nil {
		return Task{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Task{}, err
	}
	return r.get(ctx, academicYear, schoolCode)
}

func (r *PostgresRepository) CompleteManual(ctx context.Context, adminID uuid.UUID, academicYear int, schoolCode string) (Task, error) {
	if err := r.requireAdmin(ctx, adminID); err != nil {
		return Task{}, err
	}
	if academicYear < 100 || academicYear > 999 || !validSchoolCode(schoolCode) {
		return Task{}, ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback(ctx)
	var oldTaskStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM brochure_discovery_tasks WHERE academic_year=$1 AND school_code=$2 FOR UPDATE`, academicYear, schoolCode).Scan(&oldTaskStatus); errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	} else if err != nil {
		return Task{}, err
	} else if oldTaskStatus == StatusCompleted {
		return Task{}, ErrInvalidStatus
	}
	var oldDocumentStatus string
	if err := tx.QueryRow(ctx, `SELECT review_status FROM brochure_documents WHERE academic_year=$1 AND school_code=$2 FOR UPDATE`, academicYear, schoolCode).Scan(&oldDocumentStatus); errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	} else if err != nil {
		return Task{}, err
	} else if oldDocumentStatus != "pending" && oldDocumentStatus != "published" {
		return Task{}, ErrInvalidStatus
	}
	if oldDocumentStatus == "pending" {
		if _, err := tx.Exec(ctx, `UPDATE brochure_documents SET review_status='published', published_at=CURRENT_TIMESTAMP, updated_at=CURRENT_TIMESTAMP WHERE academic_year=$1 AND school_code=$2`, academicYear, schoolCode); err != nil {
			return Task{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO brochure_document_events (academic_year, school_code, storage_key, original_file_name, sha256_hex, action, from_status, to_status, actor_account_id, reason) SELECT academic_year, school_code, storage_key, original_file_name, sha256_hex, 'published', 'pending', 'published', $3, 'manual upload confirmed' FROM brochure_documents WHERE academic_year=$1 AND school_code=$2`, academicYear, schoolCode, adminID); err != nil {
			return Task{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE brochure_discovery_tasks SET status='completed', completion_method='manual_upload', completed_at=CURRENT_TIMESTAMP, completed_by=$3, next_search_at=NULL, last_error_code=NULL, last_error_message=NULL WHERE academic_year=$1 AND school_code=$2`, academicYear, schoolCode, adminID); err != nil {
		return Task{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO brochure_discovery_events (academic_year, school_code, action, from_status, to_status, actor_account_id) VALUES ($1,$2,'manual_upload_completed',$3,'completed',$4)`, academicYear, schoolCode, oldTaskStatus, adminID); err != nil {
		return Task{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Task{}, err
	}
	return r.get(ctx, academicYear, schoolCode)
}

func (r *PostgresRepository) ConfirmNoBrochure(ctx context.Context, adminID uuid.UUID, academicYear int, schoolCode, reason string) (Task, error) {
	if err := r.requireAdmin(ctx, adminID); err != nil {
		return Task{}, err
	}
	reason = strings.TrimSpace(reason)
	if academicYear < 100 || academicYear > 999 || !validSchoolCode(schoolCode) || reason == "" || len([]rune(reason)) > 2000 {
		return Task{}, ErrInvalid
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback(ctx)
	var oldStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM brochure_discovery_tasks WHERE academic_year=$1 AND school_code=$2 FOR UPDATE`, academicYear, schoolCode).Scan(&oldStatus); errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	} else if err != nil {
		return Task{}, err
	} else if oldStatus == StatusCompleted || oldStatus == StatusUnderReview {
		return Task{}, ErrInvalidStatus
	}
	if _, err := tx.Exec(ctx, `UPDATE brochure_discovery_tasks SET status='completed', completion_method='no_brochure_confirmed', completed_at=CURRENT_TIMESTAMP, completed_by=$3, next_search_at=NULL, last_error_code=NULL, last_error_message=NULL WHERE academic_year=$1 AND school_code=$2`, academicYear, schoolCode, adminID); err != nil {
		return Task{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO brochure_discovery_events (academic_year, school_code, action, from_status, to_status, actor_account_id, details) VALUES ($1,$2,'no_brochure_confirmed',$3,'completed',$4,jsonb_build_object('reason',$5::text))`, academicYear, schoolCode, oldStatus, adminID, reason); err != nil {
		return Task{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Task{}, err
	}
	return r.get(ctx, academicYear, schoolCode)
}

func (r *PostgresRepository) get(ctx context.Context, academicYear int, schoolCode string) (Task, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT t.academic_year, t.school_code, s.school_name, t.status,
		       COALESCE(t.completion_method, ''), t.attempt_count,
		       COALESCE(t.candidate_source_url, ''), COALESCE(t.candidate_document_url, ''),
		       COALESCE(t.candidate_sha256_hex, ''), t.candidate_confidence,
		       COALESCE(t.candidate_evidence, '{}'::jsonb),
		       COALESCE(t.last_error_code, ''), COALESCE(t.last_error_message, ''),
		       t.last_searched_at, t.next_search_at, t.completed_at, t.completed_by,
		       t.created_at, t.updated_at
		FROM brochure_discovery_tasks t JOIN schools s ON s.school_code=t.school_code
		WHERE t.academic_year=$1 AND t.school_code=$2
	`, academicYear, schoolCode)
	item, err := scanTask(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	return item, err
}

type rowScanner interface{ Scan(...any) error }

func scanTask(row rowScanner) (Task, error) {
	var item Task
	var evidence []byte
	err := row.Scan(&item.AcademicYear, &item.SchoolCode, &item.SchoolName, &item.Status,
		&item.CompletionMethod, &item.AttemptCount, &item.CandidateSourceURL, &item.CandidateDocumentURL,
		&item.CandidateSHA256, &item.CandidateConfidence, &evidence,
		&item.LastErrorCode, &item.LastErrorMessage, &item.LastSearchedAt, &item.NextSearchAt,
		&item.CompletedAt, &item.CompletedBy, &item.CreatedAt, &item.UpdatedAt)
	if err == nil && len(evidence) > 0 {
		err = json.Unmarshal(evidence, &item.CandidateEvidence)
	}
	return item, err
}

func validSchoolCode(value string) bool {
	return len(value) == 3 && value != "000" && strings.Trim(value, "0123456789") == ""
}
