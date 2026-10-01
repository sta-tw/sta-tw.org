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
	"regexp"
	"strings"

	"sta-backend/internal/accountapplications"
	"sta-backend/internal/emailinquiries"
	"sta-backend/internal/mailintake"
	"sta-backend/internal/mailroutes"
)

// Handler is the one webhook Postfix's pipe transport calls for every
// address registered in internal/mailroutes. It never holds domain logic
// itself — it only parses the raw message and routes it to whichever of
// accountapplications/emailinquiries owns the thread: a reply on an
// existing application, a reply on an existing inquiry, or (the only way a
// *new* row is ever created here) a brand new inquiry tagged with whichever
// mail_routes row the envelope recipient matched. An application is never
// created this way — /apply is the only path for that.
type Handler struct {
	token        string
	applications *accountapplications.Service
	inquiries    *emailinquiries.Service
	mailRoutes   mailroutes.Repository
}

func NewHandler(token string, applications *accountapplications.Service, inquiries *emailinquiries.Service, mailRoutesRepo mailroutes.Repository) (*Handler, error) {
	if applications == nil || inquiries == nil || mailRoutesRepo == nil {
		return nil, errors.New("mail-intake handler dependencies are missing")
	}
	return &Handler{token: strings.TrimSpace(token), applications: applications, inquiries: inquiries, mailRoutes: mailRoutesRepo}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/internal/mail-intake", h.intake)
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
	headers := emailinquiries.InboundHeaders{
		SenderIP:    senderIPFromReceived(message.Header.Get("Received")),
		AuthResults: strings.TrimSpace(message.Header.Get("Authentication-Results")),
		ToAddress:   mailintake.DecodeHeaderWord(message.Header.Get("To")),
		InReplyTo:   strings.TrimSpace(inReplyTo),
		References:  strings.TrimSpace(references),
		// X-Mailer/User-Agent are self-reported by the sending client — not
		// a security guarantee, just a triage signal (see InboundHeaders'
		// doc comment). Whichever is present; a legitimate MUA sets one or
		// the other, rarely both.
		Mailer: firstNonEmpty(message.Header.Get("X-Mailer"), message.Header.Get("User-Agent")),
	}

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
		if err := h.inquiries.HandleInboundReply(r.Context(), applicationID, from.Address, bodyText, sourceMessageID, subject, headers, attachments); err != nil {
			writeError(w, http.StatusBadRequest, "reply_failed", "could not record reply")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "reply_recorded"}})
		return
	}

	if inquiryID, ok := emailinquiries.ExtractInquiryIDFromHeaders(inReplyTo, references); ok {
		if err := h.inquiries.HandleInboundReply(r.Context(), inquiryID, from.Address, bodyText, sourceMessageID, subject, headers, attachments); err != nil {
			writeError(w, http.StatusBadRequest, "reply_failed", "could not record reply")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "reply_recorded"}})
		return
	}

	// Not a reply to anything we know about: a brand new email always
	// becomes an inquiry, never an application — /apply is the only way to
	// apply for an account.
	recipientLocalPart := localPart(r.Header.Get("X-STA-Recipient"))
	if recipientLocalPart == "" {
		writeError(w, http.StatusBadRequest, "invalid_recipient", "missing recipient local part")
		return
	}
	route, err := h.mailRoutes.GetByLocalPart(r.Context(), recipientLocalPart)
	if err != nil {
		// Postfix's own pgsql lookup map already guaranteed this local part
		// exists at SMTP time — a mismatch here (the route was deleted in
		// the few seconds since) isn't something the sender should see as a
		// failure, so just drop it silently.
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "dropped_unknown_route"}})
		return
	}
	inquiry, err := h.inquiries.IntakeFromEmail(r.Context(), route.ID, route.DiscordForumChannelID, from.Address, subject, bodyText, sourceMessageID, headers, attachments)
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

// receivedIPPattern pulls the bracketed IP address out of our own Postfix's
// first (topmost) "Received:" header — the hop where postfix accepted the
// connection directly from the sending server, e.g. "from mail.example.com
// (mail.example.com [203.0.113.7])" or "(HELO x) [2001:db8::1]". This is the
// one IP in the whole header chain that isn't self-reported by the sender.
var receivedIPPattern = regexp.MustCompile(`\[([0-9a-fA-F:.]+)\]`)

// senderIPFromReceived extracts the connecting client's address from a raw
// "Received:" header value. net/mail.Header.Get only ever returns the first
// occurrence for a repeated header, which for "Received" is exactly the one
// we want (headers are prepended, so the first in the raw message is the
// most recent hop — our own Postfix). Returns "" if the header is missing or
// doesn't contain a bracketed address (e.g. a malformed or hand-crafted one).
func senderIPFromReceived(received string) string {
	match := receivedIPPattern.FindStringSubmatch(received)
	if match == nil {
		return ""
	}
	return match[1]
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// localPart extracts the part before "@" from a bare address (or an
// address:port-free header value like "brochure@mail.sta-tw.org"),
// lower-cased. Postfix's pipe transport argv passes the raw envelope
// recipient, which may include the domain — only the local part is a
// mailroutes.MailRoute key.
func localPart(address string) string {
	address = strings.TrimSpace(address)
	if at := strings.IndexByte(address, '@'); at >= 0 {
		address = address[:at]
	}
	return strings.ToLower(address)
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
