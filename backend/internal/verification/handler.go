package verification

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/auth"
	"sta-backend/internal/security"
	"sta-backend/internal/storage"
)

type Handler struct {
	authService   *auth.Service
	service       *Service
	repository    Repository
	blobStore     storage.BlobStore
	scanner       storage.Scanner
	uploadLimiter *security.FixedWindowLimiter
}

func NewHandler(authService *auth.Service, service *Service, repository Repository, blobStore storage.BlobStore) (*Handler, error) {
	return NewHandlerWithScanner(authService, service, repository, blobStore, nil)
}

func NewHandlerWithScanner(authService *auth.Service, service *Service, repository Repository, blobStore storage.BlobStore, scanner storage.Scanner) (*Handler, error) {
	if authService == nil || service == nil || repository == nil {
		return nil, errors.New("verification handler dependencies are missing")
	}
	return &Handler{
		authService:   authService,
		service:       service,
		repository:    repository,
		blobStore:     blobStore,
		scanner:       scanner,
		uploadLimiter: security.NewFixedWindowLimiter(20, time.Minute, 10000),
	}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/verification/requests", h.listRequests)
	mux.HandleFunc("POST /api/v1/verification/requests/school-email", h.createSchoolEmailRequest)
	mux.HandleFunc("POST /api/v1/verification/requests/{requestID}/verify-email", h.verifyEmail)
	mux.HandleFunc("POST /api/v1/verification/requests/document", h.createDocumentRequest)
	mux.HandleFunc("POST /api/v1/verification/requests/{requestID}/documents", h.uploadDocument)
	mux.HandleFunc("GET /api/v1/admin/verification/requests/pending", h.listPendingRequests)
	mux.HandleFunc("GET /api/v1/admin/verification/requests/{requestID}/documents", h.listDocuments)
	mux.HandleFunc("GET /api/v1/admin/verification/requests/{requestID}/documents/{documentID}/download", h.downloadDocument)
	mux.HandleFunc("POST /api/v1/admin/verification/requests/{requestID}/review", h.reviewDocumentRequest)
	mux.HandleFunc("GET /api/v1/admin/verification/domains", h.listDomains)
	mux.HandleFunc("POST /api/v1/admin/verification/domains", h.addDomain)
	mux.HandleFunc("POST /api/v1/admin/verification/domains/{domainID}/active", h.setDomainActive)
}

