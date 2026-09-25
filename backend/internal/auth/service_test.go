package auth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

type fakePasswordResetChallenge struct {
	accountID        uuid.UUID
	activatesAccount bool
	expiresAt        time.Time
	consumed         bool
}

type fakeAuthStore struct {
	accounts        map[string]Account
	passwords       map[string]string
	emailHashes     map[string]uuid.UUID
	sessions        map[string]Session
	bindings        map[string]Account
	states          map[string]OAuthState
	resetChallenges map[string]fakePasswordResetChallenge
	calendarGrants  map[string]fakeCalendarGrant
}

type fakeCalendarGrant struct {
	ciphertext []byte
	scope      string
}

func newFakeAuthStore() *fakeAuthStore {
	return &fakeAuthStore{
		accounts:        make(map[string]Account),
		passwords:       make(map[string]string),
		emailHashes:     make(map[string]uuid.UUID),
		sessions:        make(map[string]Session),
		bindings:        make(map[string]Account),
		states:          make(map[string]OAuthState),
		resetChallenges: make(map[string]fakePasswordResetChallenge),
	}
}

func (f *fakeAuthStore) createAccount(username string, emailLookupHash []byte, passwordHash, accountStatus string) (Account, error) {
	if _, ok := f.accounts[username]; ok {
		return Account{}, ErrConflict
	}
	key := string(emailLookupHash)
	if _, ok := f.emailHashes[key]; ok {
		return Account{}, ErrConflict
	}
	account := Account{ID: uuid.New(), Username: username, IdentityStatus: "temporary", AccountStatus: accountStatus}
	f.accounts[username] = account
	f.passwords[username] = passwordHash
	f.emailHashes[key] = account.ID
	return account, nil
}

func (f *fakeAuthStore) CreateAccount(_ context.Context, username string, _, emailLookupHash []byte, passwordHash string) (Account, error) {
	return f.createAccount(username, emailLookupHash, passwordHash, "active")
}

func (f *fakeAuthStore) CreatePendingAccount(_ context.Context, username string, _, emailLookupHash []byte, passwordHash string) (Account, error) {
	return f.createAccount(username, emailLookupHash, passwordHash, "pending_verification")
}

func (f *fakeAuthStore) LookupAccountIDByEmailHashes(_ context.Context, emailLookupHashes [][]byte) (uuid.UUID, error) {
	for _, hash := range emailLookupHashes {
		if id, ok := f.emailHashes[string(hash)]; ok {
			if account, found := f.accountByID(id); found && account.AccountStatus == "active" {
				return id, nil
			}
		}
	}
	return uuid.Nil, ErrNotFound
}

func (f *fakeAuthStore) CreatePasswordResetChallenge(_ context.Context, accountID uuid.UUID, tokenHash []byte, expiresAt time.Time, activatesAccount bool) error {
	f.resetChallenges[string(tokenHash)] = fakePasswordResetChallenge{accountID: accountID, activatesAccount: activatesAccount, expiresAt: expiresAt}
	return nil
}

func (f *fakeAuthStore) ConsumePasswordResetChallenge(_ context.Context, tokenHash []byte, newPasswordHash string, now time.Time) error {
	challenge, ok := f.resetChallenges[string(tokenHash)]
	if !ok || challenge.consumed || now.After(challenge.expiresAt) {
		return ErrExpired
	}
	account, ok := f.accountByID(challenge.accountID)
	if !ok {
		return ErrNotFound
	}
	if challenge.activatesAccount {
		if account.AccountStatus != "pending_verification" {
			return ErrNotFound
		}
		account.AccountStatus = "active"
		account.IdentityStatus = "student"
	} else if account.AccountStatus != "active" {
		return ErrNotFound
	}
	f.accounts[account.Username] = account
	f.passwords[account.Username] = newPasswordHash
	challenge.consumed = true
	f.resetChallenges[string(tokenHash)] = challenge
	return nil
}

