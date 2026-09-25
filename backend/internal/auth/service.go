package auth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"sta-backend/internal/email"
	"sta-backend/internal/security"
)

var (
	ErrInvalidInput         = errors.New("invalid input")
	ErrInvalidCredentials   = errors.New("invalid credentials")
	ErrNotConfigured        = errors.New("auth is not configured")
	ErrInvalidSession       = errors.New("invalid session")
	ErrCSRF                 = errors.New("invalid CSRF token")
	ErrRateLimited          = errors.New("rate limited")
	ErrRateLimitUnavailable = errors.New("rate limiter unavailable")
	ErrOAuthProvider        = errors.New("OAuth provider is not configured")
	ErrOAuthState           = errors.New("invalid OAuth state")
	ErrOAuthNotBound        = errors.New("OAuth identity is not bound")
	ErrOAuthSubject         = errors.New("invalid OAuth identity")
	ErrInvalidToken         = errors.New("invalid or expired token")
)

const sessionCookieName = "sta_session"
const csrfCookieName = "sta_csrf"

type Service struct {
	store              Store
	emailCipher        *FieldCipher
	lookupHMACKey      []byte
	lookupHasher       *LookupHasher
	sessionTTL         time.Duration
	cookieSecure       bool
	loginLimiter       *security.FixedWindowLimiter
	registerLimiter    *security.FixedWindowLimiter
	emailLimiter       *security.FixedWindowLimiter
	mfaLimiter         *security.FixedWindowLimiter
	distributedLimiter security.DistributedLimiter
	now                func() time.Time
	oauthProviders     map[string]oauthProvider
	oauthHTTPClient    *http.Client
	emailNotifier      EmailVerificationNotifier
	publicBaseURL      string
	requireAdminMFA    bool
	adminMFAGrantTTL   time.Duration
}

type RegisterInput struct {
	Username string `json:"username"`
	// Email is the account's own contact address — any domain.
	Email string `json:"email"`
	// SchoolEmail must be a *.edu.tw address; the account stays inactive
	// until the password-set link mailed there is used. See Register.
	SchoolEmail string `json:"school_email"`
}

type LoginInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type SessionResult struct {
	Account   Account
	Token     string
	CSRFToken string
	ExpiresAt time.Time
}

type OAuthProviderSettings struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	AuthURL      string
	TokenURL     string
	UserInfoURL  string
	// Scopes, when non-empty, are requested on every authorization for this
	// provider. Google's calendar scope lives here so login and calendar
	// consent happen in the same round trip — see CalendarScope.
	Scopes []string
}

// CalendarScope is the Google scope that grants write access to the
// signed-in user's calendar. Requesting it alongside the ordinary login
// scopes is what lets "add to calendar" write events directly instead of
// handing the visitor a file to import by hand.
const CalendarScope = "https://www.googleapis.com/auth/calendar.events"

type OAuthResult struct {
	Account  Account
	Session  *SessionResult
	Bound    bool
	ReturnTo string
}

type oauthProvider struct {
	config      oauth2.Config
	userInfoURL string
}

type RequestSession struct {
	Session   Session
	TokenKind tokenKind
	RawToken  string
}

func (s RequestSession) UsesCookie() bool {
	return s.TokenKind == tokenKindCookie
}

type tokenKind string

const (
	tokenKindCookie tokenKind = "cookie"
	tokenKindBearer tokenKind = "bearer"
)

