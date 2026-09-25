//go:build integration

package auth

import (
	"context"
	"testing"
	"time"

	"sta-backend/internal/dbtest"
)

func TestAdminMFAGrantWindow(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	store, err := NewPostgresStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := NewFieldCipher(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}

	clock := time.Unix(1_700_000_000, 0).UTC()
	svc, err := NewService(store, cipher, make([]byte, 32), time.Hour, false)
	if err != nil {
		t.Fatal(err)
	}
	svc.now = func() time.Time { return clock }
	svc.ConfigureAdminMFA(true)
	svc.ConfigureAdminMFAGrant(15 * time.Minute)

	admin := dbtest.InsertAdmin(t, ctx, pool)

	// Enrol + enable MFA.
	setup, err := svc.BeginAdminMFA(ctx, admin, "admin")
	if err != nil {
		t.Fatalf("BeginAdminMFA: %v", err)
	}
	secret, err := decodeTOTPSecret(setup.Secret)
	if err != nil {
		t.Fatal(err)
	}
	codeAt := func(at time.Time) string {
		return totpCode(secret, at.Unix()/int64(totpPeriod/time.Second))
	}
	if err := svc.EnableAdminMFA(ctx, admin, codeAt(clock)); err != nil {
		t.Fatalf("EnableAdminMFA: %v", err)
	}

	// Right after enabling, a code-less admin request is covered by the grant.
	if err := svc.RequireAdminMFA(ctx, admin, ""); err != nil {
		t.Fatalf("grant right after enable: %v", err)
	}

	// 14 minutes later: still inside the window.
	clock = clock.Add(14 * time.Minute)
	if err := svc.RequireAdminMFA(ctx, admin, ""); err != nil {
		t.Fatalf("grant at 14m: %v", err)
	}

	// 16 minutes after the last verification: window closed, code required.
	clock = clock.Add(16 * time.Minute)
	if err := svc.RequireAdminMFA(ctx, admin, ""); err != ErrAdminMFARequired {
		t.Fatalf("expired grant = %v, want ErrAdminMFARequired", err)
	}

	// An explicit verify reopens the window and returns its expiry.
	expiresAt, err := svc.VerifyAdminMFA(ctx, admin, codeAt(clock))
	if err != nil {
		t.Fatalf("VerifyAdminMFA: %v", err)
	}
	if !expiresAt.Equal(clock.Add(15 * time.Minute)) {
		t.Fatalf("expires_at = %v, want %v", expiresAt, clock.Add(15*time.Minute))
	}
	if err := svc.RequireAdminMFA(ctx, admin, ""); err != nil {
		t.Fatalf("grant after explicit verify: %v", err)
	}

	// A wrong code never opens the window.
	clock = clock.Add(20 * time.Minute)
	if _, err := svc.VerifyAdminMFA(ctx, admin, "000000"); err != ErrAdminMFAInvalid {
		t.Fatalf("VerifyAdminMFA(wrong) = %v, want ErrAdminMFAInvalid", err)
	}
	if err := svc.RequireAdminMFA(ctx, admin, ""); err != ErrAdminMFARequired {
		t.Fatalf("grant after failed verify = %v, want ErrAdminMFARequired", err)
	}

	// With the grant disabled, every request needs a code again.
	svc.adminMFAGrantTTL = 0
	if _, err := svc.VerifyAdminMFA(ctx, admin, codeAt(clock)); err != nil {
		t.Fatalf("VerifyAdminMFA (ttl 0): %v", err)
	}
	if err := svc.RequireAdminMFA(ctx, admin, ""); err != ErrAdminMFARequired {
		t.Fatalf("grant disabled = %v, want ErrAdminMFARequired", err)
	}
}
