package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	ErrExpired  = errors.New("expired")
)

type Account struct {
	ID             uuid.UUID `json:"id"`
	Username       string    `json:"username"`
	IdentityStatus string    `json:"identity_status"`
	AccountStatus  string    `json:"account_status"`
	EmailVerified  bool      `json:"email_verified"`
}

type Session struct {
	ID        uuid.UUID
	Account   Account
	CSRFHash  []byte
	ExpiresAt time.Time
	TokenHash []byte
}

type OAuthState struct {
	Provider               string
	AccountID              *uuid.UUID
	CodeVerifierCiphertext []byte
	RedirectURL            string
}

type Store interface {
	CreateAccount(ctx context.Context, username string, emailCiphertext, emailLookupHash []byte, passwordHash string) (Account, error)
	FindAccountByUsername(ctx context.Context, username string) (Account, string, error)
	FindAccountByID(ctx context.Context, accountID uuid.UUID) (Account, error)
	CreateSession(ctx context.Context, accountID uuid.UUID, tokenHash, csrfHash []byte, expiresAt time.Time, ipHash, userAgentHash []byte) (uuid.UUID, error)
	FindActiveSession(ctx context.Context, tokenHash []byte, now time.Time) (Session, error)
	TouchSession(ctx context.Context, sessionID uuid.UUID, now time.Time) error
	RevokeSession(ctx context.Context, sessionID uuid.UUID, now time.Time) error
	CreateOAuthBinding(ctx context.Context, accountID uuid.UUID, provider string, providerSubjectHash []byte) error
	FindAccountByOAuthSubject(ctx context.Context, provider string, providerSubjectHash []byte) (Account, error)
	CreateOAuthState(ctx context.Context, provider string, accountID *uuid.UUID, stateHash, codeVerifierCiphertext []byte, redirectURL string, expiresAt time.Time) error
	ConsumeOAuthState(ctx context.Context, provider string, stateHash []byte, now time.Time) (OAuthState, error)
}

type EmailVerificationStore interface {
	CreateEmailVerificationChallenge(context.Context, uuid.UUID, []byte, time.Time) error
	ConsumeEmailVerificationChallenge(context.Context, uuid.UUID, []byte, time.Time) error
}

type EmailVerificationTokenStore interface {
	ConsumeEmailVerificationToken(context.Context, []byte, time.Time) error
}

type EmailVerificationNotifier interface {
	EnqueueEmailForAccount(context.Context, uuid.UUID, string, string, string, string) error
}

// AdminRoleStore is implemented by stores that can answer whether an account
// has the administrator role. It is kept separate from Store so lightweight
// service fakes do not need to model role management.
type AdminRoleStore interface {
	IsAdmin(context.Context, uuid.UUID) (bool, error)
}

type AdminMFARecord struct {
	SecretCiphertext []byte
	EnabledAt        *time.Time
	PendingExpiresAt *time.Time
}

// AdminMFAStore stores only the encrypted TOTP seed. The plaintext seed is
// returned only by the setup response and is never logged or persisted by the
// service.
type AdminMFAStore interface {
	GetAdminMFA(context.Context, uuid.UUID) (AdminMFARecord, error)
	SaveAdminMFASecret(context.Context, uuid.UUID, []byte, time.Time) error
	EnableAdminMFA(context.Context, uuid.UUID, time.Time) error
	DisableAdminMFA(context.Context, uuid.UUID) error
}