func (f *fakeAuthStore) UpdatePasswordForAccount(_ context.Context, accountID uuid.UUID, newPasswordHash string, _ uuid.UUID) error {
	account, ok := f.accountByID(accountID)
	if !ok {
		return ErrNotFound
	}
	f.passwords[account.Username] = newPasswordHash
	return nil
}

// fakeEmailNotifier captures outbound emails instead of sending them, so
// tests can pull the token out of the body.
type fakeEmailNotifier struct {
	sent []fakeSentEmail
}

type fakeSentEmail struct {
	accountID uuid.UUID
	subject   string
	body      string
}

func (n *fakeEmailNotifier) EnqueueEmailForAccount(_ context.Context, accountID uuid.UUID, _, subject, text, _, _ string) error {
	n.sent = append(n.sent, fakeSentEmail{accountID: accountID, subject: subject, body: text})
	return nil
}

func (n *fakeEmailNotifier) EnqueueEmailTo(_ context.Context, accountID uuid.UUID, _ []byte, _, subject, text, _ string) error {
	n.sent = append(n.sent, fakeSentEmail{accountID: accountID, subject: subject, body: text})
	return nil
}

// lastTokenFor pulls the token= query value out of the reset/activation link
// in the most recent email's plain-text part sent to accountID (the body no
// longer prints the raw token separately — see auth.Service.Register).
func (n *fakeEmailNotifier) lastTokenFor(accountID uuid.UUID) string {
	for i := len(n.sent) - 1; i >= 0; i-- {
		if n.sent[i].accountID != accountID {
			continue
		}
		const marker = "token="
		if idx := strings.LastIndex(n.sent[i].body, marker); idx >= 0 {
			value := n.sent[i].body[idx+len(marker):]
			if end := strings.IndexAny(value, " \n\r"); end >= 0 {
				value = value[:end]
			}
			return value
		}
	}
	return ""
}

func (f *fakeAuthStore) FindAccountByUsername(_ context.Context, username string) (Account, string, error) {
	account, ok := f.accounts[username]
	if !ok {
		return Account{}, "", ErrNotFound
	}
	return account, f.passwords[username], nil
}

func (f *fakeAuthStore) FindAccountByID(_ context.Context, accountID uuid.UUID) (Account, error) {
	account, ok := f.accountByID(accountID)
	if !ok {
		return Account{}, ErrNotFound
	}
	return account, nil
}

func (f *fakeAuthStore) CreateSession(_ context.Context, accountID uuid.UUID, tokenHash, csrfHash []byte, expiresAt time.Time, _, _ []byte) (uuid.UUID, error) {
	account, ok := f.accountByID(accountID)
	if !ok {
		return uuid.Nil, ErrNotFound
	}
	session := Session{ID: uuid.New(), Account: account, CSRFHash: append([]byte(nil), csrfHash...), ExpiresAt: expiresAt, TokenHash: append([]byte(nil), tokenHash...)}
	f.sessions[string(tokenHash)] = session
	return session.ID, nil
}

func (f *fakeAuthStore) FindActiveSession(_ context.Context, tokenHash []byte, now time.Time) (Session, error) {
	session, ok := f.sessions[string(tokenHash)]
	if !ok || !session.ExpiresAt.After(now) {
		return Session{}, ErrNotFound
	}
	if session.Account.AccountStatus != "active" {
		return Session{}, ErrNotFound
	}
	return session, nil
}

func (f *fakeAuthStore) TouchSession(_ context.Context, _ uuid.UUID, _ time.Time) error { return nil }

func (f *fakeAuthStore) RevokeSession(_ context.Context, sessionID uuid.UUID, _ time.Time) error {
	for key, session := range f.sessions {
		if session.ID == sessionID {
			delete(f.sessions, key)
			return nil
		}
	}
	return nil
}

func (f *fakeAuthStore) CreateOAuthBinding(_ context.Context, accountID uuid.UUID, provider string, subjectHash []byte) error {
	key := provider + ":" + string(subjectHash)
	if _, exists := f.bindings[key]; exists {
		return ErrConflict
	}
	account, ok := f.accountByID(accountID)
	if !ok {
		return ErrNotFound
	}
	f.bindings[key] = account
	return nil
}

