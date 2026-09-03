package admissions

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"sta-backend/internal/auth"
)

type AdminHandler struct {
	authService *auth.Service
	repository  AdminRepository
}

func NewAdminHandler(authService *auth.Service, repository AdminRepository) (*AdminHandler, error) {
	if authService == nil || repository == nil {
		return nil, errors.New("admission admin handler dependencies are missing")
	}
	return &AdminHandler{authService: authService, repository: repository}, nil
}

func (h *AdminHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/admin/admissions/programs", h.list)
	mux.HandleFunc("GET /api/v1/admin/admissions/programs/{identifier}", h.get)
	mux.HandleFunc("GET /api/v1/admin/admissions/programs/{identifier}/history", h.history)
	mux.HandleFunc("POST /api/v1/admin/admissions/programs/sync", h.sync)
	mux.HandleFunc("PUT /api/v1/admin/admissions/programs/{identifier}", h.update)
	mux.HandleFunc("POST /api/v1/admin/admissions/programs/{identifier}/review", h.review)
}

func (h *AdminHandler) list(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	query, err := parseProgramAdminQuery(r)
	if err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_query", "admin admission query is invalid")
		return
	}
	items, err := h.repository.ListAdminPrograms(r.Context(), session.Session.Account.ID, query)
	if err != nil {
		h.writeAdminError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeAdmissionJSON(w, http.StatusOK, map[string]any{
		"data": items,
		"meta": map[string]any{"limit": query.Limit, "offset": query.Offset, "count": len(items)},
	})
}

func (h *AdminHandler) get(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	identifier, err := parseAdminProgramIdentifier(r)
	if err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_program_identifier", "program identifier is invalid")
		return
	}
	item, err := h.repository.GetAdminProgram(r.Context(), session.Session.Account.ID, identifier)
	if err != nil {
		h.writeAdminError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeAdmissionJSON(w, http.StatusOK, map[string]any{"data": item})
}

func (h *AdminHandler) history(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	identifier, err := parseAdminProgramIdentifier(r)
	if err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_program_identifier", "program identifier is invalid")
		return
	}
	items, err := h.repository.ListProgramHistory(r.Context(), session.Session.Account.ID, identifier)
	if err != nil {
		h.writeAdminError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeAdmissionJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *AdminHandler) sync(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	var input ProgramBatchInput
	if err := decodeBrochureJSON(r, &input); err != nil || input.Validate() != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_sync", "admission program sync data is invalid")
		return
	}
	items, err := h.repository.UpsertPrograms(r.Context(), session.Session.Account.ID, input)
	if err != nil {
		h.writeAdminError(w, err)
		return
	}
	writeAdmissionJSON(w, http.StatusOK, map[string]any{
		"data": items,
		"meta": map[string]any{"count": len(items)},
	})
}

func (h *AdminHandler) update(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	identifier, err := parseAdminProgramIdentifier(r)
	if err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_program_identifier", "program identifier is invalid")
		return
	}
	var input ProgramUpdateInput
	if err := decodeBrochureJSON(r, &input); err != nil || input.Validate() != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_program", "admission program data is invalid")
		return
	}
	item := normalizeProgramInput(input.Item)
	itemIdentifier, err := item.identifier()
	if err != nil || itemIdentifier != identifier {
		writeAdmissionError(w, http.StatusBadRequest, "identifier_mismatch", "program fields do not match the path identifier")
		return
	}
	items, err := h.repository.UpsertPrograms(r.Context(), session.Session.Account.ID, ProgramBatchInput{Reason: input.Reason, Items: []ProgramInput{item}})
	if err != nil {
		h.writeAdminError(w, err)
		return
	}
	writeAdmissionJSON(w, http.StatusOK, map[string]any{"data": items[0]})
}

func (h *AdminHandler) review(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	identifier, err := parseAdminProgramIdentifier(r)
	if err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_program_identifier", "program identifier is invalid")
		return
	}
	var input ProgramReviewInput
	if err := decodeBrochureJSON(r, &input); err != nil || input.Validate() != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_review", "admission program review is invalid")
		return
	}
	item, err := h.repository.ReviewProgram(r.Context(), session.Session.Account.ID, identifier, input)
	if err != nil {
		h.writeAdminError(w, err)
		return
	}
	writeAdmissionJSON(w, http.StatusOK, map[string]any{"data": item})
}

func (h *AdminHandler) requireAdmin(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, err := h.authService.Authenticate(r.Context(), r)
	if err != nil {
		if errors.Is(err, auth.ErrAdminMFARequired) || errors.Is(err, auth.ErrAdminMFAInvalid) {
			writeAdmissionError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
			return auth.RequestSession{}, false
		}
		writeAdmissionError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return auth.RequestSession{}, false
	}
	isAdmin, err := h.repository.IsAdmin(r.Context(), session.Session.Account.ID)
	if err != nil {
		writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return auth.RequestSession{}, false
	}
	if !isAdmin {
		writeAdmissionError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *AdminHandler) requireAdminMutation(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return auth.RequestSession{}, false
	}
	if err := h.authService.AuthorizeMutation(r, session); err != nil {
		writeAdmissionError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *AdminHandler) writeAdminError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidIdentifier), errors.Is(err, ErrInvalidProgram):
		writeAdmissionError(w, http.StatusBadRequest, "invalid_program", "admission program data is invalid")
	case errors.Is(err, ErrAdminRequired):
		writeAdmissionError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
	case errors.Is(err, ErrInvalidStatus):
		writeAdmissionError(w, http.StatusConflict, "invalid_status", "admission program state does not allow this operation")
	case errors.Is(err, ErrNotFound):
		writeAdmissionError(w, http.StatusNotFound, "not_found", "admission program was not found")
	default:
		writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func parseProgramAdminQuery(r *http.Request) (ProgramAdminQuery, error) {
	year, err := parseYear(r.URL.Query().Get("academic_year"))
	if err != nil {
		return ProgramAdminQuery{}, err
	}
	schoolCode := strings.TrimSpace(r.URL.Query().Get("school_code"))
	if schoolCode != "" && !validSchoolCode(schoolCode) {
		return ProgramAdminQuery{}, ErrInvalidProgram
	}
	reviewStatus := strings.TrimSpace(r.URL.Query().Get("review_status"))
	if reviewStatus != "" && !validProgramReviewStatus(reviewStatus) {
		return ProgramAdminQuery{}, ErrInvalidProgram
	}
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(search)) > 100 {
		return ProgramAdminQuery{}, ErrInvalidProgram
	}
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			return ProgramAdminQuery{}, ErrInvalidProgram
		}
	}
	offset := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 || offset > 10000 {
			return ProgramAdminQuery{}, ErrInvalidProgram
		}
	}
	return ProgramAdminQuery{AcademicYear: year, SchoolCode: schoolCode, ReviewStatus: reviewStatus, Search: search, Limit: limit, Offset: offset}, nil
}

func parseAdminProgramIdentifier(r *http.Request) (ProgramIdentifier, error) {
	return ParseProgramIdentifier(strings.TrimSpace(r.PathValue("identifier")))
}
