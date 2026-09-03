package support

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/auth"
	"sta-backend/internal/security"
	"sta-backend/internal/storage"
)

type Handler struct {
	authService          *auth.Service
	repository           Repository
	blobStore            storage.BlobStore
	discordWebhookSecret string
	emailWebhookSecret   string
	lookupKey            []byte
	messageLimiter       *security.FixedWindowLimiter
	distributedLimiter   security.DistributedLimiter
	scanner              storage.Scanner
}

func (h *Handler) ConfigureDistributedLimiter(limiter security.DistributedLimiter) {
	if h != nil {
		h.distributedLimiter = limiter
	}
}

func NewHandler(authService *auth.Service, repository Repository, discordWebhookSecret string, lookupKey []byte) (*Handler, error) {
	return NewHandlerWithConfig(authService, repository, discordWebhookSecret, "", lookupKey, nil)
}

func NewHandlerWithBlobStore(authService *auth.Service, repository Repository, discordWebhookSecret string, lookupKey []byte, blobStore storage.BlobStore) (*Handler, error) {
	return NewHandlerWithConfig(authService, repository, discordWebhookSecret, "", lookupKey, blobStore)
}

func NewHandlerWithConfig(authService *auth.Service, repository Repository, discordWebhookSecret, emailWebhookSecret string, lookupKey []byte, blobStore storage.BlobStore) (*Handler, error) {
	return NewHandlerWithConfigAndScanner(authService, repository, discordWebhookSecret, emailWebhookSecret, lookupKey, blobStore, nil)
}

func NewHandlerWithConfigAndScanner(authService *auth.Service, repository Repository, discordWebhookSecret, emailWebhookSecret string, lookupKey []byte, blobStore storage.BlobStore, scanner storage.Scanner) (*Handler, error) {
	if authService == nil || repository == nil {
		return nil, errors.New("support handler dependencies are missing")
	}
	return &Handler{
		authService:          authService,
		repository:           repository,
		blobStore:            blobStore,
		discordWebhookSecret: strings.TrimSpace(discordWebhookSecret),
		emailWebhookSecret:   strings.TrimSpace(emailWebhookSecret),
		lookupKey:            append([]byte(nil), lookupKey...),
		messageLimiter:       security.NewFixedWindowLimiter(30, time.Minute, 10000),
		scanner:              scanner,
	}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/support/tickets", h.listTickets)
	mux.HandleFunc("POST /api/v1/support/tickets", h.createTicket)
	mux.HandleFunc("GET /api/v1/support/tickets/{ticketID}", h.getTicket)
	mux.HandleFunc("GET /api/v1/support/tickets/{ticketID}/attachments/{attachmentID}/download", h.downloadUserAttachment)
	mux.HandleFunc("POST /api/v1/support/tickets/{ticketID}/messages", h.addUserMessage)
	mux.HandleFunc("POST /api/v1/support/tickets/{ticketID}/close", h.closeUserTicket)
	mux.HandleFunc("POST /api/v1/support/tickets/{ticketID}/reopen", h.reopenUserTicket)

	mux.HandleFunc("GET /api/v1/admin/support/tickets", h.listAdminTickets)
	mux.HandleFunc("GET /api/v1/admin/support/tickets/{ticketID}", h.getAdminTicket)
	mux.HandleFunc("GET /api/v1/admin/support/tickets/{ticketID}/attachments/{attachmentID}/download", h.downloadAdminAttachment)
	mux.HandleFunc("PATCH /api/v1/admin/support/tickets/{ticketID}", h.updateAdminTicket)
	mux.HandleFunc("POST /api/v1/admin/support/tickets/{ticketID}/messages", h.addAdminMessage)
	mux.HandleFunc("POST /api/v1/admin/support/tickets/{ticketID}/close", h.closeAdminTicket)
	mux.HandleFunc("POST /api/v1/support/webhooks/discord", h.discordWebhook)
	mux.HandleFunc("POST /api/v1/support/webhooks/email", h.emailWebhook)
}

