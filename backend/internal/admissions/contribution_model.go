package admissions

import (
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/auth"
)

const (
	ContributionApplicants = "applicants"
	ContributionSecond     = "second_stage"
	ContributionWaitlist   = "waitlist"
	ContributionBrochure   = "historical_brochure"
	ContributionPending    = "pending"
	ContributionAccepted   = "accepted"
	ContributionRejected   = "rejected"
	maxContributionRows    = 100
)

var (
	ErrInvalidContribution       = errors.New("admissions contribution is invalid")
	ErrContributionNotFound      = errors.New("admissions contribution was not found")
	ErrContributionInvalidStatus = errors.New("admissions contribution is no longer pending")
	ErrContributionReplyInvalid  = errors.New("admissions contribution reply is invalid")
)

// ContributionInput contains the public form values. Email is encrypted before
// it reaches Postgres; it is never returned by the public submission endpoint.
type ContributionInput struct {
	Email           string
	AcademicYear    int
	Category        string
	SchoolName      string
	ProgramRows     []ContributionProgramRow
	Files           []ContributionFile
	DataText        string
	ReferenceURL    string
	Nickname        string
	PublishNickname bool
}

type ContributionProgramRow struct {
	ProgramName string
	People      int
}

type ContributionFile struct {
	StorageKey string
	Filename   string
	MIMEType   string
	SizeBytes  int64
	SHA256     string
}

type ContributionAttachment struct {
	ID         uuid.UUID `json:"id"`
	Filename   string    `json:"file_name"`
	MIMEType   string    `json:"mime_type"`
	SizeBytes  int64     `json:"size_bytes"`
	StorageKey string    `json:"-"`
}

