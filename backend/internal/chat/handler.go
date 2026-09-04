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

	"github.com/google/uuid"
	"sta-backend/internal/auth"
	"sta-backend/internal/pagination"
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
	mux.HandleFunc("GET /api/v1/chat/channels", h.listChannels)
	mux.HandleFunc("GET /api/v1/chat/channels/{channelKey}/messages", h.listChannelMessages)
	mux.HandleFunc("POST /api/v1/chat/channels/{channelKey}/messages", h.createChannelMessage)
	mux.HandleFunc("GET /api/v1/chat/channels/{channelKey}/pins", h.listPins)
	mux.HandleFunc("GET /api/v1/chat/messages/{messageID}/replies", h.listReplies)
	mux.HandleFunc("PUT /api/v1/chat/messages/{messageID}/reactions/{emoji}", h.addReaction)
	mux.HandleFunc("DELETE /api/v1/chat/messages/{messageID}/reactions/{emoji}", h.removeReaction)
	mux.HandleFunc("POST /api/v1/chat/messages/{messageID}/pin", h.pinMessage)
	mux.HandleFunc("DELETE /api/v1/chat/messages/{messageID}/pin", h.unpinMessage)
	mux.HandleFunc("POST /api/v1/chat/webhooks/discord", h.discordWebhook)
	mux.HandleFunc("POST /api/v1/chat/webhooks/telegram", h.telegramWebhook)
}