func (h *Handler) createTicket(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireVerifiedMutation(w, r)
	if !ok {
		return
	}
	input, uploads, err := h.parseCreateTicket(w, r, session.Session.Account.ID)
	if err != nil {
		uploads.remove(r.Context(), h.blobStore)
		if h.writeAttachmentUploadError(w, err) {
			return
		}
		writeSupportError(w, http.StatusBadRequest, "invalid_ticket", "ticket data is invalid")
		return
	}
	if ValidateCreateTicket(input) != nil {
		uploads.remove(r.Context(), h.blobStore)
		writeSupportError(w, http.StatusBadRequest, "invalid_ticket", "ticket data is invalid")
		return
	}
	var ticket TicketDetail
	if len(uploads.inputs) == 0 {
		ticket, err = h.repository.CreateTicket(r.Context(), session.Session.Account.ID, input)
	} else if attachmentRepository, ok := h.repository.(AttachmentRepository); ok {
		ticket, err = attachmentRepository.CreateTicketWithAttachments(r.Context(), session.Session.Account.ID, input, uploads.inputs)
	} else {
		err = errors.New("support attachment repository is not configured")
	}
	if err != nil {
		uploads.remove(r.Context(), h.blobStore)
		h.writeRepositoryError(w, err)
		return
	}
	writeSupportJSON(w, http.StatusCreated, map[string]any{"data": ticket})
}

