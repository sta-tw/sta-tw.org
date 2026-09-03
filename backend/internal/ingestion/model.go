package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/admissions"
	"sta-backend/internal/jobs"
)

const (
	BrochureJobType      = "admissions.brochure.extract"
	CandidateListJobType = "admissions.candidate-list.extract"
	DefaultProcessor     = "local-extraction-v1"
	DefaultRunStatus     = "processing"
	RunStatusPending     = "pending_review"
	RunStatusApproved    = "approved"
	RunStatusRejected    = "rejected"
	CandidatePending     = "pending"
	CandidateApproved    = "approved"
	CandidateRejected    = "rejected"
)

var (
	ErrNotFound            = errors.New("ingestion resource not found")
	ErrInvalid             = errors.New("ingestion input is invalid")
	ErrConflict            = errors.New("ingestion resource conflict")
	ErrAdminRequired       = errors.New("administrator role is required")
	ErrInvalidStatus       = errors.New("ingestion status transition is invalid")
	ErrDispatchUnavailable = errors.New("ingestion dispatch is unavailable")
)

type Run struct {
	ID               uuid.UUID       `json:"id"`
	IngestionJobID   uuid.UUID       `json:"ingestion_job_id"`
	AcademicYear     int             `json:"academic_year"`
	SchoolCode       string          `json:"school_code"`
	SourceSHA256     string          `json:"source_sha256"`
	ProcessorVersion string          `json:"processor_version"`
	Status           string          `json:"status"`
	RawExtraction    json.RawMessage `json:"raw_extraction,omitempty"`
	ErrorCode        string          `json:"error_code,omitempty"`
	ErrorMessage     string          `json:"error_message,omitempty"`
	ReviewedBy       *uuid.UUID      `json:"reviewed_by,omitempty"`
	ReviewedAt       *time.Time      `json:"reviewed_at,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	Candidates       []Candidate     `json:"candidates,omitempty"`
}

type Candidate struct {
	ID            uuid.UUID       `json:"id"`
	RunID         uuid.UUID       `json:"run_id"`
	ProgramCode   string          `json:"program_code"`
	ExtractedData json.RawMessage `json:"extracted_data"`
	SourcePage    *int            `json:"source_page,omitempty"`
	Confidence    *float64        `json:"confidence,omitempty"`
	ReviewStatus  string          `json:"review_status"`
	ReviewedBy    *uuid.UUID      `json:"reviewed_by,omitempty"`
	ReviewedAt    *time.Time      `json:"reviewed_at,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type RunQuery struct {
	AcademicYear int
	Status       string
	Limit        int
	Offset       int
}

// JobStatus is intentionally storage-key-free. Extraction services receive a
// storage key only from the authenticated claim endpoint; normal status
// polling must not disclose object-storage layout.
type JobStatus struct {
	ID               uuid.UUID `json:"job_id"`
	JobType          string    `json:"job_type"`
	SourceType       string    `json:"source_type"`
	AcademicYear     int       `json:"academic_year"`
	SchoolCode       string    `json:"school_code"`
	Status           string    `json:"status"`
	AttemptCount     int       `json:"attempt_count"`
	LastErrorCode    string    `json:"last_error_code,omitempty"`
	LastErrorMessage string    `json:"last_error_message,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// JobFailureInput is used by the HTTP extraction transport when a worker
// cannot produce a result. Retryable failures return to the queue after a
// short delay; malformed sources are marked failed and remain inspectable.
type JobFailureInput struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type ReviewInput struct {
	Approved bool                     `json:"approved"`
	Reason   string                   `json:"reason"`
	Program  *admissions.ProgramInput `json:"program,omitempty"`
}

type Repository interface {
	IsAdmin(context.Context, uuid.UUID) (bool, error)
	ListRuns(context.Context, uuid.UUID, RunQuery) ([]Run, error)
	GetRun(context.Context, uuid.UUID, uuid.UUID) (Run, error)
	GetJobStatus(context.Context, uuid.UUID) (JobStatus, error)
	ReviewRun(context.Context, uuid.UUID, uuid.UUID, ReviewInput) (Run, error)
	ReviewCandidate(context.Context, uuid.UUID, uuid.UUID, ReviewInput) (Candidate, error)
	RequeueJob(context.Context, uuid.UUID, uuid.UUID) (jobs.BrochureExtractJob, error)
	ApplyExtractionResult(context.Context, jobs.BrochureExtractionResult) error
}

type brochureJobRecord struct {
	Job           jobs.BrochureExtractJob
	Status        string
	ShouldPublish bool
}
