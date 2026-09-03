package telegramcrosscheck

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/admissions"
	"sta-backend/internal/results"
)

var (
	ErrInvalidInput         = errors.New("invalid Telegram cross-check input")
	ErrNotFound             = errors.New("Telegram cross-check resource not found")
	ErrConflict             = errors.New("Telegram cross-check resource conflict")
	ErrAdminRequired        = errors.New("administrator role is required")
	ErrProvisioningDisabled = errors.New("Telegram test provisioning is disabled")
	ErrInvalidState         = errors.New("Telegram delivery state is invalid")
)

type Choice string

const (
	ChoiceNotConsidering Choice = "not_considering"
	ChoiceLowInterest    Choice = "low_interest"
	ChoiceConsidering    Choice = "considering"
	ChoiceLeaningYes     Choice = "leaning_yes"
	ChoiceHighInterest   Choice = "high_interest"
	ChoiceDefinite       Choice = "definite"
)

var choiceValues = map[Choice]int16{
	ChoiceNotConsidering: 0,
	ChoiceLowInterest:    20,
	ChoiceConsidering:    40,
	ChoiceLeaningYes:     60,
	ChoiceHighInterest:   80,
	ChoiceDefinite:       100,
}

var choiceLabels = map[Choice]string{
	ChoiceNotConsidering: "完全不考慮",
	ChoiceLowInterest:    "意願偏低",
	ChoiceConsidering:    "還在考慮",
	ChoiceLeaningYes:     "傾向選擇",
	ChoiceHighInterest:   "高度有意願",
	ChoiceDefinite:       "確定選擇",
}

func (choice Choice) Validate() error {
	if _, ok := choiceValues[choice]; !ok {
		return ErrInvalidInput
	}
	return nil
}

func (choice Choice) InternalValue() (int16, error) {
	value, ok := choiceValues[choice]
	if !ok {
		return 0, ErrInvalidInput
	}
	return value, nil
}

func (choice Choice) Label() string {
	return choiceLabels[choice]
}

func ChoiceFromInternalValue(value int16) (Choice, error) {
	for choice, candidate := range choiceValues {
		if candidate == value {
			return choice, nil
		}
	}
	return "", ErrInvalidInput
}

type ParticipantSyncInput struct {
	Reason       string             `json:"reason"`
	Participants []ParticipantInput `json:"participants"`
}

type ParticipantInput struct {
	TelegramUserID int64             `json:"telegram_user_id"`
	Assignments    []AssignmentInput `json:"assignments"`
}

type AssignmentInput struct {
	ProgramIdentifier string `json:"program_identifier"`
	CandidateNumber   string `json:"candidate_number"`
}

func (input ParticipantSyncInput) Validate() error {
	if strings.TrimSpace(input.Reason) == "" || len([]rune(input.Reason)) > 1000 || len(input.Participants) == 0 || len(input.Participants) > 500 {
		return ErrInvalidInput
	}
	seenUsers := make(map[int64]struct{}, len(input.Participants))
	for _, participant := range input.Participants {
		if participant.TelegramUserID <= 0 || len(participant.Assignments) == 0 || len(participant.Assignments) > 50 {
			return ErrInvalidInput
		}
		if _, exists := seenUsers[participant.TelegramUserID]; exists {
			return ErrInvalidInput
		}
		seenUsers[participant.TelegramUserID] = struct{}{}
		seenPrograms := make(map[string]struct{}, len(participant.Assignments))
		for _, assignment := range participant.Assignments {
			identifier := strings.TrimSpace(assignment.ProgramIdentifier)
			if _, err := admissions.ParseProgramIdentifier(identifier); err != nil {
				return ErrInvalidInput
			}
			if _, exists := seenPrograms[identifier]; exists {
				return ErrInvalidInput
			}
			seenPrograms[identifier] = struct{}{}
			if _, err := results.NormalizeCandidateNumber(assignment.CandidateNumber); err != nil {
				return ErrInvalidInput
			}
		}
	}
	return nil
}

type PreparedParticipant struct {
	TelegramUserID  int64
	Username        string
	EmailCiphertext []byte
	EmailLookupHash []byte
	Assignments     []PreparedAssignment
}

type PreparedAssignment struct {
	Identifier                admissions.ProgramIdentifier
	CandidateNumberCiphertext []byte
	CandidateNumberLookupHash []byte
	CandidateNumberLast4      string
}

type ParticipantSyncResult struct {
	TelegramUserID int64                  `json:"telegram_user_id"`
	AlreadyStarted bool                   `json:"already_started"`
	Assignments    []AssignmentSyncResult `json:"assignments"`
}

type AdminStatus struct {
	ParticipantCount          int            `json:"participant_count"`
	StartedCount              int            `json:"started_count"`
	NotificationsEnabledCount int            `json:"notifications_enabled_count"`
	OutboxByStatus            map[string]int `json:"outbox_by_status"`
}

