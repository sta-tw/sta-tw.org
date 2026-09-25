package auth

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type fakePendingAccountPurger struct {
	calls int32
	err   error
}

func (p *fakePendingAccountPurger) PurgeExpiredPendingAccounts(context.Context, time.Time) (int, error) {
	atomic.AddInt32(&p.calls, 1)
	return 1, p.err
}

func TestPendingAccountCleanupWorkerRunsOnEveryTick(t *testing.T) {
	purger := &fakePendingAccountPurger{}
	worker := &PendingAccountCleanupWorker{Store: purger, PollInterval: time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := worker.Run(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want context.DeadlineExceeded", err)
	}
	if atomic.LoadInt32(&purger.calls) < 2 {
		t.Fatalf("expected multiple purge passes before deadline, got %d", purger.calls)
	}
}

func TestPendingAccountCleanupWorkerSurvivesPurgeError(t *testing.T) {
	purger := &fakePendingAccountPurger{err: errors.New("db unavailable")}
	worker := &PendingAccountCleanupWorker{Store: purger, PollInterval: time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	if err := worker.Run(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want context.DeadlineExceeded", err)
	}
	if atomic.LoadInt32(&purger.calls) == 0 {
		t.Fatal("expected purge to be attempted despite prior errors")
	}
}

func TestPendingAccountCleanupWorkerRequiresStore(t *testing.T) {
	worker := &PendingAccountCleanupWorker{}
	if err := worker.Run(context.Background()); err == nil {
		t.Fatal("expected error when Store is not configured")
	}
}
