package security

import (
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

func (l *FixedWindowLimiter) Allow(key string, now time.Time) bool {
	if l == nil {
		return true
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
		return true
	}
	if entry.count >= l.limit {
		return false
	}
	entry.count++
	l.entries[key] = entry
	return true
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
