package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeAdminMFAStore struct {
	*fakeAuthStore
	isAdmin              bool
	record               AdminMFARecord
	loaded               bool
	requireAdminMFA      bool
	requireAdminMFAIsSet bool
}

func (f *fakeAdminMFAStore) GetRequireAdminMFA(context.Context) (bool, bool, error) {
	return f.requireAdminMFA, f.requireAdminMFAIsSet, nil
}

func (f *fakeAdminMFAStore) IsAdmin(context.Context, uuid.UUID) (bool, error) {
	return f.isAdmin, nil
}

func (f *fakeAdminMFAStore) IsServiceAccount(context.Context, uuid.UUID) (bool, error) {
	return false, nil
}

func (f *fakeAdminMFAStore) GetAdminMFA(context.Context, uuid.UUID) (AdminMFARecord, error) {
	if !f.loaded {
		return AdminMFARecord{}, ErrNotFound
	}
	return f.record, nil
}

func (f *fakeAdminMFAStore) SaveAdminMFASecret(_ context.Context, _ uuid.UUID, secret []byte, expiresAt time.Time) error {
	f.record = AdminMFARecord{SecretCiphertext: append([]byte(nil), secret...), PendingExpiresAt: &expiresAt}
	f.loaded = true
	return nil
}

func (f *fakeAdminMFAStore) EnableAdminMFA(_ context.Context, _ uuid.UUID, enabledAt time.Time) error {
	f.record.EnabledAt = &enabledAt
	f.record.PendingExpiresAt = nil
	return nil
}

func (f *fakeAdminMFAStore) DisableAdminMFA(context.Context, uuid.UUID) error {
	f.loaded = false
	f.record = AdminMFARecord{}
	return nil
}

func (f *fakeAdminMFAStore) TouchAdminMFAVerified(_ context.Context, _ uuid.UUID, at time.Time) error {
	f.record.LastVerifiedAt = &at
	return nil
}

func TestTOTPVerificationAllowsSmallClockSkew(t *testing.T) {
	secret := []byte("12345678901234567890")
	now := time.Unix(1_000_000, 0).UTC()
	step := now.Unix() / int64(totpPeriod/time.Second)
	code := totpCode(secret, step)
	if got, ok := verifyTOTP(secret, code, now); !ok || got != step {
		t.Fatalf("verifyTOTP() = (%d, %v), want current step", got, ok)
	}
	if _, ok := verifyTOTP(secret, "abcdef", now); ok {
		t.Fatal("verifyTOTP accepted non-numeric code")
	}
}

func TestAdminMFASetupEnableRequireAndDisable(t *testing.T) {
	base := newFakeAuthStore()
	store := &fakeAdminMFAStore{fakeAuthStore: base, isAdmin: true}
	cipher, err := NewFieldCipher(make([]byte, 32))
	if err != nil {
		t.Fatalf("NewFieldCipher() error = %v", err)
	}
	service, err := NewService(store, cipher, make([]byte, 32), time.Hour, false)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	now := time.Unix(1_000_000, 0).UTC()
	service.now = func() time.Time { return now }
	account := Account{ID: uuid.New(), Username: "admin", AccountStatus: "active"}
	base.accounts[account.Username] = account

	setup, err := service.BeginAdminMFA(context.Background(), account.ID, account.Username)
	if err != nil {
		t.Fatalf("BeginAdminMFA() error = %v", err)
	}
	secret, err := decodeTOTPSecret(setup.Secret)
	if err != nil {
		t.Fatalf("decode setup secret: %v", err)
	}
	code := totpCode(secret, now.Unix()/int64(totpPeriod/time.Second))
	if err := service.EnableAdminMFA(context.Background(), account.ID, code); err != nil {
		t.Fatalf("EnableAdminMFA() error = %v", err)
	}
	if err := service.RequireAdminMFA(context.Background(), account.ID, code); err != nil {
		t.Fatalf("RequireAdminMFA(valid) error = %v", err)
	}
	if err := service.RequireAdminMFA(context.Background(), account.ID, "000000"); !errors.Is(err, ErrAdminMFAInvalid) {
		t.Fatalf("RequireAdminMFA(invalid) = %v, want invalid MFA error", err)
	}
	enabled, err := service.AdminMFAStatus(context.Background(), account.ID)
	if err != nil || !enabled {
		t.Fatalf("AdminMFAStatus() = (%v, %v), want enabled", enabled, err)
	}
	if err := service.DisableAdminMFA(context.Background(), account.ID, code); err != nil {
		t.Fatalf("DisableAdminMFA() error = %v", err)
	}
	enabled, err = service.AdminMFAStatus(context.Background(), account.ID)
	if err != nil || enabled {
		t.Fatalf("AdminMFAStatus(after disable) = (%v, %v), want disabled", enabled, err)
	}
}

// TestRequireAdminMFADBSettingOverridesStaticDefault locks in the toggle
// added for the admin backend: an app_settings row must win over whatever
// STA_REQUIRE_ADMIN_MFA was at process start, in both directions, and an
// unset row must fall back to the static default rather than being treated
// as "off".
func TestRequireAdminMFADBSettingOverridesStaticDefault(t *testing.T) {
	base := newFakeAuthStore()
	store := &fakeAdminMFAStore{fakeAuthStore: base, isAdmin: true}
	cipher, err := NewFieldCipher(make([]byte, 32))
	if err != nil {
		t.Fatalf("NewFieldCipher() error = %v", err)
	}
	service, err := NewService(store, cipher, make([]byte, 32), time.Hour, false)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	account := Account{ID: uuid.New(), Username: "admin", AccountStatus: "active"}
	base.accounts[account.Username] = account

	// Static default is "off" (never configured) — never-enrolled account
	// isn't blocked.
	if err := service.RequireAdminMFA(context.Background(), account.ID, ""); err != nil {
		t.Fatalf("RequireAdminMFA(no setting, static off) error = %v, want nil", err)
	}

	// DB says "on" — now it must block, even though the static default is
	// still "off".
	store.requireAdminMFA, store.requireAdminMFAIsSet = true, true
	if err := service.RequireAdminMFA(context.Background(), account.ID, ""); !errors.Is(err, ErrAdminMFARequired) {
		t.Fatalf("RequireAdminMFA(DB on) error = %v, want ErrAdminMFARequired", err)
	}

	// Flip the static default to "on" via ConfigureAdminMFA, but leave the
	// DB row explicitly "off" — the DB row must still win.
	service.ConfigureAdminMFA(true)
	store.requireAdminMFA, store.requireAdminMFAIsSet = false, true
	if err := service.RequireAdminMFA(context.Background(), account.ID, ""); err != nil {
		t.Fatalf("RequireAdminMFA(DB off, static on) error = %v, want nil", err)
	}
}
