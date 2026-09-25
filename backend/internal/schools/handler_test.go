package schools

import (
	"net/http/httptest"
	"testing"
)

func TestWriteSchoolListDoesNotCacheAdminResultsPublicly(t *testing.T) {
	query := Query{Text: "", Limit: 1}
	items := []School{{SchoolCode: "001", SchoolName: "測試學校", IsActive: true}}

	adminRecorder := httptest.NewRecorder()
	writeSchoolList(adminRecorder, query, items, false)
	if got := adminRecorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("admin cache policy = %q, want no-store", got)
	}

	publicRecorder := httptest.NewRecorder()
	writeSchoolList(publicRecorder, query, items, true)
	if got := publicRecorder.Header().Get("Cache-Control"); got != "public, max-age=300" {
		t.Fatalf("public cache policy = %q, want public cache", got)
	}
}
