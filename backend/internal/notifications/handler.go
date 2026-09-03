package notifications

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"sta-backend/internal/auth"
	"sta-backend/internal/pagination"
)

type Handler struct {
	authService *auth.Service
	repository  Repository
}

func NewHandler(authService *auth.Service, repository Repository) (*Handler, error) {
	if authService == nil || repository == nil {
		return nil, errors.New("notification handler dependencies are missing")
	}
	return &Handler{authService: authService, repository: repository}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/notifications", h.list)
	mux.HandleFunc("POST /api/v1/notifications/{notificationID}/read", h.markRead)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	session, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	limit, cursor, err := notificationPageQuery(r)
	if err != nil {
		writeNotificationError(w, http.StatusBadRequest, "invalid_query", "notification query is invalid")
		return
	}
	items, nextCursor, err := h.repository.List(r.Context(), session.Session.Account.ID, limit, cursor)
	if err != nil {
		writeNotificationError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeNotificationJSON(w, http.StatusOK, map[string]any{"data": items, "next_cursor": nextCursor})
}

func (h *Handler) markRead(w http.ResponseWriter, r *http.Request) {
	session, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if err := h.authService.AuthorizeMutation(r, session); err != nil {
		writeNotificationError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return
	}
	notificationID, err := uuid.Parse(r.PathValue("notificationID"))
	if err != nil {
		writeNotificationError(w, http.StatusBadRequest, "invalid_notification_id", "notification id is invalid")
		return
	}
	if err := h.repository.MarkRead(r.Context(), session.Session.Account.ID, notificationID); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeNotificationError(w, http.StatusNotFound, "not_found", "notification not found")
			return
		}
		writeNotificationError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, err := h.authService.Authenticate(r.Context(), r)
	if err != nil {
		writeNotificationError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return auth.RequestSession{}, false
	}
	return session, true
}

func notificationPageQuery(r *http.Request) (int, pagination.Cursor, error) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			return 0, pagination.Cursor{}, errors.New("invalid notification page")
		}
		limit = parsed
	}
	cursor, err := pagination.Decode(r.URL.Query().Get("cursor"))
	if err != nil {
		return 0, pagination.Cursor{}, err
	}
	return limit, cursor, nil
}

type notificationErrorBody struct {
	Error notificationError `json:"error"`
}

type notificationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeNotificationJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if status != http.StatusNoContent {
		_ = json.NewEncoder(w).Encode(payload)
	}
}

func writeNotificationError(w http.ResponseWriter, status int, code, message string) {
	writeNotificationJSON(w, status, notificationErrorBody{Error: notificationError{Code: code, Message: message}})
}

func decodeNotificationJSON(r *http.Request, destination any) error {
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
