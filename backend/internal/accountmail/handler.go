// Package accountmail is the single webhook Postfix's account@ pipe
// transport calls. It holds no domain logic of its own — only routing
// between internal/accountapplications and internal/emailinquiries, which
// otherwise share nothing (see either package's doc comment).
package accountmail

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"strings"

	"sta-backend/internal/accountapplications"
	"sta-backend/internal/emailinquiries"
	"sta-backend/internal/mailintake"
)

// Handler is the one webhook Postfix's account@ pipe transport calls. It
// never holds domain logic itself — it only parses the raw message and
// routes it to whichever of accountapplications/emailinquiries owns the
// thread: a reply on an existing application, a reply on an existing
// inquiry, or (the only way a *new* row is ever created here) a brand new
// inquiry. An application is never created this way — /apply is the only
// path for that.
type Handler struct {
	token        string
	applications *accountapplications.Service
	inquiries    *emailinquiries.Service
}

func NewHandler(token string, applications *accountapplications.Service, inquiries *emailinquiries.Service) (*Handler, error) {
	if applications == nil || inquiries == nil {
		return nil, errors.New("mail-intake handler dependencies are missing")
	}
	return &Handler{token: strings.TrimSpace(token), applications: applications, inquiries: inquiries}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/internal/account-applications/mail-intake", h.intake)
}

// intake receives one raw RFC 5322 message (as delivered by Postfix's pipe
// transport for account@<mail domain>). Never fails loudly to the caller
// (Postfix) on content problems — a malformed submission just doesn't
// produce anything.
func (h *Handler) intake(w http.ResponseWriter, r *http.Request) {
	if !h.requireServiceToken(w, r) {
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
	subject := mailintake.DecodeHeaderWord(message.Header.Get("Subject"))
	bodyText, attachments := mailintake.ParseBody(message.Header.Get("Content-Type"), message.Header.Get("Content-Transfer-Encoding"), message.Body)
	bodyText = mailintake.StripQuotedReply(bodyText)
	sourceMessageID := mailintake.SanitizeMessageID(message.Header.Get("Message-ID"))
	inReplyTo := message.Header.Get("In-Reply-To")
	references := message.Header.Get("References")

	if applicationID, ok := accountapplications.ExtractApplicationIDFromHeaders(inReplyTo, references); ok {
		err := h.applications.HandleInboundReply(r.Context(), applicationID, from.Address, bodyText, sourceMessageID, attachments)
		if err == nil {
			writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "reply_recorded"}})
			return
		}
		if !errors.Is(err, accountapplications.ErrNotFound) {
			writeError(w, http.StatusBadRequest, "reply_failed", "could not record reply")
			return
		}
		// The thread may have been migrated to email_inquiries (a legacy
		// email-sourced application, before the two were split) — same id,
		// different table. Try there before giving up.
		if err := h.inquiries.HandleInboundReply(r.Context(), applicationID, from.Address, bodyText, sourceMessageID, attachments); err != nil {
			writeError(w, http.StatusBadRequest, "reply_failed", "could not record reply")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "reply_recorded"}})
		return
	}

	if inquiryID, ok := emailinquiries.ExtractInquiryIDFromHeaders(inReplyTo, references); ok {
		if err := h.inquiries.HandleInboundReply(r.Context(), inquiryID, from.Address, bodyText, sourceMessageID, attachments); err != nil {
			writeError(w, http.StatusBadRequest, "reply_failed", "could not record reply")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "reply_recorded"}})
		return
	}

	// Not a reply to anything we know about: a brand new email always
	// becomes an inquiry, never an application — /apply is the only way to
	// apply for an account.
	inquiry, err := h.inquiries.IntakeFromEmail(r.Context(), from.Address, subject, bodyText, sourceMessageID, attachments)
	if err != nil {
		writeError(w, http.StatusBadRequest, "intake_failed", "could not create email inquiry")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": inquiry})
}

func (h *Handler) requireServiceToken(w http.ResponseWriter, r *http.Request) bool {
	if h.token == "" {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "this endpoint is not configured")
		return false
	}
	provided := bearerToken(r.Header.Get("Authorization"))
	expectedHash := sha256.Sum256([]byte(h.token))
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
