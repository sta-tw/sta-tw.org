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
	distributedLimiter security.DistributedLimiter
	now                func() time.Time
	oauthProviders     map[string]oauthProvider
	oauthHTTPClient    *http.Client
	emailNotifier      EmailVerificationNotifier
	publicBaseURL      string
	requireEduEmail    bool
	requireAdminMFA    bool
}

type RegisterInput struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
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
}

type OAuthResult struct {
	Account Account
	Session *SessionResult
	Bound   bool
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
		now:             time.Now,
		oauthProviders:  make(map[string]oauthProvider),
		oauthHTTPClient: &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (s *Service) ConfigureEmailVerification(notifier EmailVerificationNotifier, publicBaseURL string) {
	s.emailNotifier = notifier
	s.publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
}

func (s *Service) ConfigureRegistrationPolicy(requireEduEmail bool) {
	if s != nil {
		s.requireEduEmail = requireEduEmail
	}
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
	link := ""
	if s.publicBaseURL != "" {
		link = s.publicBaseURL + "/verify-email?token=" + url.QueryEscape(token)
	}
	body := "STA Email 驗證碼元件已建立。請在 30 分鐘內完成驗證。\n"
	if link != "" {
		body += "驗證連結：" + link + "\n"
	}
	body += "若前端未提供連結頁，請將本郵件中的 token 送至 POST /api/v1/auth/email-verification/confirm。\nToken：" + token
	// A new challenge invalidates the previous one. The outbox key therefore
	// must also change for every challenge; minute-level keys could suppress a
	// resend while leaving the user with a token that was never delivered.
	dedupKey := "email-verification:" + accountID.String() + ":" + hex.EncodeToString(HashOpaqueToken(token))
	if err := s.emailNotifier.EnqueueEmailForAccount(ctx, accountID, dedupKey, "STA Email 驗證", body, "email_verification"); err != nil {
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
func (s *Service) RequestPasswordReset(ctx context.Context, email string, request *http.Request) error {
	store, ok := s.store.(PasswordResetStore)
	if !ok || s.emailNotifier == nil {
		return ErrNotConfigured
	}
	normalized, err := normalizeAndValidateEmail(email)
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
	if err := store.CreatePasswordResetChallenge(ctx, accountID, HashOpaqueToken(token), expiresAt); err != nil {
		return err
	}
	link := ""
	if s.publicBaseURL != "" {
		link = s.publicBaseURL + "/reset-password?token=" + url.QueryEscape(token)
	}
	body := "有人為這個帳號要求重設密碼。若不是你本人，請忽略這封信；密碼不會被更改。\n請在 30 分鐘內完成。\n"
	if link != "" {
		body += "重設連結：" + link + "\n"
	}
	body += "若前端未提供連結頁，請將 token 送至 POST /api/v1/auth/password-reset/confirm。\nToken：" + token
	dedupKey := "password-reset:" + accountID.String() + ":" + hex.EncodeToString(HashOpaqueToken(token))
	return s.emailNotifier.EnqueueEmailForAccount(ctx, accountID, dedupKey, "STA 密碼重設", body, "password_reset")
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
		},
		userInfoURL: settings.UserInfoURL,
	}
	return nil
}

func (s *Service) OAuthStart(ctx context.Context, provider string, accountID *uuid.UUID) (string, error) {
	if !s.ready() {
		return "", ErrNotConfigured
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	providerConfig, ok := s.oauthProviders[provider]
	if !ok {
		return "", ErrOAuthProvider
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
	if err := s.store.CreateOAuthState(ctx, provider, accountID, HashOpaqueToken(state), verifierCiphertext, providerConfig.config.RedirectURL, expiresAt); err != nil {
		return "", err
	}
	return providerConfig.config.AuthCodeURL(
		state,
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		oauth2.SetAuthURLParam("access_type", "online"),
	), nil
}

func (s *Service) OAuthCallback(ctx context.Context, provider, stateValue, code string, currentAccountID *uuid.UUID, request *http.Request) (OAuthResult, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	providerConfig, ok := s.oauthProviders[provider]
	if !ok || stateValue == "" || code == "" || len(stateValue) > 512 || len(code) > 4096 {
		return OAuthResult{}, ErrOAuthState
	}
	state, err := s.store.ConsumeOAuthState(ctx, provider, HashOpaqueToken(stateValue), s.now().UTC())
	if err != nil {
		return OAuthResult{}, ErrOAuthState
	}
	if state.AccountID != nil {
		if currentAccountID == nil || *currentAccountID != *state.AccountID {
			return OAuthResult{}, ErrInvalidSession
		}
	}
	verifier, err := s.emailCipher.Open(state.CodeVerifierCiphertext)
	if err != nil {
		return OAuthResult{}, ErrOAuthState
	}
	token, err := providerConfig.config.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", verifier))
	if err != nil {
		return OAuthResult{}, ErrOAuthState
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
		account, err := s.accountForSession(ctx, *state.AccountID)
		if err != nil {
			return OAuthResult{}, err
		}
		return OAuthResult{Account: account, Bound: true}, nil
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
	result, err := s.createSession(ctx, account, request)
	if err != nil {
		return OAuthResult{}, err
	}
	return OAuthResult{Account: account, Session: &result}, nil
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
	username, err := normalizeUsername(input.Username)
	if err != nil {
		return Account{}, err
	}
	email, err := normalizeAndValidateEmail(input.Email)
	if err != nil {
		return Account{}, err
	}
	if s.requireEduEmail && !IsEduEmail(email) {
		return Account{}, ErrInvalidInput
	}
	if err := validatePassword(input.Password); err != nil {
		return Account{}, err
	}
	passwordHash, err := HashPassword(input.Password)
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

func IsEduEmail(value string) bool {
	value = NormalizeEmail(value)
	at := strings.LastIndexByte(value, '@')
	if at < 1 || at == len(value)-1 {
		return false
	}
	domain := strings.TrimSuffix(value[at+1:], ".")
	return domain == "edu.tw" || strings.HasSuffix(domain, ".edu.tw")
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
