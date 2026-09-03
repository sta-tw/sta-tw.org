package admissions

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	ProgramStatusDraft     = "draft"
	ProgramStatusPending   = "pending"
	ProgramStatusApproved  = "approved"
	ProgramStatusPublished = "published"
	ProgramStatusRejected  = "rejected"
	ProgramStatusArchived  = "archived"
)

type AdminProgram struct {
	Program
	ReviewStatus string    `json:"review_status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ProgramAdminQuery struct {
	AcademicYear int
	SchoolCode   string
	ReviewStatus string
	Search       string
	Limit        int
	Offset       int
}

type ProgramBatchInput struct {
	Reason string         `json:"reason"`
	Items  []ProgramInput `json:"items"`
}

type ProgramUpdateInput struct {
	Reason string       `json:"reason"`
	Item   ProgramInput `json:"item"`
}

type ProgramReviewInput struct {
	Approved bool   `json:"approved"`
	Reason   string `json:"reason"`
}

type ProgramAuditEvent struct {
	ID         int64          `json:"id"`
	Action     string         `json:"action"`
	EntityKey  string         `json:"entity_key"`
	BeforeData map[string]any `json:"before_data,omitempty"`
	AfterData  map[string]any `json:"after_data,omitempty"`
	Reason     string         `json:"reason"`
	CreatedAt  time.Time      `json:"created_at"`
}

type AdminRepository interface {
	Repository
	IsAdmin(context.Context, uuid.UUID) (bool, error)
	ListAdminPrograms(context.Context, uuid.UUID, ProgramAdminQuery) ([]AdminProgram, error)
	GetAdminProgram(context.Context, uuid.UUID, ProgramIdentifier) (AdminProgram, error)
	UpsertPrograms(context.Context, uuid.UUID, ProgramBatchInput) ([]AdminProgram, error)
	ReviewProgram(context.Context, uuid.UUID, ProgramIdentifier, ProgramReviewInput) (AdminProgram, error)
	ListProgramHistory(context.Context, uuid.UUID, ProgramIdentifier) ([]ProgramAuditEvent, error)
}

func (input ProgramBatchInput) Validate() error {
	normalized := normalizeProgramBatch(input)
	if normalized.Reason == "" || len([]rune(normalized.Reason)) > 2000 || len(normalized.Items) == 0 || len(normalized.Items) > 500 {
		return ErrInvalidProgram
	}
	seen := make(map[string]struct{}, len(normalized.Items))
	for _, item := range normalized.Items {
		if err := item.Validate(); err != nil {
			return err
		}
		identifier, err := item.identifier()
		if err != nil {
			return err
		}
		if _, exists := seen[identifier.String()]; exists {
			return ErrInvalidProgram
		}
		seen[identifier.String()] = struct{}{}
	}
	return nil
}

func (input ProgramUpdateInput) Validate() error {
	if strings.TrimSpace(input.Reason) == "" || len([]rune(input.Reason)) > 2000 {
		return ErrInvalidProgram
	}
	return input.Item.Validate()
}

func (input ProgramReviewInput) Validate() error {
	if !input.Approved && strings.TrimSpace(input.Reason) == "" {
		return ErrInvalidProgram
	}
	if len([]rune(input.Reason)) > 2000 {
		return ErrInvalidProgram
	}
	return nil
}

func normalizeProgramBatch(input ProgramBatchInput) ProgramBatchInput {
	input.Reason = strings.TrimSpace(input.Reason)
	for index := range input.Items {
		input.Items[index] = normalizeProgramInput(input.Items[index])
	}
	return input
}

func validProgramReviewStatus(value string) bool {
	switch value {
	case ProgramStatusDraft, ProgramStatusPending, ProgramStatusApproved, ProgramStatusPublished, ProgramStatusRejected, ProgramStatusArchived:
		return true
	default:
		return false
	}
}