func (f *fakeAuthStore) FindAccountByOAuthSubjectHashes(_ context.Context, provider string, subjectHashes [][]byte) (Account, []byte, error) {
	for _, h := range subjectHashes {
		if account, ok := f.bindings[provider+":"+string(h)]; ok {
			return account, h, nil
		}
	}
	return Account{}, nil, ErrNotFound
}

func (f *fakeAuthStore) CreateOAuthState(_ context.Context, provider string, accountID *uuid.UUID, stateHash, verifier []byte, redirectURL, returnTo string, _ time.Time) error {
	var copyID *uuid.UUID
	if accountID != nil {
		id := *accountID
		copyID = &id
	}
	f.states[provider+":"+string(stateHash)] = OAuthState{Provider: provider, AccountID: copyID, CodeVerifierCiphertext: verifier, RedirectURL: redirectURL, ReturnTo: returnTo}
	return nil
}

func (f *fakeAuthStore) ConsumeOAuthState(_ context.Context, provider string, stateHash []byte, _ time.Time) (OAuthState, error) {
	key := provider + ":" + string(stateHash)
	state, ok := f.states[key]
	if !ok {
		return OAuthState{}, ErrExpired
	}
	delete(f.states, key)
	return state, nil
}

func (f *fakeAuthStore) SaveCalendarGrant(_ context.Context, accountID uuid.UUID, provider string, refreshTokenCiphertext []byte, scope string) error {
	if f.calendarGrants == nil {
		f.calendarGrants = make(map[string]fakeCalendarGrant)
	}
	f.calendarGrants[provider+":"+accountID.String()] = fakeCalendarGrant{ciphertext: append([]byte(nil), refreshTokenCiphertext...), scope: scope}
	return nil
}

func (f *fakeAuthStore) GetCalendarGrant(_ context.Context, accountID uuid.UUID, provider string) ([]byte, string, error) {
	grant, ok := f.calendarGrants[provider+":"+accountID.String()]
	if !ok {
		return nil, "", ErrNotFound
	}
	return grant.ciphertext, grant.scope, nil
}

func (f *fakeAuthStore) DeleteCalendarGrant(_ context.Context, accountID uuid.UUID, provider string) error {
	delete(f.calendarGrants, provider+":"+accountID.String())
	return nil
}

func (f *fakeAuthStore) accountByID(id uuid.UUID) (Account, bool) {
	for _, account := range f.accounts {
		if account.ID == id {
			return account, true
		}
	}
	return Account{}, false
}

// registerAndActivate registers through the school-email flow and immediately
// consumes the activation email's token, mirroring what a real user does by
// clicking the link and setting a password — tests that need a usable
// (active) account call this instead of Register directly.
func registerAndActivate(t *testing.T, service *Service, notifier *fakeEmailNotifier, username, email, schoolEmail, password string) Account {
	t.Helper()
	registerRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", nil)
	registerRequest.RemoteAddr = "192.0.2.10:1234"
	account, err := service.Register(context.Background(), RegisterInput{
		Username: username, Email: email, SchoolEmail: schoolEmail,
	}, registerRequest)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if account.AccountStatus != "pending_verification" {
		t.Fatalf("AccountStatus after Register() = %q, want pending_verification", account.AccountStatus)
	}
	token := notifier.lastTokenFor(account.ID)
	if token == "" {
		t.Fatalf("no activation token captured for account %s", account.ID)
	}
	if err := service.ConfirmPasswordReset(context.Background(), token, password); err != nil {
		t.Fatalf("ConfirmPasswordReset() error = %v", err)
	}
	activated, err := service.store.FindAccountByID(context.Background(), account.ID)
	if err != nil {
		t.Fatalf("FindAccountByID() error = %v", err)
	}
	return activated
}

