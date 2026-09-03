package results

import (
	"context"
	"errors"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/admissions"
)

var (
	ErrNotFound           = errors.New("result not found")
	ErrInvalidInput       = errors.New("invalid result input")
	ErrConflict           = errors.New("result conflict")
	ErrInvalidQuery       = errors.New("invalid result query")
	ErrInvalidStatus      = errors.New("invalid result status")
	ErrAdminRequired      = errors.New("administrator role is required")
	ErrInvalidWillingness = errors.New("willingness must be 0, 20, 40, 60, 80, or 100")
)

var validWillingness = map[int16]struct{}{0: {}, 20: {}, 40: {}, 60: {}, 80: {}, 100: {}}
var threeDigitCode = regexp.MustCompile(`^[0-9]{3}$`)
var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

const (
	ResultStatusAdmitted   = "admitted"
	ResultStatusWaitlisted = "waitlisted"
	ResultStatusRejected   = "rejected"
	ResultStatusUnknown    = "unknown"

	ResultBatchStatusProcessing    = "processing"
	ResultBatchStatusPendingReview = "pending_review"
	ResultBatchStatusApproved      = "approved"
	ResultBatchStatusRejected      = "rejected"
	ResultBatchStatusPublished     = "published"
	ResultBatchStatusSuperseded    = "superseded"

	InquiryRoundResultReleased     = "result_released"
	InquiryRoundAcceptanceDeadline = "acceptance_deadline"
)

type Report struct {
	ApplicationID         uuid.UUID `json:"application_id"`
	ResultStatus          string    `json:"result_status"`
	OfficialRank          *int      `json:"official_rank,omitempty"`
	Quota                 *int      `json:"quota,omitempty"`
	FrontCandidateCount   int       `json:"front_candidate_count"`
	FrontResponseCount    int       `json:"front_response_count"`
	PositionAfterDeclines *int      `json:"position_after_declines,omitempty"`
	ReferenceProbability  *float64  `json:"reference_probability,omitempty"`
	CurrentWillingness    *int16    `json:"current_willingness,omitempty"`
	CandidateNumberLast4  string    `json:"candidate_number_last4,omitempty"`
	CandidateNumberSet    bool      `json:"candidate_number_set"`
}

type CandidateNumberInput struct {
	CandidateNumber string `json:"candidate_number"`
}

type WillingnessInput struct {
	Value     int16      `json:"value"`
	InquiryID *uuid.UUID `json:"inquiry_id,omitempty"`
}

// WillingnessResponse is the privacy-minimized result returned after a
// response is applied to the program's numeric willingness array. It does
// not expose a user ID or candidate number.
type WillingnessResponse struct {
	ResponseID   int64  `json:"response_id"`
	AcademicYear int    `json:"academic_year"`
	SchoolCode   string `json:"school_code"`
	ProgramCode  string `json:"program_code"`
	ResultStatus string `json:"result_status"`
	OfficialRank int    `json:"admission_rank"`
	Willingness  int16  `json:"willingness"`
}

