package auth

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// PendingAccountPurger is the storage side of PendingAccountCleanupWorker —
// kept as its own small interface (rather than folding into Store) so a
// worker that only needs this doesn't have to construct the rest of auth's
// dependencies.
type PendingAccountPurger interface {
	PurgeExpiredPendingAccounts(ctx context.Context, now time.Time) (int, error)
}

// PendingAccountCleanupWorker periodically deletes 'pending_verification'
// accounts whose activation window has expired, so the username/email is
// free for a fresh registration attempt. See PostgresStore.
// PurgeExpiredPendingAccounts for the current-vs-future tradeoff this
// implements.
type PendingAccountCleanupWorker struct {
	Store        PendingAccountPurger
	PollInterval time.Duration
	Logger       *slog.Logger
	Now          func() time.Time
}

func (w *PendingAccountCleanupWorker) Run(ctx context.Context) error {
	if w == nil || w.Store == nil {
		return errors.New("pending account cleanup worker is not configured")
	}
	interval := w.PollInterval
	if interval <= 0 {
		interval = time.Hour
	}
	now := w.Now
	if now == nil {
		now = time.Now
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		deleted, err := w.Store.PurgeExpiredPendingAccounts(ctx, now().UTC())
		if err != nil {
			if w.Logger != nil {
				w.Logger.Warn("pending account cleanup pass failed", "error", err)
			}
		} else if deleted > 0 && w.Logger != nil {
			w.Logger.Info("purged expired pending accounts", "count", deleted)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
