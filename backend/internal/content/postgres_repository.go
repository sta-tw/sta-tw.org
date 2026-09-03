package content

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"sta-backend/internal/pagination"
)

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) (*PostgresRepository, error) {
	if pool == nil {
		return nil, errors.New("postgres pool is nil")
	}
	return &PostgresRepository{pool: pool}, nil
}

func (r *PostgresRepository) ListSpaces(ctx context.Context, accountID *uuid.UUID) ([]Space, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT f.id::text, f.space_type, f.display_name, f.academic_year,
		       COALESCE(f.school_code, ''), COALESCE(f.program_code, ''),
		       EXISTS (SELECT 1 FROM forum_memberships m WHERE m.space_id = f.id AND m.account_id = $1 AND m.status = 'active')
		FROM forum_spaces f
		WHERE f.is_active = TRUE
		  AND (
			f.space_type = 'global'
			OR ($1::uuid IS NOT NULL AND f.space_type = 'annual' AND EXISTS (
				SELECT 1 FROM applications a JOIN accounts ac ON ac.id = a.account_id
				WHERE a.account_id = $1 AND ac.identity_status = 'student' AND a.academic_year = f.academic_year AND a.status = 'confirmed'
			))
			OR ($1::uuid IS NOT NULL AND f.space_type = 'school_program' AND EXISTS (
				SELECT 1 FROM applications a WHERE a.account_id = $1 AND a.academic_year = f.academic_year
				  AND a.school_code = f.school_code AND a.program_code = f.program_code AND a.status = 'confirmed'
			))
		  )
		ORDER BY f.space_type, f.academic_year DESC NULLS LAST, f.school_code, f.program_code
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("list forum spaces: %w", err)
	}
	defer rows.Close()
	result := make([]Space, 0)
	for rows.Next() {
		var space Space
		var idText string
		if err := rows.Scan(&idText, &space.SpaceType, &space.DisplayName, &space.AcademicYear, &space.SchoolCode, &space.ProgramCode, &space.Joined); err != nil {
			return nil, fmt.Errorf("scan forum space: %w", err)
		}
		space.ID, err = uuid.Parse(idText)
		if err != nil {
			return nil, fmt.Errorf("parse forum space id: %w", err)
		}
		result = append(result, space)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) JoinSpace(ctx context.Context, accountID, spaceID uuid.UUID) error {
	command, err := r.pool.Exec(ctx, `
		INSERT INTO forum_memberships (account_id, space_id, status, joined_at, removed_at)
		SELECT $1, f.id, 'active', CURRENT_TIMESTAMP, NULL
		FROM forum_spaces f
		WHERE f.id = $2 AND f.is_active = TRUE
		  AND (
			f.space_type = 'global'
			OR (f.space_type = 'annual' AND EXISTS (SELECT 1 FROM applications a JOIN accounts ac ON ac.id = a.account_id WHERE a.account_id = $1 AND ac.identity_status = 'student' AND a.academic_year = f.academic_year AND a.status = 'confirmed'))
			OR (f.space_type = 'school_program' AND EXISTS (SELECT 1 FROM applications a WHERE a.account_id = $1 AND a.academic_year = f.academic_year AND a.school_code = f.school_code AND a.program_code = f.program_code AND a.status = 'confirmed'))
		  )
		ON CONFLICT (account_id, space_id) DO UPDATE SET status = 'active', removed_at = NULL
	`, accountID, spaceID)
	if err != nil {
		return mapContentError(err)
	}
	if command.RowsAffected() != 1 {
		return ErrForbidden
	}
	return nil
}

