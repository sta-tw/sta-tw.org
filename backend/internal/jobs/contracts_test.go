package jobs

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestBrochureExtractJobValidation(t *testing.T) {
	job := BrochureExtractJob{
		JobID:        uuid.New(),
		AcademicYear: 116,
		SchoolCode:   "001",
		StorageKey:   "brochures/116/001/116-001.pdf",
		SHA256Hex:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		RequestedAt:  time.Now(),
	}
	if err := job.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	job.SchoolCode = "1"
	if err := job.Validate(); err == nil {
		t.Fatal("Validate(invalid school) error = nil")
	}
}
