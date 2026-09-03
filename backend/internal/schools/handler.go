package schools

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"sta-backend/internal/auth"
)

type Handler struct {
	authService *auth.Service
	repository  Repository
}

func NewHandler(authService *auth.Service, repository Repository) (*Handler, error) {
	if authService == nil || repository == nil {
		return nil, errors.New("school handler dependencies are missing")
	}
	return &Handler{authService: authService, repository: repository}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/schools", h.listPublic)
	mux.HandleFunc("GET /api/v1/admin/schools", h.listAdmin)
	mux.HandleFunc("GET /api/v1/admin/schools/{schoolCode}/history", h.history)
	mux.HandleFunc("POST /api/v1/admin/schools/sync", h.sync)
	mux.HandleFunc("PUT /api/v1/admin/schools/{schoolCode}", h.update)
}

func (h *Handler) listPublic(w http.ResponseWriter, r *http.Request) {
	query, err := parseQuery(r)
	if err != nil {
		writeSchoolError(w, http.StatusBadRequest, "invalid_query", "school query is invalid")
		return
	}
	items, err := h.repository.List(r.Context(), false)
	if err != nil {
		writeSchoolError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	matched := Search(items, query.Text, query.Limit+1)
	writeSchoolList(w, query, matched, true)
}

func (h *Handler) listAdmin(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	query, err := parseQuery(r)
	if err != nil {
		writeSchoolError(w, http.StatusBadRequest, "invalid_query", "school query is invalid")
		return
	}
	items, err := h.repository.List(r.Context(), true)
	if err != nil {
		writeSchoolError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	matched := SearchAll(items, query.Text, query.Limit+1)
	writeSchoolList(w, query, matched, false)
}

func (h *Handler) sync(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	var input BatchInput
	if err := decodeSchoolJSON(r, &input); err != nil || input.Validate() != nil {
		writeSchoolError(w, http.StatusBadRequest, "invalid_sync", "school sync data is invalid")
		return
	}
	items, err := h.repository.Upsert(r.Context(), session.Session.Account.ID, input)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeSchoolJSON(w, http.StatusOK, map[string]any{
		"data": items,
		"meta": map[string]any{"count": len(items)},
	})
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	code := strings.TrimSpace(r.PathValue("schoolCode"))
	if err := validateSchoolCode(code); err != nil {
		writeSchoolError(w, http.StatusBadRequest, "invalid_school_code", "school code is invalid")
		return
	}
	var input struct {
		SchoolName      string `json:"school_name"`
		InstitutionType string `json:"institution_type"`
		IsActive        *bool  `json:"is_active"`
		Reason          string `json:"reason"`
	}
	if err := decodeSchoolJSON(r, &input); err != nil {
		writeSchoolError(w, http.StatusBadRequest, "invalid_school", "school data is invalid")
		return
	}
	item := SchoolInput{
		SchoolCode:      code,
		SchoolName:      input.SchoolName,
		InstitutionType: input.InstitutionType,
		IsActive:        input.IsActive,
	}
	batch := BatchInput{Reason: input.Reason, Items: []SchoolInput{item}}
	if err := batch.Validate(); err != nil {
		writeSchoolError(w, http.StatusBadRequest, "invalid_school", "school data is invalid")
		return
	}
	items, err := h.repository.Upsert(r.Context(), session.Session.Account.ID, batch)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeSchoolJSON(w, http.StatusOK, map[string]any{"data": items[0]})
}

func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	code := strings.TrimSpace(r.PathValue("schoolCode"))
	if err := validateSchoolCode(code); err != nil {
		writeSchoolError(w, http.StatusBadRequest, "invalid_school_code", "school code is invalid")
		return
	}
	items, err := h.repository.ListHistory(r.Context(), session.Session.Account.ID, code)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeSchoolJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *Handler) requireAdmin(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, err := h.authService.Authenticate(r.Context(), r)
	if err != nil {
		if errors.Is(err, auth.ErrAdminMFARequired) || errors.Is(err, auth.ErrAdminMFAInvalid) {
			writeSchoolError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
			return auth.RequestSession{}, false
		}
		writeSchoolError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return auth.RequestSession{}, false
	}
	isAdmin, err := h.repository.IsAdmin(r.Context(), session.Session.Account.ID)
	if err != nil {
		writeSchoolError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return auth.RequestSession{}, false
	}
	if !isAdmin {
		writeSchoolError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
		return auth.RequestSession{}, false
	}
	if err := h.authService.RequireAdminMFA(r.Context(), session.Session.Account.ID, r.Header.Get("X-MFA-Code")); err != nil {
		writeSchoolError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
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
		writeSchoolError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return auth.RequestSession{}, false
	}
	if err := h.authService.RequireAdminMFA(r.Context(), session.Session.Account.ID, r.Header.Get("X-MFA-Code")); err != nil {
		writeSchoolError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *Handler) writeRepositoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		writeSchoolError(w, http.StatusBadRequest, "invalid_school", "school data is invalid")
	case errors.Is(err, ErrAdminRequired):
		writeSchoolError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
	case errors.Is(err, ErrNotFound):
		writeSchoolError(w, http.StatusNotFound, "not_found", "school was not found")
	default:
		writeSchoolError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func parseQuery(r *http.Request) (Query, error) {
	text := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(text)) > 100 {
		return Query{}, ErrInvalidInput
	}
	limit := 200
	if text != "" {
		limit = 30
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 200 {
			return Query{}, ErrInvalidInput
		}
		limit = parsed
	}
	return Query{Text: text, Limit: limit}, nil
}

func writeSchoolList(w http.ResponseWriter, query Query, matched []School, public bool) {
	hasMore := len(matched) > query.Limit
	if hasMore {
		matched = matched[:query.Limit]
	}
	if public {
		w.Header().Set("Cache-Control", "public, max-age=300")
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}
	writeSchoolJSON(w, http.StatusOK, map[string]any{
		"data": matched,
		"meta": map[string]any{
			"query":    query.Text,
			"limit":    query.Limit,
			"count":    len(matched),
			"has_more": hasMore,
		},
	})
}

func decodeSchoolJSON(r *http.Request, target any) error {
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

func writeSchoolJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeSchoolError(w http.ResponseWriter, status int, code, message string) {
	writeSchoolJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}