func TestServiceRegisterLoginAndCSRF(t *testing.T) {
	store := newFakeAuthStore()
	cipher, err := NewFieldCipher(make([]byte, 32))
	if err != nil {
		t.Fatalf("NewFieldCipher() error = %v", err)
	}
	service, err := NewService(store, cipher, make([]byte, 32), time.Hour, false)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	notifier := &fakeEmailNotifier{}
	service.ConfigureEmailVerification(notifier, "")

	account := registerAndActivate(t, service, notifier, "  Alice_01 ", " contact@example.test ", "student@ntu.edu.tw", "correct horse battery staple")
	if account.Username != "alice_01" {
		t.Fatalf("Username = %q, want normalized username", account.Username)
	}
	if account.AccountStatus != "active" || account.IdentityStatus != "student" {
		t.Fatalf("account after activation = %+v, want active/student", account)
	}

	loginRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	loginRequest.RemoteAddr = "192.0.2.10:1234"
	result, err := service.Login(context.Background(), LoginInput{Username: "alice_01", Password: "correct horse battery staple"}, loginRequest)
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if result.Token == "" || result.CSRFToken == "" {
		t.Fatal("Login() returned empty session credentials")
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: result.Token})
	request.AddCookie(&http.Cookie{Name: csrfCookieName, Value: result.CSRFToken})
	session, err := service.Authenticate(context.Background(), request)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if err := service.AuthorizeMutation(request, session); !errors.Is(err, ErrCSRF) {
		t.Fatalf("AuthorizeMutation() without header = %v, want CSRF error", err)
	}
	request.Header.Set("X-CSRF-Token", result.CSRFToken)
	if err := service.AuthorizeMutation(request, session); err != nil {
		t.Fatalf("AuthorizeMutation() error = %v", err)
	}
	if err := service.Logout(context.Background(), session); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if _, err := service.Authenticate(context.Background(), request); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("Authenticate(revoked) = %v, want invalid session", err)
	}
}

func TestServiceRejectsNonSchoolEmail(t *testing.T) {
	store := newFakeAuthStore()
	cipher, _ := NewFieldCipher(make([]byte, 32))
	service, _ := NewService(store, cipher, make([]byte, 32), time.Hour, false)
	notifier := &fakeEmailNotifier{}
	service.ConfigureEmailVerification(notifier, "")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", nil)

	_, err := service.Register(context.Background(), RegisterInput{Username: "user", Email: "user@example.test", SchoolEmail: "user@notedu.tw"}, request)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Register(non-school email) = %v, want invalid input", err)
	}
}

func TestActivationRejectsWeakPassword(t *testing.T) {
	store := newFakeAuthStore()
	cipher, _ := NewFieldCipher(make([]byte, 32))
	service, _ := NewService(store, cipher, make([]byte, 32), time.Hour, false)
	notifier := &fakeEmailNotifier{}
	service.ConfigureEmailVerification(notifier, "")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", nil)

	account, err := service.Register(context.Background(), RegisterInput{Username: "user", Email: "user@example.test", SchoolEmail: "user@ntu.edu.tw"}, request)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	token := notifier.lastTokenFor(account.ID)
	if err := service.ConfirmPasswordReset(context.Background(), token, "short"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("ConfirmPasswordReset(weak password) = %v, want invalid input", err)
	}
}

