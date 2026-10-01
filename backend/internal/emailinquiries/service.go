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
	Create(ctx context.Context, mailRouteID uuid.UUID, note string, emailCiphertext, emailLookupHash []byte) (Inquiry, error)
	AddDocument(ctx context.Context, inquiryID uuid.UUID, storageKey, filename, contentType string, sizeBytes int64, sha256Hex string) (Document, error)
	SetDiscordThread(ctx context.Context, inquiryID uuid.UUID, threadID string) error
	Get(ctx context.Context, inquiryID uuid.UUID) (Inquiry, error)
	EmailCiphertext(ctx context.Context, inquiryID uuid.UUID) ([]byte, error)
	AddMessage(ctx context.Context, inquiryID uuid.UUID, direction, body, sourceMessageID, actor string, meta MessageMeta) error
	LatestInboundMessageID(ctx context.Context, inquiryID uuid.UUID) (string, error)
	FindByDiscordThread(ctx context.Context, threadID string) (Inquiry, error)
	ListMessages(ctx context.Context, inquiryID uuid.UUID) ([]Message, error)
	ListByMailRoute(ctx context.Context, mailRouteID uuid.UUID) ([]Inquiry, error)
	// ReplyTemplate resolves the template for mailRouteID: that route's own
	// override if it has one, else the global admin-editable app_settings
	// row (internal/admin's /admin/settings/mail-reply-template) — ok is
	// false when neither is set, in which case the caller falls back to its
	// own default.
	ReplyTemplate(ctx context.Context, mailRouteID uuid.UUID) (value string, ok bool, err error)
}

// MessageMeta carries the tracking/security fields AddMessage records
// alongside a message but that aren't part of its visible content —
// inbound: the connecting SMTP client's IP and Postfix/OpenDKIM's
// Authentication-Results verdict (SPF/DKIM/DMARC), both pulled straight out
// of the raw message's own headers by the caller (accountmail.Handler);
// outbound: the replying staff member's Discord snowflake, an identity that
// (unlike their display name in Actor) can't be changed after the fact.
// Whichever fields don't apply to a given direction are left zero-valued.
type MessageMeta struct {
	Subject        string
	SenderIP       string
	AuthResults    string
	ActorDiscordID string
	ToAddress      string
	InReplyTo      string
	References     string
	Mailer         string
}

// InboundHeaders bundles the raw-header fields accountmail.Handler already
// parses out of an inbound email but that IntakeFromEmail/HandleInboundReply
// otherwise had no way to receive without the parameter list growing every
// time another one turned out to matter (see MessageMeta, which this maps
// directly into for AddMessage).
type InboundHeaders struct {
	SenderIP    string
	AuthResults string
	ToAddress   string
	InReplyTo   string
	References  string
	Mailer      string
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

// EmbedKind picks the Discord embed's color/title so inbound mail, outbound
// replies and system notes are visually distinguishable at a glance when
// scrolling back through a thread's history.
type EmbedKind int

const (
	EmbedKindInbound EmbedKind = iota
	EmbedKindOutbound
	EmbedKindSystem
)

// Embed is one entry in a mail thread's Discord history — every inbound
// email and outbound reply becomes one of these instead of a plain-text
// message, so a thread's whole conversation can be read back by scrolling
// through the embeds themselves.
type Embed struct {
	Kind   EmbedKind
	Author string // sender email (inbound) or staff name (outbound)
	Body   string
}

// ForumNotifier posts a new Discord forum thread per inquiry and one embed
// per follow-up (inbound reply, or a confirmation that an outbound reply
// went out) — unlike Telegram's single-message-edit-in-place card, each
// entry in the conversation is its own message, for readable context.
type ForumNotifier interface {
	// CreateThread starts a new forum post in forumChannelID named after
	// the email's subject, with embed as its first message; returns the new
	// thread's id (also used as the Discord channel id for posting further
	// embeds into it). mentionRoleIDs are @-mentioned alongside the embed
	// (new mail arriving, not a staff confirmation) so the route's
	// configured roles actually get notified — an embed alone never pings
	// anyone, Discord only rings from plain message content.
	CreateThread(ctx context.Context, forumChannelID, subject string, embed Embed, mentionRoleIDs []string) (threadID string, err error)
	PostEmbed(ctx context.Context, threadID string, embed Embed, mentionRoleIDs []string) error
}

// RouteRoles resolves a mail_routes row's visible_role_ids — the same roles
// picked in /admin/mail-routes for channel visibility double as who gets
// @-mentioned when mail arrives for that route (see ForumNotifier).
type RouteRoles interface {
	VisibleRoleIDs(ctx context.Context, mailRouteID uuid.UUID) ([]string, error)
}

type Service struct {
	repository    Repository
	blobStore     storage.BlobStore
	scanner       storage.Scanner
	emailCipher   *auth.FieldCipher
	lookupHMACKey []byte
	forum         ForumNotifier
	routeRoles    RouteRoles
	mailer        email.Sender
	mailDomain    string
	logger        *slog.Logger
}

func NewService(repository Repository, blobStore storage.BlobStore, scanner storage.Scanner, emailCipher *auth.FieldCipher, lookupHMACKey []byte, forum ForumNotifier, routeRoles RouteRoles, mailer email.Sender, mailDomain string, logger *slog.Logger) (*Service, error) {
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
		forum: forum, routeRoles: routeRoles, mailer: mailer, mailDomain: mailDomain, logger: logger,
	}, nil
}

