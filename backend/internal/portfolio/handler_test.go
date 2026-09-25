package portfolio

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"sta-backend/internal/auth"
)

func TestWritePortfolioJSONDisablesCaching(t *testing.T) {
	recorder := httptest.NewRecorder()
	writePortfolioJSON(recorder, http.StatusOK, map[string]string{"ok": "true"})
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}

func TestParseAdminFileQuery(t *testing.T) {
	projectID := uuid.New()
	r := &http.Request{URL: &url.URL{RawQuery: "status=pending_review&project_id=" + projectID.String() + "&limit=25&offset=50"}}
	query, err := parseAdminFileQuery(r)
	if err != nil {
		t.Fatalf("parseAdminFileQuery() error = %v", err)
	}
	if query.Status != FileStatusPendingReview || query.ProjectID != projectID || query.Limit != 25 || query.Offset != 50 {
		t.Fatalf("parseAdminFileQuery() = %+v", query)
	}
}

func TestParseAdminFileQueryDefaultsAndRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name      string
		rawQuery  string
		wantError bool
		wantLimit int
	}{
		{name: "defaults", wantLimit: 50},
		{name: "bad status", rawQuery: "status=draft", wantError: true},
		{name: "bad limit", rawQuery: "limit=0", wantError: true},
		{name: "bad offset", rawQuery: "offset=-1", wantError: true},
		{name: "bad project", rawQuery: "project_id=not-a-uuid", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{URL: &url.URL{RawQuery: tt.rawQuery}}
			query, err := parseAdminFileQuery(r)
			if tt.wantError {
				if err != ErrInvalidQuery {
					t.Fatalf("parseAdminFileQuery() error = %v, want %v", err, ErrInvalidQuery)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseAdminFileQuery() error = %v", err)
			}
			if query.Limit != tt.wantLimit || query.Offset != 0 {
				t.Fatalf("parseAdminFileQuery() = %+v", query)
			}
		})
	}
}

func TestPrivatePortfolioDownloadRequiresVerifiedOwnerOrAdmin(t *testing.T) {
	tests := []struct {
		name    string
		status  string
		owner   bool
		admin   bool
		allowed bool
	}{
		{name: "verified owner", status: "student", owner: true, allowed: true},
		{name: "unverified owner", status: "temporary", owner: true, allowed: false},
		{name: "admin", owner: false, admin: true, allowed: true},
		{name: "other account", owner: false, allowed: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ownerID := uuid.New()
			accountID := uuid.New()
			if tt.owner {
				accountID = ownerID
			}
			allowed := canDownloadPrivateFile(auth.Account{ID: accountID, IdentityStatus: tt.status}, ownerID, tt.admin)
			if allowed != tt.allowed {
				t.Fatalf("private download authorization = %v, want %v", allowed, tt.allowed)
			}
		})
	}
}
