package portfolio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/auth"
	"sta-backend/internal/storage"
)

type Handler struct {
	authService *auth.Service
	repository  Repository
	blobStore   storage.BlobStore
	scanner     storage.Scanner
}

func NewHandler(authService *auth.Service, repository Repository, blobStore storage.BlobStore) (*Handler, error) {
	return NewHandlerWithScanner(authService, repository, blobStore, nil)
}

func NewHandlerWithScanner(authService *auth.Service, repository Repository, blobStore storage.BlobStore, scanner storage.Scanner) (*Handler, error) {
	if authService == nil || repository == nil || blobStore == nil {
		return nil, errors.New("portfolio handler dependencies are missing")
	}
	return &Handler{authService: authService, repository: repository, blobStore: blobStore, scanner: scanner}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/portfolio/projects", h.listProjects)
	mux.HandleFunc("POST /api/v1/portfolio/projects", h.createProject)
	mux.HandleFunc("GET /api/v1/portfolio/projects/{projectID}/files", h.listFiles)
	mux.HandleFunc("POST /api/v1/portfolio/projects/{projectID}/files", h.uploadFile)
	mux.HandleFunc("POST /api/v1/portfolio/files/{fileID}/submit", h.submitFile)
	mux.HandleFunc("POST /api/v1/portfolio/files/{fileID}/unpublish", h.unpublishFile)
	mux.HandleFunc("POST /api/v1/portfolio/files/{fileID}/hide", h.hideFile)
	mux.HandleFunc("GET /api/v1/portfolio/files/{fileID}/events", h.listFileEvents)
	mux.HandleFunc("GET /api/v1/portfolio/files/{fileID}/download", h.downloadFile)
	mux.HandleFunc("GET /api/v1/admin/portfolio/files", h.listAdminFiles)
	mux.HandleFunc("GET /api/v1/admin/portfolio/files/{fileID}/events", h.listAdminFileEvents)
	mux.HandleFunc("POST /api/v1/admin/portfolio/files/{fileID}/review", h.reviewFile)
}