type AssignmentSyncResult struct {
	ApplicationID     uuid.UUID `json:"application_id"`
	ProgramIdentifier string    `json:"program_identifier"`
	SchoolName        string    `json:"school_name"`
	ProgramName       string    `json:"program_name"`
}

type BindInput struct {
	TelegramUserID int64 `json:"telegram_user_id"`
	PrivateChatID  int64 `json:"private_chat_id"`
}

func (input BindInput) Validate() error {
	// Telegram private chats use the user's numeric ID as the chat ID. Keeping
	// this equality at the API boundary prevents a service caller from routing a
	// participant's private result to another chat.
	if input.TelegramUserID <= 0 || input.PrivateChatID != input.TelegramUserID {
		return ErrInvalidInput
	}
	return nil
}

type Dashboard struct {
	TelegramUserID       int64                `json:"telegram_user_id"`
	NotificationsEnabled bool                 `json:"notifications_enabled"`
	Applications         []ApplicationSummary `json:"applications"`
}

type ApplicationSummary struct {
	ApplicationID      uuid.UUID       `json:"application_id"`
	ProgramIdentifier  string          `json:"program_identifier"`
	AcademicYear       int             `json:"academic_year"`
	SchoolCode         string          `json:"school_code"`
	SchoolName         string          `json:"school_name"`
	ProgramCode        string          `json:"program_code"`
	ProgramName        string          `json:"program_name"`
	ResultStatus       string          `json:"result_status,omitempty"`
	OfficialRank       *int            `json:"official_rank,omitempty"`
	Quota              *int            `json:"quota,omitempty"`
	CurrentChoice      *Choice         `json:"current_choice,omitempty"`
	CurrentChoiceLabel string          `json:"current_choice_label,omitempty"`
	PendingInquiry     *InquirySummary `json:"pending_inquiry,omitempty"`
}

type InquirySummary struct {
	ID               uuid.UUID  `json:"id"`
	Round            string     `json:"round"`
	ResponseDeadline *time.Time `json:"response_deadline,omitempty"`
}

type HistoryEvent struct {
	ID                int64     `json:"id"`
	ApplicationID     uuid.UUID `json:"application_id"`
	ProgramIdentifier string    `json:"program_identifier"`
	SchoolName        string    `json:"school_name"`
	ProgramName       string    `json:"program_name"`
	InquiryRound      string    `json:"inquiry_round"`
	Choice            Choice    `json:"choice"`
	ChoiceLabel       string    `json:"choice_label"`
	CreatedAt         time.Time `json:"created_at"`
}

type RespondInput struct {
	TelegramUserID int64     `json:"telegram_user_id"`
	InquiryID      uuid.UUID `json:"inquiry_id"`
	Choice         Choice    `json:"choice"`
	CallbackID     string    `json:"callback_id"`
}

func (input RespondInput) Validate() error {
	if input.TelegramUserID <= 0 || input.InquiryID == uuid.Nil || strings.TrimSpace(input.CallbackID) == "" || len(input.CallbackID) > 200 {
		return ErrInvalidInput
	}
	return input.Choice.Validate()
}

type RespondResult struct {
	ApplicationID     uuid.UUID `json:"application_id"`
	ProgramIdentifier string    `json:"program_identifier"`
	SchoolName        string    `json:"school_name"`
	ProgramName       string    `json:"program_name"`
	Choice            Choice    `json:"choice"`
	ChoiceLabel       string    `json:"choice_label"`
}

type Delivery struct {
	ID                uuid.UUID  `json:"id"`
	TelegramUserID    int64      `json:"telegram_user_id"`
	ChatID            int64      `json:"chat_id"`
	InquiryID         uuid.UUID  `json:"inquiry_id"`
	ApplicationID     uuid.UUID  `json:"application_id"`
	ProgramIdentifier string     `json:"program_identifier"`
	AcademicYear      int        `json:"academic_year"`
	SchoolName        string     `json:"school_name"`
	ProgramName       string     `json:"program_name"`
	ResultStatus      string     `json:"result_status"`
	OfficialRank      *int       `json:"official_rank,omitempty"`
	Quota             *int       `json:"quota,omitempty"`
	InquiryRound      string     `json:"inquiry_round"`
	ResponseDeadline  *time.Time `json:"response_deadline,omitempty"`
}

type ClaimInput struct {
	Limit int `json:"limit"`
}

func (input *ClaimInput) Normalize() error {
	if input.Limit == 0 {
		input.Limit = 25
	}
	if input.Limit < 1 || input.Limit > 100 {
		return ErrInvalidInput
	}
	return nil
}

type SentInput struct {
	TelegramMessageID int64 `json:"telegram_message_id"`
}

type FailedInput struct {
	Error     string `json:"error"`
	Retryable bool   `json:"retryable"`
}

func (input FailedInput) Validate() error {
	if strings.TrimSpace(input.Error) == "" || len([]rune(input.Error)) > 1000 {
		return ErrInvalidInput
	}
	return nil
}
