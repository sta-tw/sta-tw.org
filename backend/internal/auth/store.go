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
	// ReturnTo is the frontend page to send the browser back to once the
	// OAuth round trip finishes; empty means "use the default". Validated
	// as a same-origin relative path before it is ever stored.
	ReturnTo string
}

type Store interface {
	CreateAccount(ctx context.Context, username string, emailCiphertext, emailLookupHash []byte, passwordHash string) (Account, error)
	// CreatePendingAccount is CreateAccount but the account starts
	// 'pending_verification' (cannot log in) instead of 'active'. Used by
	// school-email registration: the account activates only once the
	// password-set link mailed to the school address is consumed — see
	// PasswordResetStore.CreatePasswordResetChallenge's activatesAccount.
	CreatePendingAccount(ctx context.Context, username string, emailCiphertext, emailLookupHash []byte, passwordHash string) (Account, error)
	FindAccountByUsername(ctx context.Context, username string) (Account, string, error)
	FindAccountByID(ctx context.Context, accountID uuid.UUID) (Account, error)
	CreateSession(ctx context.Context, accountID uuid.UUID, tokenHash, csrfHash []byte, expiresAt time.Time, ipHash, userAgentHash []byte) (uuid.UUID, error)
	FindActiveSession(ctx context.Context, tokenHash []byte, now time.Time) (Session, error)
	TouchSession(ctx context.Context, sessionID uuid.UUID, now time.Time) error
	RevokeSession(ctx context.Context, sessionID uuid.UUID, now time.Time) error
	CreateOAuthBinding(ctx context.Context, accountID uuid.UUID, provider string, providerSubjectHash []byte) error
	// FindAccountByOAuthSubjectHashes resolves an OAuth identity by trying every
	// candidate subject hash (primary plus retired lookup keys). It returns the
	// account and the stored hash that matched, so the caller can rehash a row
	// still under an old key.
	FindAccountByOAuthSubjectHashes(ctx context.Context, provider string, providerSubjectHashes [][]byte) (Account, []byte, error)
	CreateOAuthState(ctx context.Context, provider string, accountID *uuid.UUID, stateHash, codeVerifierCiphertext []byte, redirectURL, returnTo string, expiresAt time.Time) error
	ConsumeOAuthState(ctx context.Context, provider string, stateHash []byte, now time.Time) (OAuthState, error)
}

// CalendarGrantStore persists the Google Calendar refresh token captured
// during OAuth login/bind, when the requested scope included calendar
// write access. It is optional — a Store need not implement it — so a
// deployment without calendar integration configured degrades cleanly.
type CalendarGrantStore interface {
	SaveCalendarGrant(ctx context.Context, accountID uuid.UUID, provider string, refreshTokenCiphertext []byte, scope string) error
	GetCalendarGrant(ctx context.Context, accountID uuid.UUID, provider string) (refreshTokenCiphertext []byte, scope string, err error)
	DeleteCalendarGrant(ctx context.Context, accountID uuid.UUID, provider string) error
}

type EmailVerificationStore interface {
	CreateEmailVerificationChallenge(context.Context, uuid.UUID, []byte, time.Time) error
	ConsumeEmailVerificationChallenge(context.Context, uuid.UUID, []byte, time.Time) error
}

type EmailVerificationTokenStore interface {
	ConsumeEmailVerificationToken(context.Context, []byte, time.Time) error
}

type EmailVerificationNotifier interface {
	// text is the plain-text fallback part; html (may be empty) is the
	// multipart/alternative sibling — see email.ButtonEmail.
	EnqueueEmailForAccount(ctx context.Context, accountID uuid.UUID, dedupKey, subject, text, html, kind string) error
	// EnqueueEmailTo sends to an explicit recipient rather than the account's
	// own stored (and possibly not-yet-active) email — used to mail the
	// school-email registration link to the school address itself.
	EnqueueEmailTo(ctx context.Context, accountID uuid.UUID, recipientCiphertext []byte, dedupKey, subject, text, html string) error
}

