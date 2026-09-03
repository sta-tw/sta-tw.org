package verification

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound      = errors.New("verification resource not found")
	ErrForbidden     = errors.New("verification operation is forbidden")
	ErrConflict      = errors.New("verification request conflict")
	ErrInvalid       = errors.New("verification input is invalid")
	ErrInvalidCode   = errors.New("verification code is invalid")
	ErrInvalidStatus = errors.New("verification status transition is invalid")
	ErrAdminRequired = errors.New("administrator role is required")
)

type Method string

const (
	MethodSchoolEmail Method = "school_email"
	MethodDocument    Method = "document"
)

type Request struct {
	ID            uuid.UUID  `json:"id"`
	AcademicYear  int        `json:"academic_year"`
	SchoolCode    string     `json:"school_code"`
	ProgramCode   string     `json:"program_code,omitempty"`
	Method        Method     `json:"method"`
	Status        string     `json:"status"`
	DocumentCount int        `json:"document_count"`
	CreatedAt     time.Time  `json:"created_at"`
	ReviewedAt    *time.Time `json:"reviewed_at,omitempty"`
}

type Document struct {
	ID               uuid.UUID  `json:"id"`
	RequestID        uuid.UUID  `json:"request_id"`
	OriginalFileName string     `json:"original_file_name"`
	MIMEType         string     `json:"mime_type"`
	FileSizeBytes    int64      `json:"file_size_bytes"`
	SHA256           string     `json:"sha256"`
	Status           string     `json:"status"`
	RejectionReason  string     `json:"rejection_reason,omitempty"`
	ReviewedAt       *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	StorageKey       string     `json:"-"`
}

type Domain struct {
	ID         uuid.UUID `json:"id"`
	SchoolCode string    `json:"school_code"`
	Domain     string    `json:"domain"`
	IsActive   bool      `json:"is_active"`
	CreatedAt  time.Time `json:"created_at"`
}

type Verification struct {
	ID           uuid.UUID `json:"id"`
	AcademicYear int       `json:"academic_year"`
	SchoolCode   string    `json:"school_code"`
	ProgramCode  string    `json:"program_code,omitempty"`
	Method       Method    `json:"method"`
	Status       string    `json:"status"`
	VerifiedAt   time.Time `json:"verified_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type CreateRequestInput struct {
	AcademicYear int    `json:"academic_year"`
	SchoolCode   string `json:"school_code"`
	ProgramCode  string `json:"program_code"`
	SchoolEmail  string `json:"school_email"`
}

type ReviewInput struct {
	Approved bool   `json:"approved"`
	Reason   string `json:"reason"`
}

type ReviewResult struct {
	Request      Request       `json:"request"`
	Verification *Verification `json:"verification,omitempty"`
}

type Repository interface {
	IsAdmin(context.Context, uuid.UUID) (bool, error)
	IsSchoolEmailAllowed(context.Context, string, string) (bool, error)
	CreateEmailRequest(context.Context, uuid.UUID, CreateRequestInput, []byte, []byte) (Request, error)
	CreateDocumentRequest(context.Context, uuid.UUID, CreateRequestInput) (Request, error)
	CreateEmailChallenge(context.Context, uuid.UUID, []byte, time.Time) error
	ConsumeEmailCode(context.Context, uuid.UUID, uuid.UUID, []byte, time.Time, time.Time) (Verification, error)
	CreateDocument(context.Context, uuid.UUID, uuid.UUID, string, string, string, int64, string) (Request, error)
	ListDocuments(context.Context, uuid.UUID, uuid.UUID) ([]Document, error)
	GetDocument(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (Document, error)
	ListRequests(context.Context, uuid.UUID) ([]Request, error)
	ListPendingRequests(context.Context, uuid.UUID) ([]Request, error)
	ReviewDocumentRequest(context.Context, uuid.UUID, uuid.UUID, ReviewInput, time.Time, time.Time) (ReviewResult, error)
	AddDomain(context.Context, uuid.UUID, string, string) (Domain, error)
	ListDomains(context.Context, uuid.UUID) ([]Domain, error)
	SetDomainActive(context.Context, uuid.UUID, uuid.UUID, bool) error
	PurgeAnnualData(context.Context, int, time.Time, func(context.Context, string) error) (CleanupReport, error)
}

type CleanupReport struct {
	AcademicYear             int `json:"academic_year"`
	VerificationDocuments    int `json:"verification_documents_removed"`
	VerificationRequests     int `json:"verification_requests_removed"`
	AccountsPromotedToSenior int `json:"accounts_promoted"`
}
