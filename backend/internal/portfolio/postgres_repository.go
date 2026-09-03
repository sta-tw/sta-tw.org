package portfolio

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

func (r *PostgresRepository) CreateProject(ctx context.Context, accountID, applicationID uuid.UUID, title string) (Project, error) {
	var project Project
	var idText string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO portfolio_projects (account_id, application_id, title)
		SELECT $1, a.id, $3
		FROM applications a
		WHERE a.id = $2 AND a.account_id = $1 AND a.status = 'confirmed'
		RETURNING id::text, application_id, title, created_at, updated_at
	`, accountID, applicationID, title).Scan(&idText, &project.ApplicationID, &project.Title, &project.CreatedAt, &project.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	if err != nil {
		return Project{}, mapPortfolioError(err)
	}
	project.ID, err = uuid.Parse(idText)
	if err != nil {
		return Project{}, fmt.Errorf("parse portfolio project id: %w", err)
	}
	return project, nil
}

func (r *PostgresRepository) ListProjects(ctx context.Context, accountID uuid.UUID) ([]Project, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, application_id, title, created_at, updated_at
		FROM portfolio_projects
		WHERE account_id = $1
		ORDER BY updated_at DESC
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("list portfolio projects: %w", err)
	}
	defer rows.Close()
	result := make([]Project, 0)
	for rows.Next() {
		var project Project
		var idText string
		if err := rows.Scan(&idText, &project.ApplicationID, &project.Title, &project.CreatedAt, &project.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan portfolio project: %w", err)
		}
		project.ID, err = uuid.Parse(idText)
		if err != nil {
			return nil, fmt.Errorf("parse portfolio project id: %w", err)
		}
		result = append(result, project)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) ListFiles(ctx context.Context, accountID, projectID uuid.UUID) ([]File, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT f.id::text, f.project_id, f.version_number, f.original_file_name, f.storage_key,
		       f.mime_type, f.file_size_bytes, f.sha256_hex, f.status, COALESCE(f.rejection_reason, ''),
		       p.account_id, f.created_at, f.updated_at
		FROM portfolio_files f
		JOIN portfolio_projects p ON p.id = f.project_id
		WHERE f.project_id = $1 AND p.account_id = $2
		ORDER BY f.version_number DESC
	`, projectID, accountID)
	if err != nil {
		return nil, fmt.Errorf("list portfolio files: %w", err)
	}
	defer rows.Close()
	result := make([]File, 0)
	for rows.Next() {
		file, err := scanPortfolioFile(rows)
		if err != nil {
			return nil, fmt.Errorf("scan portfolio file: %w", err)
		}
		result = append(result, file)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate portfolio files: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) CreateFile(ctx context.Context, accountID, projectID uuid.UUID, originalName, storageKey, mimeType string, size int64, sha256Hex string) (File, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return File{}, fmt.Errorf("begin portfolio file transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	var lockedProjectID uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT id
		FROM portfolio_projects
		WHERE id = $1 AND account_id = $2
		FOR UPDATE
	`, projectID, accountID).Scan(&lockedProjectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return File{}, ErrNotFound
	}
	if err != nil {
		return File{}, fmt.Errorf("lock portfolio project: %w", err)
	}
	var version int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(version_number), 0) + 1
		FROM portfolio_files
		WHERE project_id = $1
	`, lockedProjectID).Scan(&version); err != nil {
		return File{}, fmt.Errorf("get portfolio file version: %w", err)
	}
	var file File
	var fileIDText string
	err = tx.QueryRow(ctx, `
		INSERT INTO portfolio_files
			(project_id, version_number, original_file_name, storage_key, mime_type, file_size_bytes, sha256_hex, status)
		SELECT $1, $2, $3, $4, $5, $6, $7, 'hidden'
		WHERE EXISTS (SELECT 1 FROM portfolio_projects WHERE id = $1 AND account_id = $8)
		RETURNING id::text, project_id, version_number, original_file_name, storage_key, mime_type,
		          file_size_bytes, sha256_hex, status, COALESCE(rejection_reason, ''),
		          created_at, updated_at
	`, projectID, version, originalName, storageKey, mimeType, size, sha256Hex, accountID).Scan(
		&fileIDText, &file.ProjectID, &file.VersionNumber, &file.OriginalFileName, &file.StorageKey,
		&file.MimeType, &file.FileSizeBytes, &file.SHA256Hex, &file.Status, &file.RejectionReason,
		&file.CreatedAt, &file.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return File{}, ErrNotFound
	}
	if err != nil {
		return File{}, mapPortfolioError(err)
	}
	file.ID, err = uuid.Parse(fileIDText)
	if err != nil {
		return File{}, fmt.Errorf("parse portfolio file id: %w", err)
	}
	file.OwnerAccountID = accountID
	if _, err := tx.Exec(ctx, `INSERT INTO portfolio_file_events (file_id, actor_account_id, action, to_status) VALUES ($1, $2, 'uploaded', 'hidden')`, file.ID, accountID); err != nil {
		return File{}, fmt.Errorf("record portfolio upload: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return File{}, fmt.Errorf("commit portfolio file: %w", err)
	}
	return file, nil
}

func (r *PostgresRepository) GetFile(ctx context.Context, fileID uuid.UUID) (File, error) {
	file, err := scanPortfolioFile(r.pool.QueryRow(ctx, `
		SELECT f.id::text, f.project_id, f.version_number, f.original_file_name, f.storage_key,
		       f.mime_type, f.file_size_bytes, f.sha256_hex, f.status, COALESCE(f.rejection_reason, ''),
		       p.account_id, f.created_at, f.updated_at
		FROM portfolio_files f
		JOIN portfolio_projects p ON p.id = f.project_id
		WHERE f.id = $1
	`, fileID))
	if errors.Is(err, pgx.ErrNoRows) {
		return File{}, ErrNotFound
	}
	if err != nil {
		return File{}, fmt.Errorf("get portfolio file: %w", err)
	}
	return file, nil
}

func (r *PostgresRepository) ListFileEvents(ctx context.Context, accountID, fileID uuid.UUID) ([]FileEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT e.id, e.action, COALESCE(e.from_status, ''), COALESCE(e.to_status, ''),
		       e.reason, COALESCE(e.actor_account_id::text, ''), e.created_at
		FROM portfolio_file_events e
		JOIN portfolio_files f ON f.id = e.file_id
		JOIN portfolio_projects p ON p.id = f.project_id
		WHERE e.file_id = $1 AND p.account_id = $2
		ORDER BY e.created_at DESC, e.id DESC
	`, fileID, accountID)
	if err != nil {
		return nil, fmt.Errorf("list portfolio file events: %w", err)
	}
	defer rows.Close()
	events := make([]FileEvent, 0)
	for rows.Next() {
		event, err := scanPortfolioFileEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan portfolio file event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate portfolio file events: %w", err)
	}
	if len(events) == 0 {
		var exists bool
		if err := r.pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM portfolio_files f
				JOIN portfolio_projects p ON p.id = f.project_id
				WHERE f.id = $1 AND p.account_id = $2
			)
		`, fileID, accountID).Scan(&exists); err != nil {
			return nil, fmt.Errorf("check portfolio file: %w", err)
		}
		if !exists {
			return nil, ErrNotFound
		}
	}
	return events, nil
}

