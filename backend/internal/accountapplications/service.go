package accountapplications

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/auth"
	"sta-backend/internal/email"
	"sta-backend/internal/storage"
)

var ErrInvalidInput = errors.New("invalid input")

type Repository interface {
	Create(ctx context.Context, username, source, note string, emailCiphertext, emailLookupHash []byte) (Application, error)
	AddDocument(ctx context.Context, applicationID uuid.UUID, storageKey, filename, contentType string, sizeBytes int64, sha256Hex string) (Document, error)
	// SetTelegramMessage records the one message this application's whole
	// Telegram thread lives in (root, kept continuously edited — see
	// HandleInboundReply/ReplyByTelegram), and the static header text an
	// edit rebuilds from.
	SetTelegramMessage(ctx context.Context, applicationID uuid.UUID, chatID, messageID int64, headerText string) error
	Get(ctx context.Context, applicationID uuid.UUID) (Application, error)
	EmailCiphertext(ctx context.Context, applicationID uuid.UUID) ([]byte, error)
	ListDocuments(ctx context.Context, applicationID uuid.UUID) ([]Document, error)
	ListDocumentStorageKeys(ctx context.Context, applicationID uuid.UUID) ([]DocumentKey, error)
	Decide(ctx context.Context, applicationID uuid.UUID, status Status, reviewerAccountID *uuid.UUID, createdAccountID *uuid.UUID) error
	// AddMessage records one message on the thread. sourceMessageID is the
	// RFC 5322 Message-ID of the inbound email this row came from (empty
	// for a direction="outbound" row, which has no such thing) — later
	// used to set In-Reply-To on our own replies so they thread correctly
	// in the recipient's mail client. actor is who said it (see Message).
	AddMessage(ctx context.Context, applicationID uuid.UUID, direction, body, sourceMessageID, actor string) error
	// LatestInboundMessageID returns the most recent inbound message's
	// Message-ID for this application, or "" if none is on record (e.g. a
	// web-form application with no email in the thread yet).
	LatestInboundMessageID(ctx context.Context, applicationID uuid.UUID) (string, error)
	MarkAccountIdentityVerified(ctx context.Context, accountID uuid.UUID) error
	// FindByTelegramMessage resolves a staff reply typed directly in
	// Telegram (as a reply to the one card message — see
	// SetTelegramMessage) back to the application it belongs to.
	FindByTelegramMessage(ctx context.Context, chatID, messageID int64) (Application, error)
	// ListMessages returns the full inbound/outbound correspondence on this
	// application's thread, oldest first — used to rebuild the card's
	// transcript on every edit.
	ListMessages(ctx context.Context, applicationID uuid.UUID) ([]Message, error)
}

type DocumentLink struct {
	Filename string
	URL      string
}

// TelegramNotifier posts and updates the review message in the admin group.
// Kept as a small interface here (rather than importing internal/telegrambot)
// because cmd/api talks to the Telegram Bot HTTP API directly — it isn't the
// same process as the long-polling cmd/telegram-bot that receives the button
// presses.
type TelegramNotifier interface {
	// NotifyPendingApplication posts the one card for this application's
	// whole thread. headerText is the static part it rendered, returned so
	// the caller can persist it (SetTelegramMessage) for later edits.
	NotifyPendingApplication(ctx context.Context, app Application, email string, documents []DocumentLink) (chatID, messageID int64, headerText string, err error)
	// UpdateMessage edits the card in place. showDecisionButtons re-attaches
	// the ✅/❌ keyboard for a still-pending application — Telegram's
	// editMessageText silently *removes* any existing inline keyboard
	// unless the same (or a new) one is resent with the edit, so every
	// caller must pass this explicitly rather than assuming the buttons
	// survive on their own.
	UpdateMessage(ctx context.Context, chatID, messageID int64, text string, applicationID uuid.UUID, showDecisionButtons bool) error
	// DeleteMessage removes a message — used to clean up a staff member's
	// own typed reply after it's been folded into the card's transcript.
	// Best-effort: the bot may lack delete rights in the group, and a
	// failure here must never block the reply itself from going out.
	DeleteMessage(ctx context.Context, chatID, messageID int64) error
}

