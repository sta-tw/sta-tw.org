package applications

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"sta-backend/internal/admissions"
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

func (r *PostgresRepository) CreateConfirmed(ctx context.Context, accountID uuid.UUID, identifiers []admissions.ProgramIdentifier) ([]Application, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin application transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	result := make([]Application, 0, len(identifiers))
	for _, identifier := range identifiers {
		var applicationIDText string
		err := tx.QueryRow(ctx, `
		INSERT INTO applications (account_id, academic_year, school_code, program_code, status, locked_at)
		SELECT $1, p.academic_year, p.school_code, p.program_code, 'confirmed', CURRENT_TIMESTAMP
		FROM academic_programs p
		JOIN accounts ac ON ac.id = $1 AND ac.account_status = 'active' AND ac.identity_status = 'student'
		WHERE p.academic_year = $2 AND p.school_code = $3 AND p.program_code = $4
		  AND p.review_status = 'published'
			RETURNING id::text
		`, accountID, identifier.AcademicYear, identifier.SchoolCode, identifier.ProgramCode).Scan(&applicationIDText)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, mapRepositoryError(err)
		}
		applicationID, err := uuid.Parse(applicationIDText)
		if err != nil {
			return nil, fmt.Errorf("parse application id: %w", err)
		}
		application, err := loadApplication(ctx, tx, applicationID)
		if err != nil {
			return nil, err
		}
		if err := ensureForumSpaces(ctx, tx, application); err != nil {
			return nil, err
		}
		result = append(result, application)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit application transaction: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) ListByAccount(ctx context.Context, accountID uuid.UUID) ([]Application, error) {
	rows, err := r.pool.Query(ctx, applicationSelect+` WHERE a.account_id = $1 ORDER BY a.academic_year DESC, a.school_code, a.program_code`, accountID)
	if err != nil {
		return nil, fmt.Errorf("list applications: %w", err)
	}
	defer rows.Close()
	result := make([]Application, 0)
	for rows.Next() {
		application, err := scanApplication(rows)
		if err != nil {
			return nil, fmt.Errorf("scan application: %w", err)
		}
		result = append(result, application)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applications: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) CreateServiceTicket(ctx context.Context, accountID uuid.UUID, identifier admissions.ProgramIdentifier, reason string) (ServiceTicket, error) {
	var ticket ServiceTicket
	var idText string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO application_service_tickets
			(account_id, requested_academic_year, requested_school_code, requested_program_code, reason)
		SELECT $1, $2, $3, $4, $5
		FROM accounts ac
		WHERE ac.id = $1 AND ac.account_status = 'active' AND ac.identity_status = 'student'
		RETURNING id::text, reason, status, created_at
	`, accountID, identifier.AcademicYear, identifier.SchoolCode, identifier.ProgramCode, reason).Scan(&idText, &ticket.Reason, &ticket.Status, &ticket.CreatedAt)
	if err != nil {
		return ServiceTicket{}, mapRepositoryError(err)
	}
	ticket.ID, err = uuid.Parse(idText)
	if err != nil {
		return ServiceTicket{}, fmt.Errorf("parse service ticket id: %w", err)
	}
	ticket.ProgramIdentifier = identifier.String()
	return ticket, nil
}

func (r *PostgresRepository) ListOpenServiceTickets(ctx context.Context, adminID uuid.UUID) ([]ServiceTicket, error) {
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrAdminRequired
	}
	rows, err := r.pool.Query(ctx, serviceTicketSelect+` WHERE t.status = 'open' ORDER BY t.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ServiceTicket, 0)
	for rows.Next() {
		item, err := scanServiceTicket(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) ReviewServiceTicket(ctx context.Context, adminID, ticketID uuid.UUID, input ServiceTicketReviewInput) (ServiceTicket, error) {
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return ServiceTicket{}, err
	} else if !ok {
		return ServiceTicket{}, ErrAdminRequired
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ServiceTicket{}, err
	}
	defer tx.Rollback(ctx)
	var accountID uuid.UUID
	var academicYear int
	var schoolCode, programCode, status string
	err = tx.QueryRow(ctx, `
		SELECT account_id, requested_academic_year, requested_school_code, requested_program_code, status
		FROM application_service_tickets WHERE id = $1 FOR UPDATE
	`, ticketID).Scan(&accountID, &academicYear, &schoolCode, &programCode, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ServiceTicket{}, ErrNotFound
	}
	if err != nil {
		return ServiceTicket{}, err
	}
	if status != "open" {
		return ServiceTicket{}, ErrInvalidStatus
	}
	newStatus := "rejected"
	if input.Approved {
		newStatus = "approved"
		var applicationID uuid.UUID
		err := tx.QueryRow(ctx, `
			INSERT INTO applications (account_id, academic_year, school_code, program_code, status, locked_at)
			SELECT $1, p.academic_year, p.school_code, p.program_code, 'confirmed', CURRENT_TIMESTAMP
			FROM academic_programs p
			JOIN accounts ac ON ac.id = $1 AND ac.account_status = 'active' AND ac.identity_status = 'student'
			WHERE p.academic_year = $2 AND p.school_code = $3 AND p.program_code = $4 AND p.review_status = 'published'
			RETURNING id
		`, accountID, academicYear, schoolCode, programCode).Scan(&applicationID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ServiceTicket{}, ErrNotFound
		}
		if err != nil {
			return ServiceTicket{}, mapRepositoryError(err)
		}
		application, err := loadApplication(ctx, tx, applicationID)
		if err != nil {
			return ServiceTicket{}, err
		}
		if err := ensureForumSpaces(ctx, tx, application); err != nil {
			return ServiceTicket{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE application_service_tickets SET status = $2, reviewed_by = $3, reviewed_at = CURRENT_TIMESTAMP WHERE id = $1`, ticketID, newStatus, adminID); err != nil {
		return ServiceTicket{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_log (actor_account_id, action, entity_type, entity_key, before_data, after_data, reason)
		VALUES ($1, 'review', 'application_service_ticket', $2, jsonb_build_object('status', 'open'), jsonb_build_object('status', $3::text), $4)
	`, adminID, ticketID.String(), newStatus, emptyReviewReason(input.Reason)); err != nil {
		return ServiceTicket{}, err
	}
	var ticket ServiceTicket
	var idText string
	err = tx.QueryRow(ctx, serviceTicketSelect+` WHERE t.id = $1`, ticketID).Scan(
		&idText, &ticket.ProgramIdentifier, &ticket.Reason, &ticket.Status, &ticket.CreatedAt,
	)
	if err != nil {
		return ServiceTicket{}, err
	}
	ticket.ID, err = uuid.Parse(idText)
	if err != nil {
		return ServiceTicket{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ServiceTicket{}, err
	}
	return ticket, nil
}

func (r *PostgresRepository) IsAdmin(ctx context.Context, accountID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM account_roles WHERE account_id = $1 AND role = 'admin')`, accountID).Scan(&exists)
	return exists, err
}

const applicationSelect = `
	SELECT a.id::text, a.academic_year, a.school_code, s.school_name,
	       a.program_code, p.admission_program_name, p.program_identifier,
	       a.status, a.locked_at
	FROM applications a
	JOIN academic_programs p ON p.academic_year = a.academic_year
	  AND p.school_code = a.school_code AND p.program_code = a.program_code
	JOIN schools s ON s.school_code = a.school_code`

const serviceTicketSelect = `
	SELECT t.id::text, p.program_identifier, t.reason, t.status, t.created_at
	FROM application_service_tickets t
	JOIN academic_programs p ON p.academic_year = t.requested_academic_year
	  AND p.school_code = t.requested_school_code AND p.program_code = t.requested_program_code`

type applicationRowScanner interface {
	Scan(...any) error
}

type applicationQueryer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func loadApplication(ctx context.Context, queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, applicationID uuid.UUID) (Application, error) {
	return scanApplication(queryer.QueryRow(ctx, applicationSelect+` WHERE a.id = $1`, applicationID))
}

func scanApplication(row applicationRowScanner) (Application, error) {
	var result Application
	var idText string
	err := row.Scan(&idText, &result.AcademicYear, &result.SchoolCode, &result.SchoolName,
		&result.ProgramCode, &result.ProgramName, &result.ProgramIdentifier,
		&result.Status, &result.LockedAt)
	if err != nil {
		return Application{}, err
	}
	result.ID, err = uuid.Parse(idText)
	if err != nil {
		return Application{}, err
	}
	return result, nil
}

func mapRepositoryError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return ErrConflict
		case "23503":
			return ErrNotFound
		}
	}
	return err
}

func scanServiceTicket(row interface{ Scan(...any) error }) (ServiceTicket, error) {
	var ticket ServiceTicket
	var idText string
	if err := row.Scan(&idText, &ticket.ProgramIdentifier, &ticket.Reason, &ticket.Status, &ticket.CreatedAt); err != nil {
		return ServiceTicket{}, err
	}
	var err error
	ticket.ID, err = uuid.Parse(idText)
	return ticket, err
}

func emptyReviewReason(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return strings.TrimSpace(value)
}

func ensureForumSpaces(ctx context.Context, tx applicationQueryer, application Application) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO forum_spaces (space_type, display_name, academic_year)
		VALUES ('annual', ($1::smallint)::text || ' 特選論壇', $1)
		ON CONFLICT (academic_year) WHERE space_type = 'annual' DO NOTHING
	`, application.AcademicYear); err != nil {
		return fmt.Errorf("ensure annual forum space: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO forum_spaces (space_type, display_name, academic_year, school_code, program_code)
		VALUES ('school_program', $1 || ' ' || $2, $3, $4, $5)
		ON CONFLICT (academic_year, school_code, program_code) WHERE space_type = 'school_program' DO NOTHING
	`, application.SchoolName, application.ProgramName, application.AcademicYear, application.SchoolCode, application.ProgramCode); err != nil {
		return fmt.Errorf("ensure school program forum space: %w", err)
	}
	return nil
}
