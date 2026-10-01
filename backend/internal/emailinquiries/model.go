// Package emailinquiries handles cold email sent to any address registered
// in internal/mailroutes (e.g. account@mail.sta-tw.org, brochure@mail.sta-tw.org)
// that ISN'T a reply on an existing account-application thread — a plain
// question, not a request for an account. It never creates an account and
// has no approve/reject decision; it's just a Discord-forum-notified,
// threaded conversation. Account creation lives entirely in
// internal/accountapplications (the /apply web form).
package emailinquiries

import (
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/mailintake"
)

// Attachment is one file pulled out of an inbound email.
type Attachment = mailintake.Attachment

type Inquiry struct {
	ID              uuid.UUID `json:"id"`
	Note            string    `json:"note"`
	MailRouteID     uuid.UUID `json:"mail_route_id"`
	DiscordThreadID string    `json:"-"`
	CreatedAt       time.Time `json:"created_at"`
}

// InquirySummary is one row in a mail route's history view — the admin UI
// never needs the full message transcript just to list a route's inquiries,
// only enough to identify and link into each one.
type InquirySummary struct {
	ID              uuid.UUID `json:"id"`
	SenderEmail     string    `json:"sender_email"`
	Note            string    `json:"note"`
	DiscordThreadID string    `json:"discord_thread_id"`
	MessageCount    int       `json:"message_count"`
	CreatedAt       time.Time `json:"created_at"`
}

// Message is one entry in an inquiry's back-and-forth correspondence — the
// fields here mirror what a Gmail "show original" view exposes, pulled
// straight from the raw email's own headers instead of just the parsed
// body. Direction is "inbound" or "outbound". Actor is "使用者" for inbound,
// or the staff member's display name for an outbound reply sent via
// Discord's /re command (empty for a system-generated message) —
// ActorDiscordID is that same staff member's Discord snowflake, which
// (unlike the display name) can't be changed after the fact, for when a
// reply needs to be traced back to a specific account. SenderIP/AuthResults
// are inbound-only, pulled from the connecting SMTP client's address and
// the DKIM/SPF verdict Postfix records in the message's own headers — the
// two fields that actually matter for tracing or disputing where a piece of
// mail really came from. SourceMessageID/InReplyTo/References are the raw
// RFC 5322 threading headers (inbound only — an outbound reply's own
// generated Message-ID isn't tracked per-message, only via
// Service.messageIDFor at send time); Mailer is the sending client's
// self-reported X-Mailer/User-Agent header (inbound only, informational —
// it's self-reported so it proves nothing on its own, but "claims to be
// sent by a mainstream mail client" vs "no mailer header at all" is still a
// useful signal when triaging).
type Message struct {
	Direction       string    `json:"direction"`
	Subject         string    `json:"subject"`
	Body            string    `json:"body"`
	Actor           string    `json:"actor"`
	ActorDiscordID  string    `json:"actor_discord_id,omitempty"`
	ToAddress       string    `json:"to_address,omitempty"`
	SenderIP        string    `json:"sender_ip,omitempty"`
	AuthResults     string    `json:"auth_results,omitempty"`
	SourceMessageID string    `json:"source_message_id,omitempty"`
	InReplyTo       string    `json:"in_reply_to,omitempty"`
	References      string    `json:"references,omitempty"`
	Mailer          string    `json:"mailer,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}
