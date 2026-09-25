package sources

import "testing"

func TestValidateInputRequiresOfficialEvidence(t *testing.T) {
	input := Input{
		SchoolCode:   "001",
		AcademicYear: 116,
		SourceURL:    "https://admission.example.edu.tw/notice/",
		Evidence: []Evidence{{
			URL:     "https://www.example.edu.tw/admissions",
			Locator: "#notice-116",
			Text:    "招生簡章入口",
		}},
	}
	normalized, err := ValidateInput(input)
	if err != nil {
		t.Fatalf("ValidateInput() error = %v", err)
	}
	if normalized.Status != StatusCandidate || normalized.SourceType != "official_entry" {
		t.Fatalf("defaults = %#v", normalized)
	}
}

func TestValidateInputRejectsThirdPartySourceAndMissingEvidence(t *testing.T) {
	for _, input := range []Input{
		{SchoolCode: "001", AcademicYear: 116, SourceURL: "https://example.com/notice"},
		{SchoolCode: "001", AcademicYear: 116, SourceURL: "https://admission.example.edu.tw/notice", Evidence: []Evidence{{URL: "https://example.com", Locator: "#x", Text: "x"}}},
	} {
		if _, err := ValidateInput(input); err == nil {
			t.Fatalf("ValidateInput(%#v) error = nil", input)
		}
	}
	invalidSchool := Input{
		SchoolCode:   "0a1",
		AcademicYear: 116,
		SourceURL:    "https://admission.example.edu.tw/notice",
		Evidence:     []Evidence{{URL: "https://www.example.edu.tw/admissions", Locator: "#x", Text: "x"}},
	}
	if _, err := ValidateInput(invalidSchool); err == nil {
		t.Fatal("ValidateInput(non-numeric school code) error = nil")
	}
}

func TestNormalizeURL(t *testing.T) {
	normalized, hostname, err := NormalizeURL("HTTPS://Admission.Example.EDU.TW/notice/")
	if err != nil {
		t.Fatalf("NormalizeURL() error = %v", err)
	}
	if normalized != "https://admission.example.edu.tw/notice" || hostname != "admission.example.edu.tw" {
		t.Fatalf("normalized = %q, hostname = %q", normalized, hostname)
	}
}