func (h *Handler) listProjects(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireVerified(w, r)
	if !ok {
		return
	}
	projects, err := h.repository.ListProjects(r.Context(), session.Session.Account.ID)
	if err != nil {
		writePortfolioError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writePortfolioJSON(w, http.StatusOK, map[string]any{"data": projects})
}

func (h *Handler) listFiles(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireVerified(w, r)
	if !ok {
		return
	}
	projectID, err := uuid.Parse(r.PathValue("projectID"))
	if err != nil {
		writePortfolioError(w, http.StatusBadRequest, "invalid_project_id", "project id is invalid")
		return
	}
	files, err := h.repository.ListFiles(r.Context(), session.Session.Account.ID, projectID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writePortfolioJSON(w, http.StatusOK, map[string]any{"data": files})
}

func (h *Handler) createProject(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireVerified(w, r)
	if !ok {
		return
	}
	var input CreateProjectInput
	if err := decodePortfolioJSON(r, &input); err != nil || input.ApplicationID == uuid.Nil || len(strings.TrimSpace(input.Title)) > 200 {
		writePortfolioError(w, http.StatusBadRequest, "invalid_request", "portfolio project data is invalid")
		return
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = "-"
	}
	project, err := h.repository.CreateProject(r.Context(), session.Session.Account.ID, input.ApplicationID, title)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writePortfolioJSON(w, http.StatusCreated, map[string]any{"data": project})
}

func (h *Handler) uploadFile(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireVerified(w, r)
	if !ok {
		return
	}
	projectID, err := uuid.Parse(r.PathValue("projectID"))
	if err != nil {
		writePortfolioError(w, http.StatusBadRequest, "invalid_project_id", "project id is invalid")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, storage.MaxPortfolioFileBytes+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writePortfolioError(w, http.StatusBadRequest, "invalid_multipart", "multipart request is invalid")
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, header, err := r.FormFile("file")
	if err != nil {
		writePortfolioError(w, http.StatusBadRequest, "file_required", "file is required")
		return
	}
	defer file.Close()
	staged, err := storage.StageUpload(header.Filename, header.Header.Get("Content-Type"), file)
	if err != nil {
		writePortfolioError(w, http.StatusBadRequest, "invalid_file", "file is invalid")
		return
	}
	defer staged.Close()
	if err := storage.ScanStagedUpload(r.Context(), h.scanner, staged); err != nil {
		if errors.Is(err, storage.ErrMalwareDetected) {
			writePortfolioError(w, http.StatusUnprocessableEntity, "malware_detected", "uploaded file was rejected")
		} else {
			writePortfolioError(w, http.StatusServiceUnavailable, "scan_unavailable", "file scanning is temporarily unavailable")
		}
		return
	}
	fileID := uuid.New()
	storageKey := fmt.Sprintf("portfolio/%s/%s/%s", session.Session.Account.ID, projectID, fileID)
	stagedFile, err := os.Open(staged.Path)
	if err != nil {
		writePortfolioError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	err = h.blobStore.Put(r.Context(), storageKey, stagedFile, staged.Size, staged.ContentType)
	_ = stagedFile.Close()
	if err != nil {
		_ = h.blobStore.Remove(r.Context(), storageKey)
		writePortfolioError(w, http.StatusBadGateway, "storage_unavailable", "file storage is unavailable")
		return
	}
	created, err := h.repository.CreateFile(r.Context(), session.Session.Account.ID, projectID, staged.OriginalName, storageKey, staged.ContentType, staged.Size, staged.SHA256Hex)
	if err != nil {
		_ = h.blobStore.Remove(r.Context(), storageKey)
		h.writeRepositoryError(w, err)
		return
	}
	writePortfolioJSON(w, http.StatusCreated, map[string]any{"data": created})
}

func (h *Handler) submitFile(w http.ResponseWriter, r *http.Request) {
	h.changeOwnerFileStatus(w, r, h.repository.SubmitForReview)
}

func (h *Handler) unpublishFile(w http.ResponseWriter, r *http.Request) {
	h.changeOwnerFileStatus(w, r, h.repository.Unpublish)
}

func (h *Handler) hideFile(w http.ResponseWriter, r *http.Request) {
	h.changeOwnerFileStatus(w, r, h.repository.Hide)
}

func (h *Handler) changeOwnerFileStatus(w http.ResponseWriter, r *http.Request, change func(context.Context, uuid.UUID, uuid.UUID) (File, error)) {
	session, ok := h.requireVerified(w, r)
	if !ok {
		return
	}
	fileID, err := uuid.Parse(r.PathValue("fileID"))
	if err != nil {
		writePortfolioError(w, http.StatusBadRequest, "invalid_file_id", "file id is invalid")
		return
	}
	file, err := change(r.Context(), session.Session.Account.ID, fileID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writePortfolioJSON(w, http.StatusOK, map[string]any{"data": file})
}

func (h *Handler) listFileEvents(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireVerified(w, r)
	if !ok {
		return
	}
	fileID, err := uuid.Parse(r.PathValue("fileID"))
	if err != nil {
		writePortfolioError(w, http.StatusBadRequest, "invalid_file_id", "file id is invalid")
		return
	}
	events, err := h.repository.ListFileEvents(r.Context(), session.Session.Account.ID, fileID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writePortfolioJSON(w, http.StatusOK, map[string]any{"data": events})
}

func (h *Handler) downloadFile(w http.ResponseWriter, r *http.Request) {
	fileID, err := uuid.Parse(r.PathValue("fileID"))
	if err != nil {
		writePortfolioError(w, http.StatusBadRequest, "invalid_file_id", "file id is invalid")
		return
	}
	file, err := h.repository.GetFile(r.Context(), fileID)
	if errors.Is(err, ErrNotFound) {
		writePortfolioError(w, http.StatusNotFound, "not_found", "file not found")
		return
	}
	if err != nil {
		writePortfolioError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if file.Status != FileStatusPublished {
		session, authErr := h.authService.Authenticate(r.Context(), r)
		if authErr != nil {
			writePortfolioError(w, http.StatusForbidden, "file_not_public", "file is not public")
			return
		}
		admin := h.isAdmin(r, session.Session.Account.ID)
		if !canDownloadPrivateFile(session.Session.Account, file.OwnerAccountID, admin) {
			writePortfolioError(w, http.StatusForbidden, "file_not_public", "file is not public")
			return
		}
	}
	url, err := h.blobStore.PresignGet(r.Context(), file.StorageKey, 5*time.Minute)
	if err != nil {
		writePortfolioError(w, http.StatusBadGateway, "storage_unavailable", "file storage is unavailable")
		return
	}
	writePortfolioJSON(w, http.StatusOK, map[string]any{"url": url.String(), "expires_in": 300})
}

func (h *Handler) reviewFile(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	fileID, err := uuid.Parse(r.PathValue("fileID"))
	if err != nil {
		writePortfolioError(w, http.StatusBadRequest, "invalid_file_id", "file id is invalid")
		return
	}
	var input ReviewInput
	if err := decodePortfolioJSON(r, &input); err != nil || len(input.Reason) > 2000 || (!input.Approved && strings.TrimSpace(input.Reason) == "") {
		writePortfolioError(w, http.StatusBadRequest, "invalid_review", "review data is invalid")
		return
	}
	file, err := h.repository.ReviewFile(r.Context(), session.Session.Account.ID, fileID, input.Approved, strings.TrimSpace(input.Reason))
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writePortfolioJSON(w, http.StatusOK, map[string]any{"data": file})
}

func (h *Handler) listAdminFiles(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	query, err := parseAdminFileQuery(r)
	if err != nil {
		writePortfolioError(w, http.StatusBadRequest, "invalid_query", "portfolio file query is invalid")
		return
	}
	files, err := h.repository.ListAdminFiles(r.Context(), session.Session.Account.ID, query)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writePortfolioJSON(w, http.StatusOK, map[string]any{
		"data": files,
		"meta": map[string]any{"limit": query.Limit, "offset": query.Offset, "count": len(files)},
	})
}

func (h *Handler) listAdminFileEvents(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	fileID, err := uuid.Parse(r.PathValue("fileID"))
	if err != nil {
		writePortfolioError(w, http.StatusBadRequest, "invalid_file_id", "file id is invalid")
		return
	}
	events, err := h.repository.ListAdminFileEvents(r.Context(), session.Session.Account.ID, fileID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writePortfolioJSON(w, http.StatusOK, map[string]any{"data": events})
}

func (h *Handler) requireVerified(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, err := h.authService.Authenticate(r.Context(), r)
	if err != nil {
		if errors.Is(err, auth.ErrAdminMFARequired) || errors.Is(err, auth.ErrAdminMFAInvalid) {
			writePortfolioError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
			return auth.RequestSession{}, false
		}
		writePortfolioError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return auth.RequestSession{}, false
	}
	if session.Session.Account.IdentityStatus != "student" && session.Session.Account.IdentityStatus != "senior" {
		writePortfolioError(w, http.StatusForbidden, "verification_required", "verified identity is required")
		return auth.RequestSession{}, false
	}
	if r.Method != http.MethodGet {
		if err := h.authService.AuthorizeMutation(r, session); err != nil {
			writePortfolioError(w, http.StatusForbidden, "csrf_required", "request verification failed")
			return auth.RequestSession{}, false
		}
	}
	return session, true
}

func (h *Handler) requireAdmin(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, err := h.authService.Authenticate(r.Context(), r)
	if err != nil {
		writePortfolioError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return auth.RequestSession{}, false
	}
	isAdmin, err := h.repository.IsAdmin(r.Context(), session.Session.Account.ID)
	if err != nil {
		writePortfolioError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return auth.RequestSession{}, false
	}
	if !isAdmin {
		writePortfolioError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
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
		writePortfolioError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *Handler) isAdmin(r *http.Request, accountID uuid.UUID) bool {
	isAdmin, err := h.repository.IsAdmin(r.Context(), accountID)
	return err == nil && isAdmin
}

func canDownloadPrivateFile(account auth.Account, ownerAccountID uuid.UUID, admin bool) bool {
	if admin {
		return true
	}
	if account.ID != ownerAccountID {
		return false
	}
	return account.IdentityStatus == "student" || account.IdentityStatus == "senior"
}

func (h *Handler) writeRepositoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writePortfolioError(w, http.StatusNotFound, "not_found", "portfolio resource not found")
	case errors.Is(err, ErrConflict):
		writePortfolioError(w, http.StatusConflict, "conflict", "portfolio resource already exists")
	case errors.Is(err, ErrInvalidStatus):
		writePortfolioError(w, http.StatusConflict, "invalid_status", "file status transition is invalid")
	case errors.Is(err, ErrInvalidQuery):
		writePortfolioError(w, http.StatusBadRequest, "invalid_query", "portfolio file query is invalid")
	case errors.Is(err, ErrNotAdmin), errors.Is(err, ErrForbidden):
		writePortfolioError(w, http.StatusForbidden, "forbidden", "operation is not allowed")
	default:
		writePortfolioError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func decodePortfolioJSON(r *http.Request, destination any) error {
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

type portfolioErrorBody struct {
	Error portfolioError `json:"error"`
}

type portfolioError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writePortfolioJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writePortfolioError(w http.ResponseWriter, status int, code, message string) {
	writePortfolioJSON(w, status, portfolioErrorBody{Error: portfolioError{Code: code, Message: message}})
}

func parseAdminFileQuery(r *http.Request) (AdminFileQuery, error) {
	values := r.URL.Query()
	query := AdminFileQuery{Status: strings.TrimSpace(values.Get("status")), Limit: 50}
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			return AdminFileQuery{}, ErrInvalidQuery
		}
		query.Limit = limit
	}
	if raw := strings.TrimSpace(values.Get("offset")); raw != "" {
		offset, err := strconv.Atoi(raw)
		if err != nil {
			return AdminFileQuery{}, ErrInvalidQuery
		}
		query.Offset = offset
	}
	if raw := strings.TrimSpace(values.Get("project_id")); raw != "" {
		projectID, err := uuid.Parse(raw)
		if err != nil {
			return AdminFileQuery{}, ErrInvalidQuery
		}
		query.ProjectID = projectID
	}
	if err := query.Validate(); err != nil {
		return AdminFileQuery{}, err
	}
	return query, nil
}
