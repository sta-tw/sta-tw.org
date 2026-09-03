package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"sta-backend/internal/telegrambot"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("telegram bot stopped with error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	config, err := telegrambot.ConfigFromEnv()
	if err != nil {
		return err
	}
	bot, err := telegrambot.New(config)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	info, err := bot.Verify(ctx)
	if err != nil {
		return err
	}
	logger.Info("telegram bot connected", "username", info.Username, "backend", config.BackendBaseURL)
	if err := bot.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}