// Attachment is one file pulled out of an inbound application email.
type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

type Service struct {
	repository    Repository
	blobStore     storage.BlobStore
	scanner       storage.Scanner
	emailCipher   *auth.FieldCipher
	lookupHMACKey []byte
	authService   *auth.Service
	telegram      TelegramNotifier
	mailer        email.Sender
	mailDomain    string
	logger        *slog.Logger
}

func NewService(repository Repository, blobStore storage.BlobStore, scanner storage.Scanner, emailCipher *auth.FieldCipher, lookupHMACKey []byte, authService *auth.Service, telegram TelegramNotifier, mailer email.Sender, mailDomain string, logger *slog.Logger) (*Service, error) {
	if repository == nil || blobStore == nil || emailCipher == nil || len(lookupHMACKey) != 32 || authService == nil {
		return nil, errors.New("account-applications service dependencies are missing")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if mailDomain == "" {
		mailDomain = "sta-tw.org"
	}
	return &Service{
		repository: repository, blobStore: blobStore, scanner: scanner,
		emailCipher: emailCipher, lookupHMACKey: append([]byte(nil), lookupHMACKey...), authService: authService,
		telegram: telegram, mailer: mailer, mailDomain: mailDomain, logger: logger,
	}, nil
}

// messageIDFor is the deterministic Message-ID used for every outbound email
// tied to an application, so a reply's In-Reply-To/References header can be
// matched straight back to it — see ExtractApplicationIDFromHeaders.
func (s *Service) messageIDFor(applicationID uuid.UUID) string {
	return fmt.Sprintf("<account-application-%s@%s>", applicationID, s.mailDomain)
}

var replyMessageIDPattern = regexp.MustCompile(`account-application-([0-9a-fA-F-]{36})@`)

// ExtractApplicationIDFromHeaders looks for a prior messageIDFor(...) value
// inside an inbound email's In-Reply-To/References headers. ok is false when
// neither header references a known application (a brand new application,
// not a reply).
func ExtractApplicationIDFromHeaders(inReplyTo, references string) (uuid.UUID, bool) {
	for _, header := range []string{inReplyTo, references} {
		if match := replyMessageIDPattern.FindStringSubmatch(header); match != nil {
			if id, err := uuid.Parse(match[1]); err == nil {
				return id, true
			}
		}
	}
	return uuid.UUID{}, false
}

var usernameLinePattern = regexp.MustCompile(`(?im)^\s*(?:帳號名稱|帳號|username)\s*[:：]\s*(.+?)\s*$`)
var usernameSanitizePattern = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// IntakeFromEmail creates a pending application from a parsed inbound email
// that isn't a reply to an existing one (see ExtractApplicationIDFromHeaders
// — the handler routes actual replies to HandleInboundReply instead): the
// applicant's own address (used as their contact email and to derive a
// fallback username), the message body (scanned for an explicit "帳號名稱:
// ..." line), and any attachments as proof documents. Kept as a fallback for
// anyone who emails in without using the web form. sourceMessageID is the
// email's own Message-ID header, recorded so a later reply from staff can
// thread under it (see ReplyByTelegram).
func (s *Service) IntakeFromEmail(ctx context.Context, fromAddress, subject, bodyText, sourceMessageID string, attachments []Attachment) (Application, error) {
	applicantEmail := auth.NormalizeEmail(fromAddress)
	if applicantEmail == "" {
		return Application{}, fmt.Errorf("%w: sender address is empty", ErrInvalidInput)
	}
	username := extractUsername(bodyText, applicantEmail)
	// Unlike SubmitFromWeb's note (the applicant's own typed explanation),
	// this is the raw email body — reviewers need to actually read what was
	// sent, not just the subject line.
	body := strings.TrimSpace(bodyText)
	if len(body) > 3500 {
		body = body[:3500] + "…（內容過長，已截斷）"
	}
	note := body
	if subject := strings.TrimSpace(subject); subject != "" {
		note = "主旨：" + subject + "\n\n" + body
	}
	return s.createPending(ctx, "email", username, applicantEmail, note, sourceMessageID, attachments)
}

// SubmitFromWeb creates a pending application from the public application
// form: the applicant chose their own username and typed their own note, so
// neither needs the email-parsing heuristics IntakeFromEmail uses.
func (s *Service) SubmitFromWeb(ctx context.Context, rawUsername, rawEmail, note string, attachments []Attachment) (Application, error) {
	username := usernameSanitizePattern.ReplaceAllString(strings.ToLower(strings.TrimSpace(rawUsername)), "")
	if len(username) < 3 {
		return Application{}, fmt.Errorf("%w: username is too short", ErrInvalidInput)
	}
	applicantEmail := auth.NormalizeEmail(rawEmail)
	if applicantEmail == "" {
		return Application{}, fmt.Errorf("%w: email is invalid", ErrInvalidInput)
	}
	note = strings.TrimSpace(note)
	if len(note) > 2000 {
		note = note[:2000]
	}
	return s.createPending(ctx, "web", username, applicantEmail, note, "", attachments)
}

// createPending creates the application row, posts the Telegram card (a
// minimal header — see renderCard), and then stores/renders note as the
// thread's first turn. Content lives in exactly one place (the transcript),
// never baked into the header text itself, so a later edit never shows it
// twice. sourceMessageID is the originating email's Message-ID, empty for a
// web-form submission (SubmitFromWeb).
func (s *Service) createPending(ctx context.Context, source, username, applicantEmail, note, sourceMessageID string, attachments []Attachment) (Application, error) {
	emailCiphertext, err := s.emailCipher.Seal(applicantEmail)
	if err != nil {
		return Application{}, fmt.Errorf("protect applicant email: %w", err)
	}
	lookupHash, err := auth.LookupHash(s.lookupHMACKey, applicantEmail)
	if err != nil {
		return Application{}, fmt.Errorf("hash applicant email: %w", err)
	}
	app, err := s.repository.Create(ctx, username, source, note, emailCiphertext, lookupHash)
	if err != nil {
		return Application{}, err
	}

	links := s.storeAttachments(ctx, app.ID, attachments)

	if s.telegram != nil {
		chatID, messageID, headerText, err := s.telegram.NotifyPendingApplication(ctx, app, applicantEmail, links)
		if err != nil {
			s.logger.Error("failed to notify telegram about account application", "application_id", app.ID, "error", err)
			return app, nil
		}
		app.TelegramChatID = &chatID
		app.TelegramMessageID = &messageID
		app.TelegramHeaderText = headerText
		if err := s.repository.SetTelegramMessage(ctx, app.ID, chatID, messageID, headerText); err != nil {
			s.logger.Error("failed to record telegram message for account application", "application_id", app.ID, "error", err)
		}
		if note != "" {
			if err := s.repository.AddMessage(ctx, app.ID, "inbound", note, sourceMessageID, "使用者"); err != nil {
				s.logger.Error("failed to record initial account application message", "application_id", app.ID, "error", err)
			} else {
				s.refreshTelegramCard(ctx, app)
			}
		}
	}
	return app, nil
}

// storeAttachments scans, uploads and records each attachment, skipping (and
// logging) any that fail rather than aborting the whole submission.
func (s *Service) storeAttachments(ctx context.Context, applicationID uuid.UUID, attachments []Attachment) []DocumentLink {
	var links []DocumentLink
	for _, attachment := range attachments {
		if len(attachment.Data) == 0 {
			continue
		}
		staged, err := storage.StageUpload(attachment.Filename, attachment.ContentType, bytes.NewReader(attachment.Data))
		if err != nil {
			s.logger.Warn("skipping invalid account-application attachment", "application_id", applicationID, "filename", attachment.Filename, "error", err)
			continue
		}
		if err := storage.ScanStagedUpload(ctx, s.scanner, staged); err != nil {
			staged.Close()
			s.logger.Warn("skipping malware-flagged or unscannable account-application attachment", "application_id", applicationID, "filename", attachment.Filename, "error", err)
			continue
		}
		storageKey := "account-applications/" + applicationID.String() + "/" + uuid.NewString()
		file, err := os.Open(staged.Path)
		if err != nil {
			staged.Close()
			continue
		}
		putErr := s.blobStore.Put(ctx, storageKey, file, staged.Size, staged.ContentType)
		_ = file.Close()
		staged.Close()
		if putErr != nil {
			s.logger.Warn("failed to store account-application attachment", "application_id", applicationID, "error", putErr)
			continue
		}
		if _, err := s.repository.AddDocument(ctx, applicationID, storageKey, staged.OriginalName, staged.ContentType, staged.Size, staged.SHA256Hex); err != nil {
			s.logger.Warn("failed to record account-application document", "application_id", applicationID, "error", err)
			continue
		}
		if url, err := s.blobStore.PresignGet(ctx, storageKey, 72*time.Hour); err == nil {
			links = append(links, DocumentLink{Filename: staged.OriginalName, URL: url.String()})
		}
	}
	return links
}

// HandleInboundReply records a follow-up email that matched an existing
// application's deterministic Message-ID (see ExtractApplicationIDFromHeaders)
// and posts it into the same Telegram thread as a reply, instead of creating
// a second application.
func (s *Service) HandleInboundReply(ctx context.Context, applicationID uuid.UUID, fromAddress, bodyText, sourceMessageID string, attachments []Attachment) error {
	app, err := s.repository.Get(ctx, applicationID)
	if err != nil {
		return err
	}
	links := s.storeAttachments(ctx, applicationID, attachments)
	storedBody := bodyText
	for _, link := range links {
		storedBody += "\n- " + link.Filename + "\n" + link.URL
	}
	if err := s.repository.AddMessage(ctx, applicationID, "inbound", storedBody, sourceMessageID, "使用者"); err != nil {
		return err
	}

	if s.telegram != nil && app.TelegramChatID != nil && app.TelegramMessageID != nil {
		s.refreshTelegramCard(ctx, app)
	}
	return nil
}

// refreshTelegramCard rebuilds the single card message (header + running
// transcript) and edits it in place. Logs and swallows errors — a Telegram
// hiccup must never fail the email/DB side of a reply.
func (s *Service) refreshTelegramCard(ctx context.Context, app Application) {
	if s.telegram == nil || app.TelegramChatID == nil || app.TelegramMessageID == nil {
		return
	}
	text, err := s.renderCard(ctx, app)
	if err != nil {
		s.logger.Error("failed to render telegram card", "application_id", app.ID, "error", err)
		return
	}
	// An inquiry email never had decision buttons; an application still
	// awaiting a decision should keep them through this edit.
	showButtons := app.Source != "email" && app.Status == StatusPending
	if err := s.telegram.UpdateMessage(ctx, *app.TelegramChatID, *app.TelegramMessageID, text, app.ID, showButtons); err != nil {
		s.logger.Error("failed to update telegram card", "application_id", app.ID, "error", err)
	}
}

// renderCard combines the application's static header (rendered once at
// creation) with its full transcript, kept under Telegram's message length
// limit.
func (s *Service) renderCard(ctx context.Context, app Application) (string, error) {
	messages, err := s.repository.ListMessages(ctx, app.ID)
	if err != nil {
		return "", err
	}
	text := app.TelegramHeaderText
	if len(messages) > 0 {
		var transcript strings.Builder
		transcript.WriteString("\n\n── 對話紀錄 ──")
		for _, m := range messages {
			actor := m.Actor
			if actor == "" {
				if m.Direction == "inbound" {
					actor = "使用者"
				} else {
					actor = "系統"
				}
			}
			fmt.Fprintf(&transcript, "\n\n[%s %s]\n%s", actor, m.CreatedAt.Local().Format("01/02 15:04"), m.Body)
		}
		text += transcript.String()
	}
	return truncateForTelegram(text), nil
}

// originalSubject pulls the "主旨：..." line IntakeFromEmail prefixes onto
// an inquiry's Note (see there), for building a natural "Re: <subject>" on
// a reply. Falls back to a generic subject when there wasn't one (a bare
// email with no Subject header, or a web-form application's Note, which
// isn't prefixed this way).
func originalSubject(note string) string {
	const prefix = "主旨："
	if !strings.HasPrefix(note, prefix) {
		return "你的來信"
	}
	rest := note[len(prefix):]
	if newline := strings.IndexByte(rest, '\n'); newline >= 0 {
		rest = rest[:newline]
	}
	subject := strings.TrimSpace(rest)
	if subject == "" {
		return "你的來信"
	}
	return subject
}

func extractUsername(bodyText, fallbackEmail string) string {
	if match := usernameLinePattern.FindStringSubmatch(bodyText); match != nil {
		cleaned := usernameSanitizePattern.ReplaceAllString(strings.TrimSpace(match[1]), "")
		if len(cleaned) >= 3 {
			return strings.ToLower(cleaned)
		}
	}
	at := strings.IndexByte(fallbackEmail, '@')
	local := fallbackEmail
	if at > 0 {
		local = fallbackEmail[:at]
	}
	cleaned := usernameSanitizePattern.ReplaceAllString(local, "")
	if len(cleaned) < 3 {
		cleaned = cleaned + "applicant"
	}
	return strings.ToLower(cleaned)
}

// Approve creates an active, verified-student account for a pending
// application and emails the applicant a password-set link.
// reviewerAccountID is optional — Telegram group members approving a
// request don't necessarily have an STA account. People without a school
// email never go through Register (it requires *.edu.tw), so this is the
// only way they get an account at all — not a supplement to one that
// already exists.
func (s *Service) Approve(ctx context.Context, applicationID uuid.UUID, reviewerAccountID *uuid.UUID) (Application, error) {
	app, err := s.repository.Get(ctx, applicationID)
	if err != nil {
		return Application{}, err
	}
	if app.Status != StatusPending {
		return Application{}, ErrAlreadyDecided
	}
	emailCiphertext, err := s.repository.EmailCiphertext(ctx, applicationID)
	if err != nil {
		return Application{}, err
	}
	applicantEmail, err := s.emailCipher.Open(emailCiphertext)
	if err != nil {
		return Application{}, fmt.Errorf("decrypt applicant email: %w", err)
	}

	username := app.RequestedUsername
	var account auth.Account
	for attempt := 0; attempt < 5; attempt++ {
		account, err = s.authService.CreateApprovedAccount(ctx, username, applicantEmail)
		if err == nil {
			break
		}
		if errors.Is(err, auth.ErrConflict) {
			username = fmt.Sprintf("%s-%s", app.RequestedUsername, uuid.NewString()[:6])
			continue
		}
		return Application{}, err
	}
	if err != nil {
		return Application{}, err
	}
	if err := s.repository.MarkAccountIdentityVerified(ctx, account.ID); err != nil {
		return Application{}, err
	}

	if err := s.repository.Decide(ctx, applicationID, StatusApproved, reviewerAccountID, &account.ID); err != nil {
		return Application{}, err
	}
	if err := s.authService.RequestPasswordReset(ctx, applicantEmail, nil); err != nil {
		s.logger.Error("failed to send password-set email after account-application approval", "application_id", applicationID, "error", err)
	}
	app.Status = StatusApproved
	app.CreatedAccountID = &account.ID
	return app, nil
}

func (s *Service) Reject(ctx context.Context, applicationID uuid.UUID, reviewerAccountID *uuid.UUID) (Application, error) {
	app, err := s.repository.Get(ctx, applicationID)
	if err != nil {
		return Application{}, err
	}
	if app.Status != StatusPending {
		return Application{}, ErrAlreadyDecided
	}
	emailCiphertext, err := s.repository.EmailCiphertext(ctx, applicationID)
	if err != nil {
		return Application{}, err
	}
	if err := s.repository.Decide(ctx, applicationID, StatusRejected, reviewerAccountID, nil); err != nil {
		return Application{}, err
	}
	app.Status = StatusRejected
	if s.mailer != nil {
		if applicantEmail, err := s.emailCipher.Open(emailCiphertext); err == nil {
			inReplyTo, err := s.repository.LatestInboundMessageID(ctx, applicationID)
			if err != nil {
				s.logger.Warn("failed to look up latest inbound message id for rejection email", "application_id", applicationID, "error", err)
			}
			body := "很抱歉，你的 STA 帳號申請（" + app.RequestedUsername + "）未通過審核。\n如果你認為這是誤判，或想補充資料，直接回覆這封信就可以，我們會收到。"
			sendErr := s.mailer.Send(ctx, email.Message{
				To: applicantEmail, Subject: "STA 帳號申請審核結果", Text: body,
				HTML: email.InfoEmail("帳號申請審核結果", []string{
					"很抱歉，你的 STA 帳號申請（" + app.RequestedUsername + "）未通過審核。",
					"如果你認為這是誤判，或想補充資料，直接回覆這封信就可以，我們會收到。",
				}),
				MessageID: s.messageIDFor(applicationID),
				InReplyTo: inReplyTo,
			})
			if sendErr != nil {
				s.logger.Error("failed to send account-application rejection email", "application_id", applicationID, "error", sendErr)
			} else if err := s.repository.AddMessage(ctx, applicationID, "outbound", body, "", "系統"); err != nil {
				s.logger.Error("failed to record account-application rejection message", "application_id", applicationID, "error", err)
			}
		}
	}
	return app, nil
}

// ReplyByTelegram sends free text — typed by a staff member directly as a
// Telegram reply to one of our notification messages — back to the
// applicant as an email on the same thread. This is the only way to answer
// an inquiry (no approve/reject buttons apply there) and also works for a
// pending application without needing a decision first.
//
// staffMessageID/staffName identify the staff member's own typed message
// (the one Telegram is already showing in the chat): after the email goes
// out, that raw message is deleted and its content folded into the card's
// transcript instead, labelled with staffName — avoids the chat filling up
// with one message per turn, and keeps "who replied" visible to staff only
// (the outbound email is always from account@, regardless of who typed it).
func (s *Service) ReplyByTelegram(ctx context.Context, chatID, messageID, staffMessageID int64, staffName, body string) (Application, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return Application{}, ErrInvalidInput
	}
	app, err := s.repository.FindByTelegramMessage(ctx, chatID, messageID)
	if err != nil {
		return Application{}, err
	}
	emailCiphertext, err := s.repository.EmailCiphertext(ctx, app.ID)
	if err != nil {
		return Application{}, err
	}
	applicantEmail, err := s.emailCipher.Open(emailCiphertext)
	if err != nil {
		return Application{}, fmt.Errorf("decrypt applicant email: %w", err)
	}
	if s.mailer == nil {
		return Application{}, errors.New("mailer is not configured")
	}
	inReplyTo, err := s.repository.LatestInboundMessageID(ctx, app.ID)
	if err != nil {
		s.logger.Warn("failed to look up latest inbound message id for telegram reply", "application_id", app.ID, "error", err)
	}
	// An emailed-in thread is an inquiry, not an application — the reply
	// should read like an ordinary person answering an email, not a
	// branded system notification. No InfoEmail box, plain "Re: <subject>".
	// Application threads (source != "email") keep the templated look,
	// since that's still a formal review response.
	if app.Source == "email" {
		err = s.mailer.Send(ctx, email.Message{
			To: applicantEmail, Subject: "Re: " + originalSubject(app.Note), Text: body,
			MessageID: s.messageIDFor(app.ID),
			InReplyTo: inReplyTo,
		})
	} else {
		const subject = "STA 帳號申請信箱回覆"
		err = s.mailer.Send(ctx, email.Message{
			To: applicantEmail, Subject: subject, Text: body,
			HTML:      email.InfoEmail(subject, []string{body}),
			MessageID: s.messageIDFor(app.ID),
			InReplyTo: inReplyTo,
		})
	}
	if err != nil {
		return Application{}, err
	}
	if err := s.repository.AddMessage(ctx, app.ID, "outbound", body, "", staffName); err != nil {
		s.logger.Error("failed to record telegram-originated reply", "application_id", app.ID, "error", err)
	}
	if s.telegram != nil {
		s.refreshTelegramCard(ctx, app)
		if staffMessageID != 0 {
			if err := s.telegram.DeleteMessage(ctx, chatID, staffMessageID); err != nil {
				// Non-fatal: the bot may not have delete rights in this
				// group. The reply already went out either way.
				s.logger.Warn("failed to delete staff reply message", "application_id", app.ID, "error", err)
			}
		}
	}
	return app, nil
}

func truncateForTelegram(value string) string {
	const limit = 4000
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "\n…（過長，已截斷）"
}
