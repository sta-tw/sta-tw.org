package admissions

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/idna"
)

const MissingValue = "-"

// ValidateOfficialURL enforces the source boundary for public admissions
// material. It intentionally performs no network request; fetchers must still
// validate redirects and resolved addresses at connection time.
func ValidateOfficialURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == MissingValue {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return ErrInvalidProgram
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return ErrInvalidProgram
	}
	hostname := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if hostname == "" {
		return ErrInvalidProgram
	}
	hostname, err = idna.Lookup.ToASCII(hostname)
	if err != nil || net.ParseIP(hostname) != nil {
		return ErrInvalidProgram
	}
	port := parsed.Port()
	if port != "" && port != "80" && port != "443" {
		return ErrInvalidProgram
	}
	if !strings.HasSuffix(hostname, ".edu.tw") && !strings.HasSuffix(hostname, ".gov.tw") {
		return ErrInvalidProgram
	}
	if hostname == "edu.tw" || hostname == "gov.tw" {
		return ErrInvalidProgram
	}
	return nil
}

var codePattern = regexp.MustCompile(`^[0-9]{3}$`)

var (
	ErrInvalidIdentifier = errors.New("invalid program identifier")
	ErrInvalidProgram    = errors.New("invalid admission program")
	ErrNotFound          = errors.New("admission program not found")
	ErrInvalidStatus     = errors.New("invalid admission program status")
	ErrAdminRequired     = errors.New("administrator role is required")
)

type ProgramIdentifier struct {
	AcademicYear int
	SchoolCode   string
	ProgramCode  string
}

func (id ProgramIdentifier) String() string {
	return fmt.Sprintf("%03d-%s-%s", id.AcademicYear, id.SchoolCode, id.ProgramCode)
}

func ParseProgramIdentifier(raw string) (ProgramIdentifier, error) {
	parts := strings.Split(raw, "-")
	if len(parts) != 3 || len(parts[0]) != 3 || !validSchoolCode(parts[1]) || !validProgramCode(parts[2]) {
		return ProgramIdentifier{}, ErrInvalidIdentifier
	}
	year, err := strconv.Atoi(parts[0])
	if err != nil || year < 100 || year > 999 {
		return ProgramIdentifier{}, ErrInvalidIdentifier
	}
	return ProgramIdentifier{AcademicYear: year, SchoolCode: parts[1], ProgramCode: parts[2]}, nil
}

type Program struct {
	AcademicYear                  int        `json:"academic_year"`
	ProgramIdentifier             string     `json:"program_identifier"`
	SchoolCode                    string     `json:"school_code"`
	SchoolName                    string     `json:"school_name"`
	ProgramCode                   string     `json:"program_code"`
	AdmissionProgramName          string     `json:"admission_program_name"`
	AdmissionQuota                int        `json:"admission_quota"`
	WillingnessValues             []int16    `json:"willingness_values"`
	ExamItems                     []ExamItem `json:"exam_items"`
	BrochureIsTentative           bool       `json:"brochure_is_tentative"`
	BrochureAnnouncementDate      string     `json:"brochure_announcement_date"`
	BrochureScheduledDate         string     `json:"brochure_scheduled_date"`
	RegistrationStartDate         string     `json:"registration_start_date"`
	RegistrationEndDate           string     `json:"registration_end_date"`
	ExamStartDate                 string     `json:"exam_start_date"`
	ExamEndDate                   string     `json:"exam_end_date"`
	ResultDate                    string     `json:"result_date"`
	ConsultationPhone             string     `json:"consultation_phone"`
	BrochureURL                   string     `json:"brochure_url"`
	SpecialTalentTarget           string     `json:"special_talent_target"`
	DifferentEducationBackgrounds string     `json:"different_education_backgrounds"`
	DifferentEducationOther       string     `json:"different_education_other"`
	Notes                         string     `json:"notes"`
	SourceLocator                 string     `json:"source_locator"`
}

