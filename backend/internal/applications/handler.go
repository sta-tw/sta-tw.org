package applications

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"sta-backend/internal/admissions"
	"sta-backend/internal/auth"
)

type Handler struct {
	authService *auth.Service
	repository  Repository
}

func NewHandler(authService *auth.Service, repository Repository) (*Handler, error) {
	if authService == nil || repository == nil {
		return nil, errors.New("application handler dependencies are missing")
	}
	return &Handler{authService: authService, repository: repository}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/applications", h.list)
	mux.HandleFunc("POST /api/v1/applications", h.create)
	mux.HandleFunc("POST /api/v1/applications/service-tickets", h.createTicket)
	mux.HandleFunc("GET /api/v1/admin/applications/service-tickets", h.listOpenTickets)
	mux.HandleFunc("POST /api/v1/admin/applications/service-tickets/{ticketID}/review", h.reviewTicket)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireVerified(w, r)
	if !ok {
		return
	}
	applications, err := h.repository.ListByAccount(r.Context(), session.Session.Account.ID)
	if err != nil {
		writeApplicationError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeApplicationJSON(w, http.StatusOK, map[string]any{"data": applications})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireStudent(w, r)
	if !ok {
		return
	}
	var input CreateInput
	if err := decodeApplicationJSON(r, &input); err != nil || len(input.ProgramIdentifiers) == 0 || len(input.ProgramIdentifiers) > 50 {
		writeApplicationError(w, http.StatusBadRequest, "invalid_request", "program_identifiers is invalid")
		return
	}
	identifiers := make([]admissions.ProgramIdentifier, 0, len(input.ProgramIdentifiers))
	seen := make(map[string]struct{}, len(input.ProgramIdentifiers))
	for _, raw := range input.ProgramIdentifiers {
		identifier, err := admissions.ParseProgramIdentifier(strings.TrimSpace(raw))
		if err != nil {
			writeApplicationError(w, http.StatusBadRequest, "invalid_program_identifier", "program identifier is invalid")
			return
		}
		if _, exists := seen[identifier.String()]; exists {
			writeApplicationError(w, http.StatusBadRequest, "duplicate_program", "program identifier is duplicated")
			return
		}
		seen[identifier.String()] = struct{}{}
		identifiers = append(identifiers, identifier)
	}
	applications, err := h.repository.CreateConfirmed(r.Context(), session.Session.Account.ID, identifiers)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeApplicationJSON(w, http.StatusCreated, map[string]any{"data": applications})
}

func (h *Handler) createTicket(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireStudent(w, r)
	if !ok {
		return
	}
	var input ServiceTicketInput
	if err := decodeApplicationJSON(r, &input); err != nil || len(strings.TrimSpace(input.Reason)) < 1 || len(input.Reason) > 2000 {
		writeApplicationError(w, http.StatusBadRequest, "invalid_request", "ticket data is invalid")
		return
	}
	identifier, err := admissions.ParseProgramIdentifier(strings.TrimSpace(input.ProgramIdentifier))
	if err != nil {
		writeApplicationError(w, http.StatusBadRequest, "invalid_program_identifier", "program identifier is invalid")
		return
	}
	ticket, err := h.repository.CreateServiceTicket(r.Context(), session.Session.Account.ID, identifier, strings.TrimSpace(input.Reason))
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeApplicationJSON(w, http.StatusCreated, map[string]any{"data": ticket})
}

func (h *Handler) listOpenTickets(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	tickets, err := h.repository.ListOpenServiceTickets(r.Context(), session.Session.Account.ID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeApplicationJSON(w, http.StatusOK, map[string]any{"data": tickets})
}

func (h *Handler) reviewTicket(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return
	}
	ticketID, err := uuid.Parse(r.PathValue("ticketID"))
	if err != nil {
		writeApplicationError(w, http.StatusBadRequest, "invalid_ticket_id", "ticket id is invalid")
		return
	}
	var input ServiceTicketReviewInput
	if err := decodeApplicationJSON(r, &input); err != nil || len(input.Reason) > 2000 || (!input.Approved && strings.TrimSpace(input.Reason) == "") {
		writeApplicationError(w, http.StatusBadRequest, "invalid_review", "ticket review is invalid")
		return
	}
	ticket, err := h.repository.ReviewServiceTicket(r.Context(), session.Session.Account.ID, ticketID, input)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeApplicationJSON(w, http.StatusOK, map[string]any{"data": ticket})
}

func (h *Handler) requireAdmin(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, err := h.authService.Authenticate(r.Context(), r)
	if err != nil {
		if errors.Is(err, auth.ErrAdminMFARequired) || errors.Is(err, auth.ErrAdminMFAInvalid) {
			writeApplicationError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
			return auth.RequestSession{}, false
		}
		writeApplicationError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return auth.RequestSession{}, false
	}
	if err := h.authService.AuthorizeMutation(r, session); err != nil && r.Method != http.MethodGet {
		writeApplicationError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return auth.RequestSession{}, false
	}
	admin, err := h.repository.IsAdmin(r.Context(), session.Session.Account.ID)
	if err != nil {
		writeApplicationError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return auth.RequestSession{}, false
	}
	if !admin {
		writeApplicationError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *Handler) requireVerified(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	return h.requireIdentity(w, r, true)
}

func (h *Handler) requireStudent(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	return h.requireIdentity(w, r, false)
}

func (h *Handler) requireIdentity(w http.ResponseWriter, r *http.Request, allowSenior bool) (auth.RequestSession, bool) {
	session, err := h.authService.Authenticate(r.Context(), r)
	if err != nil {
		writeApplicationError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return auth.RequestSession{}, false
	}
	validIdentity := session.Session.Account.IdentityStatus == "student"
	if allowSenior {
		validIdentity = validIdentity || session.Session.Account.IdentityStatus == "senior"
	}
	if !validIdentity {
		writeApplicationError(w, http.StatusForbidden, "verification_required", "verified identity is required")
		return auth.RequestSession{}, false
	}
	if err := h.authService.AuthorizeMutation(r, session); err != nil && r.Method != http.MethodGet {
		writeApplicationError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *Handler) writeRepositoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrConflict):
		writeApplicationError(w, http.StatusConflict, "application_conflict", "application already exists")
	case errors.Is(err, ErrNotFound):
		writeApplicationError(w, http.StatusNotFound, "program_not_found", "admission program not found")
	case errors.Is(err, ErrAdminRequired):
		writeApplicationError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
	case errors.Is(err, ErrInvalidStatus):
		writeApplicationError(w, http.StatusConflict, "invalid_status", "service ticket status is invalid")
	default:
		writeApplicationError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func decodeApplicationJSON(r *http.Request, destination any) error {
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

type applicationErrorBody struct {
	Error applicationError `json:"error"`
}

type applicationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeApplicationJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeApplicationError(w http.ResponseWriter, status int, code, message string) {
	writeApplicationJSON(w, status, applicationErrorBody{Error: applicationError{Code: code, Message: message}})
}
