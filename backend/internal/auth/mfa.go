package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" // RFC 6238 TOTP uses HMAC-SHA-1 for compatibility.
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	totpPeriod      = 30 * time.Second
	totpDigits      = 6
	mfaSecretBytes  = 20
	mfaIssuer       = "STA"
	mfaSecretMaxAge = 15 * time.Minute
	totpClockSkew   = int64(1)
)

var (
	ErrAdminMFARequired    = errors.New("administrator MFA is required")
	ErrAdminMFAInvalid     = errors.New("administrator MFA code is invalid")
	ErrAdminMFAConflict    = errors.New("administrator MFA is already enabled")
	ErrAdminMFARateLimited = errors.New("too many administrator MFA attempts")
	ErrAdminRequired       = errors.New("administrator role is required")
)

type AdminMFASetup struct {
	Secret     string
	OTPAuthURL string
	ExpiresAt  time.Time
}

func newTOTPSecret() ([]byte, string, error) {
	secret := make([]byte, mfaSecretBytes)
	if _, err := rand.Read(secret); err != nil {
		return nil, "", fmt.Errorf("generate MFA secret: %w", err)
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
	return secret, encoded, nil
}

func decodeTOTPSecret(raw string) ([]byte, error) {
	value := strings.ToUpper(strings.TrimSpace(raw))
	if value == "" || len(value) > 64 {
		return nil, ErrAdminMFAInvalid
	}
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(value)
	if err != nil || len(secret) < 16 || len(secret) > 64 {
		return nil, ErrAdminMFAInvalid
	}
	return secret, nil
}

func totpCode(secret []byte, step int64) string {
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac := hmac.New(sha1.New, secret)
	_, _ = mac.Write(counter[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", totpDigits, value%1000000)
}

func verifyTOTP(secret []byte, code string, now time.Time) (int64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != totpDigits {
		return 0, false
	}
	for _, character := range code {
		if character < '0' || character > '9' {
			return 0, false
		}
	}
	currentStep := now.Unix() / int64(totpPeriod/time.Second)
	for offset := -totpClockSkew; offset <= totpClockSkew; offset++ {
		step := currentStep + offset
		if step < 0 {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(totpCode(secret, step)), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}

func (s *Service) ConfigureAdminMFA(required bool) {
	if s != nil {
		s.requireAdminMFA = required
	}
}

// verifyAdminTOTP wraps a TOTP check with a per-account failure limiter so a
// stolen admin session cannot brute-force the 6-digit X-MFA-Code. Only failed
// attempts count; a correct code always passes and never trips the limit.
func (s *Service) verifyAdminTOTP(ctx context.Context, accountID uuid.UUID, secret []byte, code string) error {
	if s.mfaLimiter != nil {
		if !s.mfaLimiter.Peek(accountID.String(), s.now().UTC()).Allowed {
			return ErrAdminMFARateLimited
		}
	}
	if _, valid := verifyTOTP(secret, code, s.now()); valid {
		return nil
	}
	if s.mfaLimiter != nil {
		s.mfaLimiter.Take(accountID.String(), s.now().UTC())
	}
	if s.distributedLimiter != nil {
		if allowed, err := s.distributedLimiter.Allow(ctx, "auth-admin-mfa-fail", accountID.String(), mfaFailureLimit, mfaFailureWindow, s.now().UTC()); err == nil && !allowed {
			return ErrAdminMFARateLimited
		}
	}
	return ErrAdminMFAInvalid
}

const (
	mfaFailureLimit  = 8
	mfaFailureWindow = 15 * time.Minute
)

func (s *Service) AdminMFARequired() bool {
	return s != nil && s.requireAdminMFA
}

func (s *Service) AdminMFAStatus(ctx context.Context, accountID uuid.UUID) (bool, error) {
	if err := s.requireAdminAccount(ctx, accountID); err != nil {
		return false, err
	}
	store, ok := s.store.(AdminMFAStore)
	if !ok {
		return false, ErrNotConfigured
	}
	record, err := store.GetAdminMFA(ctx, accountID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return record.EnabledAt != nil, nil
}

func (s *Service) BeginAdminMFA(ctx context.Context, accountID uuid.UUID, username string) (AdminMFASetup, error) {
	if err := s.requireAdminAccount(ctx, accountID); err != nil {
		return AdminMFASetup{}, err
	}
	store, ok := s.store.(AdminMFAStore)
	if !ok || s.emailCipher == nil {
		return AdminMFASetup{}, ErrNotConfigured
	}
	if record, err := store.GetAdminMFA(ctx, accountID); err == nil && record.EnabledAt != nil {
		return AdminMFASetup{}, ErrAdminMFAConflict
	} else if err != nil && !errors.Is(err, ErrNotFound) {
		return AdminMFASetup{}, err
	}
	_, encoded, err := newTOTPSecret()
	if err != nil {
		return AdminMFASetup{}, err
	}
	ciphertext, err := s.emailCipher.Seal(encoded)
	if err != nil {
		return AdminMFASetup{}, err
	}
	expiresAt := s.now().UTC().Add(mfaSecretMaxAge)
	if err := store.SaveAdminMFASecret(ctx, accountID, ciphertext, expiresAt); err != nil {
		return AdminMFASetup{}, err
	}
	return AdminMFASetup{
		Secret:     encoded,
		OTPAuthURL: buildOTPAuthURL(username, encoded),
		ExpiresAt:  expiresAt,
	}, nil
}

func (s *Service) EnableAdminMFA(ctx context.Context, accountID uuid.UUID, code string) error {
	if err := s.requireAdminAccount(ctx, accountID); err != nil {
		return err
	}
	store, ok := s.store.(AdminMFAStore)
	if !ok || s.emailCipher == nil {
		return ErrNotConfigured
	}
	record, err := store.GetAdminMFA(ctx, accountID)
	if errors.Is(err, ErrNotFound) {
		return ErrAdminMFARequired
	}
	if err != nil {
		return err
	}
	if record.EnabledAt != nil {
		return ErrAdminMFAConflict
	}
	if record.PendingExpiresAt == nil || !record.PendingExpiresAt.After(s.now().UTC()) {
		return ErrAdminMFARequired
	}
	secretText, err := s.emailCipher.Open(record.SecretCiphertext)
	if err != nil {
		return ErrAdminMFAInvalid
	}
	secret, err := decodeTOTPSecret(secretText)
	if err != nil {
		return err
	}
	if err := s.verifyAdminTOTP(ctx, accountID, secret, code); err != nil {
		return err
	}
	return store.EnableAdminMFA(ctx, accountID, s.now().UTC())
}

func (s *Service) DisableAdminMFA(ctx context.Context, accountID uuid.UUID, code string) error {
	if err := s.requireAdminAccount(ctx, accountID); err != nil {
		return err
	}
	store, ok := s.store.(AdminMFAStore)
	if !ok || s.emailCipher == nil {
		return ErrNotConfigured
	}
	record, err := store.GetAdminMFA(ctx, accountID)
	if errors.Is(err, ErrNotFound) {
		return ErrAdminMFARequired
	}
	if err != nil {
		return err
	}
	if record.EnabledAt == nil {
		return ErrAdminMFARequired
	}
	secretText, err := s.emailCipher.Open(record.SecretCiphertext)
	if err != nil {
		return ErrAdminMFAInvalid
	}
	secret, err := decodeTOTPSecret(secretText)
	if err != nil {
		return err
	}
	if err := s.verifyAdminTOTP(ctx, accountID, secret, code); err != nil {
		return err
	}
	return store.DisableAdminMFA(ctx, accountID)
}

// RequireAdminMFA is called after the endpoint has established the admin role.
// A code is deliberately required on every admin request instead of creating a
// long-lived second session token, so a stolen session alone cannot access the
// admin surface while MFA is enabled.
func (s *Service) RequireAdminMFA(ctx context.Context, accountID uuid.UUID, code string) error {
	store, ok := s.store.(AdminMFAStore)
	if !ok || s.emailCipher == nil {
		if s.requireAdminMFA {
			return ErrNotConfigured
		}
		return nil
	}
	record, err := store.GetAdminMFA(ctx, accountID)
	if errors.Is(err, ErrNotFound) {
		if s.requireAdminMFA {
			return ErrAdminMFARequired
		}
		return nil
	}
	if err != nil {
		return err
	}
	if record.EnabledAt == nil {
		if s.requireAdminMFA {
			return ErrAdminMFARequired
		}
		return nil
	}
	secretText, err := s.emailCipher.Open(record.SecretCiphertext)
	if err != nil {
		return ErrAdminMFAInvalid
	}
	secret, err := decodeTOTPSecret(secretText)
	if err != nil {
		return ErrAdminMFAInvalid
	}
	return s.verifyAdminTOTP(ctx, accountID, secret, code)
}

func (s *Service) requireAdminAccount(ctx context.Context, accountID uuid.UUID) error {
	store, ok := s.store.(AdminRoleStore)
	if !ok {
		return ErrNotConfigured
	}
	admin, err := store.IsAdmin(ctx, accountID)
	if err != nil {
		return err
	}
	if !admin {
		return ErrAdminRequired
	}
	return nil
}

func buildOTPAuthURL(username, secret string) string {
	label := url.QueryEscape(mfaIssuer + ":" + strings.TrimSpace(username))
	return "otpauth://totp/" + label + "?secret=" + url.QueryEscape(secret) + "&issuer=" + url.QueryEscape(mfaIssuer) + "&algorithm=SHA1&digits=6&period=30"
}
