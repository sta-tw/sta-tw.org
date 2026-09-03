package chat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sta-backend/internal/auth"
	"sta-backend/internal/security"
)

type Handler struct {
	authService           *auth.Service
	repository            Repository
	discordWebhookSecret  string
	telegramWebhookSecret string
	lookupKey             []byte
	messageLimiter        *security.FixedWindowLimiter
	distributedLimiter    security.DistributedLimiter
}

func (h *Handler) ConfigureDistributedLimiter(limiter security.DistributedLimiter) {
	if h != nil {
		h.distributedLimiter = limiter
	}
}

func NewHandler(authService *auth.Service, repository Repository, discordSecret, telegramSecret string, lookupKey []byte) (*Handler, error) {
	if authService == nil || repository == nil || len(lookupKey) != 32 {
		return nil, errors.New("chat handler dependencies are missing")
	}
	return &Handler{
		authService:           authService,
		repository:            repository,
		discordWebhookSecret:  discordSecret,
		telegramWebhookSecret: telegramSecret,
		lookupKey:             append([]byte(nil), lookupKey...),
		messageLimiter:        security.NewFixedWindowLimiter(20, time.Minute, 10000),
	}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/chat/lounge/messages", h.listMessages)
	mux.HandleFunc("POST /api/v1/chat/lounge/messages", h.createWebsiteMessage)
	mux.HandleFunc("POST /api/v1/chat/webhooks/discord", h.discordWebhook)
	mux.HandleFunc("POST /api/v1/chat/webhooks/telegram", h.telegramWebhook)
}

func (h *Handler) listMessages(w http.ResponseWriter, r *http.Request) {
	limit, offset, err := pageQuery(r)
	if err != nil {
		writeChatError(w, http.StatusBadRequest, "invalid_query", "chat query is invalid")
		return
	}
	messages, err := h.repository.ListMessages(r.Context(), limit, offset)
	if err != nil {
		writeChatError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeChatJSON(w, http.StatusOK, map[string]any{"data": messages})
}

func (h *Handler) createWebsiteMessage(w http.ResponseWriter, r *http.Request) {
	session, err := h.authService.Authenticate(r.Context(), r)
	if err != nil {
		writeChatError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	if err := h.authService.AuthorizeMutation(r, session); err != nil {
		writeChatError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return
	}
	allowed, limiterErr := h.allowMessage(r.Context(), session.Session.Account.ID.String(), time.Now().UTC())
	if limiterErr != nil {
		writeChatError(w, http.StatusServiceUnavailable, "rate_limit_unavailable", "request protection is temporarily unavailable")
		return
	}
	if !allowed {
		w.Header().Set("Retry-After", "60")
		writeChatError(w, http.StatusTooManyRequests, "rate_limited", "too many messages")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	var input struct {
		Body string `json:"body"`
	}
	if err := decodeChatJSON(r, &input); err != nil || strings.TrimSpace(input.Body) == "" || len(input.Body) > MaxMessageLength || containsDisallowedBodyControl(input.Body) {
		writeChatError(w, http.StatusBadRequest, "invalid_message", "message is invalid")
		return
	}
	message, err := h.repository.CreateWebsiteMessage(r.Context(), session.Session.Account.ID, strings.TrimSpace(input.Body))
	if err != nil {
		writeChatError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeChatJSON(w, http.StatusCreated, map[string]any{"data": message})
}

func (h *Handler) allowMessage(ctx context.Context, key string, now time.Time) (bool, error) {
	if h.messageLimiter != nil && !h.messageLimiter.Allow(key, now) {
		return false, nil
	}
	if h.distributedLimiter == nil {
		return true, nil
	}
	return h.distributedLimiter.Allow(ctx, "chat-messages", key, 20, time.Minute, now)
}

func (h *Handler) discordWebhook(w http.ResponseWriter, r *http.Request) {
	h.handleWebhook(w, r, PlatformDiscord, h.discordWebhookSecret)
}

func (h *Handler) telegramWebhook(w http.ResponseWriter, r *http.Request) {
	h.handleWebhook(w, r, PlatformTelegram, h.telegramWebhookSecret)
}

func (h *Handler) handleWebhook(w http.ResponseWriter, r *http.Request, platform Platform, secret string) {
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	body, err := io.ReadAll(r.Body)
	if err != nil || !VerifyWebhookSignature(secret, string(body), r.Header.Get("X-STA-Signature")) {
		writeChatError(w, http.StatusUnauthorized, "invalid_signature", "webhook signature is invalid")
		return
	}
	var input ExternalMessage
	if err := json.Unmarshal(body, &input); err != nil {
		writeChatError(w, http.StatusBadRequest, "invalid_message", "webhook payload is invalid")
		return
	}
	input.Platform = platform
	if input.CreatedAt.IsZero() {
		input.CreatedAt = time.Now().UTC()
	}
	if err := ValidateExternalMessage(input); err != nil {
		writeChatError(w, http.StatusBadRequest, "invalid_message", "webhook payload is invalid")
		return
	}
	message, err := h.repository.ApplyExternalMessage(r.Context(), input, h.lookupKey)
	if err != nil {
		writeChatError(w, http.StatusBadRequest, "sync_failed", "webhook message could not be synchronized")
		return
	}
	writeChatJSON(w, http.StatusOK, map[string]any{"data": message})
}

func pageQuery(r *http.Request) (int, int, error) {
	limit, offset := 50, 0
	var err error
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
	}
	if err != nil || limit < 1 || limit > 100 || offset < 0 || offset > 10000 {
		return 0, 0, ErrInvalidMessage
	}
	return limit, offset, nil
}

func decodeChatJSON(r *http.Request, destination any) error {
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

type chatErrorBody struct {
	Error chatError `json:"error"`
}
type chatError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeChatJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeChatError(w http.ResponseWriter, status int, code, message string) {
	writeChatJSON(w, status, chatErrorBody{Error: chatError{Code: code, Message: message}})
}
