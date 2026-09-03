package brochurediscovery

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/admissions"
)

const (
	StatusCompleted      = "completed"
	StatusUnderReview    = "under_review"
	StatusSearching      = "searching"
	StatusPendingSearch  = "pending_search"
	StatusNeedsAttention = "needs_attention"

	// CompletionExternalConfirmed keeps the existing database enum value for
	// compatibility. It means a discovery agent supplied a candidate that an
	// administrator confirmed; it does not imply an internal AI service.
	CompletionExternalConfirmed = "ai_confirmed"
	// CompletionAIConfirmed is retained as a source-compatible alias for older
	// callers. New code should use CompletionExternalConfirmed.
	CompletionAIConfirmed  = CompletionExternalConfirmed
	CompletionManualUpload = "manual_upload"
	CompletionNoBrochure   = "no_brochure_confirmed"

	CycleDraft  = "draft"
	CycleActive = "active"
	CycleClosed = "closed"
)

var (
	ErrInvalid       = errors.New("brochure discovery input is invalid")
	ErrNotFound      = errors.New("brochure discovery task not found")
	ErrInvalidStatus = errors.New("brochure discovery status transition is invalid")
	ErrAdminRequired = errors.New("administrator role is required")
	sha256Pattern    = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type Task struct {
	AcademicYear         int            `json:"academic_year"`
	SchoolCode           string         `json:"school_code"`
	SchoolName           string         `json:"school_name"`
	Status               string         `json:"status"`
	CompletionMethod     string         `json:"completion_method,omitempty"`
	AttemptCount         int            `json:"attempt_count"`
	CandidateSourceURL   string         `json:"candidate_source_url,omitempty"`
	CandidateDocumentURL string         `json:"candidate_document_url,omitempty"`
	CandidateSHA256      string         `json:"candidate_sha256,omitempty"`
	CandidateConfidence  *float64       `json:"candidate_confidence,omitempty"`
	CandidateEvidence    map[string]any `json:"candidate_evidence,omitempty"`
	LastErrorCode        string         `json:"last_error_code,omitempty"`
	LastErrorMessage     string         `json:"last_error_message,omitempty"`
	LastSearchedAt       *time.Time     `json:"last_searched_at,omitempty"`
	NextSearchAt         *time.Time     `json:"next_search_at,omitempty"`
	CompletedAt          *time.Time     `json:"completed_at,omitempty"`
	CompletedBy          *uuid.UUID     `json:"completed_by,omitempty"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
}

type Cycle struct {
	AcademicYear int        `json:"academic_year"`
	Status       string     `json:"status"`
	CreatedBy    *uuid.UUID `json:"created_by,omitempty"`
	StartedBy    *uuid.UUID `json:"started_by,omitempty"`
	ClosedBy     *uuid.UUID `json:"closed_by,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	ClosedAt     *time.Time `json:"closed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type CreateCycleInput struct {
	AcademicYear int `json:"academic_year"`
}

func (input CreateCycleInput) Validate() error {
	if input.AcademicYear < 100 || input.AcademicYear > 999 {
		return ErrInvalid
	}
	return nil
}

type Query struct {
	Status string
	Limit  int
	Offset int
}

type Event struct {
	ID             int64          `json:"id"`
	AcademicYear   int            `json:"academic_year"`
	SchoolCode     string         `json:"school_code"`
	Action         string         `json:"action"`
	FromStatus     string         `json:"from_status,omitempty"`
	ToStatus       string         `json:"to_status"`
	ActorAccountID *uuid.UUID     `json:"actor_account_id,omitempty"`
	Details        map[string]any `json:"details"`
	CreatedAt      time.Time      `json:"created_at"`
}

type CandidateInput struct {
	DetectedAcademicYear int            `json:"detected_academic_year"`
	SourceURL            string         `json:"source_url"`
	DocumentURL          string         `json:"document_url"`
	SHA256               string         `json:"sha256"`
	Confidence           *float64       `json:"confidence,omitempty"`
	Evidence             map[string]any `json:"evidence"`
}

type StoredDocumentInput struct {
	OriginalFileName string
	StorageKey       string
	MIMEType         string
	FileSizeBytes    int64
	SHA256           string
}

type ExtractionDispatcher interface {
	QueueDiscoveredBrochureExtraction(context.Context, int, string, string, string) error
}

func (input CandidateInput) Validate() error {
	if input.DetectedAcademicYear < 100 || input.DetectedAcademicYear > 999 ||
		admissions.ValidateOfficialURL(input.SourceURL) != nil ||
		admissions.ValidateOfficialURL(input.DocumentURL) != nil ||
		strings.TrimSpace(input.SourceURL) == "" || strings.TrimSpace(input.DocumentURL) == "" ||
		strings.TrimSpace(input.SourceURL) == admissions.MissingValue || strings.TrimSpace(input.DocumentURL) == admissions.MissingValue ||
		!sha256Pattern.MatchString(strings.ToLower(strings.TrimSpace(input.SHA256))) || input.Evidence == nil {
		return ErrInvalid
	}
	if input.Confidence != nil && (*input.Confidence < 0 || *input.Confidence > 1) {
		return ErrInvalid
	}
	return nil
}

type FailureInput struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (input FailureInput) Validate() error {
	if strings.TrimSpace(input.Code) == "" || len(input.Code) > 64 || strings.TrimSpace(input.Message) == "" || len([]rune(input.Message)) > 2000 {
		return ErrInvalid
	}
	return nil
}

type ReviewInput struct {
	Approved bool   `json:"approved"`
	Reason   string `json:"reason"`
}

type Repository interface {
	IsAdmin(context.Context, uuid.UUID) (bool, error)
	CreateCycle(context.Context, uuid.UUID, int) (Cycle, int64, error)
	ListCycles(context.Context, uuid.UUID) ([]Cycle, error)
	StartCycle(context.Context, uuid.UUID, int) (Cycle, error)
	CloseCycle(context.Context, uuid.UUID, int) (Cycle, error)
	List(context.Context, uuid.UUID, int, Query) ([]Task, error)
	ListEvents(context.Context, uuid.UUID, int, string) ([]Event, error)
	ClaimNext(context.Context, uuid.UUID, time.Duration) (Task, error)
	ClaimNextSystem(context.Context, time.Duration) (Task, error)
	SubmitCandidate(context.Context, uuid.UUID, int, string, CandidateInput) (Task, error)
	StoreCandidateSystem(context.Context, int, string, CandidateInput, StoredDocumentInput) (Task, string, error)
	MarkFailure(context.Context, uuid.UUID, int, string, FailureInput) (Task, error)
	MarkFailureSystem(context.Context, int, string, FailureInput) (Task, error)
	ReportNoMatchSystem(context.Context, int, string) (Task, error)
	Retry(context.Context, uuid.UUID, int, string) (Task, error)
	Review(context.Context, uuid.UUID, int, string, ReviewInput) (Task, error)
	CompleteManual(context.Context, uuid.UUID, int, string) (Task, error)
	ConfirmNoBrochure(context.Context, uuid.UUID, int, string, string) (Task, error)
}

func (input ReviewInput) Validate() error {
	if !input.Approved && strings.TrimSpace(input.Reason) == "" {
		return ErrInvalid
	}
	if len([]rune(input.Reason)) > 2000 {
		return ErrInvalid
	}
	return nil
}

func ValidStatus(value string) bool {
	switch value {
	case StatusCompleted, StatusUnderReview, StatusSearching, StatusPendingSearch, StatusNeedsAttention:
		return true
	default:
		return false
	}
}