type Inquiry struct {
	ID                 uuid.UUID  `json:"id"`
	Round              string     `json:"round"`
	ResponseDeadline   *time.Time `json:"response_deadline,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	Responded          bool       `json:"responded"`
	CurrentWillingness *int16     `json:"current_willingness,omitempty"`
}

type OfficialResultRow struct {
	AcademicYear    int    `json:"academic_year"`
	SchoolCode      string `json:"school_code"`
	ProgramCode     string `json:"program_code"`
	CandidateNumber string `json:"candidate_number"`
	MaskedName      string `json:"masked_name"`
	ResultStatus    string `json:"result_status"`
	OfficialRank    *int   `json:"official_rank"`
	Quota           *int   `json:"quota"`
	SourcePage      int    `json:"source_page"`
}

func (r OfficialResultRow) Validate() error {
	if r.AcademicYear < 100 || r.AcademicYear > 999 || !threeDigitCode.MatchString(r.SchoolCode) || !threeDigitCode.MatchString(r.ProgramCode) || len(r.MaskedName) > 200 || r.SourcePage < 1 || r.SourcePage > 999 {
		return ErrInvalidInput
	}
	if _, err := NormalizeCandidateNumber(r.CandidateNumber); err != nil {
		return err
	}
	if !validResultStatus(r.ResultStatus) || (r.OfficialRank != nil && *r.OfficialRank < 1) || (r.Quota != nil && *r.Quota < 0) {
		return ErrInvalidInput
	}
	if isWillingnessResultStatus(r.ResultStatus) && r.OfficialRank == nil {
		return ErrInvalidInput
	}
	return nil
}

func (i ImportBatchInput) Validate() error {
	if i.AcademicYear < 100 || i.AcademicYear > 999 || !threeDigitCode.MatchString(i.SchoolCode) || len(i.SourceURL) > 2048 || len(i.Rows) == 0 || len(i.Rows) > 10000 || !sha256Hex.MatchString(strings.ToLower(strings.TrimSpace(i.SourceSHA256))) {
		return ErrInvalidInput
	}
	if err := ValidateOfficialSourceURL(i.SourceURL); err != nil {
		return ErrInvalidInput
	}
	for _, row := range i.Rows {
		if row.AcademicYear != i.AcademicYear || row.SchoolCode != i.SchoolCode {
			return ErrInvalidInput
		}
		if err := row.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// ValidateOfficialSourceURL keeps result evidence inside the official-source
// boundary. Search results may discover a URL, but the stored evidence must
// itself be on an official education or government domain.
func ValidateOfficialSourceURL(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" || value == admissions.MissingValue || admissions.ValidateOfficialURL(value) != nil {
		return ErrInvalidInput
	}
	return nil
}

type ImportBatchInput struct {
	AcademicYear int                 `json:"academic_year"`
	SchoolCode   string              `json:"school_code"`
	SourceURL    string              `json:"source_url"`
	SourceSHA256 string              `json:"source_sha256"`
	Rows         []OfficialResultRow `json:"rows"`
}

type OfficialResultCorrectionInput struct {
	ResultStatus string `json:"result_status"`
	OfficialRank *int   `json:"official_rank"`
	Quota        *int   `json:"quota"`
	MaskedName   string `json:"masked_name"`
	Reason       string `json:"reason"`
}

func (i OfficialResultCorrectionInput) Validate() error {
	if !validResultStatus(i.ResultStatus) || strings.TrimSpace(i.Reason) == "" || len(i.Reason) > 2000 || len(i.MaskedName) > 200 || (i.OfficialRank != nil && *i.OfficialRank < 1) || (i.Quota != nil && *i.Quota < 0) {
		return ErrInvalidInput
	}
	if isWillingnessResultStatus(i.ResultStatus) && i.OfficialRank == nil {
		return ErrInvalidInput
	}
	return nil
}

type AdminResultBatchQuery struct {
	AcademicYear int
	SchoolCode   string
	Status       string
	Limit        int
	Offset       int
}

type AdminResultBatch struct {
	ID           uuid.UUID  `json:"id"`
	AcademicYear int        `json:"academic_year"`
	SchoolCode   string     `json:"school_code"`
	SourceURL    string     `json:"source_url"`
	SourceSHA256 string     `json:"source_sha256"`
	Status       string     `json:"status"`
	ImportedAt   time.Time  `json:"imported_at"`
	ReviewedAt   *time.Time `json:"reviewed_at,omitempty"`
	ResultCount  int        `json:"result_count"`
	MatchedCount int        `json:"matched_count"`
	InquiryCount int        `json:"inquiry_count"`
}

type AdminResultRow struct {
	ID                   uuid.UUID `json:"id"`
	ProgramCode          string    `json:"program_code"`
	CandidateNumberLast4 string    `json:"candidate_number_last4"`
	MaskedName           string    `json:"masked_name"`
	ResultStatus         string    `json:"result_status"`
	OfficialRank         *int      `json:"official_rank,omitempty"`
	Quota                *int      `json:"quota,omitempty"`
	SourcePage           int       `json:"source_page"`
	ApplicationMatched   bool      `json:"application_matched"`
}

type AdminResultBatchDetail struct {
	Batch AdminResultBatch `json:"batch"`
	Rows  []AdminResultRow `json:"rows"`
}

func (q AdminResultBatchQuery) Validate() error {
	if q.AcademicYear != 0 && (q.AcademicYear < 100 || q.AcademicYear > 999) {
		return ErrInvalidQuery
	}
	if q.SchoolCode != "" && !threeDigitCode.MatchString(q.SchoolCode) {
		return ErrInvalidQuery
	}
	if q.Status != "" && !validBatchStatus(q.Status) {
		return ErrInvalidQuery
	}
	if q.Limit < 1 || q.Limit > 100 || q.Offset < 0 || q.Offset > 10000 {
		return ErrInvalidQuery
	}
	return nil
}

func validResultStatus(status string) bool {
	switch status {
	case ResultStatusAdmitted, ResultStatusWaitlisted, ResultStatusRejected, ResultStatusUnknown:
		return true
	default:
		return false
	}
}

func isWillingnessResultStatus(status string) bool {
	return status == ResultStatusAdmitted || status == ResultStatusWaitlisted
}

func validBatchStatus(status string) bool {
	switch status {
	case ResultBatchStatusProcessing, ResultBatchStatusPendingReview, ResultBatchStatusApproved, ResultBatchStatusRejected, ResultBatchStatusPublished, ResultBatchStatusSuperseded:
		return true
	default:
		return false
	}
}

type Repository interface {
	SetCandidateNumber(context.Context, uuid.UUID, uuid.UUID, []byte, []byte, string) error
	GetReport(context.Context, uuid.UUID, uuid.UUID) (Report, error)
	ListInquiries(context.Context, uuid.UUID, uuid.UUID) ([]Inquiry, error)
	SetWillingness(context.Context, uuid.UUID, uuid.UUID, int16, *uuid.UUID) (WillingnessResponse, error)
}

type Importer interface {
	IsAdmin(context.Context, uuid.UUID) (bool, error)
	ImportOfficialBatch(context.Context, uuid.UUID, ImportBatchInput) (uuid.UUID, error)
	PublishOfficialBatch(context.Context, uuid.UUID, uuid.UUID) error
	CreateAcceptanceDeadlineInquiries(context.Context, uuid.UUID, uuid.UUID, time.Time) error
	CorrectOfficialResult(context.Context, uuid.UUID, uuid.UUID, OfficialResultCorrectionInput) error
	ListAdminBatches(context.Context, uuid.UUID, AdminResultBatchQuery) ([]AdminResultBatch, error)
	GetAdminBatch(context.Context, uuid.UUID, uuid.UUID) (AdminResultBatchDetail, error)
}

func ValidateWillingness(value int16) error {
	if _, ok := validWillingness[value]; !ok {
		return ErrInvalidWillingness
	}
	return nil
}

func ReferenceProbability(values []int16) (*float64, int) {
	if len(values) == 0 {
		return nil, 0
	}
	var total int
	for _, value := range values {
		total += int(value)
	}
	average := float64(total) / float64(len(values))
	average = math.Round(average*100) / 100
	return &average, len(values)
}

func NormalizeCandidateNumber(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if len(value) < 4 || len(value) > 64 {
		return "", ErrInvalidInput
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || strings.ContainsRune("-_", character) {
			continue
		}
		return "", ErrInvalidInput
	}
	for _, character := range LastFour(value) {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') {
			continue
		}
		return "", ErrInvalidInput
	}
	return value, nil
}

func LastFour(value string) string {
	if len(value) <= 4 {
		return value
	}
	return value[len(value)-4:]
}
