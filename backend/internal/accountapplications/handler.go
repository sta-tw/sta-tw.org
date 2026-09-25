package accountapplications

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/security"
)

type Handler struct {
	service          *Service
	mailIntakeToken  string
	telegramBotToken string
	formLimiter      *security.FixedWindowLimiter
}

func NewHandler(service *Service, mailIntakeToken, telegramBotToken string) (*Handler, error) {
	if service == nil {
		return nil, errors.New("account-applications handler dependencies are missing")
	}
	return &Handler{
		service: service, mailIntakeToken: strings.TrimSpace(mailIntakeToken), telegramBotToken: strings.TrimSpace(telegramBotToken),
		formLimiter: security.NewFixedWindowLimiter(5, time.Hour, 10000),
	}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/account-applications", h.submitForm)
	mux.HandleFunc("POST /api/v1/internal/account-applications/mail-intake", h.mailIntake)
	mux.HandleFunc("POST /api/v1/internal/account-applications/{id}/decision", h.decision)
	mux.HandleFunc("POST /api/v1/internal/account-applications/reply-by-telegram", h.replyByTelegram)
}

// submitForm is the public, unauthenticated application form: the primary
// way to apply without a school email. Rate-limited by IP since anyone can
// call it.
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

// mailIntake receives one raw RFC 5322 message (as delivered by Postfix's
// pipe transport for account@<mail domain>). A message whose In-Reply-To or
// References header points at a known application (see
// ExtractApplicationIDFromHeaders) is threaded onto it via
// HandleInboundReply; anything else is treated as a new application via
// IntakeFromEmail — a fallback for people who email in without using the web
// form. Never fails loudly to the caller (Postfix) on content problems — a
// malformed submission just doesn't produce anything.
func (h *Handler) mailIntake(w http.ResponseWriter, r *http.Request) {
	if !h.requireServiceToken(w, r, h.mailIntakeToken) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 25<<20)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "could not read message body")
		return
	}
	message, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_message", "could not parse email message")
		return
	}
	from, err := mail.ParseAddress(message.Header.Get("From"))
	if err != nil || from.Address == "" {
		writeError(w, http.StatusBadRequest, "invalid_sender", "email has no usable From address")
		return
	}
	subject := decodeHeaderWord(message.Header.Get("Subject"))
	bodyText, attachments := parseBody(message.Header.Get("Content-Type"), message.Header.Get("Content-Transfer-Encoding"), message.Body)
	bodyText = stripQuotedReply(bodyText)
	sourceMessageID := sanitizeMessageID(message.Header.Get("Message-ID"))

	if applicationID, ok := ExtractApplicationIDFromHeaders(message.Header.Get("In-Reply-To"), message.Header.Get("References")); ok {
		if err := h.service.HandleInboundReply(r.Context(), applicationID, from.Address, bodyText, sourceMessageID, attachments); err != nil {
			writeError(w, http.StatusBadRequest, "reply_failed", "could not record reply")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "reply_recorded"}})
		return
	}

	app, err := h.service.IntakeFromEmail(r.Context(), from.Address, subject, bodyText, sourceMessageID, attachments)
	if err != nil {
		writeError(w, http.StatusBadRequest, "intake_failed", "could not create account application")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": app})
}

type decisionInput struct {
	Action            string `json:"action"`
	ReviewerAccountID string `json:"reviewer_account_id"`
}