// ProgramInput is the manual admission-data form. The public identifiers are
// deliberately absent: program_identifier and source_locator are derived from
// the input fields and must never be accepted as editable data.
type ProgramInput struct {
	AcademicYear int    `json:"academic_year"`
	SchoolCode   string `json:"school_code"`
	// SchoolName is resolved from the school master by the repository and is
	// intentionally not accepted from API clients.
	SchoolName                    string     `json:"-"`
	ProgramCode                   string     `json:"program_code"`
	AdmissionProgramName          string     `json:"admission_program_name"`
	AdmissionQuota                int        `json:"admission_quota"`
	ExamItems                     []ExamItem `json:"exam_items"`
	BrochureIsTentative           bool       `json:"brochure_is_tentative"`
	BrochureAnnouncementDate      string     `json:"brochure_announcement_date"`
	BrochureScheduledDate         string     `json:"brochure_scheduled_date"`
	RegistrationStartDate         string     `json:"registration_start_date"`
	RegistrationEndDate           string     `json:"registration_end_date"`
	ExamStartDate                 string     `json:"exam_start_date"`
	ExamEndDate                   string     `json:"exam_end_date"`
	ResultDate                    string     `json:"result_date"`
	ConsultationPhone             string     `json:"consultation_phone"`
	BrochureURL                   string     `json:"brochure_url"`
	SpecialTalentTarget           string     `json:"special_talent_target"`
	DifferentEducationBackgrounds string     `json:"different_education_backgrounds"`
	DifferentEducationOther       string     `json:"different_education_other"`
	Notes                         string     `json:"notes"`
	SourcePage                    *int       `json:"source_page,omitempty"`
}

// Materialize derives the two system fields from the manual input. The
// database generated columns remain the final source of truth when persisted.
func (input ProgramInput) Materialize() (Program, error) {
	return input.materialize(input.SchoolName)
}

// MaterializeWithSchoolName lets the repository fill the canonical school
// name from schools before validating and persisting the input.
func (input ProgramInput) MaterializeWithSchoolName(schoolName string) (Program, error) {
	return input.materialize(schoolName)
}

func (input ProgramInput) materialize(schoolName string) (Program, error) {
	input = normalizeProgramInput(input)
	identifier, err := input.identifier()
	if err != nil {
		return Program{}, err
	}
	locator, err := sourceLocator(input.SchoolCode, input.SourcePage)
	if err != nil {
		return Program{}, err
	}
	program := Program{
		AcademicYear:                  input.AcademicYear,
		ProgramIdentifier:             identifier.String(),
		SchoolCode:                    input.SchoolCode,
		SchoolName:                    strings.TrimSpace(schoolName),
		ProgramCode:                   input.ProgramCode,
		AdmissionProgramName:          input.AdmissionProgramName,
		AdmissionQuota:                input.AdmissionQuota,
		WillingnessValues:             make([]int16, 0),
		ExamItems:                     input.ExamItems,
		BrochureIsTentative:           input.BrochureIsTentative,
		BrochureAnnouncementDate:      input.BrochureAnnouncementDate,
		BrochureScheduledDate:         input.BrochureScheduledDate,
		RegistrationStartDate:         input.RegistrationStartDate,
		RegistrationEndDate:           input.RegistrationEndDate,
		ExamStartDate:                 input.ExamStartDate,
		ExamEndDate:                   input.ExamEndDate,
		ResultDate:                    input.ResultDate,
		ConsultationPhone:             input.ConsultationPhone,
		BrochureURL:                   input.BrochureURL,
		SpecialTalentTarget:           input.SpecialTalentTarget,
		DifferentEducationBackgrounds: input.DifferentEducationBackgrounds,
		DifferentEducationOther:       input.DifferentEducationOther,
		Notes:                         input.Notes,
		SourceLocator:                 locator,
	}
	if err := program.Validate(); err != nil {
		return Program{}, err
	}
	return program, nil
}

