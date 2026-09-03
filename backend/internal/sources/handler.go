package sources

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
}

func NewHandler(authService *auth.Service, repository Repository) (*Handler, error) {
	if authService == nil || repository == nil {
		return nil, errors.New("admissions source handler dependencies are missing")
	}
	return &Handler{authService: authService, repository: repository}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/admin/admission-sources", h.list)
	mux.HandleFunc("POST /api/v1/admin/admission-sources", h.create)
	mux.HandleFunc("PATCH /api/v1/admin/admission-sources/{sourceID}", h.update)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	query, err := parseQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_query", "source query is invalid")
		return
	}
	items, err := h.repository.List(r.Context(), session.Session.Account.ID, query)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "meta": map[string]any{"limit": query.Limit, "offset": query.Offset, "count": len(items)}})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	var input Input
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_source", "source data is invalid")
		return
	}
	item, err := h.repository.Create(r.Context(), session.Session.Account.ID, input)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": item})
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	sourceID, err := uuid.Parse(r.PathValue("sourceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_source_id", "source id is invalid")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	var input Input
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_source", "source data is invalid")
		return
	}
	item, err := h.repository.Update(r.Context(), session.Session.Account.ID, sourceID, input)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": item})
}

func (h *Handler) requireAdmin(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, err := h.authService.Authenticate(r.Context(), r)
	if err != nil {
		if errors.Is(err, auth.ErrAdminMFARequired) || errors.Is(err, auth.ErrAdminMFAInvalid) {
			writeError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
			return auth.RequestSession{}, false
		}
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return auth.RequestSession{}, false
	}
	ok, err := h.repository.IsAdmin(r.Context(), session.Session.Account.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return auth.RequestSession{}, false
	}
	if !ok {
		writeError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
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
		writeError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *Handler) writeRepositoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_source", "source data is invalid")
	case errors.Is(err, ErrAdminRequired):
		writeError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "source was not found")
	case errors.Is(err, ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "source already exists")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func parseQuery(r *http.Request) (Query, error) {
	query := Query{Limit: 50}
	query.SchoolCode = strings.TrimSpace(r.URL.Query().Get("school_code"))
	query.Status = strings.TrimSpace(r.URL.Query().Get("status"))
	if query.SchoolCode != "" && (len(query.SchoolCode) != 3 || strings.Trim(query.SchoolCode, "0123456789") != "") {
		return Query{}, ErrInvalid
	}
	if query.Status != "" && !validStatus(query.Status) {
		return Query{}, ErrInvalid
	}
	var err error
	if raw := strings.TrimSpace(r.URL.Query().Get("academic_year")); raw != "" {
		query.AcademicYear, err = strconv.Atoi(raw)
		if err != nil || query.AcademicYear < 100 || query.AcademicYear > 999 {
			return Query{}, ErrInvalid
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		query.Limit, err = strconv.Atoi(raw)
		if err != nil || query.Limit < 1 || query.Limit > 100 {
			return Query{}, ErrInvalid
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		query.Offset, err = strconv.Atoi(raw)
		if err != nil || query.Offset < 0 || query.Offset > 10000 {
			return Query{}, ErrInvalid
		}
	}
	return query, nil
}

func decodeJSON(r *http.Request, target any) error {
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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
