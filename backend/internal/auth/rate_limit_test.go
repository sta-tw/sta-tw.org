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
