package jobs

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	ExchangeName                     = "sta.events"
	ExtractRoutingKey                = "admissions.brochure.extract"
	ExtractedRoutingKey              = "admissions.brochure.extracted"
	CandidateListExtractRoutingKey   = "admissions.candidate-list.extract"
	CandidateListExtractedRoutingKey = "admissions.candidate-list.extracted"
	// CandidateListExtractedKey is retained for callers that used the initial
	// local-only candidate-list contract.
	CandidateListExtractedKey = CandidateListExtractedRoutingKey
	DeadLetterExchangeSuffix  = ".dlx"

	SourceTypeBrochure      = "brochure"
	SourceTypeCandidateList = "candidate_list"
)

var schoolCodePattern = regexp.MustCompile(`^[0-9]{3}$`)

type BrochureExtractJob struct {
	JobID         uuid.UUID `json:"job_id"`
	AcademicYear  int       `json:"academic_year"`
	SchoolCode    string    `json:"school_code"`
	StorageKey    string    `json:"storage_key"`
	SHA256Hex     string    `json:"sha256_hex"`
	RequestedAt   time.Time `json:"requested_at"`
	ProcessorHint string    `json:"processor_hint,omitempty"`
	// SourceType lets one Python extraction service process both brochures and
	// candidate lists. Empty values are treated as brochures for compatibility
	// with jobs created before the local-only extractor was introduced.
	SourceType  string `json:"source_type,omitempty"`
	SourceURL   string `json:"source_url,omitempty"`
	ProgramCode string `json:"program_code,omitempty"`
	// Traceparent carries the W3C trace context of the request that created
	// the job, so the extractor and the result callback can log the same
	// trace_id. Optional; older jobs have none.
	Traceparent string `json:"traceparent,omitempty"`
}

type BrochureExtractionResult struct {
	ResultType   string                `json:"result_type,omitempty"`
	JobID        uuid.UUID             `json:"job_id"`
	AcademicYear int                   `json:"academic_year"`
	SchoolCode   string                `json:"school_code"`
	SHA256Hex    string                `json:"sha256_hex"`
	Processor    string                `json:"processor"`
	Candidates   []ExtractionCandidate `json:"candidates"`
	GeneratedAt  time.Time             `json:"generated_at"`
}

type ExtractionCandidate struct {
	ProgramCode string         `json:"program_code"`
	Data        map[string]any `json:"data"`
	SourcePage  int            `json:"source_page,omitempty"`
	Confidence  *float64       `json:"confidence,omitempty"`
}

// CandidateListExtractionResult is the result contract for the same Python
// service when the source is an official candidate/result list. Candidate
// numbers and names exist only in this transient internal message; the Go
// result repository hashes the number and masks the name before persistence.
type CandidateListExtractionResult struct {
	ResultType   string             `json:"result_type"`
	JobID        uuid.UUID          `json:"job_id"`
	AcademicYear int                `json:"academic_year"`
	SchoolCode   string             `json:"school_code"`
	SHA256Hex    string             `json:"sha256_hex"`
	SourceURL    string             `json:"source_url,omitempty"`
	Processor    string             `json:"processor"`
	Rows         []CandidateListRow `json:"rows"`
	GeneratedAt  time.Time          `json:"generated_at"`
}

type CandidateListRow struct {
	ProgramCode     string `json:"program_code,omitempty"`
	CandidateNumber string `json:"candidate_number"`
	CandidateName   string `json:"candidate_name,omitempty"`
	MaskedName      string `json:"masked_name,omitempty"`
	ResultStatus    string `json:"result_status,omitempty"`
	OfficialRank    *int   `json:"official_rank,omitempty"`
	Quota           *int   `json:"quota,omitempty"`
	SourcePage      int    `json:"source_page"`
}

