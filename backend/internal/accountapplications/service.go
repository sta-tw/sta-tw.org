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
	"sta-backend/internal/emailinquiries"
	"sta-backend/internal/storage"
)

var (
	ErrInvalidInput              = errors.New("invalid input")
	ErrReviewForumUnavailable    = errors.New("account application review forum is unavailable")
	ErrReviewNotificationFailed  = errors.New("account application reviewer notification failed")
	ErrApplicantEmailUnavailable = errors.New("account application email sender is unavailable")
	ErrApplicantEmailFailed      = errors.New("account application receipt email failed")
)

type Repository interface {
	Create(ctx context.Context, username, note string, emailCiphertext, emailLookupHash []byte) (Application, error)
	AddDocument(ctx context.Context, applicationID uuid.UUID, storageKey, filename, contentType string, sizeBytes int64, sha256Hex string) (Document, error)
	// SetTelegramMessage records the notification message and static header
	// text used by the legacy Telegram card updater.
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
	// Message-ID for this application, or "" if none is on record.
	LatestInboundMessageID(ctx context.Context, applicationID uuid.UUID) (string, error)
	MarkAccountIdentityVerified(ctx context.Context, accountID uuid.UUID) error
	// ListMessages returns the full inbound/outbound correspondence on this
	// application's thread, oldest first — used to rebuild the card's
	// transcript on every edit.
	ListMessages(ctx context.Context, applicationID uuid.UUID) ([]Message, error)
}

type DocumentLink struct {
	Filename string
	URL      string
}

// DiscordForumNotifier creates the review post and appends later applicant or
// staff messages in the same way as the ordinary email inquiry forum.
type DiscordForumNotifier interface {
	CreateAccountApplicationThread(ctx context.Context, forumChannelID, subject, body string, mentionRoleIDs []string, approveCustomID, rejectCustomID string) (string, error)
	PostAccountApplicationMessage(ctx context.Context, threadID, title, author, body string, inbound bool) error
}

type discordThreadRepository interface {
	SetDiscordThread(ctx context.Context, applicationID uuid.UUID, threadID string) error
	FindByDiscordThread(ctx context.Context, threadID string) (Application, error)
}

type discordMessageRepository interface {
	AddDiscordMessage(ctx context.Context, applicationID uuid.UUID, direction, body, sourceMessageID, actor, actorDiscordID string) error
}

