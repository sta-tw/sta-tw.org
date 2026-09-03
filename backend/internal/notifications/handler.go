package notifications

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"sta-backend/internal/auth"
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
	limit, offset, err := notificationPageQuery(r)
	if err != nil {
		writeNotificationError(w, http.StatusBadRequest, "invalid_query", "notification query is invalid")
		return
	}
	items, err := h.repository.List(r.Context(), session.Session.Account.ID, limit, offset)
	if err != nil {
		writeNotificationError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeNotificationJSON(w, http.StatusOK, map[string]any{"data": items})
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

func notificationPageQuery(r *http.Request) (int, int, error) {
	limit, offset := 50, 0
	var err error
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
	}
	if err != nil || limit < 1 || limit > 100 || offset < 0 || offset > 10000 {
		return 0, 0, errors.New("invalid notification page")
	}
	return limit, offset, nil
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
