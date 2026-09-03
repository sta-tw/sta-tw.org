// Command reindex rebuilds the Meilisearch indexes from the current public
// PostgreSQL rows. Run it after a bulk data change or on a schedule.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"sta-backend/internal/config"
	"sta-backend/internal/db"
	"sta-backend/internal/search"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("reindex failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return errWith("STA_DATABASE_URL is required for reindex")
	}
	client, err := search.NewClient(cfg.MeilisearchURL, cfg.MeilisearchKey)
	if err != nil {
		return err
	}
	if client == nil {
		return errWith("STA_MEILISEARCH_URL is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, err := db.OpenPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	counts, err := search.Reindex(ctx, client, pool)
	if err != nil {
		return err
	}
	logger.Info("reindex complete", "counts", counts)
	return nil
}

type stringError string

func (e stringError) Error() string { return string(e) }
func errWith(msg string) error      { return stringError(msg) }