// Validate checks all client-supplied fields except SchoolName, which is
// deliberately resolved from the school master. It is used before opening a
// database transaction so malformed batches fail early.
func (input ProgramInput) Validate() error {
	input = normalizeProgramInput(input)
	if _, err := input.identifier(); err != nil {
		return err
	}
	if strings.TrimSpace(input.AdmissionProgramName) == "" || len([]rune(input.AdmissionProgramName)) > 500 || input.AdmissionQuota < 0 {
		return ErrInvalidProgram
	}
	if err := ValidateOfficialURL(input.BrochureURL); err != nil {
		return err
	}
	for _, value := range []string{
		input.BrochureAnnouncementDate, input.BrochureScheduledDate,
		input.RegistrationStartDate, input.RegistrationEndDate,
		input.ExamStartDate, input.ExamEndDate, input.ResultDate,
		input.ConsultationPhone, input.BrochureURL, input.SpecialTalentTarget,
		input.DifferentEducationBackgrounds, input.DifferentEducationOther, input.Notes,
	} {
		if len([]rune(value)) > 10000 {
			return ErrInvalidProgram
		}
	}
	if !validDateRange(input.RegistrationStartDate, input.RegistrationEndDate) || !validDateRange(input.ExamStartDate, input.ExamEndDate) {
		return ErrInvalidProgram
	}
	for _, value := range []string{
		input.BrochureAnnouncementDate, input.BrochureScheduledDate,
		input.RegistrationStartDate, input.RegistrationEndDate,
		input.ExamStartDate, input.ExamEndDate, input.ResultDate,
	} {
		if value != MissingValue {
			if _, err := time.Parse("2006-01-02", value); err != nil {
				return ErrInvalidProgram
			}
		}
	}
	if input.SourcePage != nil && (*input.SourcePage < 1 || *input.SourcePage > 999) {
		return ErrInvalidProgram
	}
	if len(input.ExamItems) == 0 || len(input.ExamItems) > 100 {
		return ErrInvalidProgram
	}
	seenSortOrders := make(map[int]struct{}, len(input.ExamItems))
	for _, item := range input.ExamItems {
		if err := validateExamItem(item); err != nil {
			return err
		}
		if _, exists := seenSortOrders[item.SortOrder]; exists {
			return ErrInvalidProgram
		}
		seenSortOrders[item.SortOrder] = struct{}{}
	}
	return nil
}

func (input ProgramInput) identifier() (ProgramIdentifier, error) {
	if input.AcademicYear < 100 || input.AcademicYear > 999 ||
		!validSchoolCode(input.SchoolCode) || !validProgramCode(input.ProgramCode) {
		return ProgramIdentifier{}, ErrInvalidProgram
	}
	return ProgramIdentifier{
		AcademicYear: input.AcademicYear,
		SchoolCode:   input.SchoolCode,
		ProgramCode:  input.ProgramCode,
	}, nil
}

func sourceLocator(schoolCode string, sourcePage *int) (string, error) {
	if !validSchoolCode(schoolCode) {
		return "", ErrInvalidProgram
	}
	if sourcePage == nil {
		return MissingValue, nil
	}
	if *sourcePage < 1 || *sourcePage > 999 {
		return "", ErrInvalidProgram
	}
	return fmt.Sprintf("%s-%03d", schoolCode, *sourcePage), nil
}

type ExamItem struct {
	Name          string   `json:"name"`
	SortOrder     int      `json:"sort_order"`
	WeightPercent *float64 `json:"weight_percent,omitempty"`
	Multiplier    *float64 `json:"multiplier,omitempty"`
	Description   string   `json:"description"`
	SourcePage    string   `json:"source_page"`
}

type School struct {
	SchoolCode string `json:"school_code"`
	SchoolName string `json:"school_name"`
}

