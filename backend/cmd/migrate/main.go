package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"sta-backend/internal/migrate"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	directory := flag.String("dir", "migrations", "directory containing numbered SQL migrations")
	includeTelegram := flag.Bool("include-telegram", false, "apply the detached Telegram adapter migrations")
	flag.Parse()

	opts := migrate.Options{IncludeTelegram: *includeTelegram, Logger: logger}
	if err := migrate.Run(context.Background(), *directory, opts); err != nil {
		logger.Error("migration failed", "error", err)
		os.Exit(1)
	}
}
