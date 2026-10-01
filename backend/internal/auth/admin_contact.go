package auth

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/url"
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/email"
)

// AdminAccountContact decrypts an account's contact and school email for
// display in the admin user-management UI. schoolEmail is "" when the
// account was never created through school-email registration (see
// AdminContactStore.GetAccountContact).
func (s *Service) AdminAccountContact(ctx context.Context, accountID uuid.UUID) (contactEmail, schoolEmail string, err error) {
	store, ok := s.store.(AdminContactStore)
	if !ok || s.emailCipher == nil {
		return "", "", ErrNotConfigured
	}
	emailCiphertext, schoolEmailCiphertext, _, err := store.GetAccountContact(ctx, accountID)
	if err != nil {
		return "", "", err
	}
	contactEmail, err = s.emailCipher.Open(emailCiphertext)
	if err != nil {
		return "", "", fmt.Errorf("decrypt email: %w", err)
	}
	if len(schoolEmailCiphertext) > 0 {
		schoolEmail, err = s.emailCipher.Open(schoolEmailCiphertext)
		if err != nil {
			return "", "", fmt.Errorf("decrypt school email: %w", err)
		}
	}
	return contactEmail, schoolEmail, nil
}

// AdminUpdateAccountEmail replaces an account's contact (login-notification)
// email — the address self-service password reset and email-verification
// mail go to. ErrConflict means another account already uses this address.
func (s *Service) AdminUpdateAccountEmail(ctx context.Context, accountID uuid.UUID, rawEmail string) error {
	store, ok := s.store.(AdminContactStore)
	if !ok || s.emailCipher == nil {
		return ErrNotConfigured
	}
	normalized, err := normalizeAndValidateEmail(rawEmail)
	if err != nil {
		return err
	}
	ciphertext, err := s.emailCipher.Seal(normalized)
	if err != nil {
		return fmt.Errorf("protect email: %w", err)
	}
	return store.UpdateAccountEmail(ctx, accountID, ciphertext, s.lookupHasher.Hash(normalized))
}

// AdminUpdateAccountSchoolEmail replaces the school email an admin corrects
// after a registration typo — must still be a *.edu.tw address like
// Register requires. ErrConflict means another account already used this
// exact address (the accounts_school_email_lookup_hash_key unique index).
func (s *Service) AdminUpdateAccountSchoolEmail(ctx context.Context, accountID uuid.UUID, rawEmail string) error {
	store, ok := s.store.(AdminContactStore)
	if !ok || s.emailCipher == nil {
		return ErrNotConfigured
	}
	normalized, err := normalizeAndValidateEmail(rawEmail)
	if err != nil {
		return err
	}
	if !isSchoolEmail(normalized) {
		return fmt.Errorf("%w: school email must be a *.edu.tw address", ErrInvalidInput)
	}
	ciphertext, err := s.emailCipher.Seal(normalized)
	if err != nil {
		return fmt.Errorf("protect school email: %w", err)
	}
	return store.UpdateAccountSchoolEmail(ctx, accountID, ciphertext, s.lookupHasher.Hash(normalized))
}

// ErrNoSchoolEmailOnFile is returned by AdminResendActivation when the
// account has no stored school email to resend to — either it predates the
// school_email_ciphertext column, or it was never created through
// school-email registration (see AdminContactStore.GetAccountContact).
var ErrNoSchoolEmailOnFile = fmt.Errorf("%w: no school email on file for this account", ErrInvalidInput)

// AdminResendActivation re-sends the same activation email Register sends,
// to the account's currently stored school email — for an account stuck
// 'pending_verification' because the original address was mistyped or
// unreachable. Call AdminUpdateAccountSchoolEmail first to correct the
// address; this always sends to whatever is on file right now.
func (s *Service) AdminResendActivation(ctx context.Context, accountID uuid.UUID) (time.Time, error) {
	pendingStore, ok := s.store.(PasswordResetStore)
	contactStore, contactOK := s.store.(AdminContactStore)
	if !ok || !contactOK || s.emailNotifier == nil || s.emailCipher == nil {
		return time.Time{}, ErrNotConfigured
	}
	_, schoolEmailCiphertext, accountStatus, err := contactStore.GetAccountContact(ctx, accountID)
	if err != nil {
		return time.Time{}, err
	}
	if accountStatus != "pending_verification" {
		return time.Time{}, fmt.Errorf("%w: account is not pending verification", ErrInvalidInput)
	}
	if len(schoolEmailCiphertext) == 0 {
		return time.Time{}, ErrNoSchoolEmailOnFile
	}

	token, err := NewOpaqueToken(32)
	if err != nil {
		return time.Time{}, err
	}
	expiresAt := s.now().UTC().Add(24 * time.Hour)
	if err := pendingStore.CreatePasswordResetChallenge(ctx, accountID, HashOpaqueToken(token), expiresAt, true); err != nil {
		return time.Time{}, err
	}
	link := s.publicBaseURL + "/reset-password?token=" + url.QueryEscape(token)
	textBody := "請在 24 小時內點擊以下連結，設定密碼以啟用帳號：\n" + link
	logoURL := ""
	if s.publicBaseURL != "" {
		logoURL = s.publicBaseURL + "/logo.svg"
	}
	htmlBody := email.AccountActivationEmail(email.AccountActivationEmailData{
		LogoURL: logoURL,
		SetURL:  link,
	})
	dedupKey := "admin-resend-activation:" + accountID.String() + ":" + hex.EncodeToString(HashOpaqueToken(token))
	if err := s.emailNotifier.EnqueueEmailTo(ctx, accountID, schoolEmailCiphertext, dedupKey, "[特殊選才資源網] 帳號啟用信", textBody, htmlBody); err != nil {
		return time.Time{}, err
	}
	return expiresAt, nil
}