type BrochureDocument struct {
	AcademicYear     int        `json:"academic_year"`
	SchoolCode       string     `json:"school_code"`
	OriginalFileName string     `json:"original_file_name"`
	MIMEType         string     `json:"mime_type"`
	FileSizeBytes    int64      `json:"file_size_bytes"`
	SHA256           string     `json:"sha256"`
	SourceURL        string     `json:"source_url"`
	ReviewStatus     string     `json:"review_status"`
	PublishedAt      *time.Time `json:"published_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	storageKey       string
}

type BrochureEvent struct {
	ID           int64     `json:"id"`
	AcademicYear int       `json:"academic_year"`
	SchoolCode   string    `json:"school_code"`
	Action       string    `json:"action"`
	FromStatus   string    `json:"from_status,omitempty"`
	ToStatus     string    `json:"to_status,omitempty"`
	OriginalName string    `json:"original_file_name"`
	SHA256       string    `json:"sha256"`
	Reason       string    `json:"reason"`
	CreatedAt    time.Time `json:"created_at"`
}

type BrochureDocumentInput struct {
	AcademicYear     int
	SchoolCode       string
	OriginalFileName string
	StorageKey       string
	MIMEType         string
	FileSizeBytes    int64
	SHA256           string
	SourceURL        string
}

// BrochureExtractionDispatcher is intentionally a narrow boundary: uploading
// a PDF creates a durable asynchronous job, while the admissions package does
// not know how RabbitMQ or the Python worker are deployed.
type BrochureExtractionDispatcher interface {
	QueueBrochureExtraction(context.Context, uuid.UUID, int, string, string, string) error
}

// BrochureExtractionJobDispatcher is implemented by the built-in ingestion
// service so upload callers can receive the durable job ID. The embedded
// legacy interface keeps lightweight adapters source-compatible.
type BrochureExtractionJobDispatcher interface {
	BrochureExtractionDispatcher
	QueueBrochureExtractionWithID(context.Context, uuid.UUID, int, string, string, string) (uuid.UUID, error)
}

type BrochureRepository interface {
	IsAdmin(context.Context, uuid.UUID) (bool, error)
	CreateBrochure(context.Context, uuid.UUID, BrochureDocumentInput) (BrochureDocument, string, error)
	ListBrochures(context.Context, uuid.UUID, int) ([]BrochureDocument, error)
	ListBrochureEvents(context.Context, uuid.UUID, int, string) ([]BrochureEvent, error)
	ReviewBrochure(context.Context, uuid.UUID, int, string, bool, string) (BrochureDocument, error)
	SetBrochurePublished(context.Context, uuid.UUID, int, string, bool, string) (BrochureDocument, error)
	GetBrochure(context.Context, uuid.UUID, int, string) (BrochureDocument, error)
	GetPublishedBrochure(context.Context, int, string) (BrochureDocument, error)
}

// SystemBrochureRepository is the narrow write boundary for authenticated
// ingestion clients. It intentionally does not expose admin review or
// publication operations.
type SystemBrochureRepository interface {
	CreateBrochureSystem(context.Context, BrochureDocumentInput) (BrochureDocument, string, error)
}

type ProgramQuery struct {
	AcademicYear int
	SchoolCode   string
	Search       string
	Limit        int
	Offset       int
}

func (p Program) Validate() error {
	identifier, err := ParseProgramIdentifier(p.ProgramIdentifier)
	if err != nil || identifier.AcademicYear != p.AcademicYear || identifier.SchoolCode != p.SchoolCode || identifier.ProgramCode != p.ProgramCode {
		return ErrInvalidProgram
	}
	if strings.TrimSpace(p.SchoolName) == "" || strings.TrimSpace(p.AdmissionProgramName) == "" || p.AdmissionQuota < 0 {
		return ErrInvalidProgram
	}
	if err := ValidateOfficialURL(p.BrochureURL); err != nil {
		return err
	}
	for _, value := range []string{
		p.BrochureAnnouncementDate, p.BrochureScheduledDate,
		p.RegistrationStartDate, p.RegistrationEndDate,
		p.ExamStartDate, p.ExamEndDate, p.ResultDate,
		p.ConsultationPhone, p.BrochureURL, p.SpecialTalentTarget,
		p.DifferentEducationBackgrounds, p.DifferentEducationOther, p.Notes,
		p.SourceLocator,
	} {
		if strings.TrimSpace(value) == "" {
			return ErrInvalidProgram
		}
	}
	if !validDateRange(p.RegistrationStartDate, p.RegistrationEndDate) || !validDateRange(p.ExamStartDate, p.ExamEndDate) {
		return ErrInvalidProgram
	}
	for _, value := range []string{
		p.BrochureAnnouncementDate, p.BrochureScheduledDate,
		p.RegistrationStartDate, p.RegistrationEndDate,
		p.ExamStartDate, p.ExamEndDate, p.ResultDate,
	} {
		if value != MissingValue {
			if _, err := time.Parse("2006-01-02", value); err != nil {
				return ErrInvalidProgram
			}
		}
	}
	if p.SourceLocator != MissingValue {
		parts := strings.Split(p.SourceLocator, "-")
		if len(parts) != 2 || parts[0] != p.SchoolCode || len(parts[1]) != 3 || !codePattern.MatchString(parts[1]) {
			return ErrInvalidProgram
		}
	}
	if len(p.ExamItems) == 0 || len(p.ExamItems) > 100 {
		return ErrInvalidProgram
	}
	seenSortOrders := make(map[int]struct{}, len(p.ExamItems))
	for _, item := range p.ExamItems {
		if err := validateExamItem(item); err != nil {
			return ErrInvalidProgram
		}
		if _, exists := seenSortOrders[item.SortOrder]; exists {
			return ErrInvalidProgram
		}
		seenSortOrders[item.SortOrder] = struct{}{}
	}
	return nil
}

func validSchoolCode(value string) bool {
	return codePattern.MatchString(value) && value != "000"
}

func validProgramCode(value string) bool {
	return codePattern.MatchString(value) && value != "000"
}

func validateExamItem(item ExamItem) error {
	if strings.TrimSpace(item.Name) == "" || len([]rune(item.Name)) > 500 || item.SortOrder < 1 || (item.WeightPercent == nil && item.Multiplier == nil) {
		return ErrInvalidProgram
	}
	if len([]rune(item.Description)) > 10000 || len([]rune(item.SourcePage)) > 16 {
		return ErrInvalidProgram
	}
	if item.WeightPercent != nil && (*item.WeightPercent < 0 || *item.WeightPercent > 100) {
		return ErrInvalidProgram
	}
	if item.Multiplier != nil && *item.Multiplier < 0 {
		return ErrInvalidProgram
	}
	if _, err := parseExamItemPage(item.SourcePage); err != nil {
		return err
	}
	return nil
}

func parseExamItemPage(value string) (*int, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == MissingValue {
		return nil, nil
	}
	page, err := strconv.Atoi(value)
	if err != nil || page < 1 || page > 999 {
		return nil, ErrInvalidProgram
	}
	return &page, nil
}

func normalizeProgramInput(input ProgramInput) ProgramInput {
	input.SchoolCode = strings.TrimSpace(input.SchoolCode)
	input.ProgramCode = strings.TrimSpace(input.ProgramCode)
	input.AdmissionProgramName = strings.TrimSpace(input.AdmissionProgramName)
	input.BrochureAnnouncementDate = normalizeDash(input.BrochureAnnouncementDate)
	input.BrochureScheduledDate = normalizeDash(input.BrochureScheduledDate)
	input.RegistrationStartDate = normalizeDash(input.RegistrationStartDate)
	input.RegistrationEndDate = normalizeDash(input.RegistrationEndDate)
	input.ExamStartDate = normalizeDash(input.ExamStartDate)
	input.ExamEndDate = normalizeDash(input.ExamEndDate)
	input.ResultDate = normalizeDash(input.ResultDate)
	input.ConsultationPhone = normalizeDash(input.ConsultationPhone)
	input.BrochureURL = normalizeDash(input.BrochureURL)
	input.SpecialTalentTarget = normalizeDash(input.SpecialTalentTarget)
	input.DifferentEducationBackgrounds = normalizeDash(input.DifferentEducationBackgrounds)
	input.DifferentEducationOther = normalizeDash(input.DifferentEducationOther)
	input.Notes = normalizeDash(input.Notes)
	for index := range input.ExamItems {
		input.ExamItems[index].Name = strings.TrimSpace(input.ExamItems[index].Name)
		input.ExamItems[index].Description = normalizeDash(input.ExamItems[index].Description)
		input.ExamItems[index].SourcePage = normalizeDash(input.ExamItems[index].SourcePage)
		if page, err := parseExamItemPage(input.ExamItems[index].SourcePage); err == nil && page != nil {
			input.ExamItems[index].SourcePage = strconv.Itoa(*page)
		}
	}
	return input
}

func normalizeDash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return MissingValue
	}
	return value
}

func validDateRange(start, end string) bool {
	if start == MissingValue || end == MissingValue {
		return true
	}
	parsedStart, err := time.Parse("2006-01-02", start)
	if err != nil {
		return false
	}
	parsedEnd, err := time.Parse("2006-01-02", end)
	return err == nil && !parsedEnd.Before(parsedStart)
}