func (h *Handler) listTickets(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireVerified(w, r)
	if !ok {
		return
	}
	limit, offset, err := supportPageQuery(r)
	if err != nil {
		writeSupportError(w, http.StatusBadRequest, "invalid_query", "support ticket query is invalid")
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && ValidateStatus(TicketStatus(status), true) != nil {
		writeSupportError(w, http.StatusBadRequest, "invalid_query", "support ticket status is invalid")
		return
	}
	tickets, err := h.repository.ListTickets(r.Context(), &session.Session.Account.ID, status, limit, offset)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeSupportJSON(w, http.StatusOK, map[string]any{"data": tickets})
}

func (h *Handler) getTicket(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireVerified(w, r)
	if !ok {
		return
	}
	ticketID, ok := parseTicketID(w, r)
	if !ok {
		return
	}
	ticket, err := h.repository.GetTicket(r.Context(), &session.Session.Account.ID, ticketID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeSupportJSON(w, http.StatusOK, map[string]any{"data": ticket})
}

func (h *Handler) addUserMessage(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireVerifiedMutation(w, r)
	if !ok {
		return
	}
	allowed, limiterErr := h.allowMessage(r.Context(), session.Session.Account.ID.String(), time.Now().UTC())
	if limiterErr != nil {
		writeSupportError(w, http.StatusServiceUnavailable, "rate_limit_unavailable", "request protection is temporarily unavailable")
		return
	}
	if !allowed {
		w.Header().Set("Retry-After", "60")
		writeSupportError(w, http.StatusTooManyRequests, "rate_limited", "too many support messages")
		return
	}
	ticketID, ok := parseTicketID(w, r)
	if !ok {
		return
	}
	input, uploads, err := h.parseMessage(w, r, session.Session.Account.ID)
	if err != nil {
		uploads.remove(r.Context(), h.blobStore)
		if h.writeAttachmentUploadError(w, err) {
			return
		}
		writeSupportError(w, http.StatusBadRequest, "invalid_message", "support message is invalid")
		return
	}
	if ValidateMessageBody(input.Body) != nil {
		uploads.remove(r.Context(), h.blobStore)
		writeSupportError(w, http.StatusBadRequest, "invalid_message", "support message is invalid")
		return
	}
	var message Message
	if len(uploads.inputs) == 0 {
		message, err = h.repository.AddUserMessage(r.Context(), session.Session.Account.ID, ticketID, strings.TrimSpace(input.Body))
	} else if attachmentRepository, ok := h.repository.(AttachmentRepository); ok {
		message, err = attachmentRepository.AddUserMessageWithAttachments(r.Context(), session.Session.Account.ID, ticketID, strings.TrimSpace(input.Body), uploads.inputs)
	} else {
		err = errors.New("support attachment repository is not configured")
	}
	if err != nil {
		uploads.remove(r.Context(), h.blobStore)
		h.writeRepositoryError(w, err)
		return
	}
	writeSupportJSON(w, http.StatusCreated, map[string]any{"data": message})
}

func (h *Handler) closeUserTicket(w http.ResponseWriter, r *http.Request) {
	h.changeUserStatus(w, r, StatusClosed)
}

func (h *Handler) reopenUserTicket(w http.ResponseWriter, r *http.Request) {
	h.changeUserStatus(w, r, StatusWaitingStaff)
}

func (h *Handler) changeUserStatus(w http.ResponseWriter, r *http.Request, status TicketStatus) {
	session, ok := h.requireVerifiedMutation(w, r)
	if !ok {
		return
	}
	ticketID, ok := parseTicketID(w, r)
	if !ok {
		return
	}
	ticket, err := h.repository.SetTicketStatus(r.Context(), session.Session.Account.ID, ticketID, status, nil, false)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeSupportJSON(w, http.StatusOK, map[string]any{"data": ticket})
}

func (h *Handler) listAdminTickets(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	limit, offset, err := supportPageQuery(r)
	if err != nil {
		writeSupportError(w, http.StatusBadRequest, "invalid_query", "support ticket query is invalid")
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && ValidateStatus(TicketStatus(status), true) != nil {
		writeSupportError(w, http.StatusBadRequest, "invalid_query", "support ticket status is invalid")
		return
	}
	tickets, err := h.repository.ListTickets(r.Context(), nil, status, limit, offset)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeSupportJSON(w, http.StatusOK, map[string]any{"data": tickets})
}

func (h *Handler) getAdminTicket(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	ticketID, ok := parseTicketID(w, r)
	if !ok {
		return
	}
	ticket, err := h.repository.GetTicket(r.Context(), nil, ticketID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeSupportJSON(w, http.StatusOK, map[string]any{"data": ticket})
}

func (h *Handler) updateAdminTicket(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	ticketID, ok := parseTicketID(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var input UpdateTicketInput
	if err := decodeSupportJSON(r, &input); err != nil || (input.Status == nil && input.AssignedTo == nil) {
		writeSupportError(w, http.StatusBadRequest, "invalid_ticket_update", "ticket update is invalid")
		return
	}
	status := StatusWaitingStaff
	if input.Status == nil {
		existing, getErr := h.repository.GetTicket(r.Context(), nil, ticketID)
		if getErr != nil {
			h.writeRepositoryError(w, getErr)
			return
		}
		status = TicketStatus(existing.Ticket.Status)
	} else {
		status = TicketStatus(strings.TrimSpace(*input.Status))
	}
	ticket, err := h.repository.SetTicketStatus(r.Context(), session.Session.Account.ID, ticketID, status, input.AssignedTo, true)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeSupportJSON(w, http.StatusOK, map[string]any{"data": ticket})
}

func (h *Handler) addAdminMessage(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	allowed, limiterErr := h.allowMessage(r.Context(), session.Session.Account.ID.String(), time.Now().UTC())
	if limiterErr != nil {
		writeSupportError(w, http.StatusServiceUnavailable, "rate_limit_unavailable", "request protection is temporarily unavailable")
		return
	}
	if !allowed {
		w.Header().Set("Retry-After", "60")
		writeSupportError(w, http.StatusTooManyRequests, "rate_limited", "too many support messages")
		return
	}
	ticketID, ok := parseTicketID(w, r)
	if !ok {
		return
	}
	input, uploads, err := h.parseMessage(w, r, session.Session.Account.ID)
	if err != nil {
		uploads.remove(r.Context(), h.blobStore)
		if h.writeAttachmentUploadError(w, err) {
			return
		}
		writeSupportError(w, http.StatusBadRequest, "invalid_message", "support message is invalid")
		return
	}
	if ValidateMessageBody(input.Body) != nil {
		uploads.remove(r.Context(), h.blobStore)
		writeSupportError(w, http.StatusBadRequest, "invalid_message", "support message is invalid")
		return
	}
	var message Message
	if len(uploads.inputs) == 0 {
		message, err = h.repository.AddAdminMessage(r.Context(), session.Session.Account.ID, ticketID, strings.TrimSpace(input.Body))
	} else if attachmentRepository, ok := h.repository.(AttachmentRepository); ok {
		message, err = attachmentRepository.AddAdminMessageWithAttachments(r.Context(), session.Session.Account.ID, ticketID, strings.TrimSpace(input.Body), uploads.inputs)
	} else {
		err = errors.New("support attachment repository is not configured")
	}
	if err != nil {
		uploads.remove(r.Context(), h.blobStore)
		h.writeRepositoryError(w, err)
		return
	}
	writeSupportJSON(w, http.StatusCreated, map[string]any{"data": message})
}

func (h *Handler) closeAdminTicket(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	ticketID, ok := parseTicketID(w, r)
	if !ok {
		return
	}
	ticket, err := h.repository.SetTicketStatus(r.Context(), session.Session.Account.ID, ticketID, StatusClosed, nil, true)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeSupportJSON(w, http.StatusOK, map[string]any{"data": ticket})
}

func (h *Handler) downloadUserAttachment(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireVerified(w, r)
	if !ok {
		return
	}
	h.downloadAttachment(w, r, &session.Session.Account.ID)
}

func (h *Handler) downloadAdminAttachment(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	h.downloadAttachment(w, r, nil)
}

func (h *Handler) downloadAttachment(w http.ResponseWriter, r *http.Request, accountID *uuid.UUID) {
	if h.blobStore == nil {
		writeSupportError(w, http.StatusServiceUnavailable, "storage_unavailable", "support attachment storage is unavailable")
		return
	}
	attachmentRepository, ok := h.repository.(AttachmentRepository)
	if !ok {
		writeSupportError(w, http.StatusServiceUnavailable, "attachment_unavailable", "support attachments are not configured")
		return
	}
	ticketID, err := uuid.Parse(r.PathValue("ticketID"))
	if err != nil {
		writeSupportError(w, http.StatusBadRequest, "invalid_ticket_id", "ticket id is invalid")
		return
	}
	attachmentID, err := uuid.Parse(r.PathValue("attachmentID"))
	if err != nil {
		writeSupportError(w, http.StatusBadRequest, "invalid_attachment_id", "attachment id is invalid")
		return
	}
	attachment, err := attachmentRepository.GetAttachment(r.Context(), accountID, ticketID, attachmentID)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	url, err := h.blobStore.PresignGet(r.Context(), attachment.storageKey, 5*time.Minute)
	if err != nil {
		writeSupportError(w, http.StatusBadGateway, "storage_unavailable", "support attachment storage is unavailable")
		return
	}
	writeSupportJSON(w, http.StatusOK, map[string]any{"data": attachment, "url": url.String(), "expires_in": 300})
}

type uploadedAttachments struct {
	inputs []AttachmentInput
	keys   []string
}

func (u uploadedAttachments) remove(ctx context.Context, store storage.BlobStore) {
	if store == nil {
		return
	}
	for _, key := range u.keys {
		_ = store.Remove(ctx, key)
	}
}

func (h *Handler) parseCreateTicket(w http.ResponseWriter, r *http.Request, accountID uuid.UUID) (CreateTicketInput, uploadedAttachments, error) {
	if !isMultipart(r) {
		r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
		var input CreateTicketInput
		if err := decodeSupportJSON(r, &input); err != nil {
			return CreateTicketInput{}, uploadedAttachments{}, err
		}
		return input, uploadedAttachments{}, nil
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxAttachmentTotalBytes+1<<20)
	if err := r.ParseMultipartForm(int64(8 << 20)); err != nil {
		return CreateTicketInput{}, uploadedAttachments{}, err
	}
	defer r.MultipartForm.RemoveAll()
	uploads, err := h.uploadAttachments(r, accountID)
	if err != nil {
		return CreateTicketInput{}, uploadedAttachments{}, err
	}
	return CreateTicketInput{Category: r.FormValue("category"), Subject: r.FormValue("subject"), Body: r.FormValue("body")}, uploads, nil
}

func (h *Handler) parseMessage(w http.ResponseWriter, r *http.Request, accountID uuid.UUID) (MessageInput, uploadedAttachments, error) {
	if !isMultipart(r) {
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		var input MessageInput
		if err := decodeSupportJSON(r, &input); err != nil {
			return MessageInput{}, uploadedAttachments{}, err
		}
		return input, uploadedAttachments{}, nil
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxAttachmentTotalBytes+1<<20)
	if err := r.ParseMultipartForm(int64(8 << 20)); err != nil {
		return MessageInput{}, uploadedAttachments{}, err
	}
	defer r.MultipartForm.RemoveAll()
	uploads, err := h.uploadAttachments(r, accountID)
	if err != nil {
		return MessageInput{}, uploadedAttachments{}, err
	}
	return MessageInput{Body: r.FormValue("body")}, uploads, nil
}

func (h *Handler) uploadAttachments(r *http.Request, accountID uuid.UUID) (uploadedAttachments, error) {
	files := r.MultipartForm.File["attachments"]
	if len(files) == 0 {
		files = r.MultipartForm.File["file"]
	}
	if len(files) > MaxAttachmentCount {
		return uploadedAttachments{}, ErrInvalidInput
	}
	if len(files) > 0 && h.blobStore == nil {
		return uploadedAttachments{}, errors.New("support attachment storage is not configured")
	}
	result := uploadedAttachments{inputs: make([]AttachmentInput, 0, len(files)), keys: make([]string, 0, len(files))}
	var total int64
	for _, header := range files {
		file, err := header.Open()
		if err != nil {
			result.remove(r.Context(), h.blobStore)
			return uploadedAttachments{}, err
		}
		staged, stageErr := storage.StageUploadWithLimit(header.Filename, header.Header.Get("Content-Type"), file, MaxAttachmentFileBytes)
		_ = file.Close()
		if stageErr != nil {
			result.remove(r.Context(), h.blobStore)
			return uploadedAttachments{}, stageErr
		}
		if scanErr := storage.ScanStagedUpload(r.Context(), h.scanner, staged); scanErr != nil {
			staged.Close()
			result.remove(r.Context(), h.blobStore)
			return uploadedAttachments{}, scanErr
		}
		total += staged.Size
		if total > MaxAttachmentTotalBytes {
			staged.Close()
			result.remove(r.Context(), h.blobStore)
			return uploadedAttachments{}, ErrInvalidInput
		}
		key := fmt.Sprintf("support/%s/%s", accountID, uuid.NewString())
		stagedFile, err := os.Open(staged.Path)
		if err == nil {
			err = h.blobStore.Put(r.Context(), key, stagedFile, staged.Size, staged.ContentType)
			_ = stagedFile.Close()
		}
		staged.Close()
		if err != nil {
			result.remove(r.Context(), h.blobStore)
			_ = h.blobStore.Remove(r.Context(), key)
			return uploadedAttachments{}, err
		}
		result.keys = append(result.keys, key)
		result.inputs = append(result.inputs, AttachmentInput{OriginalName: staged.OriginalName, StorageKey: key, MIMEType: staged.ContentType, FileSizeBytes: staged.Size, SHA256: staged.SHA256Hex})
	}
	if err := ValidateAttachments(result.inputs); err != nil {
		result.remove(r.Context(), h.blobStore)
		return uploadedAttachments{}, err
	}
	return result, nil
}

func isMultipart(r *http.Request) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0])), "multipart/form-data")
}