func (h *Handler) decision(w http.ResponseWriter, r *http.Request) {
	if !h.requireServiceToken(w, r, h.telegramBotToken) {
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

// replyByTelegram lets a staff member answer an inquiry or a pending
// application by simply typing a Telegram reply to its notification
// message — the bot forwards chat_id/message_id/text here, we resolve
// which application that message belongs to, and email the reply. Returns
// 404 for a reply to a message that isn't one of ours, which the bot
// treats as "not for me" and ignores rather than surfacing an error.
func (h *Handler) replyByTelegram(w http.ResponseWriter, r *http.Request) {
	if !h.requireServiceToken(w, r, h.telegramBotToken) {
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

func (h *Handler) requireServiceToken(w http.ResponseWriter, r *http.Request, expected string) bool {
	if expected == "" {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "this endpoint is not configured")
		return false
	}
	provided := bearerToken(r.Header.Get("Authorization"))
	expectedHash := sha256.Sum256([]byte(expected))
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

// parseBody extracts a best-effort plain-text body and any non-text
// attachments from a (possibly multipart) MIME message. transferEncoding is
// the top-level Content-Transfer-Encoding header — Gmail's "Forward" in
// particular sends a single-part message base64-encoded, and without
// decoding it here the raw base64 text ends up as the "body".
func parseBody(contentType, transferEncoding string, body io.Reader) (string, []Attachment) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		data, _ := io.ReadAll(io.LimitReader(decodeTransfer(transferEncoding, body), 5<<20))
		return string(data), nil
	}
	boundary := params["boundary"]
	if boundary == "" {
		data, _ := io.ReadAll(io.LimitReader(decodeTransfer(transferEncoding, body), 5<<20))
		return string(data), nil
	}
	reader := multipart.NewReader(body, boundary)
	var bodyText string
	var attachments []Attachment
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		partContentType := part.Header.Get("Content-Type")
		partMediaType, partParams, _ := mime.ParseMediaType(partContentType)
		disposition := part.Header.Get("Content-Disposition")

		if strings.HasPrefix(partMediaType, "multipart/") {
			if partParams["boundary"] != "" {
				innerBody, innerAttachments := parseBody(partContentType, part.Header.Get("Content-Transfer-Encoding"), part)
				if bodyText == "" {
					bodyText = innerBody
				}
				attachments = append(attachments, innerAttachments...)
			}
			continue
		}
		data, _ := io.ReadAll(io.LimitReader(decodeTransfer(part.Header.Get("Content-Transfer-Encoding"), part), 20<<20))
		isAttachment := strings.Contains(strings.ToLower(disposition), "attachment") || part.FileName() != ""
		if !isAttachment && strings.HasPrefix(partMediaType, "text/plain") && bodyText == "" {
			bodyText = string(data)
			continue
		}
		if isAttachment && len(data) > 0 {
			filename := part.FileName()
			if filename == "" {
				filename = "attachment"
			}
			attachments = append(attachments, Attachment{Filename: filename, ContentType: partMediaType, Data: data})
		}
	}
	return bodyText, attachments
}

func decodeTransfer(encoding string, r io.Reader) io.Reader {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, r)
	case "quoted-printable":
		return quotedprintable.NewReader(r)
	default:
		return r
	}
}

// stripQuotedReply cuts off a reply email's body at the point the mail
// client starts quoting what it's replying to — Gmail (and most clients)
// prepend a header line like "<sender> 於 <date> 寫道：" or "On <date>,
// <name> wrote:" followed by the entire original message re-quoted with
// "> " prefixes. Without this, every reply's stored/displayed body is
// polluted with a verbatim copy of our own previous message.
func stripQuotedReply(body string) string {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	kept := lines[:0:0]
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ">") || looksLikeQuoteHeader(trimmed) {
			break
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func looksLikeQuoteHeader(line string) bool {
	if line == "" {
		return false
	}
	if strings.Contains(strings.ToLower(line), "wrote:") {
		return true
	}
	if strings.Contains(line, "寫道") && (strings.HasSuffix(line, ":") || strings.HasSuffix(line, "：")) {
		return true
	}
	return false
}

var messageIDPattern = regexp.MustCompile(`^<[^<>\s]+>$`)

// sanitizeMessageID only accepts a well-formed "<local@domain>" msg-id —
// this value gets stored and later echoed back verbatim as In-Reply-To on
// an outbound email, so anything that doesn't look like a real Message-ID
// (or could smuggle a header via CR/LF) is dropped rather than trusted.
func sanitizeMessageID(value string) string {
	value = strings.TrimSpace(value)
	if !messageIDPattern.MatchString(value) {
		return ""
	}
	return value
}

func decodeHeaderWord(value string) string {
	decoded, err := (&mime.WordDecoder{}).DecodeHeader(value)
	if err != nil {
		return value
	}
	return decoded
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
