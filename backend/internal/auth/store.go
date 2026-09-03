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

// PasswordResetStore is the optional persistence for the native password-reset
// flow. Stores that implement it enable POST /api/v1/auth/password-reset/*.
type PasswordResetStore interface {
	// LookupAccountIDByEmailHash returns the account id for an email lookup
	// hash, or ErrNotFound. Used by the reset-request step; callers must not
	// leak whether it matched.
	LookupAccountIDByEmailHash(ctx context.Context, emailLookupHash []byte) (uuid.UUID, error)
	// CreatePasswordResetChallenge stores a new token hash and invalidates any
	// prior unconsumed challenge for the account.
	CreatePasswordResetChallenge(ctx context.Context, accountID uuid.UUID, tokenHash []byte, expiresAt time.Time) error
	// ConsumePasswordResetChallenge atomically: validates an unconsumed,
	// unexpired token; marks it consumed; sets the account password hash; and
	// revokes every session for that account. Returns ErrInvalidToken when the
	// token does not match a live challenge.
	ConsumePasswordResetChallenge(ctx context.Context, tokenHash []byte, newPasswordHash string, now time.Time) error
	// UpdatePasswordForAccount sets a new password hash for an authenticated
	// self-service change and revokes that account's sessions except
	// keepSessionID (the one making the request).
	UpdatePasswordForAccount(ctx context.Context, accountID uuid.UUID, newPasswordHash string, keepSessionID uuid.UUID) error
}

// AdminRoleStore is implemented by stores that can answer whether an account
// has the administrator role. It is kept separate from Store so lightweight
// service fakes do not need to model role management.
type AdminRoleStore interface {
	IsAdmin(context.Context, uuid.UUID) (bool, error)
}

// SessionSummary is a redacted view of one login session for the
// session-management endpoints. IP and user agent are stored only as hashes, so
// they are not exposed.
type SessionSummary struct {
	ID         uuid.UUID  `json:"id"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	ExpiresAt  time.Time  `json:"expires_at"`
}

// SessionManagementStore is the optional persistence for listing and revoking
// an account's own sessions.
type SessionManagementStore interface {
	ListActiveSessions(ctx context.Context, accountID uuid.UUID, now time.Time) ([]SessionSummary, error)
	// RevokeAccountSession revokes one session that belongs to accountID and
	// reports whether a row was affected (false = not found / not theirs).
	RevokeAccountSession(ctx context.Context, accountID, sessionID uuid.UUID, now time.Time) (bool, error)
	// RevokeOtherAccountSessions revokes every active session for accountID
	// except keepSessionID.
	RevokeOtherAccountSessions(ctx context.Context, accountID, keepSessionID uuid.UUID, now time.Time) (int64, error)
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