func (h *Handler) discordWebhook(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	body, err := io.ReadAll(r.Body)
	if err != nil || !VerifyWebhookSignature(h.discordWebhookSecret, body, r.Header.Get("X-STA-Signature")) {
		writeSupportError(w, http.StatusUnauthorized, "invalid_signature", "Discord webhook signature is invalid")
		return
	}
	var input ExternalMessage
	if err := json.Unmarshal(body, &input); err != nil || ValidateExternalMessage(input) != nil {
		writeSupportError(w, http.StatusBadRequest, "invalid_message", "Discord message is invalid")
		return
	}
	message, err := h.repository.ApplyDiscordMessage(r.Context(), input, h.lookupKey)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeSupportJSON(w, http.StatusOK, map[string]any{"data": message})
}

func (h *Handler) emailWebhook(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	body, err := io.ReadAll(r.Body)
	if err != nil || !VerifyWebhookSignature(h.emailWebhookSecret, body, r.Header.Get("X-STA-Signature")) {
		writeSupportError(w, http.StatusUnauthorized, "invalid_signature", "Email webhook signature is invalid")
		return
	}
	var input ExternalEmailMessage
	if err := json.Unmarshal(body, &input); err != nil || ValidateExternalEmailMessage(input) != nil {
		writeSupportError(w, http.StatusBadRequest, "invalid_message", "Email message is invalid")
		return
	}
	repository, ok := h.repository.(EmailInboundRepository)
	if !ok {
		writeSupportError(w, http.StatusServiceUnavailable, "email_inbound_unavailable", "Email inbound is not configured")
		return
	}
	message, err := repository.ApplyEmailMessage(r.Context(), input, h.lookupKey)
	if err != nil {
		h.writeRepositoryError(w, err)
		return
	}
	writeSupportJSON(w, http.StatusOK, map[string]any{"data": message})
}

