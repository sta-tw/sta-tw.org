package brochurediscovery

import "testing"

func TestCandidateRequiresTargetYearAndOfficialURLs(t *testing.T) {
	confidence := 0.9
	valid := CandidateInput{
		DetectedAcademicYear: 115,
		SourceURL:            "https://admission.example.edu.tw/selection/115",
		DocumentURL:          "https://admission.example.edu.tw/files/115.pdf",
		SHA256:               "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Confidence:           &confidence,
		Evidence:             map[string]any{"title": "115 academic year"},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid candidate rejected: %v", err)
	}
	futureCycle := valid
	futureCycle.DetectedAcademicYear = 116
	if err := futureCycle.Validate(); err != nil {
		t.Fatalf("a future configured cycle must be accepted: %v", err)
	}
	invalidYear := valid
	invalidYear.DetectedAcademicYear = 99
	if err := invalidYear.Validate(); err == nil {
		t.Fatal("out-of-range academic year must be rejected")
	}
	unofficial := valid
	unofficial.DocumentURL = "https://example.com/115.pdf"
	if err := unofficial.Validate(); err == nil {
		t.Fatal("unofficial document URL must be rejected")
	}
}

func TestCreateCycleAcademicYearRange(t *testing.T) {
	if err := (CreateCycleInput{AcademicYear: 116}).Validate(); err != nil {
		t.Fatalf("valid cycle rejected: %v", err)
	}
	if err := (CreateCycleInput{AcademicYear: 99}).Validate(); err == nil {
		t.Fatal("out-of-range cycle must fail")
	}
}

func TestDiscoveryStatusesAreFixed(t *testing.T) {
	for _, value := range []string{StatusCompleted, StatusUnderReview, StatusSearching, StatusPendingSearch, StatusNeedsAttention} {
		if !ValidStatus(value) {
			t.Fatalf("expected valid status %q", value)
		}
	}
	if ValidStatus("not_found") {
		t.Fatal("not_found is not part of the agreed lifecycle")
	}
}

func TestReviewRequiresReasonWhenRejected(t *testing.T) {
	if err := (ReviewInput{Approved: false}).Validate(); err == nil {
		t.Fatal("rejection without a reason must fail")
	}
	if err := (ReviewInput{Approved: true}).Validate(); err != nil {
		t.Fatalf("approval should not require a reason: %v", err)
	}
}
