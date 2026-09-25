package admissions

import (
	"context"
	"io"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

type brochureTestBlobStore struct{}

func (brochureTestBlobStore) Put(context.Context, string, io.Reader, int64, string) error {
	return nil
}

func (brochureTestBlobStore) Remove(context.Context, string) error {
	return nil
}

func (brochureTestBlobStore) PresignGet(context.Context, string, time.Duration) (*url.URL, error) {
	return url.Parse("https://storage.example.test/signed.pdf")
}

func TestParseBrochurePathUsesCanonicalSchoolCode(t *testing.T) {
	request := httptest.NewRequest("GET", "/", nil)
	request.SetPathValue("academicYear", "116")
	request.SetPathValue("schoolCode", "001")
	year, schoolCode, err := parseBrochurePath(request)
	if err != nil || year != 116 || schoolCode != "001" {
		t.Fatalf("parseBrochurePath() = %d, %q, %v", year, schoolCode, err)
	}

	for _, schoolCode := range []string{"000", "1", "abc"} {
		request.SetPathValue("schoolCode", schoolCode)
		if _, _, err := parseBrochurePath(request); err == nil {
			t.Errorf("parseBrochurePath accepted school code %q", schoolCode)
		}
	}
}

func TestParseOptionalBrochureYear(t *testing.T) {
	if year, err := parseOptionalBrochureYear(""); err != nil || year != 0 {
		t.Fatalf("empty year = %d, %v", year, err)
	}
	if year, err := parseOptionalBrochureYear("116"); err != nil || year != 116 {
		t.Fatalf("valid year = %d, %v", year, err)
	}
	for _, raw := range []string{"99", "1000", "abc"} {
		if _, err := parseOptionalBrochureYear(raw); err == nil {
			t.Errorf("parseOptionalBrochureYear accepted %q", raw)
		}
	}
}

func TestBrochureDownloadResponseIsNeverCached(t *testing.T) {
	handler := BrochureHandler{blobStore: brochureTestBlobStore{}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/", nil)
	handler.writeDownload(recorder, request, BrochureDocument{AcademicYear: 116, SchoolCode: "001"})
	if recorder.Code != 200 {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}
