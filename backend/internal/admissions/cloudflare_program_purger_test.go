package admissions

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCloudflareProgramPagePurgerInvalidatesProgramPrefix(t *testing.T) {
	var gotPrefixes []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("authorization = %q, want configured bearer token", got)
		}
		var request struct {
			Prefixes []string `json:"prefixes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode purge request: %v", err)
		}
		gotPrefixes = request.Prefixes
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"errors":[]}`))
	}))
	defer server.Close()

	purger := NewCloudflareProgramPagePurger("test-token", "zone-123", "https://STA-TW.org/", nil)
	purger.endpoint = server.URL
	purger.client = server.Client()
	purger.Invalidate(context.Background(), "116-002-001", "116-003-005")

	wantPrefixes := []string{
		"sta-tw.org/bochures/program/",
		"sta-tw.org/sitemap.xml",
		"sta-tw.org/llms.txt",
	}
	if len(gotPrefixes) != len(wantPrefixes) {
		t.Fatalf("purged prefixes = %q, want %q", gotPrefixes, wantPrefixes)
	}
	for i := range wantPrefixes {
		if gotPrefixes[i] != wantPrefixes[i] {
			t.Fatalf("purged prefixes = %q, want %q", gotPrefixes, wantPrefixes)
		}
	}
}

func TestCloudflareProgramPagePurgerIsDisabledWithoutCredentials(t *testing.T) {
	if got := NewCloudflareProgramPagePurger("", "zone-123", "https://sta-tw.org", nil); got != nil {
		t.Fatal("purger should be disabled without an API token")
	}
}
