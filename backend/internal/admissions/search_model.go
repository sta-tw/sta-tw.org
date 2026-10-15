package admissions

import "context"

type BrochureSearchFilter string

const (
	BrochureSearchAny         BrochureSearchFilter = ""
	BrochureSearchRequired    BrochureSearchFilter = "required"
	BrochureSearchNotRequired BrochureSearchFilter = "not-required"
)

type BrochureSearchQuery struct {
	Search     string
	SkillsTest BrochureSearchFilter
	Interview  BrochureSearchFilter
	Portfolio  BrochureSearchFilter
	Limit      int
	Offset     int
}

// BrochureSearchProgram contains only the fields needed to draw one search
// result card. The full admissions record remains available from the detail
// endpoint after the visitor opens a card.
type BrochureSearchProgram struct {
	AcademicYear         int    `json:"academic_year"`
	ProgramIdentifier    string `json:"program_identifier"`
	SchoolName           string `json:"school_name"`
	AdmissionProgramName string `json:"admission_program_name"`
	AdmissionQuota       int    `json:"admission_quota"`
	AdmissionGroup       string `json:"admission_group"`
	CrossGroup           string `json:"cross_group"`
	AdmissionCategory    string `json:"admission_category"`
}

type BrochureSearchRepository interface {
	SearchBrochurePrograms(context.Context, BrochureSearchQuery) ([]BrochureSearchProgram, int, error)
}
