// Package accountapplications handles account requests from people without a
// usable school email: they apply by emailing account@mail.sta-tw.org with
// proof attachments, an admin reviews the request from a Telegram
// notification, and approval creates the account.
package accountapplications

import (
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
)

type Application struct {
	ID                uuid.UUID  `json:"id"`
	RequestedUsername string     `json:"requested_username"`
	Source            string     `json:"source"`
	Note              string     `json:"note"`
	Status            Status     `json:"status"`
	CreatedAccountID  *uuid.UUID `json:"created_account_id,omitempty"`
	TelegramChatID    *int64     `json:"-"`
	TelegramMessageID *int64     `json:"-"`
	// TelegramHeaderText is the static part of the Telegram card (寄件人/
	// 內容/附件 etc.), rendered once at creation. An edit rebuilds the full
	// card as this plus the running transcript, instead of re-deriving
	// document presign links or the notification template each time.
	TelegramHeaderText string    `json:"-"`
	CreatedAt          time.Time `json:"created_at"`
}

// Message is one entry in an application's back-and-forth correspondence —
// see account_application_messages. Direction is "inbound" or "outbound".
// Actor is "使用者" for inbound, or the staff member's Telegram display name
// for an outbound reply typed in Telegram (empty for a system-generated
// email like the rejection notice).
type Message struct {
	Direction string    `json:"direction"`
	Body      string    `json:"body"`
	Actor     string    `json:"actor"`
	CreatedAt time.Time `json:"created_at"`
}

type Document struct {
	ID          uuid.UUID `json:"id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	CreatedAt   time.Time `json:"created_at"`
}

// StagedDocument is a document already scanned and staged locally, ready to
// be uploaded to object storage and attached to an application.
type StagedDocument struct {
	LocalPath   string
	Filename    string
	ContentType string
	SizeBytes   int64
	SHA256Hex   string
}