func (h *Handler) listRequests(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAuth(w, r)
	if !ok {
		return
	}
	items, err := h.repository.ListRequests(r.Context(), session.Session.Account.ID)
	if err != nil {
		writeVerificationError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeVerificationJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *Handler) createSchoolEmailRequest(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireMutation(w, r)
	if !ok {
		return
	}
	var input CreateRequestInput
	if err := decodeVerificationJSON(r, &input); err != nil {
		writeVerificationError(w, http.StatusBadRequest, "invalid_request", "verification request is invalid")
		return
	}
	request, expiresAt, err := h.service.CreateSchoolEmailRequest(r.Context(), session.Session.Account.ID, input)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeVerificationJSON(w, http.StatusCreated, map[string]any{"request": request, "code_expires_at": expiresAt})
}

func (h *Handler) verifyEmail(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireMutation(w, r)
	if !ok {
		return
	}
	requestID, err := uuid.Parse(r.PathValue("requestID"))
	if err != nil {
		writeVerificationError(w, http.StatusBadRequest, "invalid_request_id", "request id is invalid")
		return
	}
	var input struct {
		Code string `json:"code"`
	}
	if err := decodeVerificationJSON(r, &input); err != nil {
		writeVerificationError(w, http.StatusBadRequest, "invalid_code", "verification code is invalid")
		return
	}
	item, err := h.service.VerifySchoolEmail(r.Context(), session.Session.Account.ID, requestID, input.Code)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeVerificationJSON(w, http.StatusOK, map[string]any{"data": item})
}

func (h *Handler) createDocumentRequest(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireMutation(w, r)
	if !ok {
		return
	}
	var input CreateRequestInput
	if err := decodeVerificationJSON(r, &input); err != nil {
		writeVerificationError(w, http.StatusBadRequest, "invalid_request", "verification request is invalid")
		return
	}
	item, err := h.service.CreateDocumentRequest(r.Context(), session.Session.Account.ID, input)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeVerificationJSON(w, http.StatusCreated, map[string]any{"data": item})
}

func (h *Handler) uploadDocument(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireMutation(w, r)
	if !ok {
		return
	}
	if h.blobStore == nil {
		writeVerificationError(w, http.StatusServiceUnavailable, "storage_unavailable", "verification storage is unavailable")
		return
	}
	now := time.Now()
	rl := h.uploadLimiter.Take(session.Session.Account.ID.String(), now)
	security.WriteRateLimitHeaders(w, rl, now)
	if !rl.Allowed {
		writeVerificationError(w, http.StatusTooManyRequests, "rate_limited", "too many uploads")
		return
	}
	requestID, err := uuid.Parse(r.PathValue("requestID"))
	if err != nil {
		writeVerificationError(w, http.StatusBadRequest, "invalid_request_id", "request id is invalid")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, storage.MaxPortfolioFileBytes+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeVerificationError(w, http.StatusBadRequest, "invalid_multipart", "multipart request is invalid")
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, header, err := r.FormFile("file")
	if err != nil {
		writeVerificationError(w, http.StatusBadRequest, "file_required", "verification file is required")
		return
	}
	defer file.Close()
	staged, err := storage.StageUpload(header.Filename, header.Header.Get("Content-Type"), file)
	if err != nil {
		writeVerificationError(w, http.StatusBadRequest, "invalid_file", "verification file is invalid")
		return
	}
	defer staged.Close()
	if err := storage.ScanStagedUpload(r.Context(), h.scanner, staged); err != nil {
		if errors.Is(err, storage.ErrMalwareDetected) {
			writeVerificationError(w, http.StatusUnprocessableEntity, "malware_detected", "uploaded file was rejected")
		} else {
			writeVerificationError(w, http.StatusServiceUnavailable, "scan_unavailable", "file scanning is temporarily unavailable")
		}
		return
	}
	fileID := uuid.New()
	storageKey := "verification/" + session.Session.Account.ID.String() + "/" + requestID.String() + "/" + fileID.String()
	stagedFile, err := os.Open(staged.Path)
	if err != nil {
		writeVerificationError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	err = h.blobStore.Put(r.Context(), storageKey, stagedFile, staged.Size, staged.ContentType)
	_ = stagedFile.Close()
	if err != nil {
		writeVerificationError(w, http.StatusBadGateway, "storage_unavailable", "verification storage is unavailable")
		return
	}
	item, err := h.repository.CreateDocument(r.Context(), session.Session.Account.ID, requestID, staged.OriginalName, storageKey, staged.ContentType, staged.Size, staged.SHA256Hex)
	if err != nil {
		_ = h.blobStore.Remove(r.Context(), storageKey)
		h.writeServiceError(w, err)
		return
	}
	writeVerificationJSON(w, http.StatusCreated, map[string]any{"data": item})
}

func (h *Handler) listPendingRequests(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	items, err := h.repository.ListPendingRequests(r.Context(), session.Session.Account.ID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeVerificationJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *Handler) reviewDocumentRequest(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	requestID, err := uuid.Parse(r.PathValue("requestID"))
	if err != nil {
		writeVerificationError(w, http.StatusBadRequest, "invalid_request_id", "request id is invalid")
		return
	}
	var input ReviewInput
	if err := decodeVerificationJSON(r, &input); err != nil {
		writeVerificationError(w, http.StatusBadRequest, "invalid_review", "review data is invalid")
		return
	}
	result, err := h.service.ReviewDocumentRequest(r.Context(), session.Session.Account.ID, requestID, input)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeVerificationJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (h *Handler) listDocuments(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	requestID, err := uuid.Parse(r.PathValue("requestID"))
	if err != nil {
		writeVerificationError(w, http.StatusBadRequest, "invalid_request_id", "request id is invalid")
		return
	}
	items, err := h.repository.ListDocuments(r.Context(), session.Session.Account.ID, requestID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeVerificationJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *Handler) downloadDocument(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	if h.blobStore == nil {
		writeVerificationError(w, http.StatusServiceUnavailable, "storage_unavailable", "verification storage is unavailable")
		return
	}
	requestID, err := uuid.Parse(r.PathValue("requestID"))
	if err != nil {
		writeVerificationError(w, http.StatusBadRequest, "invalid_request_id", "request id is invalid")
		return
	}
	documentID, err := uuid.Parse(r.PathValue("documentID"))
	if err != nil {
		writeVerificationError(w, http.StatusBadRequest, "invalid_document_id", "document id is invalid")
		return
	}
	document, err := h.repository.GetDocument(r.Context(), session.Session.Account.ID, requestID, documentID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	url, err := h.blobStore.PresignGet(r.Context(), document.StorageKey, 5*time.Minute)
	if err != nil {
		writeVerificationError(w, http.StatusBadGateway, "storage_unavailable", "verification storage is unavailable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeVerificationJSON(w, http.StatusOK, map[string]any{"data": document, "url": url.String(), "expires_in": 300})
}

func (h *Handler) listDomains(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	items, err := h.repository.ListDomains(r.Context(), session.Session.Account.ID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeVerificationJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *Handler) addDomain(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	var input struct {
		SchoolCode string `json:"school_code"`
		Domain     string `json:"domain"`
	}
	if err := decodeVerificationJSON(r, &input); err != nil {
		writeVerificationError(w, http.StatusBadRequest, "invalid_domain", "domain data is invalid")
		return
	}
	item, err := h.service.AddDomain(r.Context(), session.Session.Account.ID, input.SchoolCode, input.Domain)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	writeVerificationJSON(w, http.StatusCreated, map[string]any{"data": item})
}

func (h *Handler) setDomainActive(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	domainID, err := uuid.Parse(r.PathValue("domainID"))
	if err != nil {
		writeVerificationError(w, http.StatusBadRequest, "invalid_domain_id", "domain id is invalid")
		return
	}
	var input struct {
		Active bool `json:"active"`
	}
	if err := decodeVerificationJSON(r, &input); err != nil {
		writeVerificationError(w, http.StatusBadRequest, "invalid_domain", "domain state is invalid")
		return
	}
	if err := h.repository.SetDomainActive(r.Context(), session.Session.Account.ID, domainID, input.Active); err != nil {
		h.writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) requireAuth(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, err := h.authService.Authenticate(r.Context(), r)
	if err != nil {
		if errors.Is(err, auth.ErrAdminMFARequired) || errors.Is(err, auth.ErrAdminMFAInvalid) {
			writeVerificationError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
			return auth.RequestSession{}, false
		}
		writeVerificationError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *Handler) requireMutation(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, ok := h.requireAuth(w, r)
	if !ok {
		return auth.RequestSession{}, false
	}
	if err := h.authService.AuthorizeMutation(r, session); err != nil {
		writeVerificationError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *Handler) requireAdmin(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, ok := h.requireAuth(w, r)
	if !ok {
		return auth.RequestSession{}, false
	}
	isAdmin, err := h.repository.IsAdmin(r.Context(), session.Session.Account.ID)
	if err != nil {
		writeVerificationError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return auth.RequestSession{}, false
	}
	if !isAdmin {
		writeVerificationError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
		return auth.RequestSession{}, false
	}
	if err := h.authService.RequireAdminMFA(r.Context(), session.Session.Account.ID, r.Header.Get("X-MFA-Code")); err != nil {
		writeVerificationError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *Handler) requireAdminMutation(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return auth.RequestSession{}, false
	}
	if err := h.authService.AuthorizeMutation(r, session); err != nil {
		writeVerificationError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return auth.RequestSession{}, false
	}
	if err := h.authService.RequireAdminMFA(r.Context(), session.Session.Account.ID, r.Header.Get("X-MFA-Code")); err != nil {
		writeVerificationError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *Handler) writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeVerificationError(w, http.StatusNotFound, "not_found", "verification resource not found")
	case errors.Is(err, ErrForbidden):
		writeVerificationError(w, http.StatusForbidden, "forbidden", "verification operation is not allowed")
	case errors.Is(err, ErrAdminRequired):
		writeVerificationError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
	case errors.Is(err, ErrInvalidCode):
		writeVerificationError(w, http.StatusUnprocessableEntity, "invalid_code", "verification code is invalid")
	case errors.Is(err, ErrConflict):
		writeVerificationError(w, http.StatusConflict, "conflict", "verification request conflicts with existing data")
	case errors.Is(err, ErrInvalidStatus):
		writeVerificationError(w, http.StatusConflict, "invalid_status", "verification status transition is invalid")
	case errors.Is(err, ErrInvalid):
		writeVerificationError(w, http.StatusBadRequest, "invalid_request", "verification data is invalid")
	default:
		writeVerificationError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func decodeVerificationJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("multiple JSON values")
	}
	return nil
}

type verificationErrorBody struct {
	Error verificationError `json:"error"`
}

type verificationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeVerificationJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if status != http.StatusNoContent {
		_ = json.NewEncoder(w).Encode(payload)
	}
}

func writeVerificationError(w http.ResponseWriter, status int, code, message string) {
	writeVerificationJSON(w, status, verificationErrorBody{Error: verificationError{Code: code, Message: message}})
}
