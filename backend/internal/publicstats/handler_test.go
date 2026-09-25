//go:build integration

package publicstats

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"sta-backend/internal/dbtest"
)

func TestGetReturnsCounts(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()

	// One published brochure and one active account should each be
	// reflected in the snapshot.
	dbtest.InsertAccount(t, ctx, pool, "")
	schoolCode := dbtest.AnySchoolCode(t, ctx, pool)
	if _, err := pool.Exec(ctx, `
		INSERT INTO brochure_documents (academic_year, school_code, storage_key, original_file_name, mime_type, file_size_bytes, sha256_hex, review_status)
		VALUES (116, $1, 'k', 'f.pdf', 'application/pdf', 1, repeat('a', 64), 'published')
	`, schoolCode); err != nil {
		t.Fatal(err)
	}

	h, err := NewHandler(pool)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats/public", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Data Snapshot `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Data.BrochureCount < 1 {
		t.Fatalf("BrochureCount = %d, want >= 1", body.Data.BrochureCount)
	}
	if body.Data.RegisteredAccounts < 1 {
		t.Fatalf("RegisteredAccounts = %d, want >= 1", body.Data.RegisteredAccounts)
	}
}
