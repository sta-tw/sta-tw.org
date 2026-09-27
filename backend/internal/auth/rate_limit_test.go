package auth

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeDistributedLimiter struct {
	allowed bool
	err     error
	calls   int
}

func (f *fakeDistributedLimiter) Allow(context.Context, string, string, int, time.Duration, time.Time) (bool, error) {
	f.calls++
	return f.allowed, f.err
}

func TestDistributedLimiterFailureFailsClosed(t *testing.T) {
	store := newFakeAuthStore()
	cipher, _ := NewFieldCipher(make([]byte, 32))
	service, err := NewService(store, cipher, make([]byte, 32), time.Hour, false)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	limiter := &fakeDistributedLimiter{err: errors.New("database unavailable")}
	service.ConfigureDistributedLimiter(limiter)
	request := httptest.NewRequest("POST", "/api/v1/auth/register", nil)
	if _, err := service.Register(context.Background(), RegisterInput{Username: "user", Email: "user@example.test", SchoolEmail: "user@ntu.edu.tw"}, request); !errors.Is(err, ErrRateLimitUnavailable) {
		t.Fatalf("Register() = %v, want rate limiter unavailable", err)
	}
	if limiter.calls != 1 {
		t.Fatalf("distributed limiter calls = %d, want 1", limiter.calls)
	}
}

func TestDistributedLimiterRejectionReturnsRateLimited(t *testing.T) {
	store := newFakeAuthStore()
	cipher, _ := NewFieldCipher(make([]byte, 32))
	service, _ := NewService(store, cipher, make([]byte, 32), time.Hour, false)
	limiter := &fakeDistributedLimiter{}
	service.ConfigureDistributedLimiter(limiter)
	request := httptest.NewRequest("POST", "/api/v1/auth/register", nil)
	if _, err := service.Register(context.Background(), RegisterInput{Username: "user", Email: "user@example.test", SchoolEmail: "user@ntu.edu.tw"}, request); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("Register() = %v, want rate limited", err)
	}
}

func TestClientIPUsesOnlyTrustedProxyHeader(t *testing.T) {
	request := httptest.NewRequest("POST", "/api/v1/auth/register", nil)
	request.Header.Set("X-STA-Client-IP", "203.0.113.42")
	request.RemoteAddr = "198.51.100.10:443"
	if got := clientIP(request); got != "198.51.100.10" {
		t.Fatalf("public peer with forged header = %q", got)
	}
	request.RemoteAddr = "172.18.0.3:45678"
	if got := clientIP(request); got != "203.0.113.42" {
		t.Fatalf("trusted proxy client IP = %q", got)
	}
	request.Header.Set("X-STA-Client-IP", "invalid")
	if got := clientIP(request); got != "172.18.0.3" {
		t.Fatalf("invalid proxy header fallback = %q", got)
	}
}

func TestRegisterLimiterAllowsExpectedLaunchBurst(t *testing.T) {
	store := newFakeAuthStore()
	cipher, _ := NewFieldCipher(make([]byte, 32))
	service, err := NewService(store, cipher, make([]byte, 32), time.Hour, false)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	request := httptest.NewRequest("POST", "/api/v1/auth/register", nil)
	request.RemoteAddr = "172.18.0.3:45678"
	request.Header.Set("X-STA-Client-IP", "203.0.113.42")
	for i := 0; i < 120; i++ {
		allowed, err := service.registerAllowed(context.Background(), request)
		if err != nil || !allowed {
			t.Fatalf("request %d: allowed=%v error=%v", i+1, allowed, err)
		}
	}
	if allowed, err := service.registerAllowed(context.Background(), request); err != nil || allowed {
		t.Fatalf("request 121: allowed=%v error=%v, want rate limit", allowed, err)
	}
}
