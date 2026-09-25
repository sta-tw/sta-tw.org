package verification

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/auth"
)

func TestAdminVerificationRouteRejectsNonAdminBeforeRepositoryOperation(t *testing.T) {
	store := &verificationAuthStore{}
	authService := newVerificationAuthService(t, store)
	repository := &verificationHandlerRepository{}
	verificationService, err := NewService(repository, mustVerificationCipher(t), make([]byte, 32), nil, nil)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	handler, err := NewHandler(authService, verificationService, repository, nil)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/verification/domains", nil)
	request.Header.Set("Authorization", "Bearer verification-test-token")
	recorder := httptest.NewRecorder()
	handler.listDomains(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if repository.listDomainsCalls != 0 {
		t.Fatal("ListDomains() was called for a non-admin request")
	}
}

func newVerificationAuthService(t *testing.T, store *verificationAuthStore) *auth.Service {
	t.Helper()
	cipher, err := auth.NewFieldCipher(make([]byte, 32))
	if err != nil {
		t.Fatalf("NewFieldCipher() error = %v", err)
	}
	service, err := auth.NewService(store, cipher, make([]byte, 32), time.Hour, false)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func mustVerificationCipher(t *testing.T) *auth.FieldCipher {
	t.Helper()
	cipher, err := auth.NewFieldCipher(make([]byte, 32))
	if err != nil {
		t.Fatalf("NewFieldCipher() error = %v", err)
	}
	return cipher
}

type verificationAuthStore struct{}

func (s *verificationAuthStore) CreateAccount(context.Context, string, []byte, []byte, string) (auth.Account, error) {
	return auth.Account{}, errors.New("not used")
}

func (s *verificationAuthStore) CreatePendingAccount(context.Context, string, []byte, []byte, string) (auth.Account, error) {
	return auth.Account{}, errors.New("not used")
}

func (s *verificationAuthStore) FindAccountByUsername(context.Context, string) (auth.Account, string, error) {
	return auth.Account{}, "", errors.New("not used")
}

func (s *verificationAuthStore) FindAccountByID(context.Context, uuid.UUID) (auth.Account, error) {
	return auth.Account{}, errors.New("not used")
}

func (s *verificationAuthStore) CreateSession(context.Context, uuid.UUID, []byte, []byte, time.Time, []byte, []byte) (uuid.UUID, error) {
	return uuid.Nil, errors.New("not used")
}

func (s *verificationAuthStore) FindActiveSession(_ context.Context, tokenHash []byte, now time.Time) (auth.Session, error) {
	want := auth.HashOpaqueToken("verification-test-token")
	if !bytes.Equal(tokenHash, want) {
		return auth.Session{}, auth.ErrNotFound
	}
	return auth.Session{
		ID: uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		Account: auth.Account{
			ID:            uuid.MustParse("00000000-0000-0000-0000-000000000002"),
			Username:      "verification-user",
			AccountStatus: "active",
		},
		CSRFHash:  auth.HashOpaqueToken("verification-csrf-token"),
		ExpiresAt: now.Add(time.Hour),
	}, nil
}

func (s *verificationAuthStore) TouchSession(context.Context, uuid.UUID, time.Time) error {
	return nil
}

func (s *verificationAuthStore) RevokeSession(context.Context, uuid.UUID, time.Time) error {
	return nil
}

func (s *verificationAuthStore) CreateOAuthBinding(context.Context, uuid.UUID, string, []byte) error {
	return errors.New("not used")
}

func (s *verificationAuthStore) FindAccountByOAuthSubjectHashes(context.Context, string, [][]byte) (auth.Account, []byte, error) {
	return auth.Account{}, nil, errors.New("not used")
}

func (s *verificationAuthStore) CreateOAuthState(context.Context, string, *uuid.UUID, []byte, []byte, string, string, time.Time) error {
	return errors.New("not used")
}

func (s *verificationAuthStore) ConsumeOAuthState(context.Context, string, []byte, time.Time) (auth.OAuthState, error) {
	return auth.OAuthState{}, errors.New("not used")
}

type verificationHandlerRepository struct {
	listDomainsCalls int
}

func (r *verificationHandlerRepository) IsAdmin(context.Context, uuid.UUID) (bool, error) {
	return false, nil
}

func (r *verificationHandlerRepository) IsSchoolEmailAllowed(context.Context, string, string) (bool, error) {
	return false, nil
}

func (r *verificationHandlerRepository) CreateEmailRequest(context.Context, uuid.UUID, CreateRequestInput, []byte, []byte) (Request, error) {
	return Request{}, errors.New("not used")
}

func (r *verificationHandlerRepository) CreateDocumentRequest(context.Context, uuid.UUID, CreateRequestInput) (Request, error) {
	return Request{}, errors.New("not used")
}

func (r *verificationHandlerRepository) CreateEmailChallenge(context.Context, uuid.UUID, []byte, time.Time) error {
	return errors.New("not used")
}

func (r *verificationHandlerRepository) ConsumeEmailCode(context.Context, uuid.UUID, uuid.UUID, []byte, time.Time, time.Time) (Verification, error) {
	return Verification{}, errors.New("not used")
}

func (r *verificationHandlerRepository) CreateDocument(context.Context, uuid.UUID, uuid.UUID, string, string, string, int64, string) (Request, error) {
	return Request{}, errors.New("not used")
}

func (r *verificationHandlerRepository) ListDocuments(context.Context, uuid.UUID, uuid.UUID) ([]Document, error) {
	return nil, errors.New("not used")
}

func (r *verificationHandlerRepository) GetDocument(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (Document, error) {
	return Document{}, errors.New("not used")
}

func (r *verificationHandlerRepository) ListRequests(context.Context, uuid.UUID) ([]Request, error) {
	return nil, errors.New("not used")
}

func (r *verificationHandlerRepository) ListPendingRequests(context.Context, uuid.UUID) ([]Request, error) {
	return nil, errors.New("not used")
}

func (r *verificationHandlerRepository) ReviewDocumentRequest(context.Context, uuid.UUID, uuid.UUID, ReviewInput, time.Time, time.Time) (ReviewResult, error) {
	return ReviewResult{}, errors.New("not used")
}

func (r *verificationHandlerRepository) AddDomain(context.Context, uuid.UUID, string, string) (Domain, error) {
	return Domain{}, errors.New("not used")
}

func (r *verificationHandlerRepository) ListDomains(context.Context, uuid.UUID) ([]Domain, error) {
	r.listDomainsCalls++
	return nil, nil
}

func (r *verificationHandlerRepository) SetDomainActive(context.Context, uuid.UUID, uuid.UUID, bool) error {
	return errors.New("not used")
}

func (r *verificationHandlerRepository) PurgeAnnualData(context.Context, int, time.Time, func(context.Context, string) error) (CleanupReport, error) {
	return CleanupReport{}, errors.New("not used")
}

var _ auth.Store = (*verificationAuthStore)(nil)
var _ Repository = (*verificationHandlerRepository)(nil)