func TestOAuthBindingRequiresExistingAccountAndSupportsLogin(t *testing.T) {
	store := newFakeAuthStore()
	cipher, _ := NewFieldCipher(make([]byte, 32))
	service, err := NewService(store, cipher, make([]byte, 32), time.Hour, false)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	notifier := &fakeEmailNotifier{}
	service.ConfigureEmailVerification(notifier, "")
	account := registerAndActivate(t, service, notifier, "oauth-user", "oauth@example.test", "oauth@ntu.edu.tw", "correct horse battery staple")

	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/token" {
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "code_verifier=") {
				return jsonResponse(http.StatusBadRequest, `{"error":"missing PKCE verifier"}`), nil
			}
			return jsonResponse(http.StatusOK, `{"access_token":"test-access-token","token_type":"Bearer"}`), nil
		}
		if r.URL.Path == "/userinfo" {
			if r.Header.Get("Authorization") != "Bearer test-access-token" {
				return jsonResponse(http.StatusUnauthorized, `{"error":"missing access token"}`), nil
			}
			return jsonResponse(http.StatusOK, `{"sub":"provider-subject-1"}`), nil
		}
		return jsonResponse(http.StatusNotFound, `{}`), nil
	})}
	service.oauthHTTPClient = client
	if err := service.ConfigureOAuth("google", OAuthProviderSettings{
		ClientID:     "client",
		ClientSecret: "secret",
		RedirectURL:  "https://sta.example.test/api/v1/auth/oauth/google/callback",
		AuthURL:      "https://provider.example.test/authorize",
		TokenURL:     "https://provider.example.test/token",
		UserInfoURL:  "https://provider.example.test/userinfo",
	}); err != nil {
		t.Fatalf("ConfigureOAuth() error = %v", err)
	}

	bindURL, err := service.OAuthStart(context.Background(), "google", &account.ID, "")
	if err != nil {
		t.Fatalf("OAuthStart(bind) error = %v", err)
	}
	bindQuery, _ := url.Parse(bindURL)
	bindState := bindQuery.Query().Get("state")
	if bindState == "" || bindQuery.Query().Get("code_challenge") == "" {
		t.Fatalf("OAuthStart(bind) URL lacks state or PKCE challenge: %s", bindURL)
	}
	oauthContext := context.WithValue(context.Background(), oauth2.HTTPClient, client)
	bound, err := service.OAuthCallback(oauthContext, "google", bindState, "authorization-code", &account.ID, httptest.NewRequest(http.MethodGet, "/callback", nil))
	if err != nil {
		t.Fatalf("OAuthCallback(bind) error = %v", err)
	}
	if !bound.Bound || bound.Account.ID != account.ID {
		t.Fatalf("OAuthCallback(bind) = %#v, want bound account", bound)
	}

	loginURL, err := service.OAuthStart(context.Background(), "google", nil, "")
	if err != nil {
		t.Fatalf("OAuthStart(login) error = %v", err)
	}
	loginQuery, _ := url.Parse(loginURL)
	loggedIn, err := service.OAuthCallback(oauthContext, "google", loginQuery.Query().Get("state"), "authorization-code", nil, httptest.NewRequest(http.MethodGet, "/callback", nil))
	if err != nil {
		t.Fatalf("OAuthCallback(login) error = %v", err)
	}
	if loggedIn.Session == nil || loggedIn.Account.ID != account.ID {
		t.Fatalf("OAuthCallback(login) = %#v, want a session for bound account", loggedIn)
	}
}

