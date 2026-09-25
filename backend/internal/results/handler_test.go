package results

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"sta-backend/internal/auth"
)

func TestParseAdminBatchQuery(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/results/batches?academic_year=116&school_code=001&status=pending_review&limit=20&offset=40", nil)
	query, err := parseAdminBatchQuery(request)
	if err != nil {
		t.Fatalf("parseAdminBatchQuery() error = %v", err)
	}
	if query.AcademicYear != 116 || query.SchoolCode != "001" || query.Status != ResultBatchStatusPendingReview || query.Limit != 20 || query.Offset != 40 {
		t.Fatalf("parseAdminBatchQuery() = %#v", query)
	}

	badRequest := httptest.NewRequest(http.MethodGet, "/api/v1/admin/results/batches?limit=101", nil)
	if _, err := parseAdminBatchQuery(badRequest); err == nil {
		t.Fatal("invalid query unexpectedly accepted")
	}
}

func TestWriteResultJSONDisablesCaching(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeResultJSON(recorder, http.StatusOK, map[string]string{"status": "ok"})
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if !strings.Contains(recorder.Body.String(), `"status":"ok"`) {
		t.Fatalf("response body = %q", recorder.Body.String())
	}
}

func TestRegisterRoutesContainsOnlyCoreResultPaths(t *testing.T) {
	cipher, err := auth.NewFieldCipher(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(&auth.Service{}, &routeProbeRepository{}, cipher, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/applications/00000000-0000-0000-0000-000000000001/result"},
		{http.MethodGet, "/api/v1/applications/00000000-0000-0000-0000-000000000001/inquiries"},
		{http.MethodPut, "/api/v1/applications/00000000-0000-0000-0000-000000000001/candidate-number"},
		{http.MethodPut, "/api/v1/applications/00000000-0000-0000-0000-000000000001/willingness"},
	} {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(route.method, route.path, nil))
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("core result path %q returned %d, want authentication gate", route.path, recorder.Code)
		}
	}

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/internal/telegram-cross-check/users/42/dashboard", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("Telegram-specific path returned %d, want not found", recorder.Code)
	}
}

type routeProbeRepository struct{}

func (*routeProbeRepository) SetCandidateNumber(context.Context, uuid.UUID, uuid.UUID, []byte, []byte, string) error {
	return nil
}

func (*routeProbeRepository) GetReport(context.Context, uuid.UUID, uuid.UUID) (Report, error) {
	return Report{}, nil
}

func (*routeProbeRepository) ListInquiries(context.Context, uuid.UUID, uuid.UUID) ([]Inquiry, error) {
	return nil, nil
}

func (*routeProbeRepository) SetWillingness(context.Context, uuid.UUID, uuid.UUID, int16, *uuid.UUID) (WillingnessResponse, error) {
	return WillingnessResponse{}, nil
}
