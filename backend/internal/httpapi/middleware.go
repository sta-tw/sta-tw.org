package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"sta-backend/internal/config"
	"sta-backend/internal/obs"
)

const extractionResultJSONBodyLimit int64 = 16 << 20

type contextKey string

const requestIDKey contextKey = "request_id"

func requestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDKey).(string)
	return requestID
}

func withMiddleware(cfg config.Config, logger *slog.Logger, opts Options, next http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	// The access log needs the matched route pattern, which it resolves from
	// the mux before serving. Pass the mux through when next is one.
	mux, _ := next.(*http.ServeMux)

	return recoverMiddleware(logger,
		requestIDMiddleware(
			accessLogMiddleware(logger, opts.Observer, mux,
				securityHeadersMiddleware(cfg,
					corsMiddleware(cfg,
						jsonBodyLimitMiddleware(cfg,
							next,
						),
					),
				),
			),
		),
	)
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := inboundRequestID(r)
		if requestID == "" {
			requestID = newRequestID()
		}
		ctx := context.WithValue(r.Context(), requestIDKey, requestID)
		w.Header().Set("X-Request-ID", requestID)

		// Continue an upstream W3C trace, or start one. The trace_id flows
		// into the extraction job and back through the result callback so a
		// single id spans API, worker and callback logs.
		trace := obs.TraceFromInbound(r.Header.Get("traceparent"))
		ctx = obs.WithTrace(ctx, trace)
		w.Header().Set("X-Trace-Id", trace.TraceID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func newRequestID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err == nil {
		return hex.EncodeToString(data[:])
	}
	// crypto/rand failure is exceptional; a timestamp still gives operators a
	// useful correlation value without exposing user-controlled input.
	return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
}

func securityHeadersMiddleware(cfg config.Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		w.Header().Set("Cache-Control", "no-store")
		if cfg.Environment == "production" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func corsMiddleware(cfg config.Config, next http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, origin := range cfg.AllowedOrigins {
		allowed[origin] = struct{}{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin != "" {
			if _, ok := allowed[origin]; !ok {
				if r.Method == http.MethodOptions {
					writeError(w, http.StatusForbidden, "origin_not_allowed", "request origin is not allowed")
					return
				}
				writeError(w, http.StatusForbidden, "origin_not_allowed", "request origin is not allowed")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, X-Trace-Id, X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset, Retry-After")
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-CSRF-Token, X-MFA-Code, X-Request-ID, traceparent")
				w.Header().Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func jsonBodyLimitMiddleware(cfg config.Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
		if strings.HasPrefix(r.URL.Path, "/api/") && (contentType == "application/json" || contentType == "") && r.Body != nil {
			limit := cfg.MaxJSONBodyBytes
			// A candidate-list result may contain up to 10,000 rows. Keep the
			// normal API limit for every other JSON endpoint, but allow the
			// authenticated extraction callback enough room for a real list.
			if r.Method == http.MethodPost &&
				strings.HasPrefix(r.URL.Path, "/api/v1/internal/extraction/jobs/") &&
				strings.HasSuffix(r.URL.Path, "/result") && limit < extractionResultJSONBodyLimit {
				limit = extractionResultJSONBodyLimit
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		next.ServeHTTP(w, r)
	})
}

func recoverMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.ErrorContext(r.Context(), "http handler panic", "request_id", requestIDFromContext(r.Context()), "panic", recovered)
				writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
