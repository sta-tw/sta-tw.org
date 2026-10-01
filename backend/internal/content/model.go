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

// AdminThread is a moderation-only view of a thread: every public Thread
// field plus the poster's identity and current status, neither of which the
// public API exposes (forum_threads.account_id is recorded on every row
// precisely so moderators can trace authorship; ordinary readers never see
// it). Returned only from the admin forum endpoints, behind requireAdmin.
type AdminThread struct {
	ID             uuid.UUID `json:"id"`
	SpaceID        uuid.UUID `json:"space_id"`
	Title          string    `json:"title"`
	Status         string    `json:"status"`
	AccountID      uuid.UUID `json:"account_id"`
	AuthorUsername string    `json:"author_username"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// AdminPost mirrors AdminThread for a single post/reply.
type AdminPost struct {
	ID                 uuid.UUID  `json:"id"`
	ThreadID           uuid.UUID  `json:"thread_id"`
	Body               string     `json:"body"`
	QuotedExperienceID *uuid.UUID `json:"quoted_experience_id,omitempty"`
	Status             string     `json:"status"`
	AccountID          uuid.UUID  `json:"account_id"`
	AuthorUsername     string     `json:"author_username"`
	CreatedAt          time.Time  `json:"created_at"`
}

// Thread status values a moderator may set via SetThreadStatus.
// ThreadStatusArchived is for content kept as evidence (e.g. a possible
// legal matter) rather than ordinary moderation — same public invisibility
// as ThreadStatusRemoved, just a different admin-facing intent/label.
const (
	ThreadStatusPublished = "published"
	ThreadStatusLocked    = "locked"
	ThreadStatusRemoved   = "removed"
	ThreadStatusArchived  = "archived"
)

// Post status values a moderator may set via SetPostStatus. See
// ThreadStatusArchived for what PostStatusArchived means.
const (
	PostStatusPublished = "published"
	PostStatusRemoved   = "removed"
	PostStatusArchived  = "archived"
)

type Post struct {
	ID                 uuid.UUID       `json:"id"`
	ThreadID           uuid.UUID       `json:"thread_id"`
	Body               string          `json:"body"`
	QuotedExperienceID *uuid.UUID      `json:"quoted_experience_id,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	Reactions          []ReactionTally `json:"reactions,omitempty"`
}

// ReactionTally is one emoji's count on a post or experience, with whether the
// caller reacted with it.
type ReactionTally struct {
	Emoji string `json:"emoji"`
	Count int    `json:"count"`
	Mine  bool   `json:"mine"`
}

// Reaction target types stored in content_reactions.target_type.
const (
	ReactionTargetPost       = "forum_post"
	ReactionTargetExperience = "experience"
	maxReactionLength        = 32
)

var ErrInvalidReaction = errors.New("invalid reaction")

// NormalizeReaction trims and bounds an emoji / :shortcode: token.
func NormalizeReaction(raw string) (string, error) {
	token := strings.TrimSpace(raw)
	if token == "" || len(token) > maxReactionLength {
		return "", ErrInvalidReaction
	}
	for _, r := range token {
		if r < 0x20 || r == 0x7f {
			return "", ErrInvalidReaction
		}
	}
	return token, nil
}

type Experience struct {
	ID                uuid.UUID       `json:"id"`
	AuthorType        string          `json:"author_type"`
	AdmissionOutcome  string          `json:"admission_outcome"`
	Visibility        string          `json:"visibility"`
	CurrentRevisionID uuid.UUID       `json:"current_revision_id"`
	Title             string          `json:"title"`
	Body              string          `json:"body"`
	RevisionNumber    int             `json:"revision_number"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
	Reactions         []ReactionTally `json:"reactions,omitempty"`
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
	SetReaction(ctx context.Context, targetType string, targetID, accountID uuid.UUID, emoji string) error
	RemoveReaction(ctx context.Context, targetType string, targetID, accountID uuid.UUID, emoji string) error
	IsAdmin(context.Context, uuid.UUID) (bool, error)
	AdminListThreads(ctx context.Context, limit int, after pagination.Cursor) ([]AdminThread, string, error)
	AdminListPosts(ctx context.Context, threadID uuid.UUID) ([]AdminPost, error)
	SetThreadStatus(ctx context.Context, threadID uuid.UUID, status string) error
	SetPostStatus(ctx context.Context, postID uuid.UUID, status string) error
}

func ValidateText(title, body string) error {
	if strings.TrimSpace(title) == "" || len(title) > 300 || strings.TrimSpace(body) == "" || len(body) > 500000 {
		return errors.New("text is invalid")
	}
	return nil
}