// TestOAuthCalendarScopeCapturesRefreshTokenAndReturnTo locks in the merged
// login+calendar design: a provider configured with CalendarScope must (a)
// request offline access with a forced consent prompt so Google actually
// hands back a refresh token, (b) persist that refresh token via
// CalendarGrantStore on a successful bind, and (c) send the caller back to
// the return_to path it supplied at OAuthStart, not a fixed default.
func TestOAuthCalendarScopeCapturesRefreshTokenAndReturnTo(t *testing.T) {
	store := newFakeAuthStore()
	cipher, _ := NewFieldCipher(make([]byte, 32))
	service, err := NewService(store, cipher, make([]byte, 32), time.Hour, false)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	notifier := &fakeEmailNotifier{}
	service.ConfigureEmailVerification(notifier, "")
	account := registerAndActivate(t, service, notifier, "calendar-user", "calendar@example.test", "calendar@ntu.edu.tw", "correct horse battery staple")

	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/token" {
			return jsonResponse(http.StatusOK, `{"access_token":"test-access-token","refresh_token":"test-refresh-token","token_type":"Bearer"}`), nil
		}
		if r.URL.Path == "/userinfo" {
			return jsonResponse(http.StatusOK, `{"sub":"provider-subject-calendar"}`), nil
		}
		return jsonResponse(http.StatusNotFound, `{}`), nil
	})}
	service.oauthHTTPClient = client
	if err := service.ConfigureOAuth("google", OAuthProviderSettings{
		ClientID:     "client",
		ClientSecret: "secret",
		RedirectURL:  "https://sta.example.test/api/v1/auth/oauth/google/callback",
		AuthURL:      "https://provider.example.test/authorize",
		TokenURL:     "https://provider.example.test/token",
		UserInfoURL:  "https://provider.example.test/userinfo",
		Scopes:       []string{"openid", "email", CalendarScope},
	}); err != nil {
		t.Fatalf("ConfigureOAuth() error = %v", err)
	}

	bindURL, err := service.OAuthStart(context.Background(), "google", &account.ID, "/bochures/program?identifier=116-051-001")
	if err != nil {
		t.Fatalf("OAuthStart(bind) error = %v", err)
	}
	bindQuery, _ := url.Parse(bindURL)
	if got := bindQuery.Query().Get("access_type"); got != "offline" {
		t.Fatalf("OAuthStart access_type = %q, want offline when calendar scope is requested", got)
	}
	if got := bindQuery.Query().Get("prompt"); got != "consent" {
		t.Fatalf("OAuthStart prompt = %q, want consent when calendar scope is requested", got)
	}

	oauthContext := context.WithValue(context.Background(), oauth2.HTTPClient, client)
	bound, err := service.OAuthCallback(oauthContext, "google", bindQuery.Query().Get("state"), "authorization-code", &account.ID, httptest.NewRequest(http.MethodGet, "/callback", nil))
	if err != nil {
		t.Fatalf("OAuthCallback(bind) error = %v", err)
	}
	if !bound.Bound {
		t.Fatalf("OAuthCallback(bind) = %#v, want bound", bound)
	}
	if bound.ReturnTo != "/bochures/program?identifier=116-051-001" {
		t.Fatalf("OAuthCallback(bind) ReturnTo = %q, want the path supplied to OAuthStart", bound.ReturnTo)
	}

	ciphertext, scope, err := store.GetCalendarGrant(context.Background(), account.ID, "google")
	if err != nil {
		t.Fatalf("GetCalendarGrant() error = %v", err)
	}
	plaintext, err := cipher.Open(ciphertext)
	if err != nil {
		t.Fatalf("decrypt stored refresh token: %v", err)
	}
	if plaintext != "test-refresh-token" {
		t.Fatalf("stored refresh token = %q, want %q", plaintext, "test-refresh-token")
	}
	if !strings.Contains(scope, CalendarScope) {
		t.Fatalf("stored scope = %q, want it to contain %q", scope, CalendarScope)
	}
}

