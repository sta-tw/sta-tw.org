package chat

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidMessage = errors.New("invalid chat message")
	ErrNotFound       = errors.New("chat message not found")
	ErrForbidden      = errors.New("chat access forbidden")
	ErrSignature      = errors.New("invalid webhook signature")
)

const MaxMessageLength = 2000

type Platform string

const (
	PlatformWebsite  Platform = "website"
	PlatformDiscord  Platform = "discord"
	PlatformTelegram Platform = "telegram"
)

type Operation string

const (
	OperationCreate Operation = "create"
	OperationEdit   Operation = "edit"
	OperationDelete Operation = "delete"
)

type Message struct {
	ID             uuid.UUID  `json:"id"`
	Body           string     `json:"body"`
	SourcePlatform Platform   `json:"source_platform"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	EditedAt       *time.Time `json:"edited_at,omitempty"`
}

type OutboxTask struct {
	ID                uuid.UUID `json:"id"`
	MessageID         uuid.UUID `json:"message_id"`
	TargetPlatform    Platform  `json:"target_platform"`
	Operation         Operation `json:"operation"`
	Body              string    `json:"body"`
	ExternalMessageID string    `json:"external_message_id,omitempty"`
}

type OutboxStore interface {
	ClaimOutbox(context.Context, int) ([]OutboxTask, error)
	MarkOutboxSent(context.Context, OutboxTask, string) error
	MarkOutboxFailed(context.Context, OutboxTask, string) error
}

type PlatformSender interface {
	Send(context.Context, OutboxTask) (string, error)
}

type ExternalMessage struct {
	Platform          Platform  `json:"platform"`
	ExternalMessageID string    `json:"external_message_id"`
	ExternalAuthorID  string    `json:"external_author_id"`
	Body              string    `json:"body"`
	Operation         Operation `json:"operation"`
	CreatedAt         time.Time `json:"created_at"`
}

type Repository interface {
	CreateWebsiteMessage(context.Context, uuid.UUID, string) (Message, error)
	ApplyExternalMessage(context.Context, ExternalMessage, []byte) (Message, error)
	ListMessages(context.Context, int, int) ([]Message, error)
}

type Publisher interface {
	Publish(string, uuid.UUID, any) error
}

func ValidateExternalMessage(input ExternalMessage) error {
	if input.Platform != PlatformDiscord && input.Platform != PlatformTelegram {
		return ErrInvalidMessage
	}
	if strings.TrimSpace(input.ExternalMessageID) == "" || len(input.ExternalMessageID) > 256 ||
		strings.TrimSpace(input.ExternalAuthorID) == "" || len(input.ExternalAuthorID) > 256 ||
		containsControl(input.ExternalMessageID) || containsControl(input.ExternalAuthorID) {
		return ErrInvalidMessage
	}
	if input.Operation != OperationCreate && input.Operation != OperationEdit && input.Operation != OperationDelete {
		return ErrInvalidMessage
	}
	if input.Operation != OperationDelete && (strings.TrimSpace(input.Body) == "" || len(input.Body) > MaxMessageLength || containsDisallowedBodyControl(input.Body)) {
		return ErrInvalidMessage
	}
	return nil
}

func containsControl(value string) bool {
	return strings.IndexFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0
}

func containsDisallowedBodyControl(value string) bool {
	return strings.IndexFunc(value, func(r rune) bool {
		return (r < 0x20 && r != '\n' && r != '\r' && r != '\t') || r == 0x7f
	}) >= 0
}
