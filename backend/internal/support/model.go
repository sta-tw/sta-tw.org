package support

import (
	"context"
	"errors"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	MaxSubjectLength              = 200
	MaxMessageLength              = 8000
	MaxAttachmentCount            = 5
	MaxAttachmentFileBytes  int64 = 10 << 20
	MaxAttachmentTotalBytes int64 = 25 << 20
)

var (
	ErrInvalidInput       = errors.New("support input is invalid")
	ErrNotFound           = errors.New("support ticket not found")
	ErrForbidden          = errors.New("support access forbidden")
	ErrConflict           = errors.New("support ticket conflict")
	ErrAdminRequired      = errors.New("administrator permission is required")
	ErrInvalidStatus      = errors.New("support ticket status is invalid")
	ErrInvalidDiscordData = errors.New("discord support data is invalid")
)

type TicketStatus string

const (
	StatusOpen         TicketStatus = "open"
	StatusWaitingStaff TicketStatus = "waiting_staff"
	StatusWaitingUser  TicketStatus = "waiting_user"
	StatusClosed       TicketStatus = "closed"
	StatusSpam         TicketStatus = "spam"
)

type Category string

const (
	CategoryAccount         Category = "account"
	CategoryAdmissions      Category = "admissions"
	CategoryBrochure        Category = "brochure"
	CategoryResults         Category = "results"
	CategoryCandidateNumber Category = "candidate_number"
	CategoryWillingness     Category = "willingness"
	CategoryTechnical       Category = "technical"
	CategoryOther           Category = "other"
)

type SourcePlatform string

const (
	SourceWebsite SourcePlatform = "website"
	SourceDiscord SourcePlatform = "discord"
	SourceEmail   SourcePlatform = "email"
	SourceSystem  SourcePlatform = "system"
)

type Operation string

const (
	OperationCreate Operation = "create"
	OperationEdit   Operation = "edit"
	OperationDelete Operation = "delete"
)

