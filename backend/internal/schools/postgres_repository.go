package schools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) (*PostgresRepository, error) {
	if pool == nil {
		return nil, errors.New("postgres pool is nil")
	}
	return &PostgresRepository{pool: pool}, nil
}

func (r *PostgresRepository) IsAdmin(ctx context.Context, accountID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM account_roles WHERE account_id = $1 AND role = 'admin')`, accountID).Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) List(ctx context.Context, includeInactive bool) ([]School, error) {
	condition := "is_active = TRUE"
	if includeInactive {
		condition = "TRUE"
	}
	rows, err := r.pool.Query(ctx, `
		SELECT school_code, school_name, institution_type, is_active, created_at, updated_at
		FROM schools
		WHERE `+condition+`
		ORDER BY school_code
	`)
	if err != nil {
		return nil, fmt.Errorf("list schools: %w", err)
	}
	defer rows.Close()
	items := make([]School, 0)
	for rows.Next() {
		item, err := scanSchool(rows)
		if err != nil {
			return nil, fmt.Errorf("scan school: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate schools: %w", err)
	}
	return items, nil
}

func (r *PostgresRepository) Upsert(ctx context.Context, adminID uuid.UUID, input BatchInput) ([]School, error) {
	input = normalizeBatch(input)
	if err := input.Validate(); err != nil {
		return nil, err
	}
	isAdmin, err := r.IsAdmin(ctx, adminID)
	if err != nil {
		return nil, err
	}
	if !isAdmin {
		return nil, ErrAdminRequired
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin school update: %w", err)
	}
	defer tx.Rollback(ctx)
	// Serialize school-master writes so a newly allocated or reactivated code
	// cannot be raced by two administrative sync requests.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('sta.school_master'))`); err != nil {
		return nil, fmt.Errorf("lock school master: %w", err)
	}

	result := make([]School, 0, len(input.Items))
	for _, item := range input.Items {
		var before School
		err := tx.QueryRow(ctx, `
			SELECT school_code, school_name, institution_type, is_active, created_at, updated_at
			FROM schools WHERE school_code = $1 FOR UPDATE
		`, item.SchoolCode).Scan(&before.SchoolCode, &before.SchoolName, &before.InstitutionType, &before.IsActive, &before.CreatedAt, &before.UpdatedAt)

		var after School
		var action string
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			err = tx.QueryRow(ctx, `
				INSERT INTO schools (school_code, school_name, institution_type, is_active)
				VALUES ($1, $2, $3, $4)
				RETURNING school_code, school_name, institution_type, is_active, created_at, updated_at
			`, item.SchoolCode, item.SchoolName, item.InstitutionType, *item.IsActive).Scan(
				&after.SchoolCode, &after.SchoolName, &after.InstitutionType, &after.IsActive, &after.CreatedAt, &after.UpdatedAt,
			)
			action = "create"
		case err == nil:
			err = tx.QueryRow(ctx, `
				UPDATE schools
				SET school_name = $2, institution_type = $3, is_active = $4, updated_at = CURRENT_TIMESTAMP
				WHERE school_code = $1
				RETURNING school_code, school_name, institution_type, is_active, created_at, updated_at
			`, item.SchoolCode, item.SchoolName, item.InstitutionType, *item.IsActive).Scan(
				&after.SchoolCode, &after.SchoolName, &after.InstitutionType, &after.IsActive, &after.CreatedAt, &after.UpdatedAt,
			)
			action = schoolAction(&before, after)
		default:
			return nil, fmt.Errorf("load school %s: %w", item.SchoolCode, err)
		}
		if err != nil {
			return nil, fmt.Errorf("upsert school %s: %w", item.SchoolCode, err)
		}
		if !sameSchoolData(before, after) {
			if err := insertAuditEvent(ctx, tx, adminID, action, after.SchoolCode, schoolSnapshot(&before, action == "create"), after.snapshot(), input.Reason); err != nil {
				return nil, err
			}
		}
		result = append(result, after)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit school update: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) ListHistory(ctx context.Context, adminID uuid.UUID, schoolCode string) ([]AuditEvent, error) {
	if err := validateSchoolCode(schoolCode); err != nil {
		return nil, invalidCodeError(schoolCode)
	}
	isAdmin, err := r.IsAdmin(ctx, adminID)
	if err != nil {
		return nil, err
	}
	if !isAdmin {
		return nil, ErrAdminRequired
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, action, entity_key, before_data, after_data, reason, created_at
		FROM audit_log
		WHERE entity_type = 'school' AND entity_key = $1
		ORDER BY created_at DESC, id DESC
	`, schoolCode)
	if err != nil {
		return nil, fmt.Errorf("list school history: %w", err)
	}
	defer rows.Close()
	result := make([]AuditEvent, 0)
	for rows.Next() {
		var event AuditEvent
		var beforeData, afterData []byte
		if err := rows.Scan(&event.ID, &event.Action, &event.EntityKey, &beforeData, &afterData, &event.Reason, &event.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan school history: %w", err)
		}
		event.BeforeData = decodeAuditData(beforeData)
		event.AfterData = decodeAuditData(afterData)
		result = append(result, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate school history: %w", err)
	}
	return result, nil
}

func scanSchool(row interface{ Scan(...any) error }) (School, error) {
	var item School
	err := row.Scan(&item.SchoolCode, &item.SchoolName, &item.InstitutionType, &item.IsActive, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func sameSchoolData(before, after School) bool {
	return before.SchoolCode == after.SchoolCode && before.SchoolName == after.SchoolName && before.InstitutionType == after.InstitutionType && before.IsActive == after.IsActive
}

func schoolSnapshot(before *School, omit bool) map[string]any {
	if omit {
		return nil
	}
	return before.snapshot()
}

func insertAuditEvent(ctx context.Context, tx pgx.Tx, adminID uuid.UUID, action, schoolCode string, before, after map[string]any, reason string) error {
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		return fmt.Errorf("marshal school audit before data: %w", err)
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		return fmt.Errorf("marshal school audit after data: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_log (actor_account_id, action, entity_type, entity_key, before_data, after_data, reason)
		VALUES ($1, $2, 'school', $3, NULLIF($4, 'null')::jsonb, $5::jsonb, $6)
	`, adminID, action, schoolCode, string(beforeJSON), string(afterJSON), strings.TrimSpace(reason)); err != nil {
		return fmt.Errorf("record school audit event: %w", err)
	}
	return nil
}

func decodeAuditData(value []byte) map[string]any {
	if len(value) == 0 || string(value) == "null" {
		return nil
	}
	var data map[string]any
	if json.Unmarshal(value, &data) != nil {
		return nil
	}
	return data
}