type ReplyTemplateResolver interface {
	ReplyTemplate(ctx context.Context, mailRouteID uuid.UUID) (value string, ok bool, err error)
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

type Service struct {
	repository       Repository
	blobStore        storage.BlobStore
	scanner          storage.Scanner
	emailCipher      *auth.FieldCipher
	lookupHMACKey    []byte
	authService      *auth.Service
	telegram         TelegramNotifier
	discordForum     DiscordForumNotifier
	forumChannelID   string
	forumRoleIDs     []string
	replyTemplates   ReplyTemplateResolver
	replyMailRouteID uuid.UUID
	mailer           email.Sender
	mailDomain       string
	logger           *slog.Logger
}

// ConfigureDiscordForum connects account applications to the account@ mail
// route's forum. The same channel is used by account@mail intake replies.
func (s *Service) ConfigureDiscordForum(notifier DiscordForumNotifier, forumChannelID string, roleIDs []string, replyMailRouteID uuid.UUID, replyTemplates ReplyTemplateResolver) {
	s.discordForum = notifier
	s.forumChannelID = strings.TrimSpace(forumChannelID)
	s.forumRoleIDs = append([]string(nil), roleIDs...)
	s.replyMailRouteID = replyMailRouteID
	s.replyTemplates = replyTemplates
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
// neither header references a known application.
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

var usernameSanitizePattern = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// SubmitFromWeb creates a pending application from the public application
// form: the only way to create an application (a cold email is an inquiry
// instead — see internal/emailinquiries).
func (s *Service) SubmitFromWeb(ctx context.Context, rawUsername, rawEmail, note string, attachments []Attachment) (Application, error) {
	if s.discordForum == nil || s.forumChannelID == "" {
		return Application{}, ErrReviewForumUnavailable
	}
	if s.mailer == nil {
		return Application{}, ErrApplicantEmailUnavailable
	}
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
	return s.createPending(ctx, username, applicantEmail, note, attachments)
}

// createPending stores the application and sends its review post to the
// account mail route's Discord forum.
func (s *Service) createPending(ctx context.Context, username, applicantEmail, note string, attachments []Attachment) (Application, error) {
	emailCiphertext, err := s.emailCipher.Seal(applicantEmail)
	if err != nil {
		return Application{}, fmt.Errorf("protect applicant email: %w", err)
	}
	lookupHash, err := auth.LookupHash(s.lookupHMACKey, applicantEmail)
	if err != nil {
		return Application{}, fmt.Errorf("hash applicant email: %w", err)
	}
	app, err := s.repository.Create(ctx, username, note, emailCiphertext, lookupHash)
	if err != nil {
		return Application{}, err
	}

	links := s.storeAttachments(ctx, app.ID, attachments)
	initialBody := note
	for _, link := range links {
		initialBody += "\n\n附件：" + link.Filename + "\n" + link.URL
	}
	if strings.TrimSpace(initialBody) != "" {
		if err := s.repository.AddMessage(ctx, app.ID, "inbound", initialBody, "", "使用者"); err != nil {
			s.logger.Error("failed to record initial account application message", "application_id", app.ID, "error", err)
		}
	}

	// Send the applicant's receipt as soon as the application has been saved.
	// Discord forum delivery is a separate notification and must not delay this
	// email or prevent it from being sent when Discord is temporarily unavailable.
	receipt := "我們已收到你的 STA 無學校信箱帳號申請（" + app.RequestedUsername + "），接下來會由工作人員人工審核。若需補充資料，直接回覆這封信即可，回覆會接回原申請；審核通過後，系統會再寄送設定密碼的帳號啟用信。"
	if err := s.mailer.Send(ctx, email.Message{
		To: applicantEmail, Subject: "STA 帳號申請已收到｜" + app.RequestedUsername,
		Text: receipt, HTML: email.InfoEmail("帳號申請已收到", []string{
			"我們已收到你的 STA 無學校信箱帳號申請（" + app.RequestedUsername + "），接下來會由工作人員人工審核。",
			"若需補充資料，直接回覆這封信即可，回覆會接回原申請；審核通過後，系統會再寄送設定密碼的帳號啟用信。",
		}), MessageID: s.messageIDFor(app.ID),
	}); err != nil {
		s.logger.Error("failed to send account application receipt email", "application_id", app.ID, "error", err)
		return app, fmt.Errorf("%w: %v", ErrApplicantEmailFailed, err)
	}
	if err := s.repository.AddMessage(ctx, app.ID, "outbound", receipt, "", "系統"); err != nil {
		s.logger.Error("failed to record account application receipt email", "application_id", app.ID, "error", err)
	}

	if s.discordForum != nil && s.forumChannelID != "" {
		body := fmt.Sprintf("申請編號：%s\n申請者信箱：%s\n希望帳號：%s\n\n申請說明：\n%s", app.ID, applicantEmail, app.RequestedUsername, note)
		for _, link := range links {
			body += "\n\n附件：" + link.Filename + "\n" + link.URL
		}
		threadID, err := s.discordForum.CreateAccountApplicationThread(
			ctx, s.forumChannelID, "無學校信箱帳號申請｜"+app.RequestedUsername, body,
			s.forumRoleIDs, AccountApplicationDecisionCustomID("approve", app.ID), AccountApplicationDecisionCustomID("reject", app.ID),
		)
		if err != nil {
			s.logger.Error("failed to notify Discord forum about account application", "application_id", app.ID, "error", err)
			return app, fmt.Errorf("%w: %v", ErrReviewNotificationFailed, err)
		}
		app.DiscordThreadID = threadID
		if repo, ok := s.repository.(discordThreadRepository); ok {
			if err := repo.SetDiscordThread(ctx, app.ID, threadID); err != nil {
				s.logger.Error("failed to record Discord thread for account application", "application_id", app.ID, "error", err)
				return app, fmt.Errorf("%w: %v", ErrReviewNotificationFailed, err)
			}
		}
	}

	if app.DiscordThreadID != "" {
		if err := s.discordForum.PostAccountApplicationMessage(ctx, app.DiscordThreadID, "✉️ 已寄出收件確認信", "系統", receipt, false); err != nil {
			s.logger.Warn("failed to post account application receipt status to Discord forum", "application_id", app.ID, "error", err)
		}
	}

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
		s.refreshTelegramCard(ctx, app)
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

	if s.discordForum != nil && app.DiscordThreadID != "" {
		if err := s.discordForum.PostAccountApplicationMessage(ctx, app.DiscordThreadID, "📨 申請者補充資料", fromAddress, storedBody, true); err != nil {
			s.logger.Warn("failed to post account application reply to Discord forum", "application_id", applicationID, "error", err)
		}
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
	showButtons := app.Status == StatusPending
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

// Approve creates an active, verified-student account for a pending
// application and emails the applicant an account-activation link.
// reviewerAccountID is optional — Discord reviewers approving a
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
	app.Status = StatusApproved
	app.CreatedAccountID = &account.ID
	if err := s.authService.SendApprovedAccountActivation(ctx, account.ID); err != nil {
		s.logger.Error("failed to send activation email after account-application approval", "application_id", applicationID, "error", err)
	} else {
		app.ActivationEmailQueued = true
	}
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

// AccountApplicationDecisionCustomID returns the signed interaction's
// application-scoped custom ID for the Discord review buttons.
func AccountApplicationDecisionCustomID(action string, applicationID uuid.UUID) string {
	return "account_application:" + action + ":" + applicationID.String()
}

// DecideByDiscord resolves the application from its forum thread before
// applying a button action, preventing a copied custom ID from acting on a
// request in a different thread.
func (s *Service) DecideByDiscord(ctx context.Context, threadID string, applicationID uuid.UUID, action, staffName, staffDiscordID string) (Application, error) {
	repo, ok := s.repository.(discordThreadRepository)
	if !ok {
		return Application{}, errors.New("account application Discord thread repository is unavailable")
	}
	app, err := s.repository.Get(ctx, applicationID)
	if err != nil {
		return Application{}, err
	}
	if app.DiscordThreadID == "" || app.DiscordThreadID != threadID {
		return Application{}, ErrNotFound
	}
	// Ensure the repository can still resolve the thread to this exact row.
	threadApp, err := repo.FindByDiscordThread(ctx, threadID)
	if err != nil || threadApp.ID != applicationID {
		if err != nil {
			return Application{}, err
		}
		return Application{}, ErrNotFound
	}
	var decided Application
	switch action {
	case "approve":
		decided, err = s.Approve(ctx, applicationID, nil)
	case "reject":
		decided, err = s.Reject(ctx, applicationID, nil)
	default:
		return Application{}, ErrInvalidInput
	}
	if err != nil {
		return Application{}, err
	}
	decisionText := "申請已通過審核。"
	if action == "approve" && !decided.ActivationEmailQueued {
		decisionText = "申請已通過審核並建立帳號，但啟用信暫時無法排入寄信佇列。"
	}
	if action == "reject" {
		decisionText = "申請未通過審核。"
	}
	if repo, ok := s.repository.(discordMessageRepository); ok {
		if err := repo.AddDiscordMessage(ctx, app.ID, "outbound", decisionText, "", staffName, staffDiscordID); err != nil {
			s.logger.Error("failed to record Discord account application decision", "application_id", app.ID, "error", err)
		}
	}
	return decided, nil
}

// ReplyByDiscord sends a staff correction request to the applicant's
// personal email and records the message in both the application transcript
// and its Discord forum thread.
func (s *Service) ReplyByDiscord(ctx context.Context, threadID, staffName, staffDiscordID, body, signature string) (Application, error) {
	body = strings.TrimSpace(body)
	signature = strings.TrimSpace(signature)
	if body == "" || signature == "" {
		return Application{}, ErrInvalidInput
	}
	repo, ok := s.repository.(discordThreadRepository)
	if !ok {
		return Application{}, errors.New("account application Discord thread repository is unavailable")
	}
	app, err := repo.FindByDiscordThread(ctx, threadID)
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
		s.logger.Warn("failed to look up latest inbound message id for account application correction", "application_id", app.ID, "error", err)
	}
	template := emailinquiries.DefaultReplyTemplate()
	if s.replyTemplates != nil && s.replyMailRouteID != uuid.Nil {
		if value, found, templateErr := s.replyTemplates.ReplyTemplate(ctx, s.replyMailRouteID); templateErr == nil && found {
			template = value
		} else if templateErr != nil {
			s.logger.Warn("failed to load account application mail reply template", "application_id", app.ID, "error", templateErr)
		}
	}
	fullBody := emailinquiries.RenderReplyBody(template, emailinquiries.ReplyTemplateVars{
		Body: body, Signature: signature, Subject: "STA 帳號申請補正", StaffName: staffName,
		SenderEmail: applicantEmail, Date: time.Now().Format("2006年01月02日"),
	})
	const subject = "Re: STA 帳號申請補正"
	if err := s.mailer.Send(ctx, email.Message{
		To: applicantEmail, Subject: subject, Text: fullBody,
		Bcc:       []string{email.SupportArchiveBCCAddress},
		MessageID: s.messageIDFor(app.ID), InReplyTo: inReplyTo,
	}); err != nil {
		return Application{}, err
	}
	if repo, ok := s.repository.(discordMessageRepository); ok {
		if err := repo.AddDiscordMessage(ctx, app.ID, "outbound", fullBody, "", staffName, staffDiscordID); err != nil {
			s.logger.Error("failed to record Discord account application reply", "application_id", app.ID, "error", err)
		}
	} else if err := s.repository.AddMessage(ctx, app.ID, "outbound", fullBody, "", staffName); err != nil {
		s.logger.Error("failed to record Discord account application reply", "application_id", app.ID, "error", err)
	}
	if s.discordForum != nil {
		if err := s.discordForum.PostAccountApplicationMessage(ctx, threadID, "✅ 已寄出補正通知", staffName, fullBody, false); err != nil {
			s.logger.Warn("failed to post account application correction to Discord forum", "application_id", app.ID, "error", err)
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
