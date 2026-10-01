package mailroutes

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"sta-backend/internal/auth"
)

type Handler struct {
	authService *auth.Service
	repository  Repository
	// permissions is optional (nil when Discord isn't configured — see
	// cmd/api/main.go) — role-visibility features degrade to a no-op rather
	// than failing route creation outright.
	permissions PermissionsClient
	logger      *slog.Logger
}

func NewHandler(authService *auth.Service, repository Repository, permissions PermissionsClient, logger *slog.Logger) (*Handler, error) {
	if authService == nil || repository == nil {
		return nil, errors.New("mail-routes handler dependencies are missing")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{authService: authService, repository: repository, permissions: permissions, logger: logger}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/admin/mail-routes", h.list)
	mux.HandleFunc("POST /api/v1/admin/mail-routes", h.create)
	mux.HandleFunc("PUT /api/v1/admin/mail-routes/{id}", h.update)
	mux.HandleFunc("DELETE /api/v1/admin/mail-routes/{id}", h.delete)
	mux.HandleFunc("GET /api/v1/admin/mail-routes/discord-roles", h.listDiscordRoles)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	items, err := h.repository.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	var input MailRouteInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is invalid")
		return
	}
	if h.permissions == nil {
		writeError(w, http.StatusServiceUnavailable, "discord_not_configured", "discord integration is not configured")
		return
	}
	label := strings.TrimSpace(input.Label)
	if label == "" {
		writeError(w, http.StatusBadRequest, "invalid_input", "label is required")
		return
	}
	channelID, err := h.permissions.CreateForumChannel(r.Context(), label)
	if err != nil {
		h.logger.Error("failed to create discord forum channel for mail route", "label", label, "error", err)
		writeError(w, http.StatusBadGateway, "discord_error", "could not create the discord forum channel")
		return
	}
	route, err := h.repository.Create(r.Context(), session.Session.Account.ID, input, channelID)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidInput):
			writeError(w, http.StatusBadRequest, "invalid_input", "local part and label are required")
		case errors.Is(err, ErrDuplicateLocalPart):
			writeError(w, http.StatusConflict, "duplicate_local_part", "this local part is already in use")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		}
		return
	}
	if len(route.VisibleRoleIDs) > 0 {
		if err := h.permissions.ApplyChannelVisibility(r.Context(), route.DiscordForumChannelID, route.VisibleRoleIDs); err != nil {
			h.logger.Error("failed to apply discord channel visibility", "mail_route_id", route.ID, "error", err)
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": route})
}

func (h *Handler) listDiscordRoles(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	if h.permissions == nil {
		writeError(w, http.StatusServiceUnavailable, "discord_not_configured", "discord integration is not configured")
		return
	}
	roles, err := h.permissions.ListGuildRoles(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "discord_error", "could not list discord roles")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": roles})
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdminMutation(w, r); !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "id is invalid")
		return
	}
	var input MailRouteInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is invalid")
		return
	}
	route, err := h.repository.Update(r.Context(), id, input.Label, input.VisibleRoleIDs, input.ReplyTemplate)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidInput):
			writeError(w, http.StatusBadRequest, "invalid_input", "label is required, and reply_template (if set) must include [內容] and [簽名]")
		case errors.Is(err, ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "mail route not found")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		}
		return
	}
	if h.permissions != nil {
		if err := h.permissions.ApplyChannelVisibility(r.Context(), route.DiscordForumChannelID, route.VisibleRoleIDs); err != nil {
			h.logger.Error("failed to apply discord channel visibility", "mail_route_id", route.ID, "error", err)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": route})
}

// delete removes the mail_routes row (Postfix stops accepting the address
// immediately) but never deletes the Discord forum channel itself — its
// post history is worth keeping. Instead the channel is locked down (denied
// to everyone, including every role that used to see it) and every active
// post in it is archived, so it's still there for the record but nobody
// stumbles into a now-dead category.
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdminMutation(w, r); !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "id is invalid")
		return
	}
	route, err := h.repository.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "mail route not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if err := h.repository.Delete(r.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "mail route not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if h.permissions != nil {
		if err := h.permissions.LockChannel(r.Context(), route.DiscordForumChannelID, route.VisibleRoleIDs); err != nil {
			h.logger.Error("failed to lock discord channel for deleted mail route", "mail_route_id", route.ID, "error", err)
		}
		if err := h.permissions.ArchiveAllThreads(r.Context(), route.DiscordForumChannelID); err != nil {
			h.logger.Error("failed to archive discord threads for deleted mail route", "mail_route_id", route.ID, "error", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
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
	isAdmin, err := h.repository.IsAdmin(r.Context(), session.Session.Account.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return auth.RequestSession{}, false
	}
	if !isAdmin {
		writeError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
		return auth.RequestSession{}, false
	}
	if err := h.authService.RequireAdminMFA(r.Context(), session.Session.Account.ID, r.Header.Get("X-MFA-Code")); err != nil {
		writeError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
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
