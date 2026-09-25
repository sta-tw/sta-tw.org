//go:build integration

package telegramcrosscheck

import (
	"context"
	"errors"
	"testing"

	"sta-backend/internal/dbtest"
)

func TestSyncParticipantProvisionsAndBinds(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	repo, err := NewPostgresRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	admin := dbtest.InsertAdmin(t, ctx, pool)
	plain := dbtest.InsertAccount(t, ctx, pool, "")

	if ok, err := repo.IsAdmin(ctx, admin); err != nil || !ok {
		t.Fatalf("IsAdmin(admin) = %v, %v", ok, err)
	}

	if _, err := repo.SyncParticipants(ctx, plain, "x", []PreparedParticipant{{TelegramUserID: 111}}); !errors.Is(err, ErrAdminRequired) {
		t.Fatalf("non-admin sync error = %v, want ErrAdminRequired", err)
	}
	if _, err := repo.SyncParticipants(ctx, admin, "x", nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty sync error = %v, want ErrInvalidInput", err)
	}

	const tgUser = int64(987654)
	results, err := repo.SyncParticipants(ctx, admin, "integration sync", []PreparedParticipant{{
		TelegramUserID:  tgUser,
		Username:        "tg-987654",
		EmailCiphertext: []byte("cipher-987654"),
		EmailLookupHash: []byte("lookup-987654"),
	}})
	if err != nil {
		t.Fatalf("SyncParticipants: %v", err)
	}
	if len(results) != 1 || results[0].TelegramUserID != tgUser {
		t.Fatalf("sync results = %+v", results)
	}

	var provisioned bool
	if err := pool.QueryRow(ctx, `
		SELECT provisioned_for_testing FROM telegram_account_links WHERE telegram_user_id = $1
	`, tgUser).Scan(&provisioned); err != nil {
		t.Fatalf("link not created: %v", err)
	}
	if !provisioned {
		t.Fatal("link should be marked provisioned_for_testing")
	}

	// Re-syncing the same user is idempotent (no duplicate account/link).
	if _, err := repo.SyncParticipants(ctx, admin, "again", []PreparedParticipant{{TelegramUserID: tgUser, Username: "tg-987654"}}); err != nil {
		t.Fatalf("re-sync: %v", err)
	}
	var links int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM telegram_account_links WHERE telegram_user_id = $1`, tgUser).Scan(&links); err != nil {
		t.Fatal(err)
	}
	if links != 1 {
		t.Fatalf("links for user = %d, want 1", links)
	}

	if err := repo.Bind(ctx, BindInput{TelegramUserID: tgUser, PrivateChatID: tgUser}); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if _, err := repo.Dashboard(ctx, tgUser); err != nil {
		t.Fatalf("Dashboard after bind: %v", err)
	}
}
