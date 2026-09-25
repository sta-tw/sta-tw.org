package admissions

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProgramInputCanResolveCanonicalSchoolName(t *testing.T) {
	page := 23
	input := ProgramInput{
		AcademicYear:         116,
		SchoolCode:           "001",
		ProgramCode:          "023",
		AdmissionProgramName: "特殊選材國文學系甲組",
		AdmissionQuota:       3,
		SourcePage:           &page,
		ExamItems: []ExamItem{{
			Name:          "書面審查",
			SortOrder:     1,
			WeightPercent: floatPointer(100),
		}},
	}
	if err := input.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	program, err := input.MaterializeWithSchoolName("國立政治大學")
	if err != nil {
		t.Fatalf("MaterializeWithSchoolName() error = %v", err)
	}
	if program.SchoolName != "國立政治大學" {
		t.Fatalf("SchoolName = %q, want canonical school name", program.SchoolName)
	}
	if program.ProgramIdentifier != "116-001-023" || program.SourceLocator != "001-023" {
		t.Fatalf("derived fields = %q / %q", program.ProgramIdentifier, program.SourceLocator)
	}
	if program.ExamItems[0].SourcePage != MissingValue || program.ExamItems[0].Description != MissingValue {
		t.Fatalf("exam item defaults = %#v", program.ExamItems[0])
	}
}

func TestProgramBatchValidationRejectsDuplicateIdentifiers(t *testing.T) {
	input := ProgramInput{
		AcademicYear:         116,
		SchoolCode:           "001",
		ProgramCode:          "023",
		AdmissionProgramName: "國文學系",
		ExamItems: []ExamItem{{
			Name:          "書審",
			SortOrder:     1,
			WeightPercent: floatPointer(100),
		}},
	}
	batch := ProgramBatchInput{Reason: "測試", Items: []ProgramInput{input, input}}
	if err := batch.Validate(); err == nil {
		t.Fatal("duplicate program identifiers should be rejected")
	}
}

func TestProgramInputRejectsInvalidExamItems(t *testing.T) {
	input := ProgramInput{
		AcademicYear:         116,
		SchoolCode:           "001",
		ProgramCode:          "023",
		AdmissionProgramName: "國文學系",
		ExamItems: []ExamItem{
			{Name: "書審", SortOrder: 1, WeightPercent: floatPointer(60)},
			{Name: "面試", SortOrder: 1, WeightPercent: floatPointer(50)},
		},
	}
	if err := input.Validate(); err == nil {
		t.Fatal("duplicate exam item sort orders should be rejected")
	}

	input.ExamItems = []ExamItem{{Name: "書審", SortOrder: 1, WeightPercent: floatPointer(101)}}
	if err := input.Validate(); err == nil {
		t.Fatal("weight percentages above 100 should be rejected")
	}
}

func TestParseProgramAdminQuery(t *testing.T) {
	request := httptest.NewRequest("GET", "/api/v1/admin/admissions/programs?academic_year=116&school_code=001&review_status=pending&q=國文&limit=20&offset=10", nil)
	query, err := parseProgramAdminQuery(request)
	if err != nil {
		t.Fatalf("parseProgramAdminQuery() error = %v", err)
	}
	if query.AcademicYear != 116 || query.SchoolCode != "001" || query.ReviewStatus != ProgramStatusPending || query.Limit != 20 || query.Offset != 10 {
		t.Fatalf("query = %#v", query)
	}
}

func TestAdmissionAdminPayloadRejectsDerivedAndCanonicalFields(t *testing.T) {
	for _, field := range []string{"program_identifier", "source_locator", "school_name", "willingness_values"} {
		request := httptest.NewRequest("POST", "/api/v1/admin/admissions/programs/sync", strings.NewReader(`{"reason":"test","items":[{"academic_year":116,"school_code":"001","program_code":"023","admission_program_name":"國文學系","admission_quota":1,"exam_items":[{"name":"書審","sort_order":1,"weight_percent":100}],"`+field+`":"forged"}]}`))
		var input ProgramBatchInput
		if err := decodeBrochureJSON(request, &input); err == nil {
			t.Errorf("decodeBrochureJSON accepted derived field %q", field)
		}
	}
}

func TestProgramReviewRejectRequiresReason(t *testing.T) {
	if err := (ProgramReviewInput{Approved: false}).Validate(); err == nil {
		t.Fatal("rejected program review without reason should fail")
	}
	if err := (ProgramReviewInput{Approved: true}).Validate(); err != nil {
		t.Fatalf("approved program review should allow an empty reason: %v", err)
	}
}
