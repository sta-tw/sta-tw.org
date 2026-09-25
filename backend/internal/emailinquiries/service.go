package emailinquiries

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
	Create(ctx context.Context, note string, emailCiphertext, emailLookupHash []byte) (Inquiry, error)
	AddDocument(ctx context.Context, inquiryID uuid.UUID, storageKey, filename, contentType string, sizeBytes int64, sha256Hex string) (Document, error)
	SetTelegramMessage(ctx context.Context, inquiryID uuid.UUID, chatID, messageID int64, headerText string) error
	Get(ctx context.Context, inquiryID uuid.UUID) (Inquiry, error)
	EmailCiphertext(ctx context.Context, inquiryID uuid.UUID) ([]byte, error)
	AddMessage(ctx context.Context, inquiryID uuid.UUID, direction, body, sourceMessageID, actor string) error
	LatestInboundMessageID(ctx context.Context, inquiryID uuid.UUID) (string, error)
	FindByTelegramMessage(ctx context.Context, chatID, messageID int64) (Inquiry, error)
	ListMessages(ctx context.Context, inquiryID uuid.UUID) ([]Message, error)
}

type DocumentLink struct {
	Filename string
	URL      string
}

type Document struct {
	ID          uuid.UUID `json:"id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	CreatedAt   time.Time `json:"created_at"`
}

// TelegramNotifier posts and updates the inquiry message in the admin group.
type TelegramNotifier interface {
	NotifyInquiry(ctx context.Context, inquiry Inquiry, email string, documents []DocumentLink) (chatID, messageID int64, headerText string, err error)
	UpdateMessage(ctx context.Context, chatID, messageID int64, text string) error
	// DeleteMessage removes a message — used to clean up a staff member's
	// own typed reply after it's been folded into the card's transcript.
	// Best-effort: the bot may lack delete rights, and a failure here must
	// never block the reply itself from going out.
	DeleteMessage(ctx context.Context, chatID, messageID int64) error
}

type Service struct {
	repository    Repository
	blobStore     storage.BlobStore
	scanner       storage.Scanner
	emailCipher   *auth.FieldCipher
	lookupHMACKey []byte
	telegram      TelegramNotifier
	mailer        email.Sender
	mailDomain    string
	logger        *slog.Logger
}

func NewService(repository Repository, blobStore storage.BlobStore, scanner storage.Scanner, emailCipher *auth.FieldCipher, lookupHMACKey []byte, telegram TelegramNotifier, mailer email.Sender, mailDomain string, logger *slog.Logger) (*Service, error) {
	if repository == nil || blobStore == nil || emailCipher == nil || len(lookupHMACKey) != 32 {
		return nil, errors.New("email-inquiries service dependencies are missing")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if mailDomain == "" {
		mailDomain = "sta-tw.org"
	}
	return &Service{
		repository: repository, blobStore: blobStore, scanner: scanner,
		emailCipher: emailCipher, lookupHMACKey: append([]byte(nil), lookupHMACKey...),
		telegram: telegram, mailer: mailer, mailDomain: mailDomain, logger: logger,
	}, nil
}

// messageIDFor is the deterministic Message-ID used for every outbound email
// tied to an inquiry, so a reply's In-Reply-To/References header can be
// matched straight back to it — see ExtractInquiryIDFromHeaders.
func (s *Service) messageIDFor(inquiryID uuid.UUID) string {
	return fmt.Sprintf("<email-inquiry-%s@%s>", inquiryID, s.mailDomain)
}

var replyMessageIDPattern = regexp.MustCompile(`email-inquiry-([0-9a-fA-F-]{36})@`)

// ExtractInquiryIDFromHeaders looks for a prior messageIDFor(...) value
// inside an inbound email's In-Reply-To/References headers. ok is false when
// neither header references a known inquiry (a brand new email, not a
// reply).
func ExtractInquiryIDFromHeaders(inReplyTo, references string) (uuid.UUID, bool) {
	for _, header := range []string{inReplyTo, references} {
		if match := replyMessageIDPattern.FindStringSubmatch(header); match != nil {
			if id, err := uuid.Parse(match[1]); err == nil {
				return id, true
			}
		}
	}
	return uuid.UUID{}, false
}

// IntakeFromEmail creates a pending inquiry from a parsed inbound email that
// isn't a reply to an existing application or inquiry thread (see
// ExtractInquiryIDFromHeaders / accountapplications.ExtractApplicationIDFromHeaders
// — the mail-intake router handles actual replies separately).
func (s *Service) IntakeFromEmail(ctx context.Context, fromAddress, subject, bodyText, sourceMessageID string, attachments []Attachment) (Inquiry, error) {
	senderEmail := auth.NormalizeEmail(fromAddress)
	if senderEmail == "" {
		return Inquiry{}, fmt.Errorf("%w: sender address is empty", ErrInvalidInput)
	}
	body := strings.TrimSpace(bodyText)
	if len(body) > 3500 {
		body = body[:3500] + "…（內容過長，已截斷）"
	}
	note := body
	if subject := strings.TrimSpace(subject); subject != "" {
		note = "主旨：" + subject + "\n\n" + body
	}

	emailCiphertext, err := s.emailCipher.Seal(senderEmail)
	if err != nil {
		return Inquiry{}, fmt.Errorf("protect sender email: %w", err)
	}
	lookupHash, err := auth.LookupHash(s.lookupHMACKey, senderEmail)
	if err != nil {
		return Inquiry{}, fmt.Errorf("hash sender email: %w", err)
	}
	inquiry, err := s.repository.Create(ctx, note, emailCiphertext, lookupHash)
	if err != nil {
		return Inquiry{}, err
	}

	links := s.storeAttachments(ctx, inquiry.ID, attachments)

	if s.telegram != nil {
		chatID, messageID, headerText, err := s.telegram.NotifyInquiry(ctx, inquiry, senderEmail, links)
		if err != nil {
			s.logger.Error("failed to notify telegram about email inquiry", "inquiry_id", inquiry.ID, "error", err)
			return inquiry, nil
		}
		inquiry.TelegramChatID = &chatID
		inquiry.TelegramMessageID = &messageID
		inquiry.TelegramHeaderText = headerText
		if err := s.repository.SetTelegramMessage(ctx, inquiry.ID, chatID, messageID, headerText); err != nil {
			s.logger.Error("failed to record telegram message for email inquiry", "inquiry_id", inquiry.ID, "error", err)
		}
		if note != "" {
			if err := s.repository.AddMessage(ctx, inquiry.ID, "inbound", note, sourceMessageID, "使用者"); err != nil {
				s.logger.Error("failed to record initial email inquiry message", "inquiry_id", inquiry.ID, "error", err)
			} else {
				s.refreshTelegramCard(ctx, inquiry)
			}
		}
	}
	return inquiry, nil
}

// storeAttachments scans, uploads and records each attachment, skipping (and
// logging) any that fail rather than aborting the whole intake.
func (s *Service) storeAttachments(ctx context.Context, inquiryID uuid.UUID, attachments []Attachment) []DocumentLink {
	var links []DocumentLink
	for _, attachment := range attachments {
		if len(attachment.Data) == 0 {
			continue
		}
		staged, err := storage.StageUpload(attachment.Filename, attachment.ContentType, bytes.NewReader(attachment.Data))
		if err != nil {
			s.logger.Warn("skipping invalid email-inquiry attachment", "inquiry_id", inquiryID, "filename", attachment.Filename, "error", err)
			continue
		}
		if err := storage.ScanStagedUpload(ctx, s.scanner, staged); err != nil {
			staged.Close()
			s.logger.Warn("skipping malware-flagged or unscannable email-inquiry attachment", "inquiry_id", inquiryID, "filename", attachment.Filename, "error", err)
			continue
		}
		storageKey := "email-inquiries/" + inquiryID.String() + "/" + uuid.NewString()
		file, err := os.Open(staged.Path)
		if err != nil {
			staged.Close()
			continue
		}
		putErr := s.blobStore.Put(ctx, storageKey, file, staged.Size, staged.ContentType)
		_ = file.Close()
		staged.Close()
		if putErr != nil {
			s.logger.Warn("failed to store email-inquiry attachment", "inquiry_id", inquiryID, "error", putErr)
			continue
		}
		if _, err := s.repository.AddDocument(ctx, inquiryID, storageKey, staged.OriginalName, staged.ContentType, staged.Size, staged.SHA256Hex); err != nil {
			s.logger.Warn("failed to record email-inquiry document", "inquiry_id", inquiryID, "error", err)
			continue
		}
		if url, err := s.blobStore.PresignGet(ctx, storageKey, 72*time.Hour); err == nil {
			links = append(links, DocumentLink{Filename: staged.OriginalName, URL: url.String()})
		}
	}
	return links
}

// HandleInboundReply records a follow-up email that matched an existing
// inquiry's deterministic Message-ID (see ExtractInquiryIDFromHeaders) and
// posts it into the same Telegram thread as a reply, instead of creating a
// second inquiry.
func (s *Service) HandleInboundReply(ctx context.Context, inquiryID uuid.UUID, fromAddress, bodyText, sourceMessageID string, attachments []Attachment) error {
	inquiry, err := s.repository.Get(ctx, inquiryID)
	if err != nil {
		return err
	}
	links := s.storeAttachments(ctx, inquiryID, attachments)
	storedBody := bodyText
	for _, link := range links {
		storedBody += "\n- " + link.Filename + "\n" + link.URL
	}
	if err := s.repository.AddMessage(ctx, inquiryID, "inbound", storedBody, sourceMessageID, "使用者"); err != nil {
		return err
	}

	if s.telegram != nil && inquiry.TelegramChatID != nil && inquiry.TelegramMessageID != nil {
		s.refreshTelegramCard(ctx, inquiry)
	}
	return nil
}

// refreshTelegramCard rebuilds the single card message (header + running
// transcript) and edits it in place. Logs and swallows errors — a Telegram
// hiccup must never fail the email/DB side of a reply.
func (s *Service) refreshTelegramCard(ctx context.Context, inquiry Inquiry) {
	if s.telegram == nil || inquiry.TelegramChatID == nil || inquiry.TelegramMessageID == nil {
		return
	}
	text, err := s.renderCard(ctx, inquiry)
	if err != nil {
		s.logger.Error("failed to render telegram card", "inquiry_id", inquiry.ID, "error", err)
		return
	}
	if err := s.telegram.UpdateMessage(ctx, *inquiry.TelegramChatID, *inquiry.TelegramMessageID, text); err != nil {
		s.logger.Error("failed to update telegram card", "inquiry_id", inquiry.ID, "error", err)
	}
}

// renderCard combines the inquiry's static header (rendered once at
// creation) with its full transcript, kept under Telegram's message length
// limit.
func (s *Service) renderCard(ctx context.Context, inquiry Inquiry) (string, error) {
	messages, err := s.repository.ListMessages(ctx, inquiry.ID)
	if err != nil {
		return "", err
	}
	text := inquiry.TelegramHeaderText
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
// an inquiry's Note, for building a natural "Re: <subject>" on a reply.
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

// ReplyByTelegram sends free text — typed by a staff member directly as a
// Telegram reply to the inquiry's notification message — back to the sender
// as a plain "Re: <subject>" email, the way an ordinary person answering an
// email would (not a branded system notification, unlike an application
// decision — see accountapplications.Service.ReplyByTelegram).
//
// staffMessageID/staffName identify the staff member's own typed message:
// after the email goes out, that raw message is deleted and its content
// folded into the card's transcript instead, labelled with staffName.
func (s *Service) ReplyByTelegram(ctx context.Context, chatID, messageID, staffMessageID int64, staffName, body string) (Inquiry, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return Inquiry{}, ErrInvalidInput
	}
	inquiry, err := s.repository.FindByTelegramMessage(ctx, chatID, messageID)
	if err != nil {
		return Inquiry{}, err
	}
	emailCiphertext, err := s.repository.EmailCiphertext(ctx, inquiry.ID)
	if err != nil {
		return Inquiry{}, err
	}
	senderEmail, err := s.emailCipher.Open(emailCiphertext)
	if err != nil {
		return Inquiry{}, fmt.Errorf("decrypt sender email: %w", err)
	}
	if s.mailer == nil {
		return Inquiry{}, errors.New("mailer is not configured")
	}
	inReplyTo, err := s.repository.LatestInboundMessageID(ctx, inquiry.ID)
	if err != nil {
		s.logger.Warn("failed to look up latest inbound message id for telegram reply", "inquiry_id", inquiry.ID, "error", err)
	}
	if err := s.mailer.Send(ctx, email.Message{
		To: senderEmail, Subject: "Re: " + originalSubject(inquiry.Note), Text: body,
		MessageID: s.messageIDFor(inquiry.ID),
		InReplyTo: inReplyTo,
	}); err != nil {
		return Inquiry{}, err
	}
	if err := s.repository.AddMessage(ctx, inquiry.ID, "outbound", body, "", staffName); err != nil {
		s.logger.Error("failed to record telegram-originated reply", "inquiry_id", inquiry.ID, "error", err)
	}
	if s.telegram != nil {
		s.refreshTelegramCard(ctx, inquiry)
		if staffMessageID != 0 {
			if err := s.telegram.DeleteMessage(ctx, chatID, staffMessageID); err != nil {
				s.logger.Warn("failed to delete staff reply message", "inquiry_id", inquiry.ID, "error", err)
			}
		}
	}
	return inquiry, nil
}

func truncateForTelegram(value string) string {
	const limit = 4000
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "\n…（過長，已截斷）"
}
