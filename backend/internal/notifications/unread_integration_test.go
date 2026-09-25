//go:build integration

package notifications

import (
	"context"
	"testing"

	"sta-backend/internal/auth"
	"sta-backend/internal/dbtest"
	"sta-backend/internal/pagination"
)

func TestUnreadCountAndMarkAllRead(t *testing.T) {
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
	acct := dbtest.InsertAccount(t, ctx, pool, "")
	other := dbtest.InsertAccount(t, ctx, pool, "")

	for i := 0; i < 4; i++ {
		if _, err := repo.CreateInApp(ctx, acct, "system", "dedup-"+string(rune('a'+i)), "t", "b"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.CreateInApp(ctx, other, "system", "x", "t", "b"); err != nil {
		t.Fatal(err)
	}

	if n, err := repo.UnreadCount(ctx, acct); err != nil || n != 4 {
		t.Fatalf("UnreadCount = %d, %v; want 4", n, err)
	}

	// Mark one read via the single-item path, unread drops to 3.
	page, _, err := repo.List(ctx, acct, 10, pagination.Cursor{})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkRead(ctx, acct, page[0].ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := repo.UnreadCount(ctx, acct); n != 3 {
		t.Fatalf("UnreadCount after one read = %d, want 3", n)
	}

	// Mark all read: 3 remaining flip, count goes to zero.
	updated, err := repo.MarkAllRead(ctx, acct)
	if err != nil || updated != 3 {
		t.Fatalf("MarkAllRead = %d, %v; want 3", updated, err)
	}
	if n, _ := repo.UnreadCount(ctx, acct); n != 0 {
		t.Fatalf("UnreadCount after mark-all = %d, want 0", n)
	}

	// A second call is a no-op and never touches the other account.
	if updated, _ := repo.MarkAllRead(ctx, acct); updated != 0 {
		t.Fatalf("second MarkAllRead affected %d rows, want 0", updated)
	}
	if n, _ := repo.UnreadCount(ctx, other); n != 1 {
		t.Fatalf("other account unread = %d, want 1 (leaked mark-all)", n)
	}
}
