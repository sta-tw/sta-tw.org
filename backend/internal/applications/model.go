package applications

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/admissions"
)

var (
	ErrConflict             = errors.New("application already exists")
	ErrNotFound             = errors.New("application not found")
	ErrVerificationRequired = errors.New("verified identity is required")
	ErrInvalidRequest       = errors.New("invalid application request")
	ErrAdminRequired        = errors.New("administrator role is required")
	ErrInvalidStatus        = errors.New("application ticket status is invalid")
)

type Application struct {
	ID                uuid.UUID  `json:"id"`
	ProgramIdentifier string     `json:"program_identifier"`
	SchoolCode        string     `json:"school_code"`
	SchoolName        string     `json:"school_name"`
	ProgramCode       string     `json:"program_code"`
	ProgramName       string     `json:"program_name"`
	AcademicYear      int        `json:"academic_year"`
	Status            string     `json:"status"`
	LockedAt          *time.Time `json:"locked_at,omitempty"`
}

type CreateInput struct {
	ProgramIdentifiers []string `json:"program_identifiers"`
}

type ServiceTicketInput struct {
	ProgramIdentifier string `json:"program_identifier"`
	Reason            string `json:"reason"`
}

type ServiceTicket struct {
	ID                uuid.UUID `json:"id"`
	ProgramIdentifier string    `json:"program_identifier"`
	Reason            string    `json:"reason"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
}

type ServiceTicketReviewInput struct {
	Approved bool   `json:"approved"`
	Reason   string `json:"reason"`
}

type Repository interface {
	IsAdmin(context.Context, uuid.UUID) (bool, error)
	CreateConfirmed(context.Context, uuid.UUID, []admissions.ProgramIdentifier) ([]Application, error)
	ListByAccount(context.Context, uuid.UUID) ([]Application, error)
	CreateServiceTicket(context.Context, uuid.UUID, admissions.ProgramIdentifier, string) (ServiceTicket, error)
	ListOpenServiceTickets(context.Context, uuid.UUID) ([]ServiceTicket, error)
	ReviewServiceTicket(context.Context, uuid.UUID, uuid.UUID, ServiceTicketReviewInput) (ServiceTicket, error)
}
