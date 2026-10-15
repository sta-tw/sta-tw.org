package discordmail

import (
	"context"
	"errors"

	"sta-backend/internal/accountapplications"
	"sta-backend/internal/emailinquiries"
)

type inquiryReplyService interface {
	ReplyByDiscord(ctx context.Context, threadID, staffName, staffDiscordID, body, signature string) (emailinquiries.Inquiry, error)
}

type applicationReplyService interface {
	ReplyByDiscord(ctx context.Context, threadID, staffName, staffDiscordID, body, signature string) (accountapplications.Application, error)
}

// ReplyRouter routes /re by the Forum thread it is used in: ordinary inquiries
// and account-application correction emails share the same personal-email
// reply flow.
type ReplyRouter struct {
	inquiries    inquiryReplyService
	applications applicationReplyService
}

func NewReplyRouter(inquiries inquiryReplyService, applications applicationReplyService) (*ReplyRouter, error) {
	if inquiries == nil || applications == nil {
		return nil, errors.New("discord mail reply dependencies are missing")
	}
	return &ReplyRouter{inquiries: inquiries, applications: applications}, nil
}

func (r *ReplyRouter) ReplyByDiscord(ctx context.Context, _ string, threadID, staffName, staffDiscordID, body, signature string) error {
	if err := r.applicationsReply(ctx, threadID, staffName, staffDiscordID, body, signature); err == nil {
		return nil
	} else if !errors.Is(err, accountapplications.ErrNotFound) {
		return normalizeApplicationReplyError(err)
	}
	_, err := r.inquiries.ReplyByDiscord(ctx, threadID, staffName, staffDiscordID, body, signature)
	return err
}

func (r *ReplyRouter) applicationsReply(ctx context.Context, threadID, staffName, staffDiscordID, body, signature string) error {
	_, err := r.applications.ReplyByDiscord(ctx, threadID, staffName, staffDiscordID, body, signature)
	return err
}

func normalizeApplicationReplyError(err error) error {
	if errors.Is(err, accountapplications.ErrNotFound) {
		return emailinquiries.ErrNotFound
	}
	if errors.Is(err, accountapplications.ErrInvalidInput) {
		return emailinquiries.ErrInvalidInput
	}
	return err
}