func (j BrochureExtractJob) Validate() error {
	if j.JobID == uuid.Nil || j.AcademicYear < 100 || j.AcademicYear > 999 || !schoolCodePattern.MatchString(j.SchoolCode) {
		return errors.New("invalid brochure extract job identity")
	}
	if strings.TrimSpace(j.StorageKey) == "" || len(j.StorageKey) > 1024 || strings.ContainsAny(j.StorageKey, "\x00\r\n") {
		return errors.New("invalid brochure storage key")
	}
	storageKey := strings.ReplaceAll(j.StorageKey, "\\", "/")
	if strings.HasPrefix(storageKey, "/") {
		return errors.New("brochure storage key must be relative")
	}
	for _, part := range strings.Split(storageKey, "/") {
		if part == ".." {
			return errors.New("brochure storage key contains parent traversal")
		}
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(j.SHA256Hex) {
		return errors.New("invalid brochure SHA-256")
	}
	if j.RequestedAt.IsZero() {
		return errors.New("requested_at is required")
	}
	if j.SourceType != "" && j.SourceType != SourceTypeBrochure && j.SourceType != SourceTypeCandidateList {
		return errors.New("invalid extraction source type")
	}
	if strings.TrimSpace(j.SourceURL) != "" && len(j.SourceURL) > 4096 {
		return errors.New("extraction source URL is too long")
	}
	if j.ProgramCode != "" && !schoolCodePattern.MatchString(j.ProgramCode) {
		return errors.New("invalid extraction program code")
	}
	return nil
}

func (j BrochureExtractJob) EffectiveSourceType() string {
	if strings.TrimSpace(j.SourceType) == "" {
		return SourceTypeBrochure
	}
	return j.SourceType
}

func (c ExtractionCandidate) Validate() error {
	if !schoolCodePattern.MatchString(c.ProgramCode) {
		return fmt.Errorf("invalid program code %q", c.ProgramCode)
	}
	if c.Data == nil {
		return errors.New("candidate data is required")
	}
	if c.SourcePage < 0 || c.SourcePage > 999 {
		return errors.New("invalid source page")
	}
	if c.Confidence != nil && (*c.Confidence < 0 || *c.Confidence > 1) {
		return errors.New("confidence must be between 0 and 1")
	}
	return nil
}

func (r BrochureExtractionResult) Validate() error {
	if r.ResultType != "" && r.ResultType != SourceTypeBrochure {
		return errors.New("invalid brochure result type")
	}
	if r.JobID == uuid.Nil || r.AcademicYear < 100 || r.AcademicYear > 999 || !schoolCodePattern.MatchString(r.SchoolCode) {
		return errors.New("invalid brochure extraction result identity")
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(r.SHA256Hex) {
		return errors.New("invalid brochure extraction result SHA-256")
	}
	if strings.TrimSpace(r.Processor) == "" || len(r.Processor) > 64 || r.GeneratedAt.IsZero() {
		return errors.New("invalid brochure extraction result metadata")
	}
	if len(r.Candidates) > 2000 {
		return errors.New("too many brochure extraction candidates")
	}
	seen := make(map[string]struct{}, len(r.Candidates))
	for _, candidate := range r.Candidates {
		if err := candidate.Validate(); err != nil {
			return err
		}
		if _, exists := seen[candidate.ProgramCode]; exists {
			return fmt.Errorf("duplicate brochure extraction candidate %q", candidate.ProgramCode)
		}
		seen[candidate.ProgramCode] = struct{}{}
	}
	return nil
}

func (r CandidateListRow) Validate() error {
	if strings.TrimSpace(r.CandidateNumber) == "" || len(r.CandidateNumber) > 64 {
		return errors.New("candidate number is required")
	}
	for _, character := range r.CandidateNumber {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || strings.ContainsRune("-_", character) {
			continue
		}
		return errors.New("candidate number contains invalid characters")
	}
	if len([]rune(r.CandidateName)) > 200 || len([]rune(r.MaskedName)) > 200 {
		return errors.New("candidate name is too long")
	}
	if strings.TrimSpace(r.CandidateName) == "" && strings.TrimSpace(r.MaskedName) == "" {
		return errors.New("candidate name is required")
	}
	if r.ProgramCode != "" && !schoolCodePattern.MatchString(r.ProgramCode) {
		return errors.New("invalid candidate list program code")
	}
	if r.SourcePage < 1 || r.SourcePage > 999 {
		return errors.New("invalid candidate list source page")
	}
	if r.OfficialRank != nil && *r.OfficialRank < 1 {
		return errors.New("invalid official rank")
	}
	if r.Quota != nil && *r.Quota < 0 {
		return errors.New("invalid quota")
	}
	return nil
}

func (r CandidateListExtractionResult) Validate() error {
	if r.ResultType != "" && r.ResultType != SourceTypeCandidateList {
		return errors.New("invalid candidate list result type")
	}
	if r.JobID == uuid.Nil || r.AcademicYear < 100 || r.AcademicYear > 999 || !schoolCodePattern.MatchString(r.SchoolCode) {
		return errors.New("invalid candidate list result identity")
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(strings.ToLower(r.SHA256Hex)) {
		return errors.New("invalid candidate list result SHA-256")
	}
	if strings.TrimSpace(r.Processor) == "" || len(r.Processor) > 64 || r.GeneratedAt.IsZero() {
		return errors.New("invalid candidate list result metadata")
	}
	if len(r.Rows) == 0 || len(r.Rows) > 10000 {
		return errors.New("candidate list rows are outside the allowed range")
	}
	for _, row := range r.Rows {
		if err := row.Validate(); err != nil {
			return err
		}
	}
	return nil
}