func (r *PostgresRepository) LeaveSpace(ctx context.Context, accountID, spaceID uuid.UUID) error {
	command, err := r.pool.Exec(ctx, `UPDATE forum_memberships SET status = 'removed', removed_at = CURRENT_TIMESTAMP WHERE account_id = $1 AND space_id = $2`, accountID, spaceID)
	if err != nil {
		return fmt.Errorf("leave forum space: %w", err)
	}
	if command.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) ListThreads(ctx context.Context, accountID *uuid.UUID, spaceID uuid.UUID, limit int, after pagination.Cursor) ([]Thread, string, error) {
	limit = pagination.ClampLimit(limit, 50, 100)
	var afterTime *time.Time
	var afterID *uuid.UUID
	if !after.Zero() {
		t := after.Time
		id := after.UUID()
		afterTime, afterID = &t, &id
	}
	rows, err := r.pool.Query(ctx, `
		SELECT t.id::text, t.space_id, t.title, t.created_at, t.updated_at
		FROM forum_threads t
		JOIN forum_spaces f ON f.id = t.space_id AND f.is_active = TRUE
		WHERE t.space_id = $1 AND t.status = 'published'
		  AND ($3::timestamptz IS NULL OR (t.updated_at, t.id) < ($3::timestamptz, $4::uuid))
		  AND (
			f.space_type = 'global'
			OR ($2::uuid IS NOT NULL AND f.space_type = 'annual' AND EXISTS (
				SELECT 1 FROM applications a JOIN accounts ac ON ac.id = a.account_id
				WHERE a.account_id = $2 AND ac.identity_status = 'student' AND a.academic_year = f.academic_year AND a.status = 'confirmed'
			))
			OR ($2::uuid IS NOT NULL AND f.space_type = 'school_program' AND EXISTS (
				SELECT 1 FROM applications a
				WHERE a.account_id = $2 AND a.academic_year = f.academic_year AND a.school_code = f.school_code
				  AND a.program_code = f.program_code AND a.status = 'confirmed'
			))
		  )
		ORDER BY t.updated_at DESC, t.id DESC
		LIMIT $5
	`, spaceID, accountID, afterTime, afterID, limit)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	result := make([]Thread, 0)
	for rows.Next() {
		var thread Thread
		var idText string
		if err := rows.Scan(&idText, &thread.SpaceID, &thread.Title, &thread.CreatedAt, &thread.UpdatedAt); err != nil {
			return nil, "", err
		}
		thread.ID, err = uuid.Parse(idText)
		if err != nil {
			return nil, "", err
		}
		result = append(result, thread)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	var next string
	if n := len(result); n > 0 {
		last := result[n-1]
		next = pagination.Next(n, limit, last.UpdatedAt, last.ID)
	}
	return result, next, nil
}

func (r *PostgresRepository) ListPosts(ctx context.Context, accountID *uuid.UUID, threadID uuid.UUID, limit int, after pagination.Cursor) ([]Post, string, error) {
	limit = pagination.ClampLimit(limit, 50, 100)
	var afterTime *time.Time
	var afterID *uuid.UUID
	if !after.Zero() {
		t := after.Time
		id := after.UUID()
		afterTime, afterID = &t, &id
	}
	rows, err := r.pool.Query(ctx, `
		SELECT p.id::text, p.thread_id, p.body, p.quoted_experience_id, p.created_at
		FROM forum_posts p
		JOIN forum_threads t ON t.id = p.thread_id AND t.status = 'published'
		JOIN forum_spaces f ON f.id = t.space_id AND f.is_active = TRUE
		WHERE p.thread_id = $1 AND p.status = 'published'
		  AND ($3::timestamptz IS NULL OR (p.created_at, p.id) > ($3::timestamptz, $4::uuid))
		  AND (
			f.space_type = 'global'
			OR ($2::uuid IS NOT NULL AND f.space_type = 'annual' AND EXISTS (
				SELECT 1 FROM applications a JOIN accounts ac ON ac.id = a.account_id
				WHERE a.account_id = $2 AND ac.identity_status = 'student' AND a.academic_year = f.academic_year AND a.status = 'confirmed'
			))
			OR ($2::uuid IS NOT NULL AND f.space_type = 'school_program' AND EXISTS (
				SELECT 1 FROM applications a
				WHERE a.account_id = $2 AND a.academic_year = f.academic_year AND a.school_code = f.school_code
				  AND a.program_code = f.program_code AND a.status = 'confirmed'
			))
		  )
		ORDER BY p.created_at ASC, p.id ASC
		LIMIT $5
	`, threadID, accountID, afterTime, afterID, limit)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	result := make([]Post, 0)
	for rows.Next() {
		var post Post
		var idText string
		if err := rows.Scan(&idText, &post.ThreadID, &post.Body, &post.QuotedExperienceID, &post.CreatedAt); err != nil {
			return nil, "", err
		}
		post.ID, err = uuid.Parse(idText)
		if err != nil {
			return nil, "", err
		}
		result = append(result, post)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	var next string
	if n := len(result); n > 0 {
		last := result[n-1]
		next = pagination.Next(n, limit, last.CreatedAt, last.ID)
	}
	return result, next, nil
}

func (r *PostgresRepository) CreateThread(ctx context.Context, accountID, spaceID uuid.UUID, title, body string) (Thread, Post, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Thread{}, Post{}, fmt.Errorf("begin forum thread: %w", err)
	}
	defer tx.Rollback(ctx)
	var threadIDText, postIDText string
	var thread Thread
	var post Post
	err = tx.QueryRow(ctx, `
		INSERT INTO forum_threads (space_id, account_id, title)
		SELECT $1, $2, $3
		WHERE EXISTS (
			SELECT 1
			FROM forum_memberships m
			JOIN forum_spaces f ON f.id = m.space_id AND f.is_active = TRUE
			WHERE m.account_id = $2 AND m.space_id = $1 AND m.status = 'active'
			  AND (
				f.space_type = 'global'
				OR (f.space_type = 'annual' AND EXISTS (
					SELECT 1 FROM applications a
					JOIN accounts ac ON ac.id = a.account_id
					WHERE a.account_id = $2 AND ac.identity_status = 'student'
					  AND a.academic_year = f.academic_year AND a.status = 'confirmed'
				))
				OR (f.space_type = 'school_program' AND EXISTS (
					SELECT 1 FROM applications a
					WHERE a.account_id = $2 AND a.academic_year = f.academic_year
					  AND a.school_code = f.school_code AND a.program_code = f.program_code
					  AND a.status = 'confirmed'
				))
			  )
		)
		RETURNING id::text, space_id, title, created_at, updated_at
	`, spaceID, accountID, title).Scan(&threadIDText, &thread.SpaceID, &thread.Title, &thread.CreatedAt, &thread.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Thread{}, Post{}, ErrForbidden
	}
	if err != nil {
		return Thread{}, Post{}, mapContentError(err)
	}
	thread.ID, err = uuid.Parse(threadIDText)
	if err != nil {
		return Thread{}, Post{}, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO forum_posts (thread_id, account_id, body) VALUES ($1, $2, $3) RETURNING id::text, thread_id, body, created_at`, thread.ID, accountID, body).Scan(&postIDText, &post.ThreadID, &post.Body, &post.CreatedAt)
	if err != nil {
		return Thread{}, Post{}, fmt.Errorf("create first forum post: %w", err)
	}
	post.ID, err = uuid.Parse(postIDText)
	if err != nil {
		return Thread{}, Post{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Thread{}, Post{}, err
	}
	return thread, post, nil
}

func (r *PostgresRepository) CreatePost(ctx context.Context, accountID, threadID uuid.UUID, body string, quote *uuid.UUID) (Post, error) {
	var post Post
	var idText string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO forum_posts (thread_id, account_id, body, quoted_experience_id)
		SELECT $1, $2, $3, $4
		WHERE EXISTS (
			SELECT 1 FROM forum_threads t
			JOIN forum_spaces f ON f.id = t.space_id AND f.is_active = TRUE
			JOIN forum_memberships m ON m.space_id = t.space_id AND m.account_id = $2 AND m.status = 'active'
			WHERE t.id = $1 AND t.status = 'published'
			  AND (
				f.space_type = 'global'
				OR (f.space_type = 'annual' AND EXISTS (
					SELECT 1 FROM applications a
					JOIN accounts ac ON ac.id = a.account_id
					WHERE a.account_id = $2 AND ac.identity_status = 'student'
					  AND a.academic_year = f.academic_year AND a.status = 'confirmed'
				))
				OR (f.space_type = 'school_program' AND EXISTS (
					SELECT 1 FROM applications a
					WHERE a.account_id = $2 AND a.academic_year = f.academic_year
					  AND a.school_code = f.school_code AND a.program_code = f.program_code
					  AND a.status = 'confirmed'
				))
			  )
			  AND ($4::uuid IS NULL OR EXISTS (SELECT 1 FROM experiences e WHERE e.id = $4 AND e.visibility = 'published'))
		)
		RETURNING id::text, thread_id, body, quoted_experience_id, created_at
	`, threadID, accountID, body, quote).Scan(&idText, &post.ThreadID, &post.Body, &post.QuotedExperienceID, &post.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Post{}, ErrForbidden
	}
	if err != nil {
		return Post{}, mapContentError(err)
	}
	post.ID, err = uuid.Parse(idText)
	return post, err
}

func (r *PostgresRepository) CreateExperience(ctx context.Context, accountID uuid.UUID, input CreateExperienceInput) (Experience, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Experience{}, err
	}
	defer tx.Rollback(ctx)
	var experienceIDText, revisionIDText string
	var experience Experience
	err = tx.QueryRow(ctx, `INSERT INTO experiences (author_account_id) VALUES ($1) RETURNING id::text, visibility, created_at, updated_at`, accountID).Scan(&experienceIDText, &experience.Visibility, &experience.CreatedAt, &experience.UpdatedAt)
	if err != nil {
		return Experience{}, err
	}
	experience.ID, err = uuid.Parse(experienceIDText)
	if err != nil {
		return Experience{}, err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO experience_revisions (experience_id, version_number, title, body, author_type, admission_outcome, review_status)
		VALUES ($1, 1, $2, $3, $4, $5, 'draft')
		RETURNING id::text
	`, experience.ID, input.Title, input.Body, emptyDash(input.AuthorType), emptyDash(input.AdmissionOutcome)).Scan(&revisionIDText)
	if err != nil {
		return Experience{}, err
	}
	experience.CurrentRevisionID, err = uuid.Parse(revisionIDText)
	if err != nil {
		return Experience{}, err
	}
	experience.RevisionNumber = 1
	experience.Title, experience.Body = input.Title, input.Body
	experience.AuthorType, experience.AdmissionOutcome = emptyDash(input.AuthorType), emptyDash(input.AdmissionOutcome)
	if _, err := tx.Exec(ctx, `INSERT INTO experience_revision_events (revision_id, actor_account_id, action, to_status) VALUES ($1, $2, 'created', 'draft')`, experience.CurrentRevisionID, accountID); err != nil {
		return Experience{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Experience{}, err
	}
	return experience, nil
}

func (r *PostgresRepository) CreateRevision(ctx context.Context, accountID, experienceID uuid.UUID, input CreateRevisionInput) (Experience, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Experience{}, err
	}
	defer tx.Rollback(ctx)
	var ownerID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM experiences WHERE id = $1 AND author_account_id = $2 FOR UPDATE`, experienceID, accountID).Scan(&ownerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Experience{}, ErrForbidden
	}
	if err != nil {
		return Experience{}, err
	}
	var revisionIDText string
	err = tx.QueryRow(ctx, `
		INSERT INTO experience_revisions (experience_id, version_number, title, body, author_type, admission_outcome, review_status)
		VALUES ($1, (SELECT COALESCE(MAX(version_number), 0) + 1 FROM experience_revisions WHERE experience_id = $1), $2, $3, $4, $5, 'draft')
		RETURNING id::text
	`, experienceID, input.Title, input.Body, emptyDash(input.AuthorType), emptyDash(input.AdmissionOutcome)).Scan(&revisionIDText)
	if err != nil {
		return Experience{}, mapContentError(err)
	}
	revisionID, err := uuid.Parse(revisionIDText)
	if err != nil {
		return Experience{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO experience_revision_events (revision_id, actor_account_id, action, to_status) VALUES ($1, $2, 'created', 'draft')`, revisionID, accountID); err != nil {
		return Experience{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Experience{}, err
	}
	return r.GetExperience(ctx, &accountID, experienceID)
}

func (r *PostgresRepository) SubmitRevision(ctx context.Context, accountID, revisionID uuid.UUID) (Experience, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Experience{}, err
	}
	defer tx.Rollback(ctx)
	var experienceID uuid.UUID
	var oldStatus string
	err = tx.QueryRow(ctx, `
		SELECT r.experience_id, r.review_status
		FROM experience_revisions r JOIN experiences e ON e.id = r.experience_id
		WHERE r.id = $1 AND e.author_account_id = $2 FOR UPDATE
	`, revisionID, accountID).Scan(&experienceID, &oldStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return Experience{}, ErrNotFound
	}
	if err != nil {
		return Experience{}, err
	}
	if oldStatus != "draft" && oldStatus != "rejected" {
		return Experience{}, ErrInvalidStatus
	}
	if _, err := tx.Exec(ctx, `UPDATE experience_revisions SET review_status = 'pending_review', rejection_reason = NULL WHERE id = $1`, revisionID); err != nil {
		return Experience{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO experience_revision_events (revision_id, actor_account_id, action, from_status, to_status) VALUES ($1, $2, 'submitted', $3, 'pending_review')`, revisionID, accountID, oldStatus); err != nil {
		return Experience{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Experience{}, err
	}
	return r.GetExperience(ctx, &accountID, experienceID)
}

func (r *PostgresRepository) UnpublishExperience(ctx context.Context, accountID, experienceID uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var visibility string
	var revisionID uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(e.current_public_revision_id, (
			SELECT r.id FROM experience_revisions r
			WHERE r.experience_id = e.id
			ORDER BY r.version_number DESC
			LIMIT 1
		)), e.visibility
		FROM experiences e
		WHERE e.id = $1 AND e.author_account_id = $2
		FOR UPDATE
	`, experienceID, accountID).Scan(&revisionID, &visibility)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if visibility == "unpublished" {
		return tx.Commit(ctx)
	}
	if _, err := tx.Exec(ctx, `UPDATE experiences SET visibility = 'unpublished' WHERE id = $1`, experienceID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO experience_revision_events
			(revision_id, actor_account_id, action, from_status, to_status, reason)
		VALUES ($1, $2, 'unpublished', $3, 'unpublished', 'author unpublished experience')
	`, revisionID, accountID, visibility); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) ListPublishedExperiences(ctx context.Context, limit int, after pagination.Cursor) ([]Experience, string, error) {
	limit = pagination.ClampLimit(limit, 50, 100)
	var afterTime *time.Time
	var afterID *uuid.UUID
	if !after.Zero() {
		t := after.Time
		id := after.UUID()
		afterTime, afterID = &t, &id
	}
	rows, err := r.pool.Query(ctx, experienceSelect+`
		WHERE e.visibility = 'published' AND r.review_status = 'approved'
		  AND ($2::timestamptz IS NULL OR (e.updated_at, e.id) < ($2::timestamptz, $3::uuid))
		ORDER BY e.updated_at DESC, e.id DESC
		LIMIT $1`, limit, afterTime, afterID)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items, err := scanExperiences(rows)
	if err != nil {
		return nil, "", err
	}
	var next string
	if n := len(items); n > 0 {
		last := items[n-1]
		next = pagination.Next(n, limit, last.UpdatedAt, last.ID)
	}
	return items, next, nil
}

func (r *PostgresRepository) GetExperience(ctx context.Context, accountID *uuid.UUID, experienceID uuid.UUID) (Experience, error) {
	var owner any
	if accountID != nil {
		owner = *accountID
	}
	rows, err := r.pool.Query(ctx, `
		SELECT e.id::text, e.visibility, r.id, r.title, r.body, r.author_type, r.admission_outcome,
		       r.version_number, e.created_at, e.updated_at
		FROM experiences e
		JOIN LATERAL (
			SELECT r.*
			FROM experience_revisions r
			WHERE r.experience_id = e.id
			  AND ((e.visibility = 'published' AND r.id = e.current_public_revision_id AND r.review_status = 'approved')
			       OR ($2::uuid IS NOT NULL AND e.author_account_id = $2))
			ORDER BY r.version_number DESC
			LIMIT 1
		) r ON TRUE
		WHERE e.id = $1
	`, experienceID, owner)
	if err != nil {
		return Experience{}, err
	}
	defer rows.Close()
	items, err := scanExperiences(rows)
	if err != nil {
		return Experience{}, err
	}
	if len(items) == 0 {
		return Experience{}, ErrNotFound
	}
	return items[0], nil
}

func (r *PostgresRepository) getLatestExperience(ctx context.Context, experienceID uuid.UUID) (Experience, error) {
	var item Experience
	var idText string
	err := r.pool.QueryRow(ctx, `
		SELECT e.id::text, e.visibility, r.id, r.title, r.body, r.author_type, r.admission_outcome,
		       r.version_number, e.created_at, e.updated_at
		FROM experiences e
		JOIN LATERAL (
			SELECT r.*
			FROM experience_revisions r
			WHERE r.experience_id = e.id
			ORDER BY r.version_number DESC
			LIMIT 1
		) r ON TRUE
		WHERE e.id = $1
	`, experienceID).Scan(&idText, &item.Visibility, &item.CurrentRevisionID, &item.Title, &item.Body,
		&item.AuthorType, &item.AdmissionOutcome, &item.RevisionNumber, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Experience{}, ErrNotFound
	}
	if err != nil {
		return Experience{}, err
	}
	item.ID, err = uuid.Parse(idText)
	if err != nil {
		return Experience{}, err
	}
	return item, nil
}

func (r *PostgresRepository) ReviewExperience(ctx context.Context, adminID, revisionID uuid.UUID, input ReviewInput) (Experience, error) {
	isAdmin, err := r.IsAdmin(ctx, adminID)
	if err != nil {
		return Experience{}, err
	}
	if !isAdmin {
		return Experience{}, ErrAdminRequired
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Experience{}, err
	}
	defer tx.Rollback(ctx)
	var experienceID uuid.UUID
	var oldStatus string
	err = tx.QueryRow(ctx, `SELECT experience_id, review_status FROM experience_revisions WHERE id = $1 FOR UPDATE`, revisionID).Scan(&experienceID, &oldStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return Experience{}, ErrNotFound
	}
	if err != nil {
		return Experience{}, err
	}
	if oldStatus != "pending_review" {
		return Experience{}, ErrInvalidStatus
	}
	newStatus := "rejected"
	action := "rejected"
	if input.Approved {
		newStatus, action = "approved", "approved"
	}
	if _, err := tx.Exec(ctx, `UPDATE experience_revisions SET review_status = $2, rejection_reason = NULLIF($3, ''), reviewed_by = $4, reviewed_at = CURRENT_TIMESTAMP WHERE id = $1`, revisionID, newStatus, input.Reason, adminID); err != nil {
		return Experience{}, err
	}
	if input.Approved {
		if _, err := tx.Exec(ctx, `UPDATE experiences SET current_public_revision_id = $2, visibility = 'published' WHERE id = $1`, experienceID, revisionID); err != nil {
			return Experience{}, err
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO experience_revision_events (revision_id, actor_account_id, action, from_status, to_status, reason) VALUES ($1, $2, $3, $4, $5, $6)`, revisionID, adminID, action, oldStatus, newStatus, input.Reason); err != nil {
		return Experience{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Experience{}, err
	}
	experience, err := r.GetExperience(ctx, nil, experienceID)
	if err == nil {
		return experience, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Experience{}, err
	}
	// A first revision that is rejected has no public revision yet. The
	// review transaction succeeded, so return the latest private revision to
	// the administrator instead of turning a successful review into a 404.
	return r.getLatestExperience(ctx, experienceID)
}

const experienceSelect = `
	SELECT e.id::text, e.visibility, e.current_public_revision_id,
	       r.title, r.body, r.author_type, r.admission_outcome, r.version_number,
	       e.created_at, e.updated_at
	FROM experiences e
	JOIN experience_revisions r ON r.id = e.current_public_revision_id`

func scanExperiences(rows pgx.Rows) ([]Experience, error) {
	result := make([]Experience, 0)
	for rows.Next() {
		var item Experience
		var idText string
		if err := rows.Scan(&idText, &item.Visibility, &item.CurrentRevisionID, &item.Title, &item.Body, &item.AuthorType, &item.AdmissionOutcome, &item.RevisionNumber, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		var err error
		item.ID, err = uuid.Parse(idText)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) IsAdmin(ctx context.Context, accountID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM account_roles WHERE account_id = $1 AND role = 'admin')`, accountID).Scan(&exists)
	return exists, err
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func mapContentError(err error) error {
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
