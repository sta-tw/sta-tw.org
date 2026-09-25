package admissions

import (
	"errors"
	"testing"
)

func TestProgramIdentifierRoundTrip(t *testing.T) {
	identifier, err := ParseProgramIdentifier("116-001-023")
	if err != nil {
		t.Fatalf("ParseProgramIdentifier() error = %v", err)
	}
	if got := identifier.String(); got != "116-001-023" {
		t.Fatalf("String() = %q", got)
	}
}

func TestProgramIdentifierRejectsNonCanonicalValues(t *testing.T) {
	for _, raw := range []string{"16-001-023", "116-1-023", "116-001-23", "116/001/023", "116-001-abc"} {
		if _, err := ParseProgramIdentifier(raw); err == nil {
			t.Errorf("ParseProgramIdentifier(%q) error = nil", raw)
		}
	}
}

func TestValidateOfficialURL(t *testing.T) {
	for _, raw := range []string{
		"https://admission.example.edu.tw/notice.pdf",
		"http://www.example.gov.tw/notice",
		"https://admission.example.edu.tw:443/notice.pdf",
		"https://admission.example.edu.tw./notice.pdf",
	} {
		if err := ValidateOfficialURL(raw); err != nil {
			t.Errorf("ValidateOfficialURL(%q) error = %v", raw, err)
		}
	}

	for _, raw := range []string{
		"",
		"-",
		"https://school.edu.tw.example.com/notice.pdf",
		"https://drive.google.com/notice.pdf",
		"https://example.edu.tw:8443/notice.pdf",
		"https://user:password@example.edu.tw/notice.pdf",
		"https://127.0.0.1/notice.pdf",
		"javascript:alert(1)",
	} {
		if err := ValidateOfficialURL(raw); err != nil && (raw == "" || raw == "-") {
			t.Errorf("ValidateOfficialURL(%q) error = %v, want optional value accepted", raw, err)
		} else if err == nil && raw != "" && raw != "-" {
			t.Errorf("ValidateOfficialURL(%q) error = nil, want rejection", raw)
		}
	}
}

func TestProgramValidationAllowsMissingSourceLocatorOnly(t *testing.T) {
	program := Program{
		AcademicYear:                  116,
		ProgramIdentifier:             "116-001-023",
		SchoolCode:                    "001",
		SchoolName:                    "測試大學",
		ProgramCode:                   "023",
		AdmissionProgramName:          "國文學系",
		AdmissionQuota:                3,
		ConsultationPhone:             MissingValue,
		BrochureURL:                   MissingValue,
		SpecialTalentTarget:           MissingValue,
		DifferentEducationBackgrounds: MissingValue,
		DifferentEducationOther:       MissingValue,
		Notes:                         MissingValue,
		SourceLocator:                 MissingValue,
		RegistrationFee:               MissingValue,
		ExamLocation:                  MissingValue,
		RecommendationLetterDeadline:  MissingValue,
		PortfolioDeadline:             MissingValue,
		CheckinWaitlistProcess:        MissingValue,
		FeeReductionEligibility:       MissingValue,
		AdmissionGroup:                MissingValue,
		CrossGroup:                    MissingValue,
		AdmissionCategory:             MissingValue,
		PriorityAdmission:             MissingValue,
		PortfolioRequired:             MissingValue,
		RecommendationLetterType:      MissingValue,
		MaxApplicablePrograms:         MissingValue,
		ApplicantCount:                MissingValue,
		InterviewCount:                MissingValue,
		AdmittedCount:                 MissingValue,
		WaitlistedCount:               MissingValue,
		AdmissionRate:                 MissingValue,
		FirstStagePassRate:            MissingValue,
		CompetitionRatio:              MissingValue,
		SchoolOfficialURL:             MissingValue,
		DepartmentOfficialURL:         MissingValue,
		ExamItems: []ExamItem{{
			Name:          "書審",
			SortOrder:     1,
			WeightPercent: floatPointer(100),
			Description:   MissingValue,
			SourcePage:    MissingValue,
		}},
	}
	if err := program.Validate(); err != nil {
		t.Fatalf("Program.Validate() error = %v", err)
	}
}

func TestProgramInputDerivesIdentifierAndLocator(t *testing.T) {
	page := 23
	input := ProgramInput{
		AcademicYear:                  116,
		SchoolCode:                    "001",
		SchoolName:                    "測試大學",
		ProgramCode:                   "023",
		AdmissionProgramName:          "特殊選材國文學系甲組",
		AdmissionQuota:                3,
		BrochureIsTentative:           false,
		ConsultationPhone:             "02-1234-5678",
		BrochureURL:                   "https://example.edu.tw/admission/116.pdf",
		SpecialTalentTarget:           "具特殊才能、特殊成就或專業表現之學生",
		DifferentEducationBackgrounds: "高中、同等學力及其他教育資歷均可報名",
		DifferentEducationOther:       "不同教育資歷須依簡章檢附相應證明文件",
		Notes:                         "本筆資料為完整建檔測試資料",
		SourcePage:                    &page,
		ExamItems: []ExamItem{
			{
				Name:          "書面審查",
				SortOrder:     1,
				WeightPercent: floatPointer(60),
				Description:   "審查學習歷程、作品與特殊才能證明",
				SourcePage:    "23",
			},
			{
				Name:          "面試",
				SortOrder:     2,
				WeightPercent: floatPointer(40),
				Description:   "評估專業興趣、表達能力與學習動機",
				SourcePage:    "24",
			},
		},
	}

	program, err := input.Materialize()
	if err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	if program.ProgramIdentifier != "116-001-023" {
		t.Fatalf("ProgramIdentifier = %q, want 116-001-023", program.ProgramIdentifier)
	}
	if program.SourceLocator != "001-023" {
		t.Fatalf("SourceLocator = %q, want 001-023", program.SourceLocator)
	}
	if program.WillingnessValues == nil || len(program.WillingnessValues) != 0 {
		t.Fatalf("WillingnessValues = %#v, want an empty array", program.WillingnessValues)
	}
}

