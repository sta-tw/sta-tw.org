package admissions

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const displayLatestAcademicYearSettingKey = "admissions_display_latest_year"

// GetDisplayLatestAcademicYear returns the configured catalog ceiling. If
// no setting exists yet, it follows the newest published year with a positive
// quota so installations keep their existing behavior until an admin picks
// a year explicitly.
func (r *PostgresRepository) GetDisplayLatestAcademicYear(ctx context.Context) (int, bool, error) {
	var storedValue string
	err := r.pool.QueryRow(ctx, `
		SELECT value
		FROM app_settings
		WHERE key = $1
	`, displayLatestAcademicYearSettingKey).Scan(&storedValue)
	if err == nil {
		year, parseErr := strconv.Atoi(strings.TrimSpace(storedValue))
		if parseErr != nil || year < 100 || year > 999 {
			return 0, true, fmt.Errorf("invalid displayed latest academic year setting")
		}
		return year, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, false, fmt.Errorf("read displayed latest academic year setting: %w", err)
	}

	var year int
	if err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(academic_year), 0)
		FROM academic_programs
		WHERE review_status = $1 AND admission_quota > 0
	`, ProgramStatusPublished).Scan(&year); err != nil {
		return 0, false, fmt.Errorf("find latest published academic year: %w", err)
	}
	return year, false, nil
}

// SetDisplayLatestAcademicYear changes the public admissions catalogue's year
// ceiling and records the change in the shared append-only audit log.
func (r *PostgresRepository) SetDisplayLatestAcademicYear(ctx context.Context, adminID uuid.UUID, year int) error {
	if year < 100 || year > 999 {
		return ErrInvalidProgram
	}
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return err
	} else if !ok {
		return ErrAdminRequired
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin displayed latest academic year update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var before string
	err = tx.QueryRow(ctx, `
		SELECT value
		FROM app_settings
		WHERE key = $1
		FOR UPDATE
	`, displayLatestAcademicYearSettingKey).Scan(&before)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read previous displayed latest academic year: %w", err)
	}
	value := strconv.Itoa(year)
	if _, err := tx.Exec(ctx, `
		INSERT INTO app_settings (key, value, updated_by)
		VALUES ($1, $2, $3)
		ON CONFLICT (key) DO UPDATE SET
			value = EXCLUDED.value,
			updated_at = CURRENT_TIMESTAMP,
			updated_by = EXCLUDED.updated_by
	`, displayLatestAcademicYearSettingKey, value, adminID); err != nil {
		return fmt.Errorf("save displayed latest academic year: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_log (
			actor_account_id, action, entity_type, entity_key, before_data, after_data, reason
		)
		VALUES (
			$1,
			'admissions.display_latest_year.updated',
			'app_setting',
			$2,
			CASE WHEN $3 = '' THEN NULL ELSE jsonb_build_object('value', $3::text) END,
			jsonb_build_object('value', $4::text),
			'Updated displayed latest admissions year from the admin console'
		)
	`, adminID, displayLatestAcademicYearSettingKey, before, value); err != nil {
		return fmt.Errorf("audit displayed latest academic year update: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit displayed latest academic year update: %w", err)
	}
	return nil
}