func (h *Handler) authed(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, err := h.authService.Authenticate(r.Context(), r)
	if err != nil {
		writeChatError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *Handler) authedMutation(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, ok := h.authed(w, r)
	if !ok {
		return auth.RequestSession{}, false
	}
	if err := h.authService.AuthorizeMutation(r, session); err != nil {
		writeChatError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *Handler) listChannels(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authed(w, r); !ok {
		return
	}
	channels, err := h.repository.ListChannels(r.Context())
	if err != nil {
		writeChatError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeChatJSON(w, http.StatusOK, map[string]any{"data": channels})
}

func (h *Handler) listChannelMessages(w http.ResponseWriter, r *http.Request) {
	session, ok := h.authed(w, r)
	if !ok {
		return
	}
	limit, cursor, err := pageQuery(r)
	if err != nil {
		writeChatError(w, http.StatusBadRequest, "invalid_query", "chat query is invalid")
		return
	}
	messages, next, err := h.repository.ListChannelMessages(r.Context(), r.PathValue("channelKey"), session.Session.Account.ID, limit, cursor)
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	writeChatJSON(w, http.StatusOK, map[string]any{"data": messages, "next_cursor": next})
}

func (h *Handler) createChannelMessage(w http.ResponseWriter, r *http.Request) {
	session, ok := h.authedMutation(w, r)
	if !ok {
		return
	}
	now := time.Now().UTC()
	rl, limiterErr := h.allowMessage(r.Context(), session.Session.Account.ID.String(), now)
	if limiterErr != nil {
		writeChatError(w, http.StatusServiceUnavailable, "rate_limit_unavailable", "request protection is temporarily unavailable")
		return
	}
	security.WriteRateLimitHeaders(w, rl, now)
	if !rl.Allowed {
		writeChatError(w, http.StatusTooManyRequests, "rate_limited", "too many messages")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	var input struct {
		Body     string `json:"body"`
		ParentID string `json:"parent_id"`
	}
	if err := decodeChatJSON(r, &input); err != nil || strings.TrimSpace(input.Body) == "" || len(input.Body) > MaxMessageLength || containsDisallowedBodyControl(input.Body) {
		writeChatError(w, http.StatusBadRequest, "invalid_message", "message is invalid")
		return
	}
	var parentID *uuid.UUID
	if strings.TrimSpace(input.ParentID) != "" {
		parsed, err := uuid.Parse(input.ParentID)
		if err != nil {
			writeChatError(w, http.StatusBadRequest, "invalid_message", "parent_id is invalid")
			return
		}
		parentID = &parsed
	}
	message, err := h.repository.CreateChannelMessage(r.Context(), r.PathValue("channelKey"), session.Session.Account.ID, strings.TrimSpace(input.Body), parentID)
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	writeChatJSON(w, http.StatusCreated, map[string]any{"data": message})
}

func (h *Handler) listPins(w http.ResponseWriter, r *http.Request) {
	session, ok := h.authed(w, r)
	if !ok {
		return
	}
	pins, err := h.repository.ListPinned(r.Context(), r.PathValue("channelKey"), session.Session.Account.ID)
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	writeChatJSON(w, http.StatusOK, map[string]any{"data": pins})
}

func (h *Handler) listReplies(w http.ResponseWriter, r *http.Request) {
	session, ok := h.authed(w, r)
	if !ok {
		return
	}
	messageID, err := uuid.Parse(r.PathValue("messageID"))
	if err != nil {
		writeChatError(w, http.StatusBadRequest, "invalid_message_id", "message id is invalid")
		return
	}
	limit, cursor, err := pageQuery(r)
	if err != nil {
		writeChatError(w, http.StatusBadRequest, "invalid_query", "chat query is invalid")
		return
	}
	replies, next, err := h.repository.ListThreadReplies(r.Context(), messageID, session.Session.Account.ID, limit, cursor)
	if err != nil {
		h.writeRepoError(w, err)
		return
	}
	writeChatJSON(w, http.StatusOK, map[string]any{"data": replies, "next_cursor": next})
}

func (h *Handler) addReaction(w http.ResponseWriter, r *http.Request) {
	session, ok := h.authedMutation(w, r)
	if !ok {
		return
	}
	messageID, err := uuid.Parse(r.PathValue("messageID"))
	if err != nil {
		writeChatError(w, http.StatusBadRequest, "invalid_message_id", "message id is invalid")
		return
	}
	emoji, err := NormalizeReaction(r.PathValue("emoji"))
	if err != nil {
		writeChatError(w, http.StatusBadRequest, "invalid_reaction", "reaction is invalid")
		return
	}
	if err := h.repository.SetReaction(r.Context(), messageID, session.Session.Account.ID, emoji); err != nil {
		h.writeRepoError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) removeReaction(w http.ResponseWriter, r *http.Request) {
	session, ok := h.authedMutation(w, r)
	if !ok {
		return
	}
	messageID, err := uuid.Parse(r.PathValue("messageID"))
	if err != nil {
		writeChatError(w, http.StatusBadRequest, "invalid_message_id", "message id is invalid")
		return
	}
	emoji, err := NormalizeReaction(r.PathValue("emoji"))
	if err != nil {
		writeChatError(w, http.StatusBadRequest, "invalid_reaction", "reaction is invalid")
		return
	}
	if err := h.repository.RemoveReaction(r.Context(), messageID, session.Session.Account.ID, emoji); err != nil {
		h.writeRepoError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) pinMessage(w http.ResponseWriter, r *http.Request)   { h.setPin(w, r, true) }
func (h *Handler) unpinMessage(w http.ResponseWriter, r *http.Request) { h.setPin(w, r, false) }

func (h *Handler) setPin(w http.ResponseWriter, r *http.Request, pinned bool) {
	session, ok := h.authedMutation(w, r)
	if !ok {
		return
	}
	isAdmin, err := h.authService.IsAdmin(r.Context(), session.Session.Account.ID)
	if err != nil {
		writeChatError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if !isAdmin {
		writeChatError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
		return
	}
	messageID, err := uuid.Parse(r.PathValue("messageID"))
	if err != nil {
		writeChatError(w, http.StatusBadRequest, "invalid_message_id", "message id is invalid")
		return
	}
	if err := h.repository.SetPinned(r.Context(), messageID, session.Session.Account.ID, pinned); err != nil {
		h.writeRepoError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) writeRepoError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeChatError(w, http.StatusNotFound, "not_found", "chat resource not found")
	case errors.Is(err, ErrInvalidMessage), errors.Is(err, ErrInvalidReaction):
		writeChatError(w, http.StatusBadRequest, "invalid_message", "request is invalid")
	default:
		writeChatError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func (h *Handler) listMessages(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := pageQuery(r)
	if err != nil {
		writeChatError(w, http.StatusBadRequest, "invalid_query", "chat query is invalid")
		return
	}
	messages, nextCursor, err := h.repository.ListMessages(r.Context(), limit, cursor)
	if err != nil {
		writeChatError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeChatJSON(w, http.StatusOK, map[string]any{"data": messages, "next_cursor": nextCursor})
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
	now := time.Now().UTC()
	rl, limiterErr := h.allowMessage(r.Context(), session.Session.Account.ID.String(), now)
	if limiterErr != nil {
		writeChatError(w, http.StatusServiceUnavailable, "rate_limit_unavailable", "request protection is temporarily unavailable")
		return
	}
	security.WriteRateLimitHeaders(w, rl, now)
	if !rl.Allowed {
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

func (h *Handler) allowMessage(ctx context.Context, key string, now time.Time) (security.Result, error) {
	local := h.messageLimiter.Take(key, now)
	if !local.Allowed {
		return local, nil
	}
	if h.distributedLimiter == nil {
		return local, nil
	}
	allowed, err := h.distributedLimiter.Allow(ctx, "chat-messages", key, 20, time.Minute, now)
	if err != nil {
		return security.Result{}, err
	}
	if !allowed {
		local.Allowed = false
		local.Remaining = 0
	}
	return local, nil
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

func pageQuery(r *http.Request) (int, pagination.Cursor, error) {
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			return 0, pagination.Cursor{}, ErrInvalidMessage
		}
		limit = parsed
	}
	cursor, err := pagination.Decode(r.URL.Query().Get("cursor"))
	if err != nil {
		return 0, pagination.Cursor{}, ErrInvalidMessage
	}
	return limit, cursor, nil
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
