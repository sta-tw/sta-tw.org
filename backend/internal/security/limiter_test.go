package security

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestFixedWindowLimiter(t *testing.T) {
	now := time.Unix(100, 0)
	limiter := NewFixedWindowLimiter(2, time.Minute, 10)

	if !limiter.Allow("client", now) || !limiter.Allow("client", now) {
		t.Fatal("first two requests should be allowed")
	}
	if limiter.Allow("client", now) {
		t.Fatal("third request should be rejected")
	}
	if !limiter.Allow("client", now.Add(time.Minute)) {
		t.Fatal("request after window should be allowed")
	}
}

func TestPeekDoesNotConsume(t *testing.T) {
	now := time.Unix(100, 0)
	limiter := NewFixedWindowLimiter(2, time.Minute, 10)

	// Peeking many times never consumes the window.
	for i := 0; i < 5; i++ {
		if !limiter.Peek("client", now).Allowed {
			t.Fatalf("peek %d should be allowed", i)
		}
	}
	if !limiter.Allow("client", now) || !limiter.Allow("client", now) {
		t.Fatal("two takes after peeks should still be allowed")
	}
	// Now the window is full: Peek reflects it without a further Take.
	if limiter.Peek("client", now).Allowed {
		t.Fatal("peek should report the full window as not allowed")
	}
	if limiter.Peek("client", now).Allowed {
		t.Fatal("a second peek must not have reset anything")
	}
	// A nil limiter always allows.
	var nilLimiter *FixedWindowLimiter
	if !nilLimiter.Peek("x", now).Allowed {
		t.Fatal("nil limiter Peek should allow")
	}
}

func TestFixedWindowLimiterEvictsKeys(t *testing.T) {
	now := time.Unix(100, 0)
	limiter := NewFixedWindowLimiter(1, time.Minute, 2)
	limiter.Allow("first", now)
	limiter.Allow("second", now.Add(time.Second))
	limiter.Allow("third", now.Add(2*time.Second))

	if !limiter.Allow("first", now.Add(3*time.Second)) {
		t.Fatal("oldest key should have been evicted and allowed as a new key")
	}
}

func TestTakeReportsWindowState(t *testing.T) {
	now := time.Unix(1000, 0)
	l := NewFixedWindowLimiter(3, time.Minute, 10)

	r1 := l.Take("c", now)
	if !r1.Allowed || r1.Limit != 3 || r1.Remaining != 2 {
		t.Fatalf("hit 1: %+v", r1)
	}
	_ = l.Take("c", now)
	r3 := l.Take("c", now)
	if !r3.Allowed || r3.Remaining != 0 {
		t.Fatalf("hit 3: %+v", r3)
	}
	r4 := l.Take("c", now.Add(time.Second))
	if r4.Allowed || r4.Remaining != 0 {
		t.Fatalf("hit 4 should be denied: %+v", r4)
	}
	if got := r4.RetryAfter(now.Add(time.Second)); got != 59 {
		t.Fatalf("RetryAfter = %d, want 59", got)
	}
	if !r4.Reset.Equal(now.Add(time.Minute)) {
		t.Fatalf("Reset = %v, want %v", r4.Reset, now.Add(time.Minute))
	}
}

func TestNilLimiterTakeAllows(t *testing.T) {
	var l *FixedWindowLimiter
	r := l.Take("x", time.Now())
	if !r.Allowed || r.Limit != 0 {
		t.Fatalf("nil limiter Take = %+v", r)
	}
}

func TestWriteRateLimitHeaders(t *testing.T) {
	now := time.Unix(2000, 0)
	w := httptest.NewRecorder()
	WriteRateLimitHeaders(w, Result{Allowed: false, Limit: 5, Remaining: 0, Reset: now.Add(30 * time.Second)}, now)
	if w.Header().Get("X-RateLimit-Limit") != "5" {
		t.Fatalf("limit header = %q", w.Header().Get("X-RateLimit-Limit"))
	}
	if w.Header().Get("X-RateLimit-Reset") != "2030" {
		t.Fatalf("reset header = %q", w.Header().Get("X-RateLimit-Reset"))
	}
	if w.Header().Get("Retry-After") != "30" {
		t.Fatalf("retry-after header = %q", w.Header().Get("Retry-After"))
	}

	// Zero Result (nil limiter) writes nothing.
	w2 := httptest.NewRecorder()
	WriteRateLimitHeaders(w2, Result{Allowed: true}, now)
	if w2.Header().Get("X-RateLimit-Limit") != "" {
		t.Fatal("zero result should not write headers")
	}
}
