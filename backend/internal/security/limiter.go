package security

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

// FixedWindowLimiter is intentionally small and process-local. It protects a
// single API process during the first deployment; a distributed limiter must be
// added before running multiple API replicas behind a load balancer.
type FixedWindowLimiter struct {
	mu      sync.Mutex
	entries map[string]windowEntry
	limit   int
	window  time.Duration
	maxKeys int
}

type windowEntry struct {
	started time.Time
	count   int
}

func NewFixedWindowLimiter(limit int, window time.Duration, maxKeys int) *FixedWindowLimiter {
	if limit < 1 {
		limit = 1
	}
	if window <= 0 {
		window = time.Minute
	}
	if maxKeys < 1 {
		maxKeys = 1024
	}
	return &FixedWindowLimiter{
		entries: make(map[string]windowEntry),
		limit:   limit,
		window:  window,
		maxKeys: maxKeys,
	}
}

// Result is the outcome of one Take call, shaped for X-RateLimit-* headers.
type Result struct {
	Allowed   bool
	Limit     int
	Remaining int
	Reset     time.Time // when the current window ends
}

// RetryAfter is how long a denied caller should wait, rounded up to whole
// seconds (minimum 1). Zero when the call was allowed.
func (r Result) RetryAfter(now time.Time) int {
	if r.Allowed {
		return 0
	}
	d := r.Reset.Sub(now)
	if d <= 0 {
		return 1
	}
	return int((d + time.Second - 1) / time.Second)
}

func (l *FixedWindowLimiter) Allow(key string, now time.Time) bool {
	return l.Take(key, now).Allowed
}

// Take records one hit against key and reports the resulting window state.
// A nil limiter always allows (Limit 0).
func (l *FixedWindowLimiter) Take(key string, now time.Time) Result {
	if l == nil {
		return Result{Allowed: true}
	}
	if key == "" {
		key = "unknown"
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	entry, exists := l.entries[key]
	if !exists || now.Sub(entry.started) >= l.window || now.Before(entry.started) {
		if !exists && len(l.entries) >= l.maxKeys {
			l.evictOldest()
		}
		l.entries[key] = windowEntry{started: now, count: 1}
		return Result{Allowed: true, Limit: l.limit, Remaining: l.limit - 1, Reset: now.Add(l.window)}
	}
	reset := entry.started.Add(l.window)
	if entry.count >= l.limit {
		return Result{Allowed: false, Limit: l.limit, Remaining: 0, Reset: reset}
	}
	entry.count++
	l.entries[key] = entry
	return Result{Allowed: true, Limit: l.limit, Remaining: l.limit - entry.count, Reset: reset}
}

// WriteRateLimitHeaders sets X-RateLimit-Limit/Remaining/Reset from r, and
// Retry-After when r denied the request. It is a no-op for a zero Result
// (nil limiter), so callers can pass it unconditionally.
func WriteRateLimitHeaders(w http.ResponseWriter, r Result, now time.Time) {
	if r.Limit == 0 {
		return
	}
	h := w.Header()
	h.Set("X-RateLimit-Limit", strconv.Itoa(r.Limit))
	h.Set("X-RateLimit-Remaining", strconv.Itoa(max(r.Remaining, 0)))
	h.Set("X-RateLimit-Reset", strconv.FormatInt(r.Reset.Unix(), 10))
	if !r.Allowed {
		h.Set("Retry-After", strconv.Itoa(r.RetryAfter(now)))
	}
}

func (l *FixedWindowLimiter) evictOldest() {
	var oldestKey string
	var oldest time.Time
	for key, entry := range l.entries {
		if oldestKey == "" || entry.started.Before(oldest) {
			oldestKey = key
			oldest = entry.started
		}
	}
	if oldestKey != "" {
		delete(l.entries, oldestKey)
	}
}
