//go:build integration

package auth

import (
	"context"
	"testing"
	"time"

	"sta-backend/internal/dbtest"
)

func TestConsumePasswordResetChallengeIsAtomic(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	store, err := NewPostgresStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	accountID := dbtest.InsertAccount(t, ctx, pool, "")

	// Two live sessions for the account.
	if _, err := store.CreateSession(ctx, accountID, []byte("tok1hash"), []byte("csrf1"), time.Now().Add(time.Hour), nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSession(ctx, accountID, []byte("tok2hash"), []byte("csrf2"), time.Now().Add(time.Hour), nil, nil); err != nil {
		t.Fatal(err)
	}

	token := "reset-token-" + accountID.String()
	if err := store.CreatePasswordResetChallenge(ctx, accountID, HashOpaqueToken(token), time.Now().Add(30*time.Minute), false); err != nil {
		t.Fatalf("CreatePasswordResetChallenge: %v", err)
	}

	// A wrong token does nothing.
	if err := store.ConsumePasswordResetChallenge(ctx, HashOpaqueToken("nope"), "argon2id$new", time.Now()); err != ErrExpired {
		t.Fatalf("wrong token error = %v, want ErrExpired", err)
	}

	if err := store.ConsumePasswordResetChallenge(ctx, HashOpaqueToken(token), "argon2id$new$hash", time.Now()); err != nil {
		t.Fatalf("ConsumePasswordResetChallenge: %v", err)
	}

	var pw string
	if err := pool.QueryRow(ctx, `SELECT password_hash FROM accounts WHERE id=$1`, accountID).Scan(&pw); err != nil {
		t.Fatal(err)
	}
	if pw != "argon2id$new$hash" {
		t.Fatalf("password_hash not updated: %q", pw)
	}

	var live int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM account_sessions WHERE account_id=$1 AND revoked_at IS NULL`, accountID).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if live != 0 {
		t.Fatalf("live sessions after reset = %d, want 0", live)
	}

	// The challenge is single-use.
	if err := store.ConsumePasswordResetChallenge(ctx, HashOpaqueToken(token), "argon2id$again", time.Now()); err != ErrExpired {
		t.Fatalf("reuse error = %v, want ErrExpired", err)
	}
}