func (h *Handler) requireVerified(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, err := h.authService.Authenticate(r.Context(), r)
	if err != nil {
		if errors.Is(err, auth.ErrAdminMFARequired) || errors.Is(err, auth.ErrAdminMFAInvalid) {
			writeSupportError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
			return auth.RequestSession{}, false
		}
		writeSupportError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return auth.RequestSession{}, false
	}
	if !session.Session.Account.EmailVerified {
		writeSupportError(w, http.StatusForbidden, "email_verification_required", "email verification is required")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *Handler) requireVerifiedMutation(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, ok := h.requireVerified(w, r)
	if !ok {
		return auth.RequestSession{}, false
	}
	if err := h.authService.AuthorizeMutation(r, session); err != nil {
		writeSupportError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *Handler) requireAdmin(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, ok := h.requireVerified(w, r)
	if !ok {
		return auth.RequestSession{}, false
	}
	isAdmin, err := h.repository.IsAdmin(r.Context(), session.Session.Account.ID)
	if err != nil {
		writeSupportError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return auth.RequestSession{}, false
	}
	if !isAdmin {
		writeSupportError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
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
		writeSupportError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *Handler) allowMessage(ctx context.Context, key string, now time.Time) (bool, error) {
	if h.messageLimiter != nil && !h.messageLimiter.Allow(key, now) {
		return false, nil
	}
	if h.distributedLimiter == nil {
		return true, nil
	}
	return h.distributedLimiter.Allow(ctx, "support-messages", key, 30, time.Minute, now)
}

func (h *Handler) writeAttachmentUploadError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, storage.ErrMalwareDetected):
		writeSupportError(w, http.StatusUnprocessableEntity, "malware_detected", "uploaded file was rejected")
		return true
	case errors.Is(err, storage.ErrScannerUnavailable):
		writeSupportError(w, http.StatusServiceUnavailable, "scan_unavailable", "file scanning is temporarily unavailable")
		return true
	default:
		return false
	}
}

