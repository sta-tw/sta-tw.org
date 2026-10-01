package emailinquiries

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"sta-backend/internal/auth"
)

// IsAdminFunc lets the caller (cmd/api/main.go) supply whichever
// repository's role check it already has wired up (e.g. mailroutes.Repository.IsAdmin)
// instead of this package needing its own database dependency just for that
// one query.
type IsAdminFunc func(ctx context.Context, accountID uuid.UUID) (bool, error)

// Handler is the admin-only read surface over inquiries — "which emails has
// this mail route received" (see internal/mailroutes' /admin/mail-routes
// page). Nothing here is mutating; replies go out via the Discord /re
// command (internal/discordmail), not through this API.
type Handler struct {
	authService *auth.Service
	service     *Service
	isAdmin     IsAdminFunc
}

func NewHandler(authService *auth.Service, service *Service, isAdmin IsAdminFunc) (*Handler, error) {
	if authService == nil || service == nil || isAdmin == nil {
		return nil, errors.New("email-inquiries handler dependencies are missing")
	}
	return &Handler{authService: authService, service: service, isAdmin: isAdmin}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/admin/mail-routes/{id}/inquiries", h.listByRoute)
	mux.HandleFunc("GET /api/v1/admin/inquiries/{id}", h.getDetail)
}

func (h *Handler) listByRoute(w http.ResponseWriter, r *http.Request) {
	session, err := h.authService.Authenticate(r.Context(), r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	isAdmin, err := h.isAdmin(r.Context(), session.Session.Account.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if !isAdmin {
		writeError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
		return
	}
	mailRouteID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "id is invalid")
		return
	}
	summaries, err := h.service.ListInquiriesByRoute(r.Context(), mailRouteID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": summaries})
}

func (h *Handler) getDetail(w http.ResponseWriter, r *http.Request) {
	session, err := h.authService.Authenticate(r.Context(), r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	isAdmin, err := h.isAdmin(r.Context(), session.Session.Account.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if !isAdmin {
		writeError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
		return
	}
	inquiryID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "id is invalid")
		return
	}
	detail, err := h.service.GetInquiryDetail(r.Context(), inquiryID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "inquiry not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": detail})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if status != http.StatusNoContent {
		_ = json.NewEncoder(w).Encode(payload)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
