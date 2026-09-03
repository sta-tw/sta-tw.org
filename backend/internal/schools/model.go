package schools

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	GeneralUniversity        = "general_university"
	TechnicalInstitution     = "technical_institution"
	JuniorCollege            = "junior_college"
	OpenUniversity           = "open_university"
	ReligiousResearchCollege = "religious_research_college"
)

var (
	ErrInvalidInput  = errors.New("invalid school input")
	ErrNotFound      = errors.New("school not found")
	ErrAdminRequired = errors.New("administrator role is required")
)

var schoolCodePattern = regexp.MustCompile(`^[0-9]{3}$`)

type School struct {
	SchoolCode      string    `json:"school_code"`
	SchoolName      string    `json:"school_name"`
	InstitutionType string    `json:"institution_type"`
	IsActive        bool      `json:"is_active"`
	CreatedAt       time.Time `json:"created_at,omitempty"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
}

type SchoolInput struct {
	SchoolCode      string `json:"school_code"`
	SchoolName      string `json:"school_name"`
	InstitutionType string `json:"institution_type"`
	IsActive        *bool  `json:"is_active"`
}

type BatchInput struct {
	Reason string        `json:"reason"`
	Items  []SchoolInput `json:"items"`
}

type Query struct {
	Text  string
	Limit int
}

type AuditEvent struct {
	ID         int64          `json:"id"`
	Action     string         `json:"action"`
	EntityKey  string         `json:"entity_key"`
	BeforeData map[string]any `json:"before_data,omitempty"`
	AfterData  map[string]any `json:"after_data,omitempty"`
	Reason     string         `json:"reason"`
	CreatedAt  time.Time      `json:"created_at"`
}

type Repository interface {
	IsAdmin(context.Context, uuid.UUID) (bool, error)
	List(context.Context, bool) ([]School, error)
	Upsert(context.Context, uuid.UUID, BatchInput) ([]School, error)
	ListHistory(context.Context, uuid.UUID, string) ([]AuditEvent, error)
}

func (input BatchInput) Validate() error {
	normalized := normalizeBatch(input)
	if normalized.Reason == "" || len([]rune(normalized.Reason)) > 2000 {
		return ErrInvalidInput
	}
	if len(normalized.Items) == 0 || len(normalized.Items) > 500 {
		return ErrInvalidInput
	}
	seen := make(map[string]struct{}, len(normalized.Items))
	for _, item := range normalized.Items {
		if err := item.Validate(); err != nil {
			return err
		}
		if _, ok := seen[item.SchoolCode]; ok {
			return ErrInvalidInput
		}
		seen[item.SchoolCode] = struct{}{}
	}
	return nil
}

func (input SchoolInput) Validate() error {
	code := strings.TrimSpace(input.SchoolCode)
	name := strings.TrimSpace(input.SchoolName)
	typeName := strings.TrimSpace(input.InstitutionType)
	if !schoolCodePattern.MatchString(code) || code == "000" {
		return ErrInvalidInput
	}
	if name == "" || len([]rune(name)) > 200 || typeName == "" || input.IsActive == nil {
		return ErrInvalidInput
	}
	if !validInstitutionType(typeName) {
		return ErrInvalidInput
	}
	return nil
}

func validInstitutionType(value string) bool {
	switch value {
	case GeneralUniversity, TechnicalInstitution, JuniorCollege, OpenUniversity, ReligiousResearchCollege:
		return true
	default:
		return false
	}
}

func validateSchoolCode(code string) error {
	if !schoolCodePattern.MatchString(code) || code == "000" {
		return ErrInvalidInput
	}
	return nil
}

func (s School) snapshot() map[string]any {
	return map[string]any{
		"school_code":      s.SchoolCode,
		"school_name":      s.SchoolName,
		"institution_type": s.InstitutionType,
		"is_active":        s.IsActive,
	}
}

func schoolAction(before *School, after School) string {
	if before == nil {
		return "create"
	}
	if before.IsActive && !after.IsActive {
		return "deactivate"
	}
	if !before.IsActive && after.IsActive {
		return "reactivate"
	}
	return "update"
}

func normalizeInput(input SchoolInput) SchoolInput {
	input.SchoolCode = strings.TrimSpace(input.SchoolCode)
	input.SchoolName = strings.TrimSpace(input.SchoolName)
	input.InstitutionType = strings.TrimSpace(input.InstitutionType)
	return input
}

func normalizeBatch(input BatchInput) BatchInput {
	input.Reason = strings.TrimSpace(input.Reason)
	for index := range input.Items {
		input.Items[index] = normalizeInput(input.Items[index])
	}
	return input
}

func invalidCodeError(code string) error {
	return fmt.Errorf("invalid school code %q: %w", code, ErrInvalidInput)
}
