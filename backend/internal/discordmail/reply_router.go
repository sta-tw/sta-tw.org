package discordmail

import (
	"context"
	"errors"

	"sta-backend/internal/accountapplications"
	"sta-backend/internal/admissions"
	"sta-backend/internal/emailinquiries"
)

type inquiryReplyService interface {
	ReplyByDiscord(ctx context.Context, threadID, staffName, staffDiscordID, body, signature string) (emailinquiries.Inquiry, error)
}

type applicationReplyService interface {
	ReplyByDiscord(ctx context.Context, threadID, staffName, staffDiscordID, body, signature string) (accountapplications.Application, error)
}

type contributionReplyService interface {
	ReplyByDiscord(ctx context.Context, command, threadID, staffName, staffDiscordID, body, signature string) error
}

// ReplyRouter routes /re by the platform-managed Forum thread it is used in:
// ordinary inquiries, account-application corrections, and admissions reports.
type ReplyRouter struct {
	inquiries     inquiryReplyService
	applications  applicationReplyService
	contributions contributionReplyService
}

func NewReplyRouter(inquiries inquiryReplyService, applications applicationReplyService, contributions contributionReplyService) (*ReplyRouter, error) {
	if inquiries == nil || applications == nil || contributions == nil {
		return nil, errors.New("discord mail reply dependencies are missing")
	}
	return &ReplyRouter{inquiries: inquiries, applications: applications, contributions: contributions}, nil
}

func (r *ReplyRouter) ReplyByDiscord(ctx context.Context, command, threadID, staffName, staffDiscordID, body, signature string) error {
	if err := r.applicationsReply(ctx, threadID, staffName, staffDiscordID, body, signature); err == nil {
		return nil
	} else if !errors.Is(err, accountapplications.ErrNotFound) {
		return normalizeApplicationReplyError(err)
	}
	_, err := r.inquiries.ReplyByDiscord(ctx, threadID, staffName, staffDiscordID, body, signature)
	if err == nil || !errors.Is(err, emailinquiries.ErrNotFound) {
		return err
	}
	err = r.contributions.ReplyByDiscord(ctx, command, threadID, staffName, staffDiscordID, body, signature)
	if errors.Is(err, admissions.ErrContributionNotFound) {
		return emailinquiries.ErrNotFound
	}
	if errors.Is(err, admissions.ErrContributionReplyInvalid) {
		return emailinquiries.ErrInvalidInput
	}
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