func TestProgramInputAllowsMissingPageWithoutManualLocator(t *testing.T) {
	input := ProgramInput{
		AcademicYear:                  116,
		SchoolCode:                    "001",
		SchoolName:                    "測試大學",
		ProgramCode:                   "023",
		AdmissionProgramName:          "國文學系",
		AdmissionQuota:                3,
		ConsultationPhone:             MissingValue,
		BrochureURL:                   MissingValue,
		SpecialTalentTarget:           MissingValue,
		DifferentEducationBackgrounds: MissingValue,
		DifferentEducationOther:       MissingValue,
		Notes:                         MissingValue,
		ExamItems: []ExamItem{{
			Name:          "書審",
			SortOrder:     1,
			WeightPercent: floatPointer(100),
			Description:   MissingValue,
			SourcePage:    MissingValue,
		}},
	}

	program, err := input.Materialize()
	if err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	if program.ProgramIdentifier != "116-001-023" {
		t.Fatalf("ProgramIdentifier = %q, want 116-001-023", program.ProgramIdentifier)
	}
	if program.SourceLocator != MissingValue {
		t.Fatalf("SourceLocator = %q, want %q", program.SourceLocator, MissingValue)
	}
}

func minimalProgramInput() ProgramInput {
	return ProgramInput{
		AcademicYear:                  116,
		SchoolCode:                    "001",
		SchoolName:                    "測試大學",
		ProgramCode:                   "023",
		AdmissionProgramName:          "國文學系",
		AdmissionQuota:                3,
		ConsultationPhone:             MissingValue,
		BrochureURL:                   MissingValue,
		SpecialTalentTarget:           MissingValue,
		DifferentEducationBackgrounds: MissingValue,
		DifferentEducationOther:       MissingValue,
		Notes:                         MissingValue,
		ExamItems: []ExamItem{{
			Name:          "書審",
			SortOrder:     1,
			WeightPercent: floatPointer(100),
			Description:   MissingValue,
			SourcePage:    MissingValue,
		}},
	}
}

// TestTimelineEventsAllowEmptyList locks in that TimelineEvents, unlike
// ExamItems, may legitimately be empty — a program whose schedule hasn't
// been extracted yet must still be a valid program.
func TestTimelineEventsAllowEmptyList(t *testing.T) {
	input := minimalProgramInput()
	if _, err := input.Materialize(); err != nil {
		t.Fatalf("Materialize() with no timeline events error = %v, want nil", err)
	}
}

func TestTimelineEventsMaterializeNormalizesAndOrders(t *testing.T) {
	input := minimalProgramInput()
	input.TimelineEvents = []TimelineEvent{
		{Name: "  網路登錄報名  ", StartDate: "-", StartTime: "09:00", EndTime: "17:00", SortOrder: 1, Notes: ""},
		{Name: "榜單公告", StartDate: "2026-11-24", SortOrder: 2, Notes: ""},
	}

	program, err := input.Materialize()
	if err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	if len(program.TimelineEvents) != 2 {
		t.Fatalf("TimelineEvents = %#v, want 2 events", program.TimelineEvents)
	}
	first := program.TimelineEvents[0]
	if first.Name != "網路登錄報名" || first.StartDate != MissingValue || first.StartTime != "09:00" || first.EndTime != "17:00" || first.Notes != MissingValue {
		t.Fatalf("first event = %#v, want trimmed name, missing-value date, preserved times, missing-value notes", first)
	}
	second := program.TimelineEvents[1]
	if second.StartDate != "2026-11-24" || second.EndDate != MissingValue {
		t.Fatalf("second event = %#v, want start date 2026-11-24 and missing-value end date", second)
	}
}

func TestTimelineEventsRejectDuplicateSortOrder(t *testing.T) {
	input := minimalProgramInput()
	input.TimelineEvents = []TimelineEvent{
		{Name: "初試結果公告", StartDate: MissingValue, SortOrder: 1},
		{Name: "複試繳費期限", StartDate: MissingValue, SortOrder: 1},
	}
	if err := input.Validate(); !errors.Is(err, ErrInvalidProgram) {
		t.Fatalf("Validate() with duplicate sort_order error = %v, want ErrInvalidProgram", err)
	}
}

func TestTimelineEventsRejectMalformedDate(t *testing.T) {
	input := minimalProgramInput()
	input.TimelineEvents = []TimelineEvent{
		{Name: "榜單公告", StartDate: "115年1月9日", SortOrder: 1},
	}
	if err := input.Validate(); !errors.Is(err, ErrInvalidProgram) {
		t.Fatalf("Validate() with a non-ISO date error = %v, want ErrInvalidProgram", err)
	}
}

func TestTimelineEventsRejectBlankName(t *testing.T) {
	input := minimalProgramInput()
	input.TimelineEvents = []TimelineEvent{
		{Name: "   ", StartDate: MissingValue, SortOrder: 1},
	}
	if err := input.Validate(); !errors.Is(err, ErrInvalidProgram) {
		t.Fatalf("Validate() with a blank event name error = %v, want ErrInvalidProgram", err)
	}
}

func floatPointer(value float64) *float64 { return &value }
