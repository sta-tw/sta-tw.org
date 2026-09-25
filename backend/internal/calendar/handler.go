package calendar

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"sta-backend/internal/auth"
)

type Handler struct {
	service *Service
	auth    *auth.Service
}

func NewHandler(service *Service, authService *auth.Service) (*Handler, error) {
	if service == nil || authService == nil {
		return nil, errors.New("calendar handler dependencies are missing")
	}
	return &Handler{service: service, auth: authService}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/calendar/google/status", h.status)
	mux.HandleFunc("POST /api/v1/calendar/google/events", h.createEvents)
	mux.HandleFunc("DELETE /api/v1/calendar/google/events/{externalID}", h.deleteEvent)
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	session, err := h.auth.Authenticate(r.Context(), r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	linked, err := h.service.IsLinked(r.Context(), session.Session.Account.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	response := map[string]any{"linked": linked}
	if linked {
		// Only worth the extra query when linked at all — an unlinked
		// account can't have any event links either.
		externalIDs, err := h.service.ListLinkedEvents(r.Context(), session.Session.Account.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		response["linked_event_ids"] = externalIDs
	}
	writeJSON(w, http.StatusOK, response)
}

type eventInput struct {
	Title      string `json:"title"`
	ISOStart   string `json:"iso_start"`
	ISOEnd     string `json:"iso_end"`
	Details    string `json:"details"`
	ExternalID string `json:"external_id"`
}

type createEventsInput struct {
	Items []eventInput `json:"items"`
}

func (h *Handler) createEvents(w http.ResponseWriter, r *http.Request) {
	session, err := h.auth.Authenticate(r.Context(), r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	if err := h.auth.AuthorizeMutation(r, session); err != nil {
		writeError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return
	}
	var input createEventsInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body is invalid")
		return
	}
	if len(input.Items) == 0 || len(input.Items) > 20 {
		writeError(w, http.StatusBadRequest, "invalid_request", "items must contain 1-20 events")
		return
	}
	events := make([]EventInput, 0, len(input.Items))
	for _, item := range input.Items {
		if item.Title == "" || !validISODate(item.ISOStart) || !validISODate(item.ISOEnd) {
			writeError(w, http.StatusBadRequest, "invalid_request", "each item needs a title and valid iso_start/iso_end dates")
			return
		}
		if len(item.ExternalID) > 200 {
			writeError(w, http.StatusBadRequest, "invalid_request", "external_id is too long")
			return
		}
		events = append(events, EventInput{
			Title:      item.Title,
			ISOStart:   item.ISOStart,
			ISOEnd:     item.ISOEnd,
			Details:    item.Details,
			ExternalID: item.ExternalID,
		})
	}
	created, err := h.service.CreateEvents(r.Context(), session.Session.Account.ID, events)
	if errors.Is(err, ErrNotLinked) {
		writeError(w, http.StatusPreconditionRequired, "calendar_not_linked", "Google Calendar is not linked for this account")
		return
	}
	if err != nil {
		// Not ErrNotLinked (handled above), so this is either a genuine
		// Google Calendar API failure or an error class insertEvent doesn't
		// yet recognize as "grant is dead, re-auth" — log the underlying
		// error so a 502 report can actually be diagnosed instead of only
		// showing a generic message to the user.
		slog.Error("calendar event insert failed", "account_id", session.Session.Account.ID, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error":   map[string]string{"code": "calendar_insert_failed", "message": "some events could not be created"},
			"created": created,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"created": created})
}

func (h *Handler) deleteEvent(w http.ResponseWriter, r *http.Request) {
	session, err := h.auth.Authenticate(r.Context(), r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	if err := h.auth.AuthorizeMutation(r, session); err != nil {
		writeError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return
	}
	externalID := r.PathValue("externalID")
	if externalID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "external id is required")
		return
	}
	err = h.service.DeleteEvent(r.Context(), session.Session.Account.ID, externalID)
	if errors.Is(err, ErrEventNotLinked) {
		writeError(w, http.StatusNotFound, "not_found", "no calendar event is linked for that id")
		return
	}
	if errors.Is(err, ErrNotLinked) {
		writeError(w, http.StatusPreconditionRequired, "calendar_not_linked", "Google Calendar is not linked for this account")
		return
	}
	if err != nil {
		slog.Error("calendar event delete failed", "account_id", session.Session.Account.ID, "error", err)
		writeError(w, http.StatusBadGateway, "calendar_delete_failed", "the event could not be removed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validISODate(value string) bool {
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
