//go:build integration

package notifications

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"sta-backend/internal/auth"
	"sta-backend/internal/dbtest"
)

// Verifies migration 000027: a failing email_outbox row backs off with a
// graduated delay and becomes terminal ('abandoned') once attempt_count
// reaches max_attempts, instead of retrying forever.
func TestMarkEmailFailedAbandonsAfterMaxAttempts(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	cipher, err := auth.NewFieldCipher(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	repo, err := NewPostgresRepository(pool, cipher)
	if err != nil {
		t.Fatal(err)
	}

	id := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO email_outbox (id, dedup_key, recipient_ciphertext, payload_ciphertext, status, attempt_count, max_attempts)
		VALUES ($1, $2, $3, $4, 'processing', 1, 3)
	`, id, "backoff-"+id.String(), []byte("recipient"), []byte("payload")); err != nil {
		t.Fatal(err)
	}

	// attempt_count (1) < max_attempts (3): stays retryable, backed off.
	if err := repo.MarkEmailFailed(ctx, id, "smtp temporary failure"); err != nil {
		t.Fatal(err)
	}
	var status string
	var backoffSeconds float64
	if err := pool.QueryRow(ctx, `
		SELECT status, EXTRACT(EPOCH FROM (available_at - CURRENT_TIMESTAMP)) FROM email_outbox WHERE id=$1
	`, id).Scan(&status, &backoffSeconds); err != nil {
		t.Fatal(err)
	}
	if status != "failed" {
		t.Fatalf("status after attempt 1 = %q, want failed", status)
	}
	if backoffSeconds < 20 {
		t.Fatalf("backoff after attempt 1 = %.0fs, want >= ~30s", backoffSeconds)
	}

	// Bump to attempt_count == max_attempts and fail again: now terminal.
	if _, err := pool.Exec(ctx, `UPDATE email_outbox SET attempt_count = max_attempts WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkEmailFailed(ctx, id, "smtp still failing"); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM email_outbox WHERE id=$1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "abandoned" {
		t.Fatalf("status after max attempts = %q, want abandoned", status)
	}
}
