package support

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
)

type DiscordSyncWorker struct {
	Store        DiscordOutboxStore
	Sender       DiscordPlatformSender
	BatchSize    int
	PollInterval time.Duration
	Logger       *slog.Logger
}

func (w *DiscordSyncWorker) ProcessOnce(ctx context.Context) error {
	if w == nil || w.Store == nil || w.Sender == nil {
		return errors.New("support Discord worker is not configured")
	}
	tasks, err := w.Store.ClaimDiscordOutbox(ctx, w.batchSize())
	if err != nil {
		return err
	}
	var firstErr error
	for _, task := range tasks {
		externalID, sendErr := w.Sender.Send(ctx, task)
		if sendErr != nil {
			if markErr := w.Store.MarkDiscordOutboxFailed(ctx, task, safeDiscordWorkerError(sendErr)); markErr != nil && firstErr == nil {
				firstErr = markErr
			}
			if firstErr == nil {
				firstErr = sendErr
			}
			continue
		}
		if markErr := w.Store.MarkDiscordOutboxSent(ctx, task, externalID); markErr != nil {
			if failedErr := w.Store.MarkDiscordOutboxFailed(ctx, task, safeDiscordWorkerError(markErr)); failedErr != nil && firstErr == nil {
				firstErr = failedErr
			}
			if firstErr == nil {
				firstErr = markErr
			}
		}
	}
	return firstErr
}

func (w *DiscordSyncWorker) Run(ctx context.Context) error {
	if w == nil || w.Store == nil || w.Sender == nil {
		return errors.New("support Discord worker is not configured")
	}
	interval := w.PollInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := w.ProcessOnce(ctx); err != nil && w.Logger != nil {
			w.Logger.Warn("support Discord sync pass failed", "error", safeDiscordWorkerError(err))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *DiscordSyncWorker) batchSize() int {
	if w.BatchSize < 1 || w.BatchSize > 100 {
		return 20
	}
	return w.BatchSize
}

func safeDiscordWorkerError(err error) string {
	if err == nil {
		return "unknown Discord support error"
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 500 {
		return message[:500]
	}
	return message
}