func NewService(store Store, emailCipher *FieldCipher, lookupHMACKey []byte, sessionTTL time.Duration, cookieSecure bool) (*Service, error) {
	if store == nil {
		return nil, ErrNotConfigured
	}
	if sessionTTL <= 0 {
		return nil, errors.New("session TTL must be positive")
	}
	if len(lookupHMACKey) != 32 {
		return nil, errors.New("lookup HMAC key must be 32 bytes")
	}
	lookupHasher, err := NewLookupHasher(lookupHMACKey)
	if err != nil {
		return nil, err
	}
	return &Service{
		store:           store,
		emailCipher:     emailCipher,
		lookupHMACKey:   append([]byte(nil), lookupHMACKey...),
		lookupHasher:    lookupHasher,
		sessionTTL:      sessionTTL,
		cookieSecure:    cookieSecure,
		loginLimiter:    security.NewFixedWindowLimiter(10, time.Minute, 10000),
		registerLimiter: security.NewFixedWindowLimiter(5, time.Minute, 10000),
		emailLimiter:    security.NewFixedWindowLimiter(3, 10*time.Minute, 10000),
		mfaLimiter:      security.NewFixedWindowLimiter(mfaFailureLimit, mfaFailureWindow, 10000),
		now:             time.Now,
		oauthProviders:  make(map[string]oauthProvider),
		oauthHTTPClient: &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (s *Service) ConfigureEmailVerification(notifier EmailVerificationNotifier, publicBaseURL string) {
	s.emailNotifier = notifier
	s.publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
}

func (s *Service) ConfigureDistributedLimiter(limiter security.DistributedLimiter) {
	if s != nil {
		s.distributedLimiter = limiter
	}
}

// ConfigureLookupKeyRotation registers retired lookup-HMAC keys. Reads then try
// the primary plus each retired key; writes still use the primary. Call once at
// startup with the keys being rotated out.
func (s *Service) ConfigureLookupKeyRotation(secondary [][]byte) error {
	if s == nil {
		return nil
	}
	hasher, err := NewLookupHasher(s.lookupHMACKey, secondary...)
	if err != nil {
		return err
	}
	s.lookupHasher = hasher
	return nil
}

// IsAdmin reports whether the account holds the administrator role. Handlers
// without their own database pool (chat, …) use this for role checks; pair it
// with RequireAdminMFA when the action is sensitive.
func (s *Service) IsAdmin(ctx context.Context, accountID uuid.UUID) (bool, error) {
	store, ok := s.store.(AdminRoleStore)
	if !ok {
		return false, nil
	}
	return store.IsAdmin(ctx, accountID)
}

// CanManageAdmissions reports whether the account can use the admissions
// (brochure) admin surface — either because it holds the full admin role, or
// the narrower admissions_moderator role granted to a specific brochure
// board moderator. The narrower role has no access to anything else under
// /admin; that scoping is enforced entirely by internal/admissions, which
// checks the same underlying role set independently of this method.
func (s *Service) CanManageAdmissions(ctx context.Context, accountID uuid.UUID) (bool, error) {
	isAdmin, err := s.IsAdmin(ctx, accountID)
	if err != nil {
		return false, err
	}
	if isAdmin {
		return true, nil
	}
	store, ok := s.store.(AdmissionsModeratorStore)
	if !ok {
		return false, nil
	}
	return store.IsAdmissionsModerator(ctx, accountID)
}

func (s *Service) EmailVerificationConfigured() bool {
	if s == nil {
		return false
	}
	_, ok := s.store.(EmailVerificationStore)
	return ok && s.emailNotifier != nil
}

func (s *Service) IssueEmailVerification(ctx context.Context, accountID uuid.UUID) (time.Time, error) {
	store, ok := s.store.(EmailVerificationStore)
	if !ok || s.emailNotifier == nil {
		return time.Time{}, ErrNotConfigured
	}
	if accountID == uuid.Nil {
		return time.Time{}, ErrRateLimited
	}
	allowed, err := s.rateAllowed(ctx, s.emailLimiter, "auth-email-verification", accountID.String(), 3, 10*time.Minute)
	if err != nil {
		return time.Time{}, err
	}
	if !allowed {
		return time.Time{}, ErrRateLimited
	}
	token, err := NewOpaqueToken(32)
	if err != nil {
		return time.Time{}, err
	}
	expiresAt := s.now().UTC().Add(30 * time.Minute)
	if err := store.CreateEmailVerificationChallenge(ctx, accountID, HashOpaqueToken(token), expiresAt); err != nil {
		return time.Time{}, err
	}
	link := s.publicBaseURL + "/verify-email?token=" + url.QueryEscape(token)
	textBody := "請在 30 分鐘內點擊以下連結完成 Email 驗證：\n" + link
	htmlBody := email.ButtonEmail(
		"確認你的 Email",
		[]string{"請在 30 分鐘內按下方按鈕，完成 Email 驗證。"},
		"完成驗證", link,
	)
	// A new challenge invalidates the previous one. The outbox key therefore
	// must also change for every challenge; minute-level keys could suppress a
	// resend while leaving the user with a token that was never delivered.
	dedupKey := "email-verification:" + accountID.String() + ":" + hex.EncodeToString(HashOpaqueToken(token))
	if err := s.emailNotifier.EnqueueEmailForAccount(ctx, accountID, dedupKey, "STA Email 驗證", textBody, htmlBody, "email_verification"); err != nil {
		return time.Time{}, err
	}
	return expiresAt, nil
}

func (s *Service) ConfirmEmailVerification(ctx context.Context, accountID uuid.UUID, token string) error {
	store, ok := s.store.(EmailVerificationStore)
	if !ok {
		return ErrNotConfigured
	}
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 512 {
		return ErrInvalidInput
	}
	return store.ConsumeEmailVerificationChallenge(ctx, accountID, HashOpaqueToken(token), s.now().UTC())
}

func (s *Service) ConfirmEmailVerificationToken(ctx context.Context, token string) error {
	store, ok := s.store.(EmailVerificationTokenStore)
	if !ok {
		return ErrNotConfigured
	}
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 512 {
		return ErrInvalidInput
	}
	return store.ConsumeEmailVerificationToken(ctx, HashOpaqueToken(token), s.now().UTC())
}

// PasswordResetConfigured reports whether the store and notifier support the
// native password-reset flow.
func (s *Service) PasswordResetConfigured() bool {
	if s == nil || s.emailNotifier == nil {
		return false
	}
	_, ok := s.store.(PasswordResetStore)
	return ok
}

// RequestPasswordReset sends a reset email if the address maps to an active
// native account. It never reveals whether it did: callers must always respond
// the same way (202) regardless of the returned error, which is only for
// internal logging of genuine failures.
func (s *Service) RequestPasswordReset(ctx context.Context, rawEmail string, request *http.Request) error {
	store, ok := s.store.(PasswordResetStore)
	if !ok || s.emailNotifier == nil {
		return ErrNotConfigured
	}
	normalized, err := normalizeAndValidateEmail(rawEmail)
	if err != nil {
		return nil // malformed address: nothing to do, stay silent
	}
	lookupHash := s.lookupHasher.Hash(normalized)
	allowed, err := s.rateAllowed(ctx, s.emailLimiter, "auth-password-reset", hex.EncodeToString(lookupHash), 3, 10*time.Minute)
	if err != nil {
		return err
	}
	if !allowed {
		return nil // over the limit: silently skip, still respond 202
	}
	accountID, err := store.LookupAccountIDByEmailHashes(ctx, s.lookupHasher.Candidates(normalized))
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	token, err := NewOpaqueToken(32)
	if err != nil {
		return err
	}
	expiresAt := s.now().UTC().Add(30 * time.Minute)
	if err := store.CreatePasswordResetChallenge(ctx, accountID, HashOpaqueToken(token), expiresAt, false); err != nil {
		return err
	}
	link := s.publicBaseURL + "/reset-password?token=" + url.QueryEscape(token)
	textBody := "我們收到一個重設 S.T.A 帳號密碼的請求。\n如果是你本人，請在 30 分鐘內使用以下連結設定新密碼。若不是你本人，可以忽略這封信，現有密碼不會被更改。\n\n設定新密碼：\n" + link + "\n\n完成重設後，其他裝置上的登入會失效。"
	logoURL := ""
	if s.publicBaseURL != "" {
		logoURL = s.publicBaseURL + "/logo.svg"
	}
	htmlBody := email.PasswordResetEmail(email.PasswordResetEmailData{
		LogoURL:  logoURL,
		ResetURL: link,
	})
	dedupKey := "password-reset:" + accountID.String() + ":" + hex.EncodeToString(HashOpaqueToken(token))
	return s.emailNotifier.EnqueueEmailForAccount(ctx, accountID, dedupKey, "重設你的 STA 密碼", textBody, htmlBody, "password_reset")
}

// ConfirmPasswordReset sets a new password from a reset token. On success every
// session for the account is revoked.
func (s *Service) ConfirmPasswordReset(ctx context.Context, token, newPassword string) error {
	store, ok := s.store.(PasswordResetStore)
	if !ok {
		return ErrNotConfigured
	}
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 512 {
		return ErrInvalidToken
	}
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	newHash, err := HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	err = store.ConsumePasswordResetChallenge(ctx, HashOpaqueToken(token), newHash, s.now().UTC())
	if errors.Is(err, ErrExpired) || errors.Is(err, ErrNotFound) {
		return ErrInvalidToken
	}
	return err
}

// ChangePassword lets a logged-in account set a new password after proving the
// current one. Other sessions are revoked; the caller's stays valid.
func (s *Service) ChangePassword(ctx context.Context, session RequestSession, currentPassword, newPassword string) error {
	store, ok := s.store.(PasswordResetStore)
	if !ok {
		return ErrNotConfigured
	}
	if session.Session.Account.ID == uuid.Nil {
		return ErrInvalidSession
	}
	_, currentHash, err := s.store.FindAccountByUsername(ctx, session.Session.Account.Username)
	if err != nil {
		return ErrInvalidCredentials
	}
	valid, err := VerifyPassword(currentPassword, currentHash)
	if err != nil || !valid {
		return ErrInvalidCredentials
	}
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	newHash, err := HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	return store.UpdatePasswordForAccount(ctx, session.Session.Account.ID, newHash, session.Session.ID)
}

// SessionListItem is one entry returned by ListSessions, with a flag marking
// the caller's current session.
type SessionListItem struct {
	SessionSummary
	Current bool `json:"current"`
}

// ListSessions returns the caller's active sessions, newest activity first.
func (s *Service) ListSessions(ctx context.Context, session RequestSession) ([]SessionListItem, error) {
	store, ok := s.store.(SessionManagementStore)
	if !ok {
		return nil, ErrNotConfigured
	}
	summaries, err := store.ListActiveSessions(ctx, session.Session.Account.ID, s.now().UTC())
	if err != nil {
		return nil, err
	}
	items := make([]SessionListItem, 0, len(summaries))
	for _, item := range summaries {
		items = append(items, SessionListItem{SessionSummary: item, Current: item.ID == session.Session.ID})
	}
	return items, nil
}

// RevokeSession revokes one of the caller's other sessions. Revoking the
// current session is rejected — use Logout for that.
func (s *Service) RevokeSession(ctx context.Context, session RequestSession, sessionID uuid.UUID) error {
	store, ok := s.store.(SessionManagementStore)
	if !ok {
		return ErrNotConfigured
	}
	if sessionID == session.Session.ID {
		return ErrInvalidInput
	}
	revoked, err := store.RevokeAccountSession(ctx, session.Session.Account.ID, sessionID, s.now().UTC())
	if err != nil {
		return err
	}
	if !revoked {
		return ErrNotFound
	}
	return nil
}

// RevokeOtherSessions revokes every session for the caller except the current
// one.
func (s *Service) RevokeOtherSessions(ctx context.Context, session RequestSession) (int64, error) {
	store, ok := s.store.(SessionManagementStore)
	if !ok {
		return 0, ErrNotConfigured
	}
	return store.RevokeOtherAccountSessions(ctx, session.Session.Account.ID, session.Session.ID, s.now().UTC())
}

func (s *Service) ConfigureOAuth(provider string, settings OAuthProviderSettings) error {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider != "google" && provider != "discord" {
		return ErrOAuthProvider
	}
	if settings.ClientID == "" || settings.ClientSecret == "" || settings.RedirectURL == "" {
		return ErrOAuthProvider
	}
	if settings.AuthURL == "" || settings.TokenURL == "" || settings.UserInfoURL == "" {
		return ErrOAuthProvider
	}
	s.oauthProviders[provider] = oauthProvider{
		config: oauth2.Config{
			ClientID:     settings.ClientID,
			ClientSecret: settings.ClientSecret,
			Endpoint: oauth2.Endpoint{
				AuthURL:  settings.AuthURL,
				TokenURL: settings.TokenURL,
			},
			RedirectURL: settings.RedirectURL,
			Scopes:      settings.Scopes,
		},
		userInfoURL: settings.UserInfoURL,
	}
	return nil
}

// isValidReturnTo restricts the post-OAuth redirect to a same-origin
// relative path (must start with a single "/", never "//" — that parses as
// a protocol-relative URL to an attacker-controlled host). Anything else is
// silently dropped in favor of the default landing page.
func isValidReturnTo(value string) bool {
	return strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") && len(value) <= 2048
}

func (s *Service) OAuthStart(ctx context.Context, provider string, accountID *uuid.UUID, returnTo string) (string, error) {
	if !s.ready() {
		return "", ErrNotConfigured
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	providerConfig, ok := s.oauthProviders[provider]
	if !ok {
		return "", ErrOAuthProvider
	}
	if !isValidReturnTo(returnTo) {
		returnTo = ""
	}
	state, err := NewOpaqueToken(32)
	if err != nil {
		return "", err
	}
	verifier, err := NewOpaqueToken(32)
	if err != nil {
		return "", err
	}
	verifierCiphertext, err := s.emailCipher.Seal(verifier)
	if err != nil {
		return "", err
	}
	challengeHash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeHash[:])
	expiresAt := s.now().UTC().Add(10 * time.Minute)
	if err := s.store.CreateOAuthState(ctx, provider, accountID, HashOpaqueToken(state), verifierCiphertext, providerConfig.config.RedirectURL, returnTo, expiresAt); err != nil {
		return "", err
	}
	params := []oauth2.AuthCodeOption{
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	}
	requestsCalendar := false
	for _, scope := range providerConfig.config.Scopes {
		if scope == CalendarScope {
			requestsCalendar = true
			break
		}
	}
	if requestsCalendar {
		// A refresh token — required to write calendar events after this
		// request completes, not just during it — is only issued by Google
		// on a consent grant. Without prompt=consent, a returning user who
		// already granted this app access silently gets no refresh token on
		// their second-and-later logins. Forcing the consent screen every
		// time trades a little repeat friction for calendar always working.
		params = append(params,
			oauth2.SetAuthURLParam("access_type", "offline"),
			oauth2.SetAuthURLParam("prompt", "consent"),
		)
	} else {
		params = append(params, oauth2.SetAuthURLParam("access_type", "online"))
	}
	return providerConfig.config.AuthCodeURL(state, params...), nil
}

func (s *Service) OAuthCallback(ctx context.Context, provider, stateValue, code string, currentAccountID *uuid.UUID, request *http.Request) (OAuthResult, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	providerConfig, ok := s.oauthProviders[provider]
	if !ok || stateValue == "" || code == "" || len(stateValue) > 512 || len(code) > 4096 {
		return OAuthResult{}, fmt.Errorf("malformed callback request: %w", ErrOAuthState)
	}
	state, err := s.store.ConsumeOAuthState(ctx, provider, HashOpaqueToken(stateValue), s.now().UTC())
	if err != nil {
		return OAuthResult{}, fmt.Errorf("consume oauth state: %w: %w", err, ErrOAuthState)
	}
	if state.AccountID != nil {
		if currentAccountID == nil || *currentAccountID != *state.AccountID {
			return OAuthResult{}, ErrInvalidSession
		}
	}
	verifier, err := s.emailCipher.Open(state.CodeVerifierCiphertext)
	if err != nil {
		return OAuthResult{}, fmt.Errorf("decrypt pkce verifier: %w: %w", err, ErrOAuthState)
	}
	token, err := providerConfig.config.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", verifier))
	if err != nil {
		return OAuthResult{}, fmt.Errorf("exchange oauth code: %w: %w", err, ErrOAuthState)
	}
	subject, err := s.fetchOAuthSubject(ctx, provider, providerConfig.userInfoURL, token)
	if err != nil {
		return OAuthResult{}, err
	}
	subjectHash := s.lookupHasher.Hash(subject)
	if state.AccountID != nil {
		if err := s.store.CreateOAuthBinding(ctx, *state.AccountID, provider, subjectHash); err != nil {
			return OAuthResult{}, err
		}
		if err := s.saveCalendarGrantIfPresent(ctx, *state.AccountID, provider, token); err != nil {
			return OAuthResult{}, err
		}
		account, err := s.accountForSession(ctx, *state.AccountID)
		if err != nil {
			return OAuthResult{}, err
		}
		return OAuthResult{Account: account, Bound: true, ReturnTo: state.ReturnTo}, nil
	}
	candidates := s.lookupHasher.Candidates(subject)
	primaryHash := candidates[0]
	account, matchedHash, err := s.store.FindAccountByOAuthSubjectHashes(ctx, provider, candidates)
	if err != nil {
		return OAuthResult{}, ErrOAuthNotBound
	}
	// Lazily migrate an identity still hashed under a retired key.
	if !bytes.Equal(matchedHash, primaryHash) {
		if rh, ok := s.store.(oauthSubjectRehasher); ok {
			_ = rh.RehashOAuthSubject(ctx, provider, matchedHash, primaryHash)
		}
	}
	if err := s.saveCalendarGrantIfPresent(ctx, account.ID, provider, token); err != nil {
		return OAuthResult{}, err
	}
	result, err := s.createSession(ctx, account, request)
	if err != nil {
		return OAuthResult{}, err
	}
	return OAuthResult{Account: account, Session: &result, ReturnTo: state.ReturnTo}, nil
}

// saveCalendarGrantIfPresent persists token.RefreshToken when the provider
// was configured with CalendarScope and Google's token response actually
// carries that scope. Google only echoes a "scope" field back when the
// granted set differs from what was requested (OAuth2 §5.1) — e.g. because
// calendar.events is a sensitive scope that Google silently drops for an
// account that isn't allow-listed as a test user while the OAuth consent
// screen is in Testing status, even though the authorize request and the
// resulting refresh token both look completely normal. Trusting our own
// requested scope list here (instead of what Google actually returned)
// would record a grant that reads as valid but fails on the very next
// Calendar API call with "insufficient authentication scopes" — exactly the
// silent-failure mode this check exists to catch before it's ever saved.
func (s *Service) saveCalendarGrantIfPresent(ctx context.Context, accountID uuid.UUID, provider string, token *oauth2.Token) error {
	if token.RefreshToken == "" {
		return nil
	}
	providerConfig, ok := s.oauthProviders[provider]
	if !ok {
		return nil
	}
	hasCalendarScope := false
	for _, scope := range providerConfig.config.Scopes {
		if scope == CalendarScope {
			hasCalendarScope = true
			break
		}
	}
	if !hasCalendarScope {
		return nil
	}
	// When Google omits "scope" entirely, the full requested set was
	// granted as-is — that's the common case and not a signal of anything
	// wrong. When present, it's authoritative: use it instead of assuming
	// our request was honored.
	grantedScope := strings.Join(providerConfig.config.Scopes, " ")
	if raw, ok := token.Extra("scope").(string); ok && raw != "" {
		grantedScope = raw
		if !strings.Contains(raw, CalendarScope) {
			return nil
		}
	}
	grantStore, ok := s.store.(CalendarGrantStore)
	if !ok {
		return nil
	}
	ciphertext, err := s.emailCipher.Seal(token.RefreshToken)
	if err != nil {
		return fmt.Errorf("seal calendar refresh token: %w", err)
	}
	if err := grantStore.SaveCalendarGrant(ctx, accountID, provider, ciphertext, grantedScope); err != nil {
		return fmt.Errorf("save calendar grant: %w", err)
	}
	return nil
}

func (s *Service) fetchOAuthSubject(ctx context.Context, provider, userInfoURL string, token *oauth2.Token) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoURL, nil)
	if err != nil {
		return "", ErrOAuthSubject
	}
	request.Header.Set("Authorization", "Bearer "+token.AccessToken)
	request.Header.Set("Accept", "application/json")
	response, err := s.oauthHTTPClient.Do(request)
	if err != nil {
		return "", ErrOAuthSubject
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", ErrOAuthSubject
	}
	var payload struct {
		Subject string `json:"sub"`
		ID      string `json:"id"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 64<<10))
	if err := decoder.Decode(&payload); err != nil {
		return "", ErrOAuthSubject
	}
	subject := strings.TrimSpace(payload.Subject)
	if provider == "discord" {
		subject = strings.TrimSpace(payload.ID)
	}
	if subject == "" || len(subject) > 512 {
		return "", ErrOAuthSubject
	}
	return subject, nil
}

func (s *Service) accountForSession(ctx context.Context, accountID uuid.UUID) (Account, error) {
	return s.store.FindAccountByID(ctx, accountID)
}

// Register creates an inactive ('pending_verification') account and emails a
// password-set link straight to the required school email — the account only
// activates (and its identity becomes 'student') once that link is used, via
// ConsumePasswordResetChallenge's activatesAccount path. There is no
// password field on RegisterInput: nothing is known until that link is
// consumed. People without a school email don't call this at all; they use
// the account-applications flow instead, which creates an already-active
// account directly on admin approval.
func (s *Service) Register(ctx context.Context, input RegisterInput, request *http.Request) (Account, error) {
	if !s.ready() {
		return Account{}, ErrNotConfigured
	}
	allowed, err := s.registerAllowed(ctx, request)
	if err != nil {
		return Account{}, err
	}
	if !allowed {
		return Account{}, ErrRateLimited
	}
	pendingStore, ok := s.store.(PasswordResetStore)
	if !ok || s.emailNotifier == nil {
		return Account{}, ErrNotConfigured
	}
	username, err := normalizeUsername(input.Username)
	if err != nil {
		return Account{}, err
	}
	contactEmail, err := normalizeAndValidateEmail(input.Email)
	if err != nil {
		return Account{}, err
	}
	schoolEmail, err := normalizeAndValidateEmail(input.SchoolEmail)
	if err != nil || !isSchoolEmail(schoolEmail) {
		return Account{}, fmt.Errorf("%w: school email must be a *.edu.tw address", ErrInvalidInput)
	}
	schoolEmailCiphertext, err := s.emailCipher.Seal(schoolEmail)
	if err != nil {
		return Account{}, fmt.Errorf("protect school email: %w", err)
	}

	randomPassword, err := NewOpaqueToken(24)
	if err != nil {
		return Account{}, err
	}
	passwordHash, err := HashPassword(randomPassword)
	if err != nil {
		return Account{}, fmt.Errorf("hash password: %w", err)
	}
	emailCiphertext, err := s.emailCipher.Seal(contactEmail)
	if err != nil {
		return Account{}, fmt.Errorf("protect email: %w", err)
	}
	account, err := s.store.CreatePendingAccount(ctx, username, emailCiphertext, s.lookupHasher.Hash(contactEmail), passwordHash)
	if err != nil {
		return Account{}, err
	}

	token, err := NewOpaqueToken(32)
	if err != nil {
		return account, err
	}
	expiresAt := s.now().UTC().Add(24 * time.Hour)
	if err := pendingStore.CreatePasswordResetChallenge(ctx, account.ID, HashOpaqueToken(token), expiresAt, true); err != nil {
		return account, err
	}
	link := s.publicBaseURL + "/reset-password?token=" + url.QueryEscape(token)
	textBody := "歡迎加入 STA！請在 24 小時內點擊以下連結，設定密碼以啟用帳號：\n" + link
	logoURL := ""
	if s.publicBaseURL != "" {
		logoURL = s.publicBaseURL + "/logo.svg"
	}
	htmlBody := email.AccountActivationEmail(email.AccountActivationEmailData{
		LogoURL: logoURL,
		SetURL:  link,
	})
	dedupKey := "register-school-email:" + account.ID.String() + ":" + hex.EncodeToString(HashOpaqueToken(token))
	if err := s.emailNotifier.EnqueueEmailTo(ctx, account.ID, schoolEmailCiphertext, dedupKey, "STA 帳號啟用", textBody, htmlBody); err != nil {
		return account, err
	}
	return account, nil
}

// isSchoolEmail reports whether value's domain is edu.tw or a subdomain of
// it. Registration-only: the account-applications review flow is the path
// for anyone without one, so this never blocks account creation itself —
// only which of the two flows applies.
func isSchoolEmail(value string) bool {
	at := strings.LastIndexByte(value, '@')
	if at < 1 || at == len(value)-1 {
		return false
	}
	domain := strings.TrimSuffix(value[at+1:], ".")
	return domain == "edu.tw" || strings.HasSuffix(domain, ".edu.tw")
}

// CreateApprovedAccount creates an already-active account with verified
// identity_status='student' directly, for someone with no school email whose
// account-application was approved in Telegram — they never went through
// Register (that path requires a *.edu.tw address), so this is the only way
// they get an account at all. Password is random and never disclosed; the
// caller follows up with RequestPasswordReset so the person can set their
// own, the same way school-email registration's activation link works.
func (s *Service) CreateApprovedAccount(ctx context.Context, rawUsername, rawEmail string) (Account, error) {
	if !s.ready() {
		return Account{}, ErrNotConfigured
	}
	username, err := normalizeUsername(rawUsername)
	if err != nil {
		return Account{}, err
	}
	email, err := normalizeAndValidateEmail(rawEmail)
	if err != nil {
		return Account{}, err
	}
	randomPassword, err := NewOpaqueToken(24)
	if err != nil {
		return Account{}, err
	}
	passwordHash, err := HashPassword(randomPassword)
	if err != nil {
		return Account{}, fmt.Errorf("hash password: %w", err)
	}
	emailCiphertext, err := s.emailCipher.Seal(email)
	if err != nil {
		return Account{}, fmt.Errorf("protect email: %w", err)
	}
	return s.store.CreateAccount(ctx, username, emailCiphertext, s.lookupHasher.Hash(email), passwordHash)
}

func (s *Service) Login(ctx context.Context, input LoginInput, request *http.Request) (SessionResult, error) {
	if !s.ready() {
		return SessionResult{}, ErrNotConfigured
	}
	allowed, err := s.loginAllowed(ctx, request)
	if err != nil {
		return SessionResult{}, err
	}
	if !allowed {
		return SessionResult{}, ErrRateLimited
	}
	username, err := normalizeUsername(input.Username)
	if err != nil {
		return SessionResult{}, ErrInvalidCredentials
	}
	account, passwordHash, err := s.store.FindAccountByUsername(ctx, username)
	if err != nil {
		return SessionResult{}, ErrInvalidCredentials
	}
	valid, err := VerifyPassword(input.Password, passwordHash)
	if err != nil || !valid || account.AccountStatus != "active" {
		return SessionResult{}, ErrInvalidCredentials
	}
	return s.createSession(ctx, account, request)
}

func (s *Service) createSession(ctx context.Context, account Account, request *http.Request) (SessionResult, error) {
	token, err := NewOpaqueToken(32)
	if err != nil {
		return SessionResult{}, err
	}
	csrfToken, err := NewOpaqueToken(32)
	if err != nil {
		return SessionResult{}, err
	}
	now := s.now().UTC()
	expiresAt := now.Add(s.sessionTTL)
	var ipHash, userAgentHash []byte
	if s.lookupHMACKey != nil && request != nil {
		ipHash, _ = LookupHash(s.lookupHMACKey, clientIP(request))
		userAgentHash, _ = LookupHash(s.lookupHMACKey, request.UserAgent())
	}
	_, err = s.store.CreateSession(ctx, account.ID, HashOpaqueToken(token), HashOpaqueToken(csrfToken), expiresAt, ipHash, userAgentHash)
	if err != nil {
		return SessionResult{}, err
	}
	return SessionResult{Account: account, Token: token, CSRFToken: csrfToken, ExpiresAt: expiresAt}, nil
}

func (s *Service) Authenticate(ctx context.Context, request *http.Request) (RequestSession, error) {
	if !s.ready() {
		return RequestSession{}, ErrNotConfigured
	}
	cookieToken, cookieErr := request.Cookie(sessionCookieName)
	bearerToken := bearerFromHeader(request.Header.Get("Authorization"))
	if cookieErr == nil && bearerToken != "" {
		return RequestSession{}, ErrInvalidSession
	}
	var token string
	kind := tokenKindBearer
	if cookieErr == nil {
		token = cookieToken.Value
		kind = tokenKindCookie
	} else {
		token = bearerToken
	}
	if token == "" || len(token) > 512 {
		return RequestSession{}, ErrInvalidSession
	}
	session, err := s.store.FindActiveSession(ctx, HashOpaqueToken(token), s.now().UTC())
	if err != nil {
		// Not a session — a bearer token (never a cookie) might instead be a
		// 'service'-role API token (see authenticateBotAPIKey), an entirely
		// separate credential from account_sessions that no password/MFA
		// applies to.
		if kind == tokenKindBearer {
			if botSession, botErr := s.authenticateBotAPIKey(ctx, token); botErr == nil {
				return botSession, nil
			}
		}
		return RequestSession{}, ErrInvalidSession
	}
	// Defence in depth: every handler's requireAdmin helper now calls
	// RequireAdminMFA explicitly after its role check, so MFA no longer depends
	// on the path. This prefix check stays as a backstop for any admin route
	// that is added without going through a requireAdmin helper.
	if strings.HasPrefix(request.URL.Path, "/api/v1/admin/") {
		if err := s.RequireAdminMFA(ctx, session.Account.ID, request.Header.Get("X-MFA-Code")); err != nil {
			return RequestSession{}, err
		}
	}
	_ = s.store.TouchSession(ctx, session.ID, s.now().UTC())
	return RequestSession{Session: session, TokenKind: kind, RawToken: token}, nil
}

// authenticateBotAPIKey verifies a 'service'-role account's opaque API
// token. Treated as a bearer token throughout (AuthorizeMutation already
// exempts bearer from CSRF), and deliberately never routed through
// RequireAdminMFA — see the dispatch comment in Authenticate and the
// 'service'-role exemption inside RequireAdminMFA itself.
func (s *Service) authenticateBotAPIKey(ctx context.Context, token string) (RequestSession, error) {
	store, ok := s.store.(BotAPIKeyStore)
	if !ok || token == "" || len(token) > 512 {
		return RequestSession{}, ErrInvalidSession
	}
	account, err := store.FindAccountByServiceToken(ctx, HashOpaqueToken(token), s.now().UTC())
	if err != nil {
		return RequestSession{}, ErrInvalidSession
	}
	return RequestSession{Session: Session{Account: account}, TokenKind: tokenKindBearer, RawToken: token}, nil
}

// CreateBotAccount creates a 'service'-role account for machine-to-machine
// use (e.g. a script calling admin endpoints directly, no AI extraction
// involved) and issues it one API token. The returned token is never
// stored or logged anywhere — this is the only time it's available in
// plaintext, so the caller must hand it off immediately.
//
// Bypasses the normal password/email flow entirely: bot accounts have no
// usable password and no contact email. Additional roles (e.g. 'admin', for
// admin-endpoint access) are the caller's responsibility to grant
// afterward — 'service' alone only identifies the account as a bot and
// exempts it from admin MFA; it grants no endpoint access by itself.
func (s *Service) CreateBotAccount(ctx context.Context, rawUsername, label string) (Account, string, error) {
	if !s.ready() {
		return Account{}, "", ErrNotConfigured
	}
	store, ok := s.store.(BotAPIKeyStore)
	if !ok {
		return Account{}, "", ErrNotConfigured
	}
	roleStore, ok := s.store.(AdminRoleGrantStore)
	if !ok {
		return Account{}, "", ErrNotConfigured
	}
	username, err := normalizeUsername(rawUsername)
	if err != nil {
		return Account{}, "", err
	}
	randomPassword, err := NewOpaqueToken(24)
	if err != nil {
		return Account{}, "", err
	}
	passwordHash, err := HashPassword(randomPassword)
	if err != nil {
		return Account{}, "", fmt.Errorf("hash password: %w", err)
	}
	// Bot accounts have no contact email; seal an empty value so the
	// encrypted-at-rest column still has a well-formed ciphertext, and use
	// a lookup hash derived from the account's own random password so it
	// can never collide with (or be found via) a real email lookup.
	emailCiphertext, err := s.emailCipher.Seal("")
	if err != nil {
		return Account{}, "", fmt.Errorf("protect email: %w", err)
	}
	account, err := s.store.CreateAccount(ctx, username, emailCiphertext, s.lookupHasher.Hash("bot:"+username+":"+randomPassword), passwordHash)
	if err != nil {
		return Account{}, "", err
	}
	if err := roleStore.GrantRole(ctx, account.ID, "service"); err != nil {
		return Account{}, "", err
	}
	token, err := NewOpaqueToken(32)
	if err != nil {
		return Account{}, "", err
	}
	if err := store.CreateServiceAPIToken(ctx, account.ID, HashOpaqueToken(token), label); err != nil {
		return Account{}, "", err
	}
	return account, token, nil
}

func (s *Service) AuthorizeMutation(request *http.Request, session RequestSession) error {
	if session.TokenKind == tokenKindBearer {
		return nil
	}
	csrfHeader := request.Header.Get("X-CSRF-Token")
	csrfCookie, err := request.Cookie(csrfCookieName)
	if err != nil || csrfHeader == "" || csrfCookie.Value == "" {
		return ErrCSRF
	}
	if subtle.ConstantTimeCompare([]byte(csrfHeader), []byte(csrfCookie.Value)) != 1 {
		return ErrCSRF
	}
	if subtle.ConstantTimeCompare(HashOpaqueToken(csrfHeader), session.Session.CSRFHash) != 1 {
		return ErrCSRF
	}
	return nil
}

func (s *Service) Logout(ctx context.Context, session RequestSession) error {
	return s.store.RevokeSession(ctx, session.Session.ID, s.now().UTC())
}

func (s *Service) SetSessionCookies(writer http.ResponseWriter, result SessionResult) {
	secure := s.cookieSecure
	sameSite := http.SameSiteLaxMode
	if secure {
		sameSite = http.SameSiteNoneMode
	}
	http.SetCookie(writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    result.Token,
		Path:     "/",
		Expires:  result.ExpiresAt,
		MaxAge:   maxAge(result.ExpiresAt, s.now()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
	})
	http.SetCookie(writer, &http.Cookie{
		Name:     csrfCookieName,
		Value:    result.CSRFToken,
		Path:     "/",
		Expires:  result.ExpiresAt,
		MaxAge:   maxAge(result.ExpiresAt, s.now()),
		HttpOnly: false,
		Secure:   secure,
		SameSite: sameSite,
	})
}

func (s *Service) ClearSessionCookies(writer http.ResponseWriter) {
	for _, name := range []string{sessionCookieName, csrfCookieName} {
		http.SetCookie(writer, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: name == sessionCookieName, Secure: s.cookieSecure})
	}
}

func (s *Service) ready() bool {
	return s != nil && s.store != nil && s.emailCipher != nil && len(s.lookupHMACKey) == 32
}

func (s *Service) loginAllowed(ctx context.Context, request *http.Request) (bool, error) {
	return s.rateAllowed(ctx, s.loginLimiter, "auth-login", clientIP(request), 10, time.Minute)
}

func (s *Service) registerAllowed(ctx context.Context, request *http.Request) (bool, error) {
	return s.rateAllowed(ctx, s.registerLimiter, "auth-register", clientIP(request), 5, time.Minute)
}

func (s *Service) rateAllowed(ctx context.Context, local *security.FixedWindowLimiter, namespace, key string, limit int, window time.Duration) (bool, error) {
	if local != nil && !local.Allow(key, s.now()) {
		return false, nil
	}
	if s.distributedLimiter == nil {
		return true, nil
	}
	allowed, err := s.distributedLimiter.Allow(ctx, namespace, key, limit, window, s.now().UTC())
	if err != nil {
		return false, ErrRateLimitUnavailable
	}
	return allowed, nil
}

func normalizeUsername(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" || utf8.RuneCountInString(value) < 3 || utf8.RuneCountInString(value) > 64 {
		return "", ErrInvalidInput
	}
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) || strings.ContainsRune("._-", character) {
			continue
		}
		return "", ErrInvalidInput
	}
	return value, nil
}

func normalizeAndValidateEmail(raw string) (string, error) {
	value := NormalizeEmail(raw)
	if value == "" || len(value) > 320 || !utf8.ValidString(value) {
		return "", ErrInvalidInput
	}
	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed.Address != value || !strings.Contains(parsed.Address, "@") {
		return "", ErrInvalidInput
	}
	return value, nil
}

func validatePassword(password string) error {
	length := utf8.RuneCountInString(password)
	if !utf8.ValidString(password) || length < 12 || length > 128 {
		return ErrInvalidInput
	}
	return nil
}

func bearerFromHeader(raw string) string {
	parts := strings.Fields(raw)
	if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
		return parts[1]
	}
	return ""
}

func clientIP(request *http.Request) string {
	if request == nil {
		return "unknown"
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(request.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	if request.RemoteAddr != "" {
		return request.RemoteAddr
	}
	return "unknown"
}

func maxAge(expiresAt, now time.Time) int {
	seconds := int(expiresAt.Sub(now).Seconds())
	if seconds < 1 {
		return 1
	}
	return seconds
}
