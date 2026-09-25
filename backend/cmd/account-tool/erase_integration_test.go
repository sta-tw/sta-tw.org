//go:build integration

package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"sta-backend/internal/dbtest"
)

func TestEraseAccountScrubsPII(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	id := dbtest.InsertAccount(t, ctx, pool, "")

	mustExec(t, ctx, pool, `
		INSERT INTO account_sessions (account_id, token_hash, csrf_token_hash, ip_hash, user_agent_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP + INTERVAL '1 day')`,
		id, []byte("tok-"+id.String()), []byte("csrf-"+id.String()), []byte("ip"), []byte("ua"))
	mustExec(t, ctx, pool, `
		INSERT INTO notifications (account_id, kind, dedup_key, title_ciphertext, body_ciphertext)
		VALUES ($1, 'system', 'd1', $2, $3)`, id, []byte("t"), []byte("b"))
	mustExec(t, ctx, pool, `
		INSERT INTO oauth_identities (account_id, provider, provider_subject_hash)
		VALUES ($1, 'google', $2)`, id, []byte("subj-"+id.String()))
	mustExec(t, ctx, pool, `
		INSERT INTO password_reset_challenges (account_id, token_hash, expires_at)
		VALUES ($1, $2, CURRENT_TIMESTAMP + INTERVAL '1 hour')`, id, []byte("prc-"+id.String()))

	// Dry run makes no changes.
	dry, err := eraseAccount(ctx, pool, id, "gdpr test", false)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if dry.Applied {
		t.Fatal("dry run reported Applied=true")
	}
	if dry.RowCounts["notifications.deleted"] != 1 || dry.RowCounts["oauth_identities.deleted"] != 1 {
		t.Fatalf("dry run counts wrong: %+v", dry.RowCounts)
	}
	var stillActive string
	if err := pool.QueryRow(ctx, `SELECT account_status FROM accounts WHERE id = $1`, id).Scan(&stillActive); err != nil {
		t.Fatal(err)
	}
	if stillActive != "active" {
		t.Fatalf("dry run mutated account_status to %q", stillActive)
	}

	// Real run.
	rep, err := eraseAccount(ctx, pool, id, "gdpr test", true)
	if err != nil {
		t.Fatalf("erase: %v", err)
	}
	if !rep.Applied {
		t.Fatal("erase reported Applied=false")
	}

	var (
		username    string
		status      string
		emailLookup []byte
		verifiedAt  *time.Time
	)
	if err := pool.QueryRow(ctx,
		`SELECT username, account_status, email_lookup_hash, email_verified_at FROM accounts WHERE id = $1`, id).
		Scan(&username, &status, &emailLookup, &verifiedAt); err != nil {
		t.Fatal(err)
	}
	if status != "deleted" {
		t.Fatalf("account_status = %q, want deleted", status)
	}
	if len(username) < 7 || username[:7] != "erased-" {
		t.Fatalf("username not tombstoned: %q", username)
	}
	if string(emailLookup) == "lookup-"+id.String() {
		t.Fatal("email_lookup_hash was not replaced")
	}
	if verifiedAt != nil {
		t.Fatal("email_verified_at not cleared")
	}

	assertCount(t, ctx, pool, 0, `SELECT count(*) FROM notifications WHERE account_id = $1`, id)
	assertCount(t, ctx, pool, 0, `SELECT count(*) FROM oauth_identities WHERE account_id = $1`, id)
	assertCount(t, ctx, pool, 0, `SELECT count(*) FROM password_reset_challenges WHERE account_id = $1`, id)
	assertCount(t, ctx, pool, 1,
		`SELECT count(*) FROM account_sessions WHERE account_id = $1 AND revoked_at IS NOT NULL AND ip_hash IS NULL`, id)
	assertCount(t, ctx, pool, 1,
		`SELECT count(*) FROM audit_log WHERE action = 'account.erased' AND entity_key = $1::text`, id)

	// A second erase must not error (the caller-level guard blocks it in
	// practice, but the SQL itself is safe to re-run).
	if _, err := eraseAccount(ctx, pool, id, "again", true); err != nil {
		t.Fatalf("second erase errored: %v", err)
	}
}

func mustExec(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("seed exec: %v", err)
	}
}

func assertCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want int, sql string, args ...any) {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != want {
		t.Fatalf("count = %d, want %d (%s)", n, want, sql)
	}
}
