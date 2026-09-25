package auth

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

// fakeBotStore is a minimal store covering exactly what bot-account
// creation and authentication need: CreateAccount, role grant/check, and
// service API tokens. Kept separate from fakeAuthStore so this test
// doesn't depend on unrelated auth flows compiling against it.
type fakeBotStore struct {
	accounts    map[string]Account // by username
	roles       map[uuid.UUID]map[string]bool
	tokenHashes map[string]uuid.UUID // hex(hash) -> account id
}

func newFakeBotStore() *fakeBotStore {
	return &fakeBotStore{
		accounts:    make(map[string]Account),
		roles:       make(map[uuid.UUID]map[string]bool),
		tokenHashes: make(map[string]uuid.UUID),
	}
}

func (f *fakeBotStore) CreateAccount(_ context.Context, username string, _, _ []byte, _ string) (Account, error) {
	if _, exists := f.accounts[username]; exists {
		return Account{}, ErrConflict
	}
	account := Account{ID: uuid.New(), Username: username, IdentityStatus: "temporary", AccountStatus: "active"}
	f.accounts[username] = account
	return account, nil
}

func (f *fakeBotStore) GrantRole(_ context.Context, accountID uuid.UUID, role string) error {
	if f.roles[accountID] == nil {
		f.roles[accountID] = make(map[string]bool)
	}
	f.roles[accountID][role] = true
	return nil
}

func (f *fakeBotStore) IsAdmin(_ context.Context, accountID uuid.UUID) (bool, error) {
	return f.roles[accountID]["admin"], nil
}

func (f *fakeBotStore) IsServiceAccount(_ context.Context, accountID uuid.UUID) (bool, error) {
	return f.roles[accountID]["service"], nil
}

func (f *fakeBotStore) CreateServiceAPIToken(_ context.Context, accountID uuid.UUID, tokenHash []byte, _ string) error {
	f.tokenHashes[string(tokenHash)] = accountID
	return nil
}

func (f *fakeBotStore) FindAccountByServiceToken(_ context.Context, tokenHash []byte, _ time.Time) (Account, error) {
	accountID, ok := f.tokenHashes[string(tokenHash)]
	if !ok {
		return Account{}, ErrNotFound
	}
	for _, account := range f.accounts {
		if account.ID == accountID {
			return account, nil
		}
	}
	return Account{}, ErrNotFound
}

// The rest of Store is unused by these tests but must compile.
func (f *fakeBotStore) CreatePendingAccount(context.Context, string, []byte, []byte, string) (Account, error) {
	return Account{}, ErrNotConfigured
}
func (f *fakeBotStore) FindAccountByUsername(context.Context, string) (Account, string, error) {
	return Account{}, "", ErrNotFound
}
func (f *fakeBotStore) FindAccountByID(_ context.Context, accountID uuid.UUID) (Account, error) {
	for _, account := range f.accounts {
		if account.ID == accountID {
			return account, nil
		}
	}
	return Account{}, ErrNotFound
}
func (f *fakeBotStore) CreateSession(context.Context, uuid.UUID, []byte, []byte, time.Time, []byte, []byte) (uuid.UUID, error) {
	return uuid.Nil, ErrNotConfigured
}
func (f *fakeBotStore) FindActiveSession(context.Context, []byte, time.Time) (Session, error) {
	return Session{}, ErrNotFound
}
func (f *fakeBotStore) TouchSession(context.Context, uuid.UUID, time.Time) error  { return nil }
func (f *fakeBotStore) RevokeSession(context.Context, uuid.UUID, time.Time) error { return nil }
func (f *fakeBotStore) CreateOAuthBinding(context.Context, uuid.UUID, string, []byte) error {
	return ErrNotConfigured
}
func (f *fakeBotStore) FindAccountByOAuthSubjectHashes(context.Context, string, [][]byte) (Account, []byte, error) {
	return Account{}, nil, ErrNotFound
}
func (f *fakeBotStore) CreateOAuthState(context.Context, string, *uuid.UUID, []byte, []byte, string, string, time.Time) error {
	return ErrNotConfigured
}
func (f *fakeBotStore) ConsumeOAuthState(context.Context, string, []byte, time.Time) (OAuthState, error) {
	return OAuthState{}, ErrNotFound
}

func newBotTestService(t *testing.T, store *fakeBotStore) *Service {
	t.Helper()
	cipher, err := NewFieldCipher(make([]byte, 32))
	if err != nil {
		t.Fatalf("NewFieldCipher() error = %v", err)
	}
	service, err := NewService(store, cipher, make([]byte, 32), time.Hour, false)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func TestCreateBotAccountIssuesWorkingToken(t *testing.T) {
	store := newFakeBotStore()
	service := newBotTestService(t, store)
	ctx := context.Background()

	account, token, err := service.CreateBotAccount(ctx, "brochure-uploader", "brochure import")
	if err != nil {
		t.Fatalf("CreateBotAccount() error = %v", err)
	}
	if token == "" {
		t.Fatal("expected a non-empty token")
	}
	if !store.roles[account.ID]["service"] {
		t.Fatal("expected the 'service' role to be granted")
	}

	request, _ := http.NewRequest(http.MethodPost, "/api/v1/admin/admissions/brochures", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	session, err := service.Authenticate(ctx, request)
	if err != nil {
		t.Fatalf("Authenticate(bot token) error = %v", err)
	}
	if session.Session.Account.ID != account.ID {
		t.Fatalf("Authenticate() resolved account %s, want %s", session.Session.Account.ID, account.ID)
	}
	if session.UsesCookie() || session.TokenKind != tokenKindBearer {
		t.Fatalf("expected bot session to be a bearer token, got %v", session.TokenKind)
	}
}

func TestRequireAdminMFAExemptsServiceAccounts(t *testing.T) {
	store := newFakeBotStore()
	service := newBotTestService(t, store)
	service.requireAdminMFA = true
	ctx := context.Background()

	account, _, err := service.CreateBotAccount(ctx, "brochure-uploader", "brochure import")
	if err != nil {
		t.Fatalf("CreateBotAccount() error = %v", err)
	}
	if err := store.GrantRole(ctx, account.ID, "admin"); err != nil {
		t.Fatalf("GrantRole() error = %v", err)
	}

	// No MFA code, no enrollment on record at all — a human admin account
	// in this state would get ErrAdminMFARequired.
	if err := service.RequireAdminMFA(ctx, account.ID, ""); err != nil {
		t.Fatalf("RequireAdminMFA(service account) error = %v, want nil", err)
	}
}

func TestAuthenticateRejectsUnknownBearerToken(t *testing.T) {
	store := newFakeBotStore()
	service := newBotTestService(t, store)
	request, _ := http.NewRequest(http.MethodGet, "/api/v1/admin/stats", nil)
	request.Header.Set("Authorization", "Bearer not-a-real-token")
	if _, err := service.Authenticate(context.Background(), request); err != ErrInvalidSession {
		t.Fatalf("Authenticate(garbage bearer) error = %v, want ErrInvalidSession", err)
	}
}