type ContributionReceipt struct {
	ID          uuid.UUID `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	EmailQueued bool      `json:"email_queued"`
	ForumPosted bool      `json:"forum_posted"`
}

// Contribution is only returned from the protected admissions admin API.
type Contribution struct {
	ID              uuid.UUID                `json:"id"`
	Email           string                   `json:"email"`
	EmailCiphertext []byte                   `json:"-"`
	AcademicYear    int                      `json:"academic_year"`
	Category        string                   `json:"category"`
	SchoolName      string                   `json:"school_name"`
	DataText        string                   `json:"data_text"`
	ReferenceURL    string                   `json:"reference_url"`
	Nickname        string                   `json:"nickname"`
	PublishNickname bool                     `json:"publish_nickname"`
	Status          string                   `json:"status"`
	AttachmentName  string                   `json:"attachment_name,omitempty"`
	AttachmentSize  int64                    `json:"attachment_size_bytes,omitempty"`
	HasAttachment   bool                     `json:"has_attachment"`
	Attachments     []ContributionAttachment `json:"attachments,omitempty"`
	StorageKey      string                   `json:"-"`
	ReviewedBy      *uuid.UUID               `json:"reviewed_by,omitempty"`
	ReviewedAt      *time.Time               `json:"reviewed_at,omitempty"`
	CreatedAt       time.Time                `json:"created_at"`
}

// ContributionReplyTarget is the minimum private data needed to reply to a
// report from its Discord forum thread.
type ContributionReplyTarget struct {
	ID              uuid.UUID
	EmailCiphertext []byte
}

type PublicContributor struct {
	Nickname   string    `json:"nickname"`
	ReviewedAt time.Time `json:"reviewed_at"`
}

func (input *ContributionInput) NormalizeAndValidate(hasAttachment bool) error {
	input.Email = auth.NormalizeEmail(input.Email)
	input.SchoolName = strings.TrimSpace(input.SchoolName)
	input.DataText = strings.TrimSpace(input.DataText)
	input.ReferenceURL = strings.TrimSpace(input.ReferenceURL)
	input.Nickname = strings.TrimSpace(input.Nickname)

	if input.AcademicYear < 104 || input.AcademicYear > 999 || len(input.Email) > 254 {
		return ErrInvalidContribution
	}
	parsedEmail, err := mail.ParseAddress(input.Email)
	if err != nil || parsedEmail.Address != input.Email || strings.ContainsAny(input.Email, "\r\n") {
		return ErrInvalidContribution
	}
	if input.Nickname == "" {
		input.PublishNickname = false
	}
	if len(input.Nickname) > 120 || len(input.SchoolName) > 240 {
		return ErrInvalidContribution
	}
	if input.ReferenceURL != "" && !validContributionURL(input.ReferenceURL) {
		return ErrInvalidContribution
	}

	switch input.Category {
	case ContributionApplicants, ContributionSecond, ContributionWaitlist:
		if input.SchoolName == "" || (hasAttachment && (len(input.ProgramRows) > 0 || input.DataText != "")) || (!hasAttachment && len(input.ProgramRows) == 0) {
			return ErrInvalidContribution
		}
		if hasAttachment {
			input.DataText = ""
			break
		}
		dataText, err := formatContributionRows(input.ProgramRows)
		if err != nil {
			return err
		}
		input.DataText = dataText
	case ContributionBrochure:
		if (!hasAttachment && input.ReferenceURL == "") || len(input.ProgramRows) > 0 || input.DataText != "" || len(input.Files) > 1 || (len(input.Files) == 1 && input.Files[0].MIMEType != "application/pdf") {
			return ErrInvalidContribution
		}
	default:
		return ErrInvalidContribution
	}
	return nil
}

func formatContributionRows(rows []ContributionProgramRow) (string, error) {
	if len(rows) == 0 || len(rows) > maxContributionRows {
		return "", ErrInvalidContribution
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		programName := strings.TrimSpace(row.ProgramName)
		if programName == "" || len(programName) > 240 || strings.ContainsAny(programName, "\r\n") || row.People < 0 || row.People > 1_000_000_000 {
			return "", ErrInvalidContribution
		}
		if strings.HasSuffix(programName, "系") {
			lines = append(lines, fmt.Sprintf("%s %d 人", programName, row.People))
		} else {
			lines = append(lines, fmt.Sprintf("%s 系 %d 人", programName, row.People))
		}
	}
	dataText := strings.Join(lines, "\n")
	if len(dataText) > 16000 {
		return "", ErrInvalidContribution
	}
	return dataText, nil
}

func parseContributionRowsText(raw string) ([]ContributionProgramRow, error) {
	if len(raw) == 0 || len(raw) > 16000 {
		return nil, ErrInvalidContribution
	}
	rows := make([]ContributionProgramRow, 0)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		programName, peopleText, found := strings.Cut(line, ",")
		if !found {
			programName, peopleText, found = strings.Cut(line, "，")
		}
		if !found {
			fields := strings.Fields(line)
			if len(fields) < 3 || fields[len(fields)-1] != "人" {
				return nil, ErrInvalidContribution
			}
			programName = strings.Join(fields[:len(fields)-2], " ")
			peopleText = fields[len(fields)-2]
		}
		people, err := strconv.Atoi(strings.TrimSpace(peopleText))
		if err != nil || people < 0 || people > 1_000_000_000 {
			return nil, ErrInvalidContribution
		}
		rows = append(rows, ContributionProgramRow{ProgramName: strings.TrimSpace(programName), People: people})
		if len(rows) > maxContributionRows {
			return nil, ErrInvalidContribution
		}
	}
	if len(rows) == 0 {
		return nil, ErrInvalidContribution
	}
	return rows, nil
}

func normalizeContributionDataText(raw string) string {
	rows, err := parseContributionRowsText(raw)
	if err != nil {
		return raw
	}
	dataText, err := formatContributionRows(rows)
	if err != nil {
		return raw
	}
	return dataText
}

func validContributionURL(raw string) bool {
	if len(raw) > 2048 {
		return false
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}
