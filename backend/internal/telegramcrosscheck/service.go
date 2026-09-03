package telegramcrosscheck

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"sta-backend/internal/results"
)

type Repository interface {
	IsAdmin(context.Context, uuid.UUID) (bool, error)
	AdminStatus(context.Context, uuid.UUID) (AdminStatus, error)
	SyncParticipants(context.Context, uuid.UUID, string, []PreparedParticipant) ([]ParticipantSyncResult, error)
	Bind(context.Context, BindInput) error
	Disable(context.Context, int64) error
	Dashboard(context.Context, int64) (Dashboard, error)
	History(context.Context, int64, int) ([]HistoryEvent, error)
	ResolveInquiry(context.Context, int64, uuid.UUID) (InquiryOwner, error)
	MarkResponded(context.Context, int64, uuid.UUID) error
	ClaimDeliveries(context.Context, int) ([]Delivery, error)
	MarkSent(context.Context, uuid.UUID, int64) error
	MarkFailed(context.Context, uuid.UUID, string, bool) error
}

type WillingnessWriter interface {
	SetWillingnessFromChannel(context.Context, uuid.UUID, uuid.UUID, int16, uuid.UUID, string, string) (results.WillingnessResponse, error)
}

type InquiryOwner struct {
	AccountID         uuid.UUID
	ApplicationID     uuid.UUID
	ProgramIdentifier string
	SchoolName        string
	ProgramName       string
}

type Service struct {
	repository Repository
	writer     WillingnessWriter
}

func NewService(repository Repository, writer WillingnessWriter) (*Service, error) {
	if repository == nil || writer == nil {
		return nil, errors.New("Telegram cross-check service dependencies are missing")
	}
	return &Service{repository: repository, writer: writer}, nil
}

func (service *Service) Bind(ctx context.Context, input BindInput) error {
	if err := input.Validate(); err != nil {
		return err
	}
	return service.repository.Bind(ctx, input)
}

func (service *Service) Disable(ctx context.Context, telegramUserID int64) error {
	if telegramUserID <= 0 {
		return ErrInvalidInput
	}
	return service.repository.Disable(ctx, telegramUserID)
}

func (service *Service) Dashboard(ctx context.Context, telegramUserID int64) (Dashboard, error) {
	if telegramUserID <= 0 {
		return Dashboard{}, ErrInvalidInput
	}
	return service.repository.Dashboard(ctx, telegramUserID)
}

func (service *Service) History(ctx context.Context, telegramUserID int64, limit int) ([]HistoryEvent, error) {
	if telegramUserID <= 0 || limit < 1 || limit > 100 {
		return nil, ErrInvalidInput
	}
	return service.repository.History(ctx, telegramUserID, limit)
}

func (service *Service) Respond(ctx context.Context, input RespondInput) (RespondResult, error) {
	if err := input.Validate(); err != nil {
		return RespondResult{}, err
	}
	owner, err := service.repository.ResolveInquiry(ctx, input.TelegramUserID, input.InquiryID)
	if err != nil {
		return RespondResult{}, err
	}
	value, err := input.Choice.InternalValue()
	if err != nil {
		return RespondResult{}, err
	}
	if _, err := service.writer.SetWillingnessFromChannel(
		ctx,
		owner.AccountID,
		owner.ApplicationID,
		value,
		input.InquiryID,
		"telegram",
		strings.TrimSpace(input.CallbackID),
	); err != nil {
		return RespondResult{}, err
	}
	if err := service.repository.MarkResponded(ctx, input.TelegramUserID, input.InquiryID); err != nil {
		return RespondResult{}, err
	}
	return RespondResult{
		ApplicationID:     owner.ApplicationID,
		ProgramIdentifier: owner.ProgramIdentifier,
		SchoolName:        owner.SchoolName,
		ProgramName:       owner.ProgramName,
		Choice:            input.Choice,
		ChoiceLabel:       input.Choice.Label(),
	}, nil
}
