package main

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// eraseReport records what an erase run did (or would do, on a dry run).
type eraseReport struct {
	AccountID   uuid.UUID        `json:"account_id"`
	Applied     bool             `json:"applied"`
	Reason      string           `json:"reason"`
	RowCounts   map[string]int64 `json:"row_counts"`
	StorageKeys struct {
		Portfolio          []string `json:"portfolio_files"`
		VerificationDocs   []string `json:"verification_documents"`
		SupportAttachments []string `json:"support_attachments"`
	} `json:"storage_keys_to_delete_out_of_band"`
}

// eraseAccount runs the whole scrub in one transaction. With confirm=false it
// rolls back at the end so the caller sees the row counts without any change.
func eraseAccount(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, reason string, confirm bool) (*eraseReport, error) {
	report := &eraseReport{AccountID: id, Applied: confirm, Reason: reason, RowCounts: map[string]int64{}}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	report.StorageKeys.Portfolio, err = collectKeys(ctx, tx,
		`SELECT f.storage_key FROM portfolio_files f JOIN portfolio_projects p ON p.id = f.project_id WHERE p.account_id = $1`, id)
	if err != nil {
		return nil, err
	}
	report.StorageKeys.VerificationDocs, err = collectKeys(ctx, tx,
		`SELECT d.storage_key FROM verification_documents d JOIN verification_requests r ON r.id = d.request_id WHERE r.account_id = $1`, id)
	if err != nil {
		return nil, err
	}
	report.StorageKeys.SupportAttachments, err = collectKeys(ctx, tx,
		`SELECT a.storage_key FROM support_attachments a WHERE a.ticket_id IN (SELECT id FROM support_tickets WHERE account_id = $1)`, id)
	if err != nil {
		return nil, err
	}

	// step is (label, sql). Each runs with $1 = account id.
	steps := []struct{ label, sql string }{
		// Revoke and de-fingerprint every session.
		{"account_sessions.revoked", `UPDATE account_sessions SET revoked_at = COALESCE(revoked_at, CURRENT_TIMESTAMP), ip_hash = NULL, user_agent_hash = NULL WHERE account_id = $1`},
		// Credentials and transient challenge rows: delete outright.
		{"account_admin_mfa.deleted", `DELETE FROM account_admin_mfa WHERE account_id = $1`},
		{"account_roles.deleted", `DELETE FROM account_roles WHERE account_id = $1`},
		{"oauth_identities.deleted", `DELETE FROM oauth_identities WHERE account_id = $1`},
		{"oauth_states.deleted", `DELETE FROM oauth_states WHERE account_id = $1`},
		{"email_verification_challenges.deleted", `DELETE FROM email_verification_challenges WHERE account_id = $1`},
		{"password_reset_challenges.deleted", `DELETE FROM password_reset_challenges WHERE account_id = $1`},
		{"notifications.deleted", `DELETE FROM notifications WHERE account_id = $1`},
		{"email_outbox.deleted", `DELETE FROM email_outbox WHERE account_id = $1`},
		{"telegram_account_links.deleted", `DELETE FROM telegram_account_links WHERE account_id = $1`},
		// verification_requests cascades to documents + email challenges.
		{"verification_requests.deleted", `DELETE FROM verification_requests WHERE account_id = $1`},
		{"student_verifications.deleted", `DELETE FROM student_verifications WHERE account_id = $1`},
		// Exam candidate number: nullable pair, scrub in place, keep the row
		// (results reconciliation and forum access depend on it).
		{"applications.candidate_scrubbed", `UPDATE applications SET candidate_number_ciphertext = NULL, candidate_number_lookup_hash = NULL, candidate_number_last4 = NULL WHERE account_id = $1`},
		// Support: keep the ticket (staff history) but drop the requester email
		// and de-identify the user's own messages.
		{"support_tickets.email_scrubbed", `UPDATE support_tickets SET requester_email_ciphertext = NULL WHERE account_id = $1`},
		{"support_messages.redacted", `UPDATE support_messages SET body = '[erased]', status = 'deleted', deleted_at = COALESCE(deleted_at, CURRENT_TIMESTAMP) WHERE author_account_id = $1`},
		// Public authored content: de-identify, do not delete.
		{"forum_posts.redacted", `UPDATE forum_posts SET body = '[erased]', status = 'removed' WHERE account_id = $1`},
		{"chat_messages.redacted", `UPDATE chat_messages SET body = '[erased]', status = 'deleted', deleted_at = COALESCE(deleted_at, CURRENT_TIMESTAMP) WHERE author_account_id = $1`},
		{"experiences.unpublished", `UPDATE experiences SET visibility = 'unpublished' WHERE author_account_id = $1`},
		{"experience_revisions.redacted", `UPDATE experience_revisions SET body = '[erased]', title = '[erased]' WHERE experience_id IN (SELECT id FROM experiences WHERE author_account_id = $1)`},
		// Portfolio: hide files and scrub the user-supplied filename.
		{"portfolio_files.redacted", `UPDATE portfolio_files SET status = 'unpublished', original_file_name = '[erased]' WHERE project_id IN (SELECT id FROM portfolio_projects WHERE account_id = $1)`},
	}
	for _, s := range steps {
		tag, err := tx.Exec(ctx, s.sql, id)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", s.label, err)
		}
		report.RowCounts[s.label] = tag.RowsAffected()
	}

	// Finally, tombstone the account row itself. email_ciphertext /
	// email_lookup_hash / username / password_hash are NOT NULL (and the first
	// two UNIQUE), so replace rather than clear.
	tomb := randomTombstone()
	emailLookup := sha256.Sum256([]byte("erased:" + id.String()))
	tag, err := tx.Exec(ctx, `
		UPDATE accounts
		SET username = $2,
		    email_ciphertext = '\x00'::bytea,
		    email_lookup_hash = $3,
		    password_hash = $4,
		    identity_status = 'temporary',
		    account_status = 'deleted',
		    email_verified_at = NULL,
		    last_login_at = NULL,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, id, tomb[:32], emailLookup[:], randomTombstone())
	if err != nil {
		return nil, fmt.Errorf("accounts.tombstoned: %w", err)
	}
	report.RowCounts["accounts.tombstoned"] = tag.RowsAffected()

	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_log (actor_account_id, action, entity_type, entity_key, reason)
		VALUES (NULL, 'account.erased', 'account', $1, $2)
	`, id.String(), reason); err != nil {
		return nil, fmt.Errorf("audit_log: %w", err)
	}

	if !confirm {
		return report, nil // deferred Rollback discards everything
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return report, nil
}

func collectKeys(ctx context.Context, tx pgx.Tx, sql string, id uuid.UUID) ([]string, error) {
	rows, err := tx.Query(ctx, sql, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := make([]string, 0)
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}
