//go:build integration

package auth

import (
	"context"
	"testing"
	"time"

	"sta-backend/internal/dbtest"
)

func TestSessionManagementStore(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	now := time.Now().UTC()

	store, err := NewPostgresStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	accountID := dbtest.InsertAccount(t, ctx, pool, "")

	current, err := store.CreateSession(ctx, accountID, []byte("cur-hash"), []byte("cur-csrf"), now.Add(time.Hour), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	other1, err := store.CreateSession(ctx, accountID, []byte("o1-hash"), []byte("o1-csrf"), now.Add(time.Hour), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSession(ctx, accountID, []byte("o2-hash"), []byte("o2-csrf"), now.Add(time.Hour), nil, nil); err != nil {
		t.Fatal(err)
	}

	list, err := store.ListActiveSessions(ctx, accountID, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("active sessions = %d, want 3", len(list))
	}

	// Revoking a specific session is scoped to the account.
	ok, err := store.RevokeAccountSession(ctx, accountID, other1, now)
	if err != nil || !ok {
		t.Fatalf("RevokeAccountSession = %v, %v", ok, err)
	}
	// A stranger's account id cannot revoke it (already revoked, but also wrong owner).
	strangerOK, err := store.RevokeAccountSession(ctx, dbtest.InsertAccount(t, ctx, pool, ""), current, now)
	if err != nil || strangerOK {
		t.Fatalf("cross-account revoke = %v, %v (want false, nil)", strangerOK, err)
	}

	n, err := store.RevokeOtherAccountSessions(ctx, accountID, current, now)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 { // other1 already revoked, so only o2 remains to revoke
		t.Fatalf("RevokeOtherAccountSessions revoked %d, want 1", n)
	}

	list, err = store.ListActiveSessions(ctx, accountID, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != current {
		t.Fatalf("after revoke-others, active = %+v, want only the current session", list)
	}
}
