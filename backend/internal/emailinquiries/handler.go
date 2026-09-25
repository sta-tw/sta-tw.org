package emailinquiries

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

type Handler struct {
	service          *Service
	telegramBotToken string
}

func NewHandler(service *Service, telegramBotToken string) (*Handler, error) {
	if service == nil {
		return nil, errors.New("email-inquiries handler dependencies are missing")
	}
	return &Handler{service: service, telegramBotToken: strings.TrimSpace(telegramBotToken)}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/internal/email-inquiries/reply-by-telegram", h.replyByTelegram)
}

type replyByTelegramInput struct {
	ChatID    int64 `json:"chat_id"`
	MessageID int64 `json:"message_id"`
	// StaffMessageID/StaffName identify the staff member's own typed
	// reply — see Service.ReplyByTelegram for why they're needed.
	StaffMessageID int64  `json:"staff_message_id"`
	StaffName      string `json:"staff_name"`
	Body           string `json:"body"`
}

// replyByTelegram lets a staff member answer an inquiry by simply typing a
// Telegram reply to its notification message — the bot forwards
// chat_id/message_id/text here, we resolve which inquiry that message
// belongs to, and email the reply. Returns 404 for a reply to a message
// that isn't one of ours, which the bot treats as "not for me" (it may also
// belong to an account-application thread) and tries elsewhere.
func (h *Handler) replyByTelegram(w http.ResponseWriter, r *http.Request) {
	if !h.requireServiceToken(w, r) {
		return
	}
	var input replyByTelegramInput
	if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is invalid")
		return
	}
	inquiry, err := h.service.ReplyByTelegram(r.Context(), input.ChatID, input.MessageID, input.StaffMessageID, input.StaffName, input.Body)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "no inquiry matches this message")
		case errors.Is(err, ErrInvalidInput):
			writeError(w, http.StatusBadRequest, "invalid_input", "reply body is empty")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": inquiry})
}

func (h *Handler) requireServiceToken(w http.ResponseWriter, r *http.Request) bool {
	if h.telegramBotToken == "" {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "this endpoint is not configured")
		return false
	}
	provided := bearerToken(r.Header.Get("Authorization"))
	expectedHash := sha256.Sum256([]byte(h.telegramBotToken))
	providedHash := sha256.Sum256([]byte(provided))
	if provided == "" || subtle.ConstantTimeCompare(expectedHash[:], providedHash[:]) != 1 {
		writeError(w, http.StatusUnauthorized, "invalid_service_token", "service authentication failed")
		return false
	}
	return true
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

type errorBody struct {
	Error errorPayload `json:"error"`
}
type errorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if status != http.StatusNoContent {
		_ = json.NewEncoder(w).Encode(payload)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: errorPayload{Code: code, Message: message}})
}
