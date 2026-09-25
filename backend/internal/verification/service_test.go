package verification

import (
	"testing"

	"github.com/google/uuid"
)

func TestValidateRequestInput(t *testing.T) {
	input, email, err := validateRequestInput(CreateRequestInput{
		AcademicYear: 115,
		SchoolCode:   "001",
		ProgramCode:  "023",
		SchoolEmail:  "Student@Mail.Example.edu.tw",
	}, true)
	if err != nil {
		t.Fatalf("validateRequestInput() error = %v", err)
	}
	if input.SchoolEmail != "student@mail.example.edu.tw" || email != input.SchoolEmail {
		t.Fatalf("normalized email = %q / %q", input.SchoolEmail, email)
	}
}

func TestValidateRequestInputRejectsInvalidDomainShape(t *testing.T) {
	if _, _, err := validateRequestInput(CreateRequestInput{
		AcademicYear: 115,
		SchoolCode:   "01",
		SchoolEmail:  "student@school.example",
	}, true); err == nil {
		t.Fatal("validateRequestInput() error = nil, want invalid school code")
	}
}

func TestCodeHashInputBindsRequest(t *testing.T) {
	left := codeHashInput(uuid.MustParse("00000000-0000-0000-0000-000000000001"), "123456")
	right := codeHashInput(uuid.MustParse("00000000-0000-0000-0000-000000000002"), "123456")
	if left == right {
		t.Fatal("verification hash input must bind the request id")
	}
}
