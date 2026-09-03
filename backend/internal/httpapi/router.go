package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"sta-backend/internal/config"
)

// ReadinessCheck reports whether the service can serve traffic. A nil error
// means ready.
type ReadinessCheck func(context.Context) error
type RouteRegistrar func(*http.ServeMux)

// NamedCheck is one labelled dependency probe for /readyz.
type NamedCheck struct {
	Name  string
	Check ReadinessCheck
}

// Readiness combines several NamedChecks into one ReadinessCheck that runs them
// all (each with its own short timeout) and fails if any fails. The per-check
// results are reported by the /readyz handler.
func Readiness(checks ...NamedCheck) ReadinessCheck {
	filtered := make([]NamedCheck, 0, len(checks))
	for _, c := range checks {
		if c.Check != nil && c.Name != "" {
			filtered = append(filtered, c)
		}
	}
	return func(ctx context.Context) error {
		_, err := runChecks(ctx, filtered)
		return err
	}
}

func runChecks(ctx context.Context, checks []NamedCheck) (map[string]string, error) {
	results := make(map[string]string, len(checks))
	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		firstErr error
	)
	for _, c := range checks {
		wg.Add(1)
		go func(c NamedCheck) {
			defer wg.Done()
			checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			err := c.Check(checkCtx)
			mu.Lock()
			if err != nil {
				results[c.Name] = "error"
				if firstErr == nil {
					firstErr = err
				}
			} else {
				results[c.Name] = "ok"
			}
			mu.Unlock()
		}(c)
	}
	wg.Wait()
	return results, firstErr
}

func NewHandler(cfg config.Config, logger *slog.Logger, readiness ReadinessCheck, registrars ...RouteRegistrar) http.Handler {
	return NewHandlerWithOptions(cfg, logger, readiness, nil, registrars...)
}

// NewHandlerWithOptions is NewHandler plus functional options (e.g. a metrics
// observer). registrars is kept as the trailing variadic for call-site clarity.
func NewHandlerWithOptions(cfg config.Config, logger *slog.Logger, readiness ReadinessCheck, options []Option, registrars ...RouteRegistrar) http.Handler {
	opts := Options{}
	for _, apply := range options {
		if apply != nil {
			apply(&opts)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler)
	mux.HandleFunc("GET /readyz", readinessHandler(readiness, opts.ReadinessChecks))
	mux.HandleFunc("GET /api/v1/meta", metaHandler)
	for _, register := range registrars {
		if register != nil {
			register(mux)
		}
	}
	mux.HandleFunc("/", notFoundHandler)

	return withMiddleware(cfg, logger, opts, mux)
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func readinessHandler(check ReadinessCheck, named []NamedCheck) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if len(named) > 0 {
			results, err := runChecks(r.Context(), named)
			body := map[string]any{"status": "ready", "checks": results}
			if err != nil {
				body["status"] = "not_ready"
				writeJSON(w, http.StatusServiceUnavailable, body)
				return
			}
			writeJSON(w, http.StatusOK, body)
			return
		}
		if check != nil {
			if err := check(r.Context()); err != nil {
				writeError(w, http.StatusServiceUnavailable, "not_ready", "service is not ready")
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}

func metaHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"api_version": "v1",
		"service":     "sta-backend",
	})
}

func notFoundHandler(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, "not_found", "resource not found")
}
