// Package emailinquiries handles cold email sent to account@mail.sta-tw.org
// that ISN'T a reply on an existing account-application thread — a plain
// question, not a request for an account. It never creates an account and
// has no approve/reject decision; it's just a Telegram-notified, threaded
// conversation. Account creation lives entirely in internal/accountapplications
// (the /apply web form).
package emailinquiries

import (
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/mailintake"
)

// Attachment is one file pulled out of an inbound email.
type Attachment = mailintake.Attachment

type Inquiry struct {
	ID                uuid.UUID `json:"id"`
	Note              string    `json:"note"`
	TelegramChatID    *int64    `json:"-"`
	TelegramMessageID *int64    `json:"-"`
	// TelegramHeaderText is the static part of the Telegram card (寄件人/
	// 附件 etc.), rendered once at creation — see accountapplications.Application
	// for why this is kept separate from the running transcript.
	TelegramHeaderText string    `json:"-"`
	CreatedAt          time.Time `json:"created_at"`
}

// Message is one entry in an inquiry's back-and-forth correspondence.
// Direction is "inbound" or "outbound". Actor is "使用者" for inbound, or the
// staff member's Telegram display name for an outbound reply typed in
// Telegram (empty for a system-generated message).
type Message struct {
	Direction string    `json:"direction"`
	Body      string    `json:"body"`
	Actor     string    `json:"actor"`
	CreatedAt time.Time `json:"created_at"`
}
