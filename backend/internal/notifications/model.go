package notifications

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/pagination"
)

var (
	ErrNotFound = errors.New("notification not found")
	ErrConflict = errors.New("notification conflict")
)

type Notification struct {
	ID        uuid.UUID  `json:"id"`
	Kind      string     `json:"kind"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type EmailTask struct {
	ID                  uuid.UUID
	RecipientCiphertext []byte
	PayloadCiphertext   []byte
}

type InquiryNotificationTask struct {
	ID               uuid.UUID
	AccountID        uuid.UUID
	ApplicationID    uuid.UUID
	InquiryRound     string
	ResponseDeadline *time.Time
}

type Repository interface {
	CreateInApp(context.Context, uuid.UUID, string, string, string, string) (Notification, error)
	EnqueueEmailForAccount(context.Context, uuid.UUID, string, string, string, string) error
	EnqueueEmailTo(context.Context, uuid.UUID, []byte, string, string, string) error
	List(context.Context, uuid.UUID, int, pagination.Cursor) ([]Notification, string, error)
	UnreadCount(context.Context, uuid.UUID) (int, error)
	MarkRead(context.Context, uuid.UUID, uuid.UUID) error
	MarkAllRead(context.Context, uuid.UUID) (int64, error)
}

type EmailOutboxStore interface {
	ClaimEmailOutbox(context.Context, int) ([]EmailTask, error)
	MarkEmailSent(context.Context, uuid.UUID) error
	MarkEmailFailed(context.Context, uuid.UUID, string) error
}

type InquiryStore interface {
	ClaimInquiryNotifications(context.Context, int) ([]InquiryNotificationTask, error)
	MarkInquiryNotificationEnqueued(context.Context, uuid.UUID) error
	MarkInquiryNotificationFailed(context.Context, uuid.UUID, string) error
}

type EmailPayload struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
}