// mentionRolesFor best-effort resolves mailRouteID's visible_role_ids for an
// inbound @-mention — nil (no mention) if routeRoles isn't configured or the
// lookup fails, never an error a caller needs to handle.
func (s *Service) mentionRolesFor(ctx context.Context, mailRouteID uuid.UUID) []string {
	if s.routeRoles == nil {
		return nil
	}
	roles, err := s.routeRoles.VisibleRoleIDs(ctx, mailRouteID)
	if err != nil {
		s.logger.Warn("failed to resolve mail route roles to mention", "mail_route_id", mailRouteID, "error", err)
		return nil
	}
	return roles
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
// — the mail-intake router handles actual replies separately). mailRouteID/
// forumChannelID identify which mailroutes.MailRoute the recipient address
// matched — resolving that row is the caller's (accountmail.Handler) job.
func (s *Service) IntakeFromEmail(ctx context.Context, mailRouteID uuid.UUID, forumChannelID, fromAddress, subject, bodyText, sourceMessageID string, headers InboundHeaders, attachments []Attachment) (Inquiry, error) {
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
	inquiry, err := s.repository.Create(ctx, mailRouteID, note, emailCiphertext, lookupHash)
	if err != nil {
		return Inquiry{}, err
	}

	links := s.storeAttachments(ctx, inquiry.ID, attachments)

	if s.forum != nil {
		threadID, err := s.forum.CreateThread(ctx, forumChannelID, subject, Embed{
			Kind:   EmbedKindInbound,
			Author: senderEmail,
			Body:   s.renderThreadBody(note, links),
		}, s.mentionRolesFor(ctx, mailRouteID))
		if err != nil {
			s.logger.Error("failed to notify discord about email inquiry", "inquiry_id", inquiry.ID, "error", err)
			return inquiry, nil
		}
		inquiry.DiscordThreadID = threadID
		if err := s.repository.SetDiscordThread(ctx, inquiry.ID, threadID); err != nil {
			s.logger.Error("failed to record discord thread for email inquiry", "inquiry_id", inquiry.ID, "error", err)
		}
		if note != "" {
			meta := MessageMeta{
				Subject: subject, SenderIP: headers.SenderIP, AuthResults: headers.AuthResults,
				ToAddress: headers.ToAddress, InReplyTo: headers.InReplyTo, References: headers.References, Mailer: headers.Mailer,
			}
			if err := s.repository.AddMessage(ctx, inquiry.ID, "inbound", note, sourceMessageID, "使用者", meta); err != nil {
				s.logger.Error("failed to record initial email inquiry message", "inquiry_id", inquiry.ID, "error", err)
			}
		}
	}
	return inquiry, nil
}

func (s *Service) renderThreadBody(note string, links []DocumentLink) string {
	var b strings.Builder
	b.WriteString(note)
	for _, link := range links {
		fmt.Fprintf(&b, "\n\n附件：%s\n%s", link.Filename, link.URL)
	}
	return b.String()
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
// posts it into the same Discord thread as a new message, instead of
// creating a second inquiry.
func (s *Service) HandleInboundReply(ctx context.Context, inquiryID uuid.UUID, fromAddress, bodyText, sourceMessageID, subject string, headers InboundHeaders, attachments []Attachment) error {
	inquiry, err := s.repository.Get(ctx, inquiryID)
	if err != nil {
		return err
	}
	links := s.storeAttachments(ctx, inquiryID, attachments)
	storedBody := bodyText
	for _, link := range links {
		storedBody += "\n- " + link.Filename + "\n" + link.URL
	}
	meta := MessageMeta{
		Subject: subject, SenderIP: headers.SenderIP, AuthResults: headers.AuthResults,
		ToAddress: headers.ToAddress, InReplyTo: headers.InReplyTo, References: headers.References, Mailer: headers.Mailer,
	}
	if err := s.repository.AddMessage(ctx, inquiryID, "inbound", storedBody, sourceMessageID, "使用者", meta); err != nil {
		return err
	}

	if s.forum != nil && inquiry.DiscordThreadID != "" {
		err := s.forum.PostEmbed(ctx, inquiry.DiscordThreadID, Embed{
			Kind:   EmbedKindInbound,
			Author: auth.NormalizeEmail(fromAddress),
			Body:   storedBody,
		}, s.mentionRolesFor(ctx, inquiry.MailRouteID))
		if err != nil {
			s.logger.Error("failed to post discord embed for email inquiry reply", "inquiry_id", inquiry.ID, "error", err)
		}
	}
	return nil
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

// ListInquiriesByRoute is the admin UI's "mail history" view for a
// mail_routes row — every inquiry filed under it, newest first, with the
// sender's email decrypted for display (an admin surface, consistent with
// how other admin views already decrypt PII for review) and each one's
// message count instead of the full transcript.
func (s *Service) ListInquiriesByRoute(ctx context.Context, mailRouteID uuid.UUID) ([]InquirySummary, error) {
	inquiries, err := s.repository.ListByMailRoute(ctx, mailRouteID)
	if err != nil {
		return nil, err
	}
	summaries := make([]InquirySummary, 0, len(inquiries))
	for _, inquiry := range inquiries {
		ciphertext, err := s.repository.EmailCiphertext(ctx, inquiry.ID)
		senderEmail := ""
		if err == nil {
			if decrypted, decryptErr := s.emailCipher.Open(ciphertext); decryptErr == nil {
				senderEmail = decrypted
			}
		}
		messages, err := s.repository.ListMessages(ctx, inquiry.ID)
		messageCount := 0
		if err == nil {
			messageCount = len(messages)
		}
		summaries = append(summaries, InquirySummary{
			ID:              inquiry.ID,
			SenderEmail:     senderEmail,
			Note:            inquiry.Note,
			DiscordThreadID: inquiry.DiscordThreadID,
			MessageCount:    messageCount,
			CreatedAt:       inquiry.CreatedAt,
		})
	}
	return summaries, nil
}

// defaultReplyTemplate mirrors admin.DefaultMailReplyTemplate (duplicated
// rather than imported to avoid a cross-package dependency for one string —
// keep the two in sync if either changes). Wraps every /re reply into a
// predictable shape — a recipient always sees the same greeting/sign-off
// shape regardless of which staff member replied. [簽名] is a separate,
// required field (see discordmail's /re command definition), not just
// appended free text, specifically so a reply can't go out without someone
// attaching their name to it. [內容]/[簽名] are the only two placeholders
// admin.setMailReplyTemplate requires be present — see ReplyTemplateVars for
// the full set a template may optionally also use.
const defaultReplyTemplate = "您好，感謝您的來信，回覆如下：\n\n[內容]\n\n如有其他問題，歡迎再次與我們聯繫。\n\n[簽名]"

// ReplyTemplateVars is every placeholder renderReplyBody substitutes into an
// admin-customized reply template. [內容]/[簽名] are mandatory (enforced by
// admin.setMailReplyTemplate and by ReplyByDiscord's own signature check);
// the rest are optional convenience — a template that doesn't reference them
// is unaffected.
type ReplyTemplateVars struct {
	Body        string // [內容] — the staff member's typed message
	Signature   string // [簽名] — the staff member's required sign-off
	Subject     string // [主旨] — the inquiry's original subject line
	StaffName   string // [客服人員] — the replying staff member's Discord display name
	SenderEmail string // [收件人] — the address this reply is going to
	Date        string // [日期] — today's date, in zh-TW long form
}

func renderReplyBody(template string, vars ReplyTemplateVars) string {
	replacer := strings.NewReplacer(
		"[內容]", vars.Body,
		"[簽名]", vars.Signature,
		"[主旨]", vars.Subject,
		"[客服人員]", vars.StaffName,
		"[收件人]", vars.SenderEmail,
		"[日期]", vars.Date,
	)
	return replacer.Replace(template)
}

// InquiryDetail is the admin UI's expanded view of one inquiry — its full
// message transcript, for "信件紀錄" (see ListInquiriesByRoute for the
// lighter-weight list view).
type InquiryDetail struct {
	SenderEmail string    `json:"sender_email"`
	Messages    []Message `json:"messages"`
}

// GetInquiryDetail decrypts the sender email and returns the full message
// transcript for one inquiry — an admin-only read, same PII-display
// justification as ListInquiriesByRoute.
func (s *Service) GetInquiryDetail(ctx context.Context, inquiryID uuid.UUID) (InquiryDetail, error) {
	ciphertext, err := s.repository.EmailCiphertext(ctx, inquiryID)
	if err != nil {
		return InquiryDetail{}, err
	}
	senderEmail, err := s.emailCipher.Open(ciphertext)
	if err != nil {
		return InquiryDetail{}, fmt.Errorf("decrypt sender email: %w", err)
	}
	messages, err := s.repository.ListMessages(ctx, inquiryID)
	if err != nil {
		return InquiryDetail{}, err
	}
	return InquiryDetail{SenderEmail: senderEmail, Messages: messages}, nil
}

// ReplyByDiscord sends free text — supplied by a staff member via the /re
// slash command inside the inquiry's Discord thread — back to the sender as
// a plain "Re: <subject>" email, the way an ordinary person answering an
// email would (not a branded system notification, unlike an application
// decision — see accountapplications.Service.ReplyByTelegram). signature is
// mandatory (enforced by the Discord command itself being a required
// option, and re-checked here) so every outbound reply is attributable to
// whoever sent it.
func (s *Service) ReplyByDiscord(ctx context.Context, threadID, staffName, staffDiscordID, body, signature string) (Inquiry, error) {
	body = strings.TrimSpace(body)
	signature = strings.TrimSpace(signature)
	if body == "" || signature == "" {
		return Inquiry{}, ErrInvalidInput
	}
	inquiry, err := s.repository.FindByDiscordThread(ctx, threadID)
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
		s.logger.Warn("failed to look up latest inbound message id for discord reply", "inquiry_id", inquiry.ID, "error", err)
	}
	template := defaultReplyTemplate
	if value, ok, err := s.repository.ReplyTemplate(ctx, inquiry.MailRouteID); err == nil && ok {
		template = value
	}
	subject := originalSubject(inquiry.Note)
	fullBody := renderReplyBody(template, ReplyTemplateVars{
		Body: body, Signature: signature, Subject: subject, StaffName: staffName,
		SenderEmail: senderEmail, Date: time.Now().Format("2006年01月02日"),
	})
	if err := s.mailer.Send(ctx, email.Message{
		To: senderEmail, Subject: "Re: " + subject, Text: fullBody,
		MessageID: s.messageIDFor(inquiry.ID),
		InReplyTo: inReplyTo,
	}); err != nil {
		return Inquiry{}, err
	}
	if err := s.repository.AddMessage(ctx, inquiry.ID, "outbound", fullBody, "", staffName, MessageMeta{ActorDiscordID: staffDiscordID, ToAddress: senderEmail}); err != nil {
		s.logger.Error("failed to record discord-originated reply", "inquiry_id", inquiry.ID, "error", err)
	}
	if s.forum != nil {
		err := s.forum.PostEmbed(ctx, threadID, Embed{Kind: EmbedKindOutbound, Author: staffName, Body: fullBody}, nil)
		if err != nil {
			s.logger.Warn("failed to post discord reply confirmation", "inquiry_id", inquiry.ID, "error", err)
		}
	}
	return inquiry, nil
}
