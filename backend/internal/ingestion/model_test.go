package ingestion

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/jobs"
)

func TestDispatchErrorIsRetryable(t *testing.T) {
	err := DispatchError{err: errors.New("broker unavailable")}
	if !errors.Is(err, ErrDispatchUnavailable) {
		t.Fatal("DispatchError does not unwrap to ErrDispatchUnavailable")
	}
	retryable, ok := any(err).(interface{ Retryable() bool })
	if !ok || !retryable.Retryable() {
		t.Fatal("DispatchError is not retryable")
	}
}

func TestValidateExtractionResultRejectsDuplicateProgramCodes(t *testing.T) {
	result := jobs.BrochureExtractionResult{
		JobID:        uuid.New(),
		AcademicYear: 116,
		SchoolCode:   "001",
		SHA256Hex:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Processor:    jobs.ExtractedRoutingKey,
		GeneratedAt:  fixedTestTime(),
		Candidates: []jobs.ExtractionCandidate{
			{ProgramCode: "023", Data: map[string]any{"raw": "one"}},
			{ProgramCode: "023", Data: map[string]any{"raw": "two"}},
		},
	}
	if err := result.Validate(); err == nil {
		t.Fatal("Validate() accepted duplicate program codes")
	}
}

func fixedTestTime() (value time.Time) {
	return time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
}