func (h *Handler) writeRepositoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeSupportError(w, http.StatusNotFound, "not_found", "support ticket not found")
	case errors.Is(err, ErrForbidden):
		writeSupportError(w, http.StatusForbidden, "forbidden", "support ticket access is forbidden")
	case errors.Is(err, ErrAdminRequired):
		writeSupportError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
	case errors.Is(err, ErrConflict):
		writeSupportError(w, http.StatusConflict, "conflict", "support ticket cannot be changed in its current state")
	case errors.Is(err, ErrInvalidStatus):
		writeSupportError(w, http.StatusBadRequest, "invalid_status", "support ticket status is invalid")
	case errors.Is(err, ErrInvalidInput), errors.Is(err, ErrInvalidDiscordData):
		writeSupportError(w, http.StatusBadRequest, "invalid_request", "support request is invalid")
	default:
		writeSupportError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func parseTicketID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	ticketID, err := uuid.Parse(r.PathValue("ticketID"))
	if err != nil {
		writeSupportError(w, http.StatusBadRequest, "invalid_ticket_id", "ticket id is invalid")
		return uuid.Nil, false
	}
	return ticketID, true
}

func supportPageQuery(r *http.Request) (int, int, error) {
	limit, offset := 50, 0
	var err error
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
	}
	if err != nil || limit < 1 || limit > 100 || offset < 0 || offset > 10000 {
		return 0, 0, ErrInvalidInput
	}
	return limit, offset, nil
}

func VerifyWebhookSignature(secret string, body []byte, signature string) bool {
	if strings.TrimSpace(secret) == "" || strings.TrimSpace(signature) == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	want := mac.Sum(nil)
	got, err := hex.DecodeString(strings.TrimSpace(signature))
	return err == nil && hmac.Equal(got, want)
}

func decodeSupportJSON(r *http.Request, destination any) error {
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

type supportErrorBody struct {
	Error supportError `json:"error"`
}

type supportError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeSupportJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if status != http.StatusNoContent {
		_ = json.NewEncoder(w).Encode(payload)
	}
}

func writeSupportError(w http.ResponseWriter, status int, code, message string) {
	writeSupportJSON(w, status, supportErrorBody{Error: supportError{Code: code, Message: message}})
}