// PasswordResetStore is the optional persistence for the native password-reset
// flow. Stores that implement it enable POST /api/v1/auth/password-reset/*.
type PasswordResetStore interface {
	// LookupAccountIDByEmailHashes returns the account id whose email lookup
	// hash matches any candidate (primary plus retired lookup keys), or
	// ErrNotFound. Used by the reset-request step; callers must not leak
	// whether it matched.
	LookupAccountIDByEmailHashes(ctx context.Context, emailLookupHashes [][]byte) (uuid.UUID, error)
	// CreatePasswordResetChallenge stores a new token hash and invalidates any
	// prior unconsumed challenge for the account. activatesAccount marks a
	// challenge issued by school-email registration: consuming it also flips
	// the account from 'pending_verification' to 'active' and its identity to
	// 'student', instead of just setting a new password on an already-active
	// account.
	CreatePasswordResetChallenge(ctx context.Context, accountID uuid.UUID, tokenHash []byte, expiresAt time.Time, activatesAccount bool) error
	// ConsumePasswordResetChallenge atomically: validates an unconsumed,
	// unexpired token; marks it consumed; sets the account password hash
	// (and, if the challenge activates the account, its account/identity
	// status); and revokes every session for that account. Returns
	// ErrInvalidToken when the token does not match a live challenge.
	ConsumePasswordResetChallenge(ctx context.Context, tokenHash []byte, newPasswordHash string, now time.Time) error
	// UpdatePasswordForAccount sets a new password hash for an authenticated
	// self-service change and revokes that account's sessions except
	// keepSessionID (the one making the request).
	UpdatePasswordForAccount(ctx context.Context, accountID uuid.UUID, newPasswordHash string, keepSessionID uuid.UUID) error
}

// oauthSubjectRehasher is the optional hook that lets the OAuth login path
// rewrite a stored subject hash from a retired lookup key to the primary one.
type oauthSubjectRehasher interface {
	RehashOAuthSubject(ctx context.Context, provider string, oldHash, newHash []byte) error
}

// AdminRoleStore is implemented by stores that can answer whether an account
// has the administrator role. It is kept separate from Store so lightweight
// service fakes do not need to model role management.
type AdminRoleStore interface {
	IsAdmin(context.Context, uuid.UUID) (bool, error)
	// IsServiceAccount reports the 'service' (bot) role — used to exempt
	// bot accounts from admin MFA, which is a human-factor concept that
	// doesn't apply to a machine credential (see RequireAdminMFA).
	IsServiceAccount(context.Context, uuid.UUID) (bool, error)
}

// AdmissionsModeratorStore is implemented by stores that can answer whether
// an account holds the narrower admissions_moderator role — content-only
// access to the brochure/admissions admin surface, granted separately from
// the full admin role. It's optional (type-asserted, like AdminMFAStore)
// so lightweight fakes that never touch this role don't need to implement
// it.
type AdmissionsModeratorStore interface {
	IsAdmissionsModerator(context.Context, uuid.UUID) (bool, error)
}

// AdminMFASettingStore lets the admin backend toggle whether admin MFA is
// enforced at runtime, without a redeploy — see RequireAdminMFA. isSet is
// false when no row exists yet, so the caller can fall back to the static
// STA_REQUIRE_ADMIN_MFA default instead of treating "unset" as "off".
type AdminMFASettingStore interface {
	GetRequireAdminMFA(ctx context.Context) (value bool, isSet bool, err error)
}

// AdminRoleGrantStore grants a role — kept separate from AdminRoleStore
// (which only reads roles) so a store that never needs to mutate them
// doesn't have to implement this too.
type AdminRoleGrantStore interface {
	GrantRole(ctx context.Context, accountID uuid.UUID, role string) error
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
	LastVerifiedAt   *time.Time
}

// AdminMFAStore stores only the encrypted TOTP seed. The plaintext seed is
// returned only by the setup response and is never logged or persisted by the
// service.
type AdminMFAStore interface {
	GetAdminMFA(context.Context, uuid.UUID) (AdminMFARecord, error)
	SaveAdminMFASecret(context.Context, uuid.UUID, []byte, time.Time) error
	EnableAdminMFA(context.Context, uuid.UUID, time.Time) error
	DisableAdminMFA(context.Context, uuid.UUID) error
	// TouchAdminMFAVerified records a successful TOTP check, opening the
	// short-lived grant window.
	TouchAdminMFAVerified(context.Context, uuid.UUID, time.Time) error
}

// BotAPIKeyStore backs 'service'-role account authentication — a single
// opaque bearer token (same mechanism migration 000041 already built for
// 'ai_system', reused here via the same account_api_tokens table) that
// never touches account_sessions/password/MFA. See
// Service.authenticateBotAPIKey.
type BotAPIKeyStore interface {
	// FindAccountByServiceToken resolves a token hash to the 'service'-role
	// account it belongs to, only if the token is on record, unrevoked, and
	// the account is active. ErrNotFound covers every failure case alike so
	// a caller can't use the error to enumerate valid tokens.
	FindAccountByServiceToken(ctx context.Context, tokenHash []byte, now time.Time) (Account, error)
	// CreateServiceAPIToken stores a freshly generated token's hash for
	// accountID. The plaintext token is never persisted — only returned
	// once, by whatever created it.
	CreateServiceAPIToken(ctx context.Context, accountID uuid.UUID, tokenHash []byte, label string) error
}