// TestOAuthCalendarScopeSkipsGrantWhenGoogleDidNotActuallyGrantIt covers a
// real incident: Google can silently drop the sensitive calendar.events
// scope for an account that isn't allow-listed as a test user while the
// OAuth consent screen is in Testing status, even though the authorize
// request included it and a refresh token still comes back. The token
// response then reports the *actual* granted scope (per OAuth2 §5.1, sent
// whenever it differs from what was requested) — this must be checked
// instead of blindly trusting the configured provider scopes, or a broken
// grant gets recorded that looks fine right up until the first real
// Calendar API call fails with insufficient permissions.
func TestOAuthCalendarScopeSkipsGrantWhenGoogleDidNotActuallyGrantIt(t *testing.T) {
	store := newFakeAuthStore()
	cipher, _ := NewFieldCipher(make([]byte, 32))
	service, err := NewService(store, cipher, make([]byte, 32), time.Hour, false)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	notifier := &fakeEmailNotifier{}
	service.ConfigureEmailVerification(notifier, "")
	account := registerAndActivate(t, service, notifier, "scope-narrowed-user", "scope@example.test", "scope@ntu.edu.tw", "correct horse battery staple")

	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/token" {
			// Google granted a refresh token but explicitly reports back
			// that calendar.events was NOT among the granted scopes.
			return jsonResponse(http.StatusOK, `{"access_token":"test-access-token","refresh_token":"test-refresh-token","token_type":"Bearer","scope":"openid email"}`), nil
		}
		if r.URL.Path == "/userinfo" {
			return jsonResponse(http.StatusOK, `{"sub":"provider-subject-scope-narrowed"}`), nil
		}
		return jsonResponse(http.StatusNotFound, `{}`), nil
	})}
	service.oauthHTTPClient = client
	if err := service.ConfigureOAuth("google", OAuthProviderSettings{
		ClientID:     "client",
		ClientSecret: "secret",
		RedirectURL:  "https://sta.example.test/api/v1/auth/oauth/google/callback",
		AuthURL:      "https://provider.example.test/authorize",
		TokenURL:     "https://provider.example.test/token",
		UserInfoURL:  "https://provider.example.test/userinfo",
		Scopes:       []string{"openid", "email", CalendarScope},
	}); err != nil {
		t.Fatalf("ConfigureOAuth() error = %v", err)
	}

	bindURL, err := service.OAuthStart(context.Background(), "google", &account.ID, "/")
	if err != nil {
		t.Fatalf("OAuthStart(bind) error = %v", err)
	}
	bindQuery, _ := url.Parse(bindURL)

	oauthContext := context.WithValue(context.Background(), oauth2.HTTPClient, client)
	bound, err := service.OAuthCallback(oauthContext, "google", bindQuery.Query().Get("state"), "authorization-code", &account.ID, httptest.NewRequest(http.MethodGet, "/callback", nil))
	if err != nil {
		t.Fatalf("OAuthCallback(bind) error = %v", err)
	}
	if !bound.Bound {
		t.Fatalf("OAuthCallback(bind) = %#v, want bound", bound)
	}

	if _, _, err := store.GetCalendarGrant(context.Background(), account.ID, "google"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetCalendarGrant() error = %v, want ErrNotFound (no grant should be saved when Google did not actually grant calendar scope)", err)
	}
}

// TestOAuthStartRejectsOpenRedirectReturnTo makes sure a return_to that
// doesn't point back into our own site (protocol-relative or absolute)
// never reaches the stored OAuth state, since the callback later redirects
// the browser there unconditionally.
func TestOAuthStartRejectsOpenRedirectReturnTo(t *testing.T) {
	store := newFakeAuthStore()
	cipher, _ := NewFieldCipher(make([]byte, 32))
	service, err := NewService(store, cipher, make([]byte, 32), time.Hour, false)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if err := service.ConfigureOAuth("google", OAuthProviderSettings{
		ClientID:     "client",
		ClientSecret: "secret",
		RedirectURL:  "https://sta.example.test/api/v1/auth/oauth/google/callback",
		AuthURL:      "https://provider.example.test/authorize",
		TokenURL:     "https://provider.example.test/token",
		UserInfoURL:  "https://provider.example.test/userinfo",
	}); err != nil {
		t.Fatalf("ConfigureOAuth() error = %v", err)
	}
	for _, malicious := range []string{"//evil.example.test/steal", "https://evil.example.test", "javascript:alert(1)"} {
		loginURL, err := service.OAuthStart(context.Background(), "google", nil, malicious)
		if err != nil {
			t.Fatalf("OAuthStart(%q) error = %v", malicious, err)
		}
		query, _ := url.Parse(loginURL)
		state := query.Query().Get("state")
		stored, ok := store.states["google:"+string(HashOpaqueToken(state))]
		if !ok {
			t.Fatalf("OAuthStart(%q) did not persist a state row", malicious)
		}
		if stored.ReturnTo != "" {
			t.Fatalf("OAuthStart(%q) stored ReturnTo = %q, want it dropped", malicious, stored.ReturnTo)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