func (r *PostgresRepository) SubmitForReview(ctx context.Context, accountID, fileID uuid.UUID) (File, error) {
	return r.changeOwnerStatus(ctx, accountID, fileID, FileStatusPendingReview, "submitted_for_review", FileStatusHidden, FileStatusUnpublished, FileStatusRejected)
}

func (r *PostgresRepository) Unpublish(ctx context.Context, accountID, fileID uuid.UUID) (File, error) {
	return r.changeOwnerStatus(ctx, accountID, fileID, FileStatusUnpublished, "unpublished", FileStatusPublished)
}

func (r *PostgresRepository) Hide(ctx context.Context, accountID, fileID uuid.UUID) (File, error) {
	return r.changeOwnerStatus(ctx, accountID, fileID, FileStatusHidden, "hidden", FileStatusPublished, FileStatusUnpublished, FileStatusRejected)
}

func (r *PostgresRepository) changeOwnerStatus(ctx context.Context, accountID, fileID uuid.UUID, target, action string, allowed ...string) (File, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return File{}, fmt.Errorf("begin portfolio status transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	var current string
	err = tx.QueryRow(ctx, `SELECT f.status FROM portfolio_files f JOIN portfolio_projects p ON p.id = f.project_id WHERE f.id = $1 AND p.account_id = $2 FOR UPDATE`, fileID, accountID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return File{}, ErrNotFound
	}
	if err != nil {
		return File{}, fmt.Errorf("lock portfolio file: %w", err)
	}
	if !contains(allowed, current) {
		return File{}, ErrInvalidStatus
	}
	if _, err := tx.Exec(ctx, `UPDATE portfolio_files SET status = $2, rejection_reason = NULL, reviewed_by = NULL, reviewed_at = NULL WHERE id = $1`, fileID, target); err != nil {
		return File{}, fmt.Errorf("change portfolio file status: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO portfolio_file_events (file_id, actor_account_id, action, from_status, to_status) VALUES ($1, $2, $3, $4, $5)`, fileID, accountID, action, current, target); err != nil {
		return File{}, fmt.Errorf("record portfolio status: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return File{}, fmt.Errorf("commit portfolio status: %w", err)
	}
	return r.GetFile(ctx, fileID)
}

func (r *PostgresRepository) ReviewFile(ctx context.Context, adminID, fileID uuid.UUID, approved bool, reason string) (File, error) {
	isAdmin, err := r.IsAdmin(ctx, adminID)
	if err != nil {
		return File{}, err
	}
	if !isAdmin {
		return File{}, ErrNotAdmin
	}
	target := "rejected"
	action := "rejected"
	if approved {
		target = "published"
		action = "approved"
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return File{}, fmt.Errorf("begin portfolio review: %w", err)
	}
	defer tx.Rollback(ctx)
	var current string
	err = tx.QueryRow(ctx, `SELECT status FROM portfolio_files WHERE id = $1 FOR UPDATE`, fileID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return File{}, ErrNotFound
	}
	if err != nil {
		return File{}, fmt.Errorf("lock portfolio review file: %w", err)
	}
	if current != FileStatusPendingReview {
		return File{}, ErrInvalidStatus
	}
	if _, err := tx.Exec(ctx, `UPDATE portfolio_files SET status = $2, rejection_reason = NULLIF($3, ''), reviewed_by = $4, reviewed_at = CURRENT_TIMESTAMP WHERE id = $1`, fileID, target, reason, adminID); err != nil {
		return File{}, fmt.Errorf("save portfolio review: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO portfolio_file_events (file_id, actor_account_id, action, from_status, to_status, reason) VALUES ($1, $2, $3, $4, $5, $6)`, fileID, adminID, action, current, target, reason); err != nil {
		return File{}, fmt.Errorf("record portfolio review: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return File{}, fmt.Errorf("commit portfolio review: %w", err)
	}
	return r.GetFile(ctx, fileID)
}

func (r *PostgresRepository) IsAdmin(ctx context.Context, accountID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM account_roles WHERE account_id = $1 AND role = 'admin')`, accountID).Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) ListAdminFiles(ctx context.Context, adminID uuid.UUID, query AdminFileQuery) ([]AdminFile, error) {
	isAdmin, err := r.IsAdmin(ctx, adminID)
	if err != nil {
		return nil, err
	}
	if !isAdmin {
		return nil, ErrNotAdmin
	}
	if err := query.Validate(); err != nil {
		return nil, err
	}

	conditions := []string{"TRUE"}
	args := make([]any, 0, 4)
	argIndex := 1
	if query.Status != "" {
		conditions = append(conditions, fmt.Sprintf("f.status = $%d", argIndex))
		args = append(args, query.Status)
		argIndex++
	}
	if query.ProjectID != uuid.Nil {
		conditions = append(conditions, fmt.Sprintf("f.project_id = $%d", argIndex))
		args = append(args, query.ProjectID)
		argIndex++
	}
	args = append(args, query.Limit, query.Offset)
	limitIndex := argIndex
	offsetIndex := argIndex + 1
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT f.id::text, f.project_id, f.version_number, f.original_file_name, f.storage_key,
		       f.mime_type, f.file_size_bytes, f.sha256_hex, f.status, COALESCE(f.rejection_reason, ''),
		       p.account_id, f.created_at, f.updated_at, p.application_id, p.title
		FROM portfolio_files f
		JOIN portfolio_projects p ON p.id = f.project_id
		WHERE %s
		ORDER BY f.updated_at DESC, f.id DESC
		LIMIT $%d OFFSET $%d
	`, strings.Join(conditions, " AND "), limitIndex, offsetIndex), args...)
	if err != nil {
		return nil, fmt.Errorf("list admin portfolio files: %w", err)
	}
	defer rows.Close()
	result := make([]AdminFile, 0)
	for rows.Next() {
		var file AdminFile
		var idText string
		if err := rows.Scan(&idText, &file.ProjectID, &file.VersionNumber, &file.OriginalFileName, &file.StorageKey,
			&file.MimeType, &file.FileSizeBytes, &file.SHA256Hex, &file.Status, &file.RejectionReason,
			&file.OwnerAccountID, &file.CreatedAt, &file.UpdatedAt, &file.ApplicationID, &file.ProjectTitle); err != nil {
			return nil, fmt.Errorf("scan admin portfolio file: %w", err)
		}
		file.ID, err = uuid.Parse(idText)
		if err != nil {
			return nil, fmt.Errorf("parse admin portfolio file id: %w", err)
		}
		result = append(result, file)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admin portfolio files: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) ListAdminFileEvents(ctx context.Context, adminID, fileID uuid.UUID) ([]FileEvent, error) {
	isAdmin, err := r.IsAdmin(ctx, adminID)
	if err != nil {
		return nil, err
	}
	if !isAdmin {
		return nil, ErrNotAdmin
	}
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM portfolio_files WHERE id = $1)`, fileID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check admin portfolio file: %w", err)
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := r.pool.Query(ctx, `
		SELECT e.id, e.action, COALESCE(e.from_status, ''), COALESCE(e.to_status, ''),
		       e.reason, COALESCE(e.actor_account_id::text, ''), e.created_at
		FROM portfolio_file_events e
		WHERE e.file_id = $1
		ORDER BY e.created_at DESC, e.id DESC
	`, fileID)
	if err != nil {
		return nil, fmt.Errorf("list admin portfolio file events: %w", err)
	}
	defer rows.Close()
	events := make([]FileEvent, 0)
	for rows.Next() {
		event, err := scanPortfolioFileEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan admin portfolio file event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admin portfolio file events: %w", err)
	}
	return events, nil
}

type portfolioRowScanner interface {
	Scan(dest ...any) error
}

func scanPortfolioFile(row portfolioRowScanner) (File, error) {
	var file File
	var idText string
	if err := row.Scan(&idText, &file.ProjectID, &file.VersionNumber, &file.OriginalFileName, &file.StorageKey,
		&file.MimeType, &file.FileSizeBytes, &file.SHA256Hex, &file.Status, &file.RejectionReason,
		&file.OwnerAccountID, &file.CreatedAt, &file.UpdatedAt); err != nil {
		return File{}, err
	}
	id, err := uuid.Parse(idText)
	if err != nil {
		return File{}, fmt.Errorf("parse portfolio file id: %w", err)
	}
	file.ID = id
	return file, nil
}

func scanPortfolioFileEvent(row portfolioRowScanner) (FileEvent, error) {
	var event FileEvent
	var actorText string
	if err := row.Scan(&event.ID, &event.Action, &event.FromStatus, &event.ToStatus, &event.Reason, &actorText, &event.CreatedAt); err != nil {
		return FileEvent{}, err
	}
	if actorText != "" {
		actorID, err := uuid.Parse(actorText)
		if err != nil {
			return FileEvent{}, fmt.Errorf("parse portfolio event actor: %w", err)
		}
		event.ActorAccountID = &actorID
	}
	return event, nil
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func mapPortfolioError(err error) error {
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