type Ticket struct {
	ID                uuid.UUID  `json:"id"`
	TicketNumber      string     `json:"ticket_number"`
	Category          string     `json:"category"`
	Subject           string     `json:"subject"`
	Status            string     `json:"status"`
	AssignedTo        *uuid.UUID `json:"assigned_to,omitempty"`
	DiscordSyncStatus string     `json:"discord_sync_status"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	ClosedAt          *time.Time `json:"closed_at,omitempty"`
	Requester         *Requester `json:"requester,omitempty"`
}

type Requester struct {
	AccountID *uuid.UUID `json:"account_id,omitempty"`
	Username  string     `json:"username,omitempty"`
	Email     string     `json:"email,omitempty"`
}

type Message struct {
	ID             uuid.UUID    `json:"id"`
	AuthorType     string       `json:"author_type"`
	SourcePlatform string       `json:"source_platform"`
	Body           string       `json:"body"`
	CreatedAt      time.Time    `json:"created_at"`
	EditedAt       *time.Time   `json:"edited_at,omitempty"`
	Status         string       `json:"status"`
	Attachments    []Attachment `json:"attachments,omitempty"`
}

type Attachment struct {
	ID            uuid.UUID `json:"id"`
	TicketID      uuid.UUID `json:"ticket_id"`
	MessageID     uuid.UUID `json:"message_id"`
	OriginalName  string    `json:"original_name"`
	MIMEType      string    `json:"mime_type"`
	FileSizeBytes int64     `json:"file_size_bytes"`
	SHA256        string    `json:"sha256"`
	CreatedAt     time.Time `json:"created_at"`
	storageKey    string
}

type AttachmentInput struct {
	OriginalName  string
	StorageKey    string
	MIMEType      string
	FileSizeBytes int64
	SHA256        string
}

type TicketDetail struct {
	Ticket   Ticket    `json:"ticket"`
	Messages []Message `json:"messages"`
}

type CreateTicketInput struct {
	Category string `json:"category"`
	Subject  string `json:"subject"`
	Body     string `json:"body"`
}

type MessageInput struct {
	Body string `json:"body"`
}

type UpdateTicketInput struct {
	Status     *string    `json:"status"`
	AssignedTo *uuid.UUID `json:"assigned_to"`
}

type ExternalMessage struct {
	ChannelID         string    `json:"channel_id"`
	ExternalMessageID string    `json:"external_message_id"`
	ExternalAuthorID  string    `json:"external_author_id"`
	Body              string    `json:"body"`
	Operation         Operation `json:"operation"`
	CreatedAt         time.Time `json:"created_at"`
}

type ExternalEmailMessage struct {
	ExternalMessageID string    `json:"external_message_id"`
	TicketNumber      string    `json:"ticket_number"`
	From              string    `json:"from"`
	Subject           string    `json:"subject"`
	Body              string    `json:"body"`
	CreatedAt         time.Time `json:"created_at"`
}

type DiscordOutboxTask struct {
	ID                uuid.UUID
	TicketID          uuid.UUID
	MessageID         *uuid.UUID
	TicketNumber      int64
	Subject           string
	ChannelID         string
	Operation         string
	Body              string
	AuthorType        string
	ExternalMessageID string
}

type Repository interface {
	CreateTicket(context.Context, uuid.UUID, CreateTicketInput) (TicketDetail, error)
	ListTickets(context.Context, *uuid.UUID, string, int, int) ([]Ticket, error)
	GetTicket(context.Context, *uuid.UUID, uuid.UUID) (TicketDetail, error)
	AddUserMessage(context.Context, uuid.UUID, uuid.UUID, string) (Message, error)
	AddAdminMessage(context.Context, uuid.UUID, uuid.UUID, string) (Message, error)
	SetTicketStatus(context.Context, uuid.UUID, uuid.UUID, TicketStatus, *uuid.UUID, bool) (Ticket, error)
	IsAdmin(context.Context, uuid.UUID) (bool, error)
	ApplyDiscordMessage(context.Context, ExternalMessage, []byte) (Message, error)
}

type AttachmentRepository interface {
	CreateTicketWithAttachments(context.Context, uuid.UUID, CreateTicketInput, []AttachmentInput) (TicketDetail, error)
	AddUserMessageWithAttachments(context.Context, uuid.UUID, uuid.UUID, string, []AttachmentInput) (Message, error)
	AddAdminMessageWithAttachments(context.Context, uuid.UUID, uuid.UUID, string, []AttachmentInput) (Message, error)
	GetAttachment(context.Context, *uuid.UUID, uuid.UUID, uuid.UUID) (Attachment, error)
}

type DiscordOutboxStore interface {
	ClaimDiscordOutbox(context.Context, int) ([]DiscordOutboxTask, error)
	MarkDiscordOutboxSent(context.Context, DiscordOutboxTask, string) error
	MarkDiscordOutboxFailed(context.Context, DiscordOutboxTask, string) error
}

type DiscordInboundStore interface {
	ApplyDiscordMessage(context.Context, ExternalMessage, []byte) (Message, error)
}

type EmailInboundRepository interface {
	ApplyEmailMessage(context.Context, ExternalEmailMessage, []byte) (Message, error)
}

type DiscordPlatformSender interface {
	Send(context.Context, DiscordOutboxTask) (string, error)
}

func ValidateCreateTicket(input CreateTicketInput) error {
	input.Category = strings.TrimSpace(input.Category)
	input.Subject = strings.TrimSpace(input.Subject)
	input.Body = strings.TrimSpace(input.Body)
	if !validCategory(input.Category) || input.Subject == "" || len(input.Subject) > MaxSubjectLength || input.Body == "" || len(input.Body) > MaxMessageLength {
		return ErrInvalidInput
	}
	if containsSubjectControl(input.Subject) || containsMessageControl(input.Body) {
		return ErrInvalidInput
	}
	return nil
}

func ValidateMessageBody(body string) error {
	body = strings.TrimSpace(body)
	if body == "" || len(body) > MaxMessageLength || containsMessageControl(body) {
		return ErrInvalidInput
	}
	return nil
}

func ValidateAttachments(attachments []AttachmentInput) error {
	if len(attachments) > MaxAttachmentCount {
		return ErrInvalidInput
	}
	var total int64
	for _, attachment := range attachments {
		if strings.TrimSpace(attachment.OriginalName) == "" || len([]rune(attachment.OriginalName)) > 255 || !validAttachmentStorageKey(attachment.StorageKey) {
			return ErrInvalidInput
		}
		if attachment.FileSizeBytes <= 0 || attachment.FileSizeBytes > MaxAttachmentFileBytes {
			return ErrInvalidInput
		}
		if strings.TrimSpace(attachment.MIMEType) == "" || len(attachment.MIMEType) > 128 || containsControl(attachment.MIMEType) || !sha256Pattern.MatchString(strings.ToLower(strings.TrimSpace(attachment.SHA256))) {
			return ErrInvalidInput
		}
		total += attachment.FileSizeBytes
		if total > MaxAttachmentTotalBytes {
			return ErrInvalidInput
		}
	}
	return nil
}

func validAttachmentStorageKey(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 1024 || strings.HasPrefix(value, "/") || strings.ContainsAny(value, "\\\x00\r\n") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var ticketNumberPattern = regexp.MustCompile(`^T-[0-9]{6}$`)

func ValidateStatus(status TicketStatus, admin bool) error {
	switch status {
	case StatusOpen, StatusWaitingStaff, StatusWaitingUser, StatusClosed, StatusSpam:
		if !admin && status != StatusWaitingStaff && status != StatusClosed {
			return ErrInvalidStatus
		}
		return nil
	default:
		return ErrInvalidStatus
	}
}

func ValidateExternalMessage(input ExternalMessage) error {
	if strings.TrimSpace(input.ChannelID) == "" || len(input.ChannelID) > 64 ||
		strings.TrimSpace(input.ExternalMessageID) == "" || len(input.ExternalMessageID) > 256 ||
		strings.TrimSpace(input.ExternalAuthorID) == "" || len(input.ExternalAuthorID) > 256 ||
		containsControl(input.ChannelID) || containsControl(input.ExternalMessageID) || containsControl(input.ExternalAuthorID) {
		return ErrInvalidDiscordData
	}
	if input.Operation != OperationCreate && input.Operation != OperationEdit && input.Operation != OperationDelete {
		return ErrInvalidDiscordData
	}
	if input.Operation != OperationDelete && (strings.TrimSpace(input.Body) == "" || len(input.Body) > MaxMessageLength || containsMessageControl(input.Body)) {
		return ErrInvalidDiscordData
	}
	return nil
}

func ValidateExternalEmailMessage(input ExternalEmailMessage) error {
	input.ExternalMessageID = strings.TrimSpace(input.ExternalMessageID)
	input.TicketNumber = strings.TrimSpace(input.TicketNumber)
	input.From = strings.TrimSpace(input.From)
	input.Subject = strings.TrimSpace(input.Subject)
	input.Body = strings.TrimSpace(input.Body)
	if input.ExternalMessageID == "" || len(input.ExternalMessageID) > 256 || containsControl(input.ExternalMessageID) {
		return ErrInvalidInput
	}
	if !ticketNumberPattern.MatchString(input.TicketNumber) || input.Subject == "" || len([]rune(input.Subject)) > MaxSubjectLength || containsSubjectControl(input.Subject) {
		return ErrInvalidInput
	}
	parsed, err := mail.ParseAddress(input.From)
	if err != nil || parsed.Address == "" || len(parsed.Address) > 320 || containsControl(parsed.Address) {
		return ErrInvalidInput
	}
	return ValidateMessageBody(input.Body)
}

func parseTicketNumberValue(value string) (int64, error) {
	if !ticketNumberPattern.MatchString(value) {
		return 0, ErrInvalidInput
	}
	return strconv.ParseInt(value[2:], 10, 64)
}

func validCategory(category string) bool {
	switch Category(category) {
	case CategoryAccount, CategoryAdmissions, CategoryBrochure, CategoryResults,
		CategoryCandidateNumber, CategoryWillingness, CategoryTechnical, CategoryOther:
		return true
	default:
		return false
	}
}

func containsControl(value string) bool {
	return strings.IndexFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0
}

func containsSubjectControl(value string) bool {
	return containsControl(value)
}

func containsMessageControl(value string) bool {
	return strings.IndexFunc(value, func(r rune) bool {
		return (r < 0x20 && r != '\n' && r != '\r' && r != '\t') || r == 0x7f
	}) >= 0
}

func ticketNumber(value int64) string {
	return "T-" + leftPadNumber(value, 6)
}

func leftPadNumber(value int64, width int) string {
	text := ""
	if value == 0 {
		text = "0"
	}
	for value > 0 {
		text = string(rune('0'+value%10)) + text
		value /= 10
	}
	for len(text) < width {
		text = "0" + text
	}
	return text
}
