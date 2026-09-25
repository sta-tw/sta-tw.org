package auth

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"sta-backend/internal/security"
)

// verifyAdminTOTP must let a correct code through indefinitely but lock out
// repeated wrong guesses per account.
func TestVerifyAdminTOTPFailureLimiter(t *testing.T) {
	fixed := time.Unix(1_700_000_000, 0).UTC()
	s := &Service{
		mfaLimiter: security.NewFixedWindowLimiter(mfaFailureLimit, mfaFailureWindow, 100),
		now:        func() time.Time { return fixed },
	}

	secret := make([]byte, 20)
	for i := range secret {
		secret[i] = byte(i + 1)
	}
	step := fixed.Unix() / int64(totpPeriod/time.Second)
	goodCode := totpCode(secret, step)

	acct := uuid.New()
	other := uuid.New()
	ctx := context.Background()

	// A correct code always passes, even repeatedly.
	for i := 0; i < mfaFailureLimit+3; i++ {
		if err := s.verifyAdminTOTP(ctx, acct, secret, goodCode); err != nil {
			t.Fatalf("good code attempt %d rejected: %v", i, err)
		}
	}

	// The first mfaFailureLimit wrong codes return "invalid"; then it locks.
	for i := 0; i < mfaFailureLimit; i++ {
		if err := s.verifyAdminTOTP(ctx, acct, secret, "000000"); err != ErrAdminMFAInvalid {
			t.Fatalf("wrong code attempt %d = %v, want ErrAdminMFAInvalid", i, err)
		}
	}
	if err := s.verifyAdminTOTP(ctx, acct, secret, "000000"); err != ErrAdminMFARateLimited {
		t.Fatalf("after %d failures = %v, want ErrAdminMFARateLimited", mfaFailureLimit, err)
	}
	// While locked, even a correct code is refused (Peek short-circuits).
	if err := s.verifyAdminTOTP(ctx, acct, secret, goodCode); err != ErrAdminMFARateLimited {
		t.Fatalf("locked account with good code = %v, want ErrAdminMFARateLimited", err)
	}
	// A different account is unaffected.
	if err := s.verifyAdminTOTP(ctx, other, secret, goodCode); err != nil {
		t.Fatalf("other account rejected: %v", err)
	}
}
