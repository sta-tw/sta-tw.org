package content

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/pagination"
)

var (
	ErrNotFound      = errors.New("content resource not found")
	ErrForbidden     = errors.New("content access forbidden")
	ErrConflict      = errors.New("content conflict")
	ErrInvalidStatus = errors.New("content status transition is invalid")
	ErrAdminRequired = errors.New("administrator role is required")
)

type Space struct {
	ID           uuid.UUID `json:"id"`
	SpaceType    string    `json:"space_type"`
	DisplayName  string    `json:"display_name"`
	AcademicYear *int      `json:"academic_year,omitempty"`
	SchoolCode   string    `json:"school_code,omitempty"`
	ProgramCode  string    `json:"program_code,omitempty"`
	Joined       bool      `json:"joined"`
}

type Thread struct {
	ID        uuid.UUID `json:"id"`
	SpaceID   uuid.UUID `json:"space_id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Post struct {
	ID                 uuid.UUID  `json:"id"`
	ThreadID           uuid.UUID  `json:"thread_id"`
	Body               string     `json:"body"`
	QuotedExperienceID *uuid.UUID `json:"quoted_experience_id,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
}

type Experience struct {
	ID                uuid.UUID `json:"id"`
	AuthorType        string    `json:"author_type"`
	AdmissionOutcome  string    `json:"admission_outcome"`
	Visibility        string    `json:"visibility"`
	CurrentRevisionID uuid.UUID `json:"current_revision_id"`
	Title             string    `json:"title"`
	Body              string    `json:"body"`
	RevisionNumber    int       `json:"revision_number"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type CreateExperienceInput struct {
	Title            string `json:"title"`
	Body             string `json:"body"`
	AuthorType       string `json:"author_type"`
	AdmissionOutcome string `json:"admission_outcome"`
}

type CreateRevisionInput struct {
	Title            string `json:"title"`
	Body             string `json:"body"`
	AuthorType       string `json:"author_type"`
	AdmissionOutcome string `json:"admission_outcome"`
}

type ReviewInput struct {
	Approved bool   `json:"approved"`
	Reason   string `json:"reason"`
}

type Repository interface {
	ListSpaces(context.Context, *uuid.UUID) ([]Space, error)
	JoinSpace(context.Context, uuid.UUID, uuid.UUID) error
	LeaveSpace(context.Context, uuid.UUID, uuid.UUID) error
	ListThreads(context.Context, *uuid.UUID, uuid.UUID, int, pagination.Cursor) ([]Thread, string, error)
	ListPosts(context.Context, *uuid.UUID, uuid.UUID, int, pagination.Cursor) ([]Post, string, error)
	CreateThread(context.Context, uuid.UUID, uuid.UUID, string, string) (Thread, Post, error)
	CreatePost(context.Context, uuid.UUID, uuid.UUID, string, *uuid.UUID) (Post, error)
	CreateExperience(context.Context, uuid.UUID, CreateExperienceInput) (Experience, error)
	CreateRevision(context.Context, uuid.UUID, uuid.UUID, CreateRevisionInput) (Experience, error)
	SubmitRevision(context.Context, uuid.UUID, uuid.UUID) (Experience, error)
	UnpublishExperience(context.Context, uuid.UUID, uuid.UUID) error
	ListPublishedExperiences(context.Context, int, pagination.Cursor) ([]Experience, string, error)
	GetExperience(context.Context, *uuid.UUID, uuid.UUID) (Experience, error)
	ReviewExperience(context.Context, uuid.UUID, uuid.UUID, ReviewInput) (Experience, error)
	IsAdmin(context.Context, uuid.UUID) (bool, error)
}

func ValidateText(title, body string) error {
	if strings.TrimSpace(title) == "" || len(title) > 300 || strings.TrimSpace(body) == "" || len(body) > 500000 {
		return errors.New("text is invalid")
	}
	return nil
}
