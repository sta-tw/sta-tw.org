package chat

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/pagination"
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

	ChannelKey string          `json:"channel_key,omitempty"`
	ParentID   *uuid.UUID      `json:"parent_id,omitempty"`
	PinnedAt   *time.Time      `json:"pinned_at,omitempty"`
	ReplyCount int             `json:"reply_count,omitempty"`
	Reactions  []ReactionTally `json:"reactions,omitempty"`
}

// ReactionTally is one emoji's count on a message, with whether the caller has
// reacted with it.
type ReactionTally struct {
	Emoji string `json:"emoji"`
	Count int    `json:"count"`
	Mine  bool   `json:"mine"`
}

// Channel is a chat channel as returned by GET /api/v1/chat/channels.
type Channel struct {
	Key         string `json:"key"`
	DisplayName string `json:"display_name"`
	Kind        string `json:"kind"`
	Topic       string `json:"topic,omitempty"`
	IsDefault   bool   `json:"is_default"`
}

const (
	// MaxReactionLength bounds a reaction token (an emoji, or a short :shortcode:).
	MaxReactionLength = 32
	// MaxThreadDepth is 1: a reply cannot itself be replied to.
	MaxThreadDepth = 1
)

var ErrInvalidReaction = errors.New("invalid reaction")

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
	ListMessages(context.Context, int, pagination.Cursor) ([]Message, string, error)

	ListChannels(context.Context) ([]Channel, error)
	ListChannelMessages(ctx context.Context, channelKey string, viewer uuid.UUID, limit int, cursor pagination.Cursor) ([]Message, string, error)
	CreateChannelMessage(ctx context.Context, channelKey string, accountID uuid.UUID, body string, parentID *uuid.UUID) (Message, error)
	ListThreadReplies(ctx context.Context, parentID, viewer uuid.UUID, limit int, cursor pagination.Cursor) ([]Message, string, error)
	ListPinned(ctx context.Context, channelKey string, viewer uuid.UUID) ([]Message, error)
	SetReaction(ctx context.Context, messageID, accountID uuid.UUID, emoji string) error
	RemoveReaction(ctx context.Context, messageID, accountID uuid.UUID, emoji string) error
	SetPinned(ctx context.Context, messageID, adminID uuid.UUID, pinned bool) error
}

// NormalizeReaction trims and validates an emoji / short :code: reaction token.
func NormalizeReaction(raw string) (string, error) {
	token := strings.TrimSpace(raw)
	if token == "" || len(token) > MaxReactionLength || containsControl(token) {
		return "", ErrInvalidReaction
	}
	return token, nil
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
