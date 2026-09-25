package results

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestReferenceProbabilityAveragesWillingnessValues(t *testing.T) {
	probability, count := ReferenceProbability([]int16{0, 60, 20})
	if probability == nil || *probability != 26.67 || count != 3 {
		t.Fatalf("ReferenceProbability() = %v, %d; want 26.67, 3", probability, count)
	}
	probability, count = ReferenceProbability(nil)
	if probability != nil || count != 0 {
		t.Fatalf("ReferenceProbability(nil) = %v, %d", probability, count)
	}
}

func TestWillingnessValues(t *testing.T) {
	for _, value := range []int16{0, 20, 40, 60, 80, 100} {
		if err := ValidateWillingness(value); err != nil {
			t.Errorf("ValidateWillingness(%d) error = %v", value, err)
		}
	}
	for _, value := range []int16{-1, 1, 19, 101} {
		if err := ValidateWillingness(value); err == nil {
			t.Errorf("ValidateWillingness(%d) error = nil", value)
		}
	}
}

func TestCandidateNumberNormalizationAndLastFour(t *testing.T) {
	value, err := NormalizeCandidateNumber("  AB-123456  ")
	if err != nil || value != "AB-123456" {
		t.Fatalf("NormalizeCandidateNumber() = %q, %v", value, err)
	}
	if got := LastFour(value); got != "3456" {
		t.Fatalf("LastFour() = %q", got)
	}
}

func TestCandidateNumberMatchesStoredLastFourConstraint(t *testing.T) {
	for _, raw := range []string{"ABC", "ABCD-", "AB__"} {
		if _, err := NormalizeCandidateNumber(raw); err == nil {
			t.Errorf("NormalizeCandidateNumber(%q) error = nil", raw)
		}
	}
}

func TestOfficialResultValidationRejectsUnsafeRows(t *testing.T) {
	valid := OfficialResultRow{
		AcademicYear: 116, SchoolCode: "001", ProgramCode: "023", CandidateNumber: "AB-123456",
		MaskedName: "王○○", ResultStatus: ResultStatusWaitlisted, OfficialRank: intPtr(4), Quota: intPtr(3), SourcePage: 42,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid result row rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*OfficialResultRow)
	}{
		{name: "source page zero", mutate: func(row *OfficialResultRow) { row.SourcePage = 0 }},
		{name: "source page too large", mutate: func(row *OfficialResultRow) { row.SourcePage = 1000 }},
		{name: "rank zero", mutate: func(row *OfficialResultRow) { row.OfficialRank = intPtr(0) }},
		{name: "negative quota", mutate: func(row *OfficialResultRow) { row.Quota = intPtr(-1) }},
		{name: "unknown status", mutate: func(row *OfficialResultRow) { row.ResultStatus = "selected" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			row := valid
			test.mutate(&row)
			if err := row.Validate(); err == nil {
				t.Fatal("expected invalid row error")
			}
		})
	}
}

func TestOfficialResultValidationRequiresRankForWillingnessSlots(t *testing.T) {
	row := OfficialResultRow{
		AcademicYear: 116, SchoolCode: "001", ProgramCode: "023", CandidateNumber: "AB-123456",
		MaskedName: "王○○", ResultStatus: ResultStatusAdmitted, SourcePage: 42,
	}
	if err := row.Validate(); err == nil {
		t.Fatal("admitted result without an official rank should be rejected")
	}

	row.ResultStatus = ResultStatusRejected
	if err := row.Validate(); err != nil {
		t.Fatalf("rejected result without a rank should remain valid: %v", err)
	}
}

func TestResultQueriesAndCorrectionsValidateBoundaries(t *testing.T) {
	valid := AdminResultBatchQuery{AcademicYear: 116, SchoolCode: "001", Status: ResultBatchStatusPendingReview, Limit: 50, Offset: 0}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid query rejected: %v", err)
	}
	for _, query := range []AdminResultBatchQuery{
		{Limit: 0, Offset: 0},
		{Limit: 101, Offset: 0},
		{Limit: 50, Offset: -1},
		{AcademicYear: 99, Limit: 50},
		{SchoolCode: "1", Limit: 50},
		{Status: "draft", Limit: 50},
	} {
		if err := query.Validate(); err == nil {
			t.Errorf("query %#v unexpectedly valid", query)
		}
	}

	correction := OfficialResultCorrectionInput{ResultStatus: ResultStatusWaitlisted, OfficialRank: intPtr(2), Quota: intPtr(1), MaskedName: "王○○", Reason: "官方更正通知"}
	if err := correction.Validate(); err != nil {
		t.Fatalf("valid correction rejected: %v", err)
	}
	correction.Reason = "   "
	if err := correction.Validate(); err == nil {
		t.Fatal("blank correction reason unexpectedly valid")
	}
}

func TestOfficialSourceURLValidation(t *testing.T) {
	for _, source := range []string{
		"https://admission.example.edu.tw/result.pdf",
		"https://www.example.gov.tw/admission/result",
		"http://admission.example.edu.tw/result.pdf",
		"https://admission.example.edu.tw:443/result.pdf",
	} {
		if err := ValidateOfficialSourceURL(source); err != nil {
			t.Errorf("ValidateOfficialSourceURL(%q) error = %v", source, err)
		}
	}
	for _, source := range []string{
		"https://example.com/result.pdf",
		"https://school.edu.tw.example.com/result.pdf",
		"https://admission.example.edu.tw:8443/result.pdf",
		"https://user:pass@example.edu.tw/result.pdf",
		"https://127.0.0.1/result.pdf",
	} {
		if err := ValidateOfficialSourceURL(source); err == nil {
			t.Errorf("ValidateOfficialSourceURL(%q) unexpectedly accepted", source)
		}
	}
}

func TestResultResponsesNeverExposeFullCandidateNumber(t *testing.T) {
	row := AdminResultRow{ID: uuid.New(), ProgramCode: "023", CandidateNumberLast4: "3456", MaskedName: "王○○", ResultStatus: ResultStatusWaitlisted, SourcePage: 42}
	payload, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(payload)
	if strings.Contains(encoded, "AB-123456") || strings.Contains(encoded, "candidate_number\":") {
		t.Fatalf("admin result response contains a full candidate number: %s", encoded)
	}
	if !strings.Contains(encoded, `"candidate_number_last4":"3456"`) {
		t.Fatalf("admin result response omitted last four digits: %s", encoded)
	}
}

func TestWillingnessResponseContainsOnlyCrossCheckFields(t *testing.T) {
	response := WillingnessResponse{
		ResponseID: 123, AcademicYear: 116, SchoolCode: "001", ProgramCode: "023",
		ResultStatus: ResultStatusWaitlisted, OfficialRank: 2, Willingness: 20,
	}
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(payload)
	for _, forbidden := range []string{"user_id", "account_id", "candidate_number"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("willingness response contains identity field %q: %s", forbidden, encoded)
		}
	}
	for _, expected := range []string{`"academic_year":116`, `"school_code":"001"`, `"program_code":"023"`, `"admission_rank":2`, `"willingness":20`} {
		if !strings.Contains(encoded, expected) {
			t.Fatalf("willingness response omitted %q: %s", expected, encoded)
		}
	}
}

func intPtr(value int) *int {
	return &value
}
