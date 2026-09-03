package chat

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
)

type SyncWorker struct {
	Store        OutboxStore
	Senders      map[Platform]PlatformSender
	BatchSize    int
	PollInterval time.Duration
	Logger       *slog.Logger
}

func (w *SyncWorker) ProcessOnce(ctx context.Context) error {
	if w == nil || w.Store == nil {
		return errors.New("chat sync worker is not configured")
	}
	tasks, err := w.Store.ClaimOutbox(ctx, w.batchSize())
	if err != nil {
		return err
	}
	var firstErr error
	for _, task := range tasks {
		sender := w.Senders[task.TargetPlatform]
		var externalID string
		if sender == nil {
			err = errors.New("target platform sender is not configured")
		} else {
			externalID, err = sender.Send(ctx, task)
		}
		if err != nil {
			if markErr := w.Store.MarkOutboxFailed(ctx, task, safeWorkerError(err)); markErr != nil && firstErr == nil {
				firstErr = markErr
			}
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := w.Store.MarkOutboxSent(ctx, task, externalID); err != nil {
			if markErr := w.Store.MarkOutboxFailed(ctx, task, safeWorkerError(err)); markErr != nil && firstErr == nil {
				firstErr = markErr
			}
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (w *SyncWorker) Run(ctx context.Context) error {
	if w == nil || w.Store == nil {
		return errors.New("chat sync worker is not configured")
	}
	interval := w.PollInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := w.ProcessOnce(ctx); err != nil && w.Logger != nil {
			w.Logger.Warn("chat sync pass failed", "error", safeWorkerError(err))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *SyncWorker) batchSize() int {
	if w.BatchSize < 1 || w.BatchSize > 100 {
		return 20
	}
	return w.BatchSize
}

func safeWorkerError(err error) string {
	if err == nil {
		return "unknown chat sync error"
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 500 {
		return message[:500]
	}
	return message
}
