package ingestion

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"sta-backend/internal/auth"
)

type Handler struct {
	authService *auth.Service
	repository  Repository
	service     *Service
}

func NewHandler(authService *auth.Service, repository Repository, service *Service) (*Handler, error) {
	if authService == nil || repository == nil || service == nil {
		return nil, errors.New("ingestion handler dependencies are missing")
	}
	return &Handler{authService: authService, repository: repository, service: service}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/admin/ingestion/brochure-runs", h.listRuns)
	mux.HandleFunc("GET /api/v1/admin/ingestion/brochure-runs/{runID}", h.getRun)
	mux.HandleFunc("GET /api/v1/admin/ingestion/jobs/{jobID}", h.getJobStatus)
	mux.HandleFunc("POST /api/v1/admin/ingestion/brochure-runs/{runID}/review", h.reviewRun)
	mux.HandleFunc("POST /api/v1/admin/ingestion/brochure-candidates/{candidateID}/review", h.reviewCandidate)
	mux.HandleFunc("POST /api/v1/admin/ingestion/jobs/{jobID}/retry", h.retryJob)
}

func (h *Handler) listRuns(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	query, err := parseRunQuery(r)
	if err != nil {
		writeIngestionError(w, http.StatusBadRequest, "invalid_query", "ingestion query is invalid")
		return
	}
	items, err := h.repository.ListRuns(r.Context(), session.Session.Account.ID, query)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeIngestionJSON(w, http.StatusOK, map[string]any{"data": items, "meta": map[string]any{"limit": query.Limit, "offset": query.Offset, "count": len(items)}})
}

func (h *Handler) getRun(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	runID, err := uuid.Parse(r.PathValue("runID"))
	if err != nil {
		writeIngestionError(w, http.StatusBadRequest, "invalid_run_id", "run id is invalid")
		return
	}
	item, err := h.repository.GetRun(r.Context(), session.Session.Account.ID, runID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeIngestionJSON(w, http.StatusOK, map[string]any{"data": item})
}

func (h *Handler) getJobStatus(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	jobID, err := uuid.Parse(r.PathValue("jobID"))
	if err != nil {
		writeIngestionError(w, http.StatusBadRequest, "invalid_job_id", "job id is invalid")
		return
	}
	item, err := h.repository.GetJobStatus(r.Context(), jobID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeIngestionJSON(w, http.StatusOK, map[string]any{"data": item})
}

func (h *Handler) reviewRun(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	runID, err := uuid.Parse(r.PathValue("runID"))
	if err != nil {
		writeIngestionError(w, http.StatusBadRequest, "invalid_run_id", "run id is invalid")
		return
	}
	var input ReviewInput
	if err := decodeIngestionJSON(r, &input); err != nil || validateReview(input) != nil {
		writeIngestionError(w, http.StatusBadRequest, "invalid_review", "ingestion review is invalid")
		return
	}
	item, err := h.repository.ReviewRun(r.Context(), session.Session.Account.ID, runID, input)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeIngestionJSON(w, http.StatusOK, map[string]any{"data": item})
}

func (h *Handler) reviewCandidate(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	candidateID, err := uuid.Parse(r.PathValue("candidateID"))
	if err != nil {
		writeIngestionError(w, http.StatusBadRequest, "invalid_candidate_id", "candidate id is invalid")
		return
	}
	var input ReviewInput
	if err := decodeIngestionJSON(r, &input); err != nil || validateReview(input) != nil {
		writeIngestionError(w, http.StatusBadRequest, "invalid_review", "ingestion review is invalid")
		return
	}
	item, err := h.repository.ReviewCandidate(r.Context(), session.Session.Account.ID, candidateID, input)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeIngestionJSON(w, http.StatusOK, map[string]any{"data": item})
}

func (h *Handler) retryJob(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	jobID, err := uuid.Parse(r.PathValue("jobID"))
	if err != nil {
		writeIngestionError(w, http.StatusBadRequest, "invalid_job_id", "job id is invalid")
		return
	}
	if err := h.service.RetryJob(r.Context(), session.Session.Account.ID, jobID); err != nil {
		if errors.Is(err, ErrDispatchUnavailable) {
			writeIngestionError(w, http.StatusServiceUnavailable, "dispatch_unavailable", "ingestion dispatch is unavailable")
			return
		}
		h.writeRepositoryError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) requireAdmin(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, err := h.authService.Authenticate(r.Context(), r)
	if err != nil {
		if errors.Is(err, auth.ErrAdminMFARequired) || errors.Is(err, auth.ErrAdminMFAInvalid) {
			writeIngestionError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
			return auth.RequestSession{}, false
		}
		writeIngestionError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return auth.RequestSession{}, false
	}
	isAdmin, err := h.repository.IsAdmin(r.Context(), session.Session.Account.ID)
	if err != nil {
		writeIngestionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return auth.RequestSession{}, false
	}
	if !isAdmin {
		writeIngestionError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
		return auth.RequestSession{}, false
	}
	if err := h.authService.RequireAdminMFA(r.Context(), session.Session.Account.ID, r.Header.Get("X-MFA-Code")); err != nil {
		writeIngestionError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
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
		writeIngestionError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return auth.RequestSession{}, false
	}
	if err := h.authService.RequireAdminMFA(r.Context(), session.Session.Account.ID, r.Header.Get("X-MFA-Code")); err != nil {
		writeIngestionError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *Handler) writeRepositoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		writeIngestionError(w, http.StatusBadRequest, "invalid_ingestion", "ingestion data is invalid")
	case errors.Is(err, ErrAdminRequired):
		writeIngestionError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
	case errors.Is(err, ErrNotFound):
		writeIngestionError(w, http.StatusNotFound, "not_found", "ingestion resource was not found")
	case errors.Is(err, ErrInvalidStatus):
		writeIngestionError(w, http.StatusConflict, "invalid_status", "ingestion resource is not in a mutable state")
	default:
		writeIngestionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func parseRunQuery(r *http.Request) (RunQuery, error) {
	query := RunQuery{Limit: 50}
	var err error
	if raw := strings.TrimSpace(r.URL.Query().Get("academic_year")); raw != "" {
		query.AcademicYear, err = strconv.Atoi(raw)
		if err != nil || query.AcademicYear < 100 || query.AcademicYear > 999 {
			return RunQuery{}, ErrInvalid
		}
	}
	query.Status = strings.TrimSpace(r.URL.Query().Get("status"))
	if query.Status != "" && query.Status != DefaultRunStatus && query.Status != RunStatusPending && query.Status != RunStatusApproved && query.Status != RunStatusRejected && query.Status != "failed" {
		return RunQuery{}, ErrInvalid
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		query.Limit, err = strconv.Atoi(raw)
		if err != nil || query.Limit < 1 || query.Limit > 100 {
			return RunQuery{}, ErrInvalid
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		query.Offset, err = strconv.Atoi(raw)
		if err != nil || query.Offset < 0 || query.Offset > 10000 {
			return RunQuery{}, ErrInvalid
		}
	}
	return query, nil
}

func validateReview(input ReviewInput) error {
	if !input.Approved && strings.TrimSpace(input.Reason) == "" {
		return ErrInvalid
	}
	if len([]rune(input.Reason)) > 2000 {
		return ErrInvalid
	}
	return nil
}

func decodeIngestionJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("multiple JSON values")
	}
	return nil
}

func writeIngestionJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeIngestionError(w http.ResponseWriter, status int, code, message string) {
	writeIngestionJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
