package portfolio

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound      = errors.New("portfolio resource not found")
	ErrConflict      = errors.New("portfolio resource conflict")
	ErrInvalidStatus = errors.New("invalid portfolio status transition")
	ErrInvalidQuery  = errors.New("invalid portfolio query")
	ErrForbidden     = errors.New("portfolio access forbidden")
	ErrNotAdmin      = errors.New("administrator role is required")
)

const (
	FileStatusHidden        = "hidden"
	FileStatusPendingReview = "pending_review"
	FileStatusPublished     = "published"
	FileStatusUnpublished   = "unpublished"
	FileStatusRejected      = "rejected"
)

type Project struct {
	ID            uuid.UUID `json:"id"`
	ApplicationID uuid.UUID `json:"application_id"`
	Title         string    `json:"title"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type File struct {
	ID               uuid.UUID `json:"id"`
	ProjectID        uuid.UUID `json:"project_id"`
	VersionNumber    int       `json:"version_number"`
	OriginalFileName string    `json:"original_file_name"`
	StorageKey       string    `json:"-"`
	MimeType         string    `json:"mime_type"`
	FileSizeBytes    int64     `json:"file_size_bytes"`
	SHA256Hex        string    `json:"sha256_hex"`
	Status           string    `json:"status"`
	RejectionReason  string    `json:"rejection_reason,omitempty"`
	OwnerAccountID   uuid.UUID `json:"-"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type AdminFile struct {
	File
	ApplicationID uuid.UUID `json:"application_id"`
	ProjectTitle  string    `json:"project_title"`
}

type FileEvent struct {
	ID             int64      `json:"id"`
	Action         string     `json:"action"`
	FromStatus     string     `json:"from_status,omitempty"`
	ToStatus       string     `json:"to_status,omitempty"`
	Reason         string     `json:"reason"`
	CreatedAt      time.Time  `json:"created_at"`
	ActorAccountID *uuid.UUID `json:"-"`
}

type AdminFileQuery struct {
	Status    string
	ProjectID uuid.UUID
	Limit     int
	Offset    int
}

type CreateProjectInput struct {
	ApplicationID uuid.UUID `json:"application_id"`
	Title         string    `json:"title"`
}

type ReviewInput struct {
	Approved bool   `json:"approved"`
	Reason   string `json:"reason"`
}

type Repository interface {
	CreateProject(context.Context, uuid.UUID, uuid.UUID, string) (Project, error)
	ListProjects(context.Context, uuid.UUID) ([]Project, error)
	CreateFile(context.Context, uuid.UUID, uuid.UUID, string, string, string, int64, string) (File, error)
	ListFiles(context.Context, uuid.UUID, uuid.UUID) ([]File, error)
	GetFile(context.Context, uuid.UUID) (File, error)
	ListFileEvents(context.Context, uuid.UUID, uuid.UUID) ([]FileEvent, error)
	SubmitForReview(context.Context, uuid.UUID, uuid.UUID) (File, error)
	Unpublish(context.Context, uuid.UUID, uuid.UUID) (File, error)
	Hide(context.Context, uuid.UUID, uuid.UUID) (File, error)
	ReviewFile(context.Context, uuid.UUID, uuid.UUID, bool, string) (File, error)
	IsAdmin(context.Context, uuid.UUID) (bool, error)
	ListAdminFiles(context.Context, uuid.UUID, AdminFileQuery) ([]AdminFile, error)
	ListAdminFileEvents(context.Context, uuid.UUID, uuid.UUID) ([]FileEvent, error)
}

func (query AdminFileQuery) Validate() error {
	if query.Status != "" && !validFileStatus(query.Status) {
		return ErrInvalidQuery
	}
	if query.Limit < 1 || query.Limit > 100 || query.Offset < 0 || query.Offset > 10000 {
		return ErrInvalidQuery
	}
	return nil
}

func validFileStatus(status string) bool {
	switch status {
	case FileStatusHidden, FileStatusPendingReview, FileStatusPublished, FileStatusUnpublished, FileStatusRejected:
		return true
	default:
		return false
	}
}
