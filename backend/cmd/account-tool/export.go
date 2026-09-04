package main

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"sta-backend/internal/auth"
)

// exportAccount gathers everything the platform stores for one account. PII
// columns are decrypted; session/OAuth secrets are never included.
func exportAccount(ctx context.Context, pool *pgxpool.Pool, cipher *auth.FieldCipher, id uuid.UUID) (map[string]any, error) {
	bundle := map[string]any{
		"exported_at": time.Now().UTC(),
		"account_id":  id,
	}

	// account
	{
		var (
			username        string
			emailCiphertext []byte
			identityStatus  string
			accountStatus   string
			emailVerifiedAt *time.Time
			lastLoginAt     *time.Time
			createdAt       time.Time
		)
		if err := pool.QueryRow(ctx, `
			SELECT username, email_ciphertext, identity_status, account_status,
			       email_verified_at, last_login_at, created_at
			FROM accounts WHERE id = $1`, id).
			Scan(&username, &emailCiphertext, &identityStatus, &accountStatus,
				&emailVerifiedAt, &lastLoginAt, &createdAt); err != nil {
			return nil, err
		}
		email, _ := cipher.Open(emailCiphertext)
		bundle["account"] = map[string]any{
			"username":          username,
			"email":             email,
			"identity_status":   identityStatus,
			"account_status":    accountStatus,
			"email_verified_at": emailVerifiedAt,
			"last_login_at":     lastLoginAt,
			"created_at":        createdAt,
		}
	}

	type q struct {
		key  string
		sql  string
		cols []string
	}
	// Straight row dumps that need no decryption.
	plain := []q{
		{"roles", `SELECT role, created_at FROM account_roles WHERE account_id = $1 ORDER BY created_at`, []string{"role", "created_at"}},
		{"sessions", `SELECT created_at, last_seen_at, expires_at, revoked_at FROM account_sessions WHERE account_id = $1 ORDER BY created_at`, []string{"created_at", "last_seen_at", "expires_at", "revoked_at"}},
		{"oauth_identities", `SELECT provider, created_at FROM oauth_identities WHERE account_id = $1 ORDER BY created_at`, []string{"provider", "created_at"}},
		{"applications", `SELECT academic_year, school_code, program_code, status, candidate_number_last4, created_at FROM applications WHERE account_id = $1 ORDER BY created_at`, []string{"academic_year", "school_code", "program_code", "status", "candidate_number_last4", "created_at"}},
		{"student_verifications", `SELECT academic_year, school_code, program_code, status, expires_at, created_at FROM student_verifications WHERE account_id = $1 ORDER BY created_at`, []string{"academic_year", "school_code", "program_code", "status", "expires_at", "created_at"}},
		{"verification_requests", `SELECT method, status, created_at, reviewed_at FROM verification_requests WHERE account_id = $1 ORDER BY created_at`, []string{"method", "status", "created_at", "reviewed_at"}},
		{"forum_posts", `SELECT thread_id, body, status, created_at FROM forum_posts WHERE account_id = $1 ORDER BY created_at`, []string{"thread_id", "body", "status", "created_at"}},
		{"chat_messages", `SELECT body, status, created_at FROM chat_messages WHERE author_account_id = $1 ORDER BY created_at`, []string{"body", "status", "created_at"}},
		{"support_messages", `SELECT ticket_id, body, status, created_at FROM support_messages WHERE author_account_id = $1 ORDER BY created_at`, []string{"ticket_id", "body", "status", "created_at"}},
		{"support_tickets", `SELECT id, subject, category, status, created_at FROM support_tickets WHERE account_id = $1 ORDER BY created_at`, []string{"id", "subject", "category", "status", "created_at"}},
		{"portfolio_files", `SELECT f.original_file_name, f.mime_type, f.file_size_bytes, f.status, f.created_at FROM portfolio_files f JOIN portfolio_projects p ON p.id = f.project_id WHERE p.account_id = $1 ORDER BY f.created_at`, []string{"original_file_name", "mime_type", "file_size_bytes", "status", "created_at"}},
		{"experiences", `SELECT id, visibility, created_at FROM experiences WHERE author_account_id = $1 ORDER BY created_at`, []string{"id", "visibility", "created_at"}},
		{"experience_revisions", `SELECT r.title, r.body, r.admission_outcome, r.review_status, r.created_at FROM experience_revisions r JOIN experiences e ON e.id = r.experience_id WHERE e.author_account_id = $1 ORDER BY r.created_at`, []string{"title", "body", "admission_outcome", "review_status", "created_at"}},
	}
	for _, item := range plain {
		rows, err := dumpRows(ctx, pool, item.sql, item.cols, id)
		if err != nil {
			return nil, err
		}
		bundle[item.key] = rows
	}

	// notifications need decryption.
	{
		rows, err := pool.Query(ctx, `
			SELECT kind, title_ciphertext, body_ciphertext, read_at, created_at
			FROM notifications WHERE account_id = $1 ORDER BY created_at`, id)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		list := make([]map[string]any, 0)
		for rows.Next() {
			var (
				kind              string
				titleC, bodyC     []byte
				readAt, createdAt *time.Time
			)
			if err := rows.Scan(&kind, &titleC, &bodyC, &readAt, &createdAt); err != nil {
				return nil, err
			}
			title, _ := cipher.Open(titleC)
			body, _ := cipher.Open(bodyC)
			list = append(list, map[string]any{
				"kind": kind, "title": title, "body": body,
				"read_at": readAt, "created_at": createdAt,
			})
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		bundle["notifications"] = list
	}

	// telegram link presence only.
	{
		var linked bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM telegram_account_links WHERE account_id = $1)`, id).Scan(&linked); err != nil {
			return nil, err
		}
		bundle["telegram_linked"] = linked
	}

	return bundle, nil
}

func dumpRows(ctx context.Context, pool *pgxpool.Pool, sql string, cols []string, args ...any) ([]map[string]any, error) {
	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		rec := make(map[string]any, len(cols))
		for i, c := range cols {
			if i < len(values) {
				rec[c] = values[i]
			}
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}
