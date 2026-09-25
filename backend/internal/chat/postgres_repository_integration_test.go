//go:build integration

package chat

import (
	"context"
	"testing"

	"sta-backend/internal/dbtest"
)

func TestWebsiteMessageEnqueuesOutboxAndBacksOff(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	repo, err := NewPostgresRepository(pool, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	author := dbtest.InsertAccount(t, ctx, pool, "")

	msg, err := repo.CreateWebsiteMessage(ctx, author, "大家好，這是整合測試訊息。")
	if err != nil {
		t.Fatalf("CreateWebsiteMessage: %v", err)
	}

	// A website message fans out to the discord + telegram outbox.
	var queued int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM chat_sync_outbox WHERE message_id = $1`, msg.ID).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 2 {
		t.Fatalf("outbox rows for message = %d, want 2 (discord + telegram)", queued)
	}

	tasks, err := repo.ClaimOutbox(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimOutbox: %v", err)
	}
	if len(tasks) == 0 {
		t.Fatal("ClaimOutbox returned nothing")
	}

	// Fail one delivery: it backs off (not retried immediately) and stays 'failed'
	// while attempt_count < max_attempts.
	if err := repo.MarkOutboxFailed(ctx, tasks[0], "telegram 502"); err != nil {
		t.Fatalf("MarkOutboxFailed: %v", err)
	}
	var status string
	var backoff float64
	if err := pool.QueryRow(ctx, `
		SELECT status, EXTRACT(EPOCH FROM (available_at - CURRENT_TIMESTAMP)) FROM chat_sync_outbox WHERE id = $1
	`, tasks[0].ID).Scan(&status, &backoff); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || backoff < 20 {
		t.Fatalf("after one failure: status=%q backoff=%.0fs, want failed / >=~30s", status, backoff)
	}

	// Exhaust attempts: next failure is terminal.
	if _, err := pool.Exec(ctx, `UPDATE chat_sync_outbox SET attempt_count = max_attempts WHERE id = $1`, tasks[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkOutboxFailed(ctx, tasks[0], "telegram still down"); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM chat_sync_outbox WHERE id = $1`, tasks[0].ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "abandoned" {
		t.Fatalf("status after max attempts = %q, want abandoned", status)
	}
}
