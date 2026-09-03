package httpapi

import (
	"log/slog"
	"net/http"
	"regexp"
	"time"
)

// RequestObserver is notified once per finished HTTP request. cmd/api wires a
// Prometheus-backed implementation; the httpapi package itself stays free of any
// metrics dependency.
type RequestObserver func(method, route string, status int, duration time.Duration, responseBytes int64)

// Options configures NewHandler beyond its required arguments.
type Options struct {
	// Observer, when set, receives per-request timing/size/status.
	Observer RequestObserver
	// ReadinessChecks, when non-empty, drives /readyz and makes it report a
	// per-dependency "checks" object. It takes precedence over the legacy
	// single ReadinessCheck argument.
	ReadinessChecks []NamedCheck
}

// Option mutates Options.
type Option func(*Options)

// WithObserver registers a per-request observer (e.g. Prometheus metrics).
func WithObserver(observer RequestObserver) Option {
	return func(o *Options) { o.Observer = observer }
}

// WithReadinessChecks makes /readyz run and report the given labelled probes.
func WithReadinessChecks(checks ...NamedCheck) Option {
	return func(o *Options) { o.ReadinessChecks = append(o.ReadinessChecks, checks...) }
}

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._~-]{8,128}$`)

// inboundRequestID returns a caller-supplied X-Request-ID when it looks like a
// safe correlation token, otherwise "" so a fresh one is generated.
func inboundRequestID(r *http.Request) string {
	candidate := r.Header.Get("X-Request-ID")
	if requestIDPattern.MatchString(candidate) {
		return candidate
	}
	return ""
}

// statusRecorder captures the response status code and byte count so the access
// log and observer can report them.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
	wrote  bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wrote {
		s.status = code
		s.wrote = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wrote {
		s.status = http.StatusOK
		s.wrote = true
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += int64(n)
	return n, err
}

// Flush forwards to the underlying writer when it supports flushing (used by the
// Discord gateway / any streaming handler).
func (s *statusRecorder) Flush() {
	if flusher, ok := s.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// quietPaths are health/metrics endpoints kept out of the access log to avoid
// drowning it in probe traffic.
var quietPaths = map[string]struct{}{
	"/healthz": {},
	"/readyz":  {},
	"/metrics": {},
}

func accessLogMiddleware(logger *slog.Logger, observer RequestObserver, mux *http.ServeMux, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		// Resolve the matched pattern before serving so metrics get a
		// low-cardinality route label ("GET /api/v1/x/{id}") instead of the
		// raw path with embedded identifiers.
		route := r.URL.Path
		if mux != nil {
			if _, pattern := mux.Handler(r); pattern != "" {
				route = pattern
			}
		}

		next.ServeHTTP(recorder, r)

		duration := time.Since(start)
		if observer != nil {
			observer(r.Method, route, recorder.status, duration, recorder.bytes)
		}
		if _, quiet := quietPaths[r.URL.Path]; quiet && recorder.status < 400 {
			return
		}
		level := slog.LevelInfo
		switch {
		case recorder.status >= 500:
			level = slog.LevelError
		case recorder.status >= 400:
			level = slog.LevelWarn
		}
		logger.LogAttrs(r.Context(), level, "http request",
			slog.String("request_id", requestIDFromContext(r.Context())),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("route", route),
			slog.Int("status", recorder.status),
			slog.Int64("bytes", recorder.bytes),
			slog.Int64("duration_ms", duration.Milliseconds()),
			slog.String("remote", clientIP(r)),
		)
	})
}

func clientIP(r *http.Request) string {
	// The API is expected to sit behind a trusted TLS proxy; prefer the
	// left-most X-Forwarded-For entry when present, else the transport peer.
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		for i := 0; i < len(forwarded); i++ {
			if forwarded[i] == ',' {
				return trimSpace(forwarded[:i])
			}
		}
		return trimSpace(forwarded)
	}
	return r.RemoteAddr
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
