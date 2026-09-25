package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sta-backend/internal/config"
)

func testConfig() config.Config {
	return config.Config{
		Environment:      "test",
		AllowedOrigins:   []string{"https://frontend.example.test"},
		MaxJSONBodyBytes: 1024,
	}
}

func TestHealthAndSecurityHeaders(t *testing.T) {
	handler := NewHandler(testConfig(), slog.Default(), nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if recorder.Header().Get("X-Request-ID") == "" {
		t.Fatal("X-Request-ID is empty")
	}
	if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if !strings.Contains(recorder.Body.String(), `"status":"ok"`) {
		t.Fatalf("body = %q, want health response", recorder.Body.String())
	}
}

func TestReadinessFailureDoesNotLeakInternalError(t *testing.T) {
	handler := NewHandler(testConfig(), slog.Default(), func(context.Context) error {
		return errors.New("database password should not be returned")
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	if strings.Contains(recorder.Body.String(), "database password") {
		t.Fatalf("body leaks internal error: %q", recorder.Body.String())
	}
}

func TestCORSAllowsConfiguredOrigin(t *testing.T) {
	handler := NewHandler(testConfig(), slog.Default(), nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/meta", nil)
	request.Header.Set("Origin", "https://frontend.example.test")
	request.Header.Set("Access-Control-Request-Method", http.MethodGet)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "https://frontend.example.test" {
		t.Fatalf("allow origin = %q", got)
	}
	if !strings.Contains(recorder.Header().Get("Access-Control-Allow-Headers"), "X-MFA-Code") {
		t.Fatalf("allow headers do not include X-MFA-Code: %q", recorder.Header().Get("Access-Control-Allow-Headers"))
	}
}

func TestCORSRejectsUnknownOrigin(t *testing.T) {
	handler := NewHandler(testConfig(), slog.Default(), nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/meta", nil)
	request.Header.Set("Origin", "https://untrusted.example.test")

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestNotFoundIsJSON(t *testing.T) {
	handler := NewHandler(testConfig(), slog.Default(), nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/missing", nil)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}
}

func TestMiddlewareRecoversFromPanic(t *testing.T) {
	handler := withMiddleware(testConfig(), slog.Default(), Options{}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("internal test panic")
	}))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if strings.Contains(recorder.Body.String(), "internal test panic") {
		t.Fatalf("panic value leaked into response: %q", recorder.Body.String())
	}
}

func TestJSONBodyLimitIsApplied(t *testing.T) {
	handler := withMiddleware(testConfig(), slog.Default(), Options{}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			writeError(w, http.StatusRequestEntityTooLarge, "body_too_large", "request body is too large")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/test", strings.NewReader(strings.Repeat("x", 2048)))
	request.Header.Set("Content-Type", "application/json")

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusRequestEntityTooLarge)
	}
}
