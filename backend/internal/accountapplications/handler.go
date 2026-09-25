package accountapplications

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/security"
)

type Handler struct {
	service          *Service
	telegramBotToken string
	formLimiter      *security.FixedWindowLimiter
}

func NewHandler(service *Service, telegramBotToken string) (*Handler, error) {
	if service == nil {
		return nil, errors.New("account-applications handler dependencies are missing")
	}
	return &Handler{
		service: service, telegramBotToken: strings.TrimSpace(telegramBotToken),
		formLimiter: security.NewFixedWindowLimiter(5, time.Hour, 10000),
	}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/account-applications", h.submitForm)
	mux.HandleFunc("POST /api/v1/internal/account-applications/{id}/decision", h.decision)
	mux.HandleFunc("POST /api/v1/internal/account-applications/reply-by-telegram", h.replyByTelegram)
}

// submitForm is the public, unauthenticated application form: the only way
// to create an application. Rate-limited by IP since anyone can call it.
func (h *Handler) submitForm(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	rl := h.formLimiter.Take(clientIP(r), now)
	security.WriteRateLimitHeaders(w, rl, now)
	if !rl.Allowed {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many applications from this address")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 25<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_multipart", "request is invalid")
		return
	}
	defer r.MultipartForm.RemoveAll()

	username := r.FormValue("username")
	email := r.FormValue("email")
	note := r.FormValue("note")

	var attachments []Attachment
	for _, header := range r.MultipartForm.File["files"] {
		file, err := header.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(file, 20<<20))
		_ = file.Close()
		if err != nil || len(data) == 0 {
			continue
		}
		attachments = append(attachments, Attachment{
			Filename: header.Filename, ContentType: header.Header.Get("Content-Type"), Data: data,
		})
	}

	app, err := h.service.SubmitFromWeb(r.Context(), username, email, note, attachments)
	if err != nil {
		if errors.Is(err, ErrInvalidInput) {
			writeError(w, http.StatusBadRequest, "invalid_input", "username or email is invalid")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": app})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

type decisionInput struct {
	Action            string `json:"action"`
	ReviewerAccountID string `json:"reviewer_account_id"`
}

func (h *Handler) decision(w http.ResponseWriter, r *http.Request) {
	if !h.requireServiceToken(w, r) {
		return
	}
	applicationID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "application id is invalid")
		return
	}
	var input decisionInput
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is invalid")
		return
	}
	var reviewerAccountID *uuid.UUID
	if input.ReviewerAccountID != "" {
		if parsed, err := uuid.Parse(input.ReviewerAccountID); err == nil {
			reviewerAccountID = &parsed
		}
	}

	var app Application
	switch input.Action {
	case "approve":
		app, err = h.service.Approve(r.Context(), applicationID, reviewerAccountID)
	case "reject":
		app, err = h.service.Reject(r.Context(), applicationID, reviewerAccountID)
	default:
		writeError(w, http.StatusBadRequest, "invalid_action", "action must be approve or reject")
		return
	}
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "application not found")
		case errors.Is(err, ErrAlreadyDecided):
			writeError(w, http.StatusConflict, "already_decided", "application was already decided")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": app})
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

// replyByTelegram lets a staff member answer a pending application by simply
// typing a Telegram reply to its notification message — the bot forwards
// chat_id/message_id/text here, we resolve which application that message
// belongs to, and email the reply. Returns 404 for a reply to a message that
// isn't one of ours, which the bot treats as "not for me" (it may belong to
// an email-inquiry thread instead) and tries elsewhere.
func (h *Handler) replyByTelegram(w http.ResponseWriter, r *http.Request) {
	if !h.requireServiceToken(w, r) {
		return
	}
	var input replyByTelegramInput
	if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is invalid")
		return
	}
	app, err := h.service.ReplyByTelegram(r.Context(), input.ChatID, input.MessageID, input.StaffMessageID, input.StaffName, input.Body)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "no application matches this message")
		case errors.Is(err, ErrInvalidInput):
			writeError(w, http.StatusBadRequest, "invalid_input", "reply body is empty")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": app})
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
