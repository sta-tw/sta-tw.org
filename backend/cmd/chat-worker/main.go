package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sta-backend/internal/chat"
	"sta-backend/internal/config"
	"sta-backend/internal/db"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("chat worker stopped with error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return errors.New("STA_DATABASE_URL is required for the chat worker")
	}
	if (cfg.DiscordChatBotToken == "") != (cfg.DiscordChatChannelID == "") {
		return errors.New("Discord bot token and channel ID must be configured together")
	}
	if (cfg.TelegramBotToken == "") != (cfg.TelegramChatID == "") {
		return errors.New("Telegram bot token and chat ID must be configured together")
	}
	if cfg.DiscordChatBotToken == "" || cfg.TelegramBotToken == "" {
		return errors.New("Discord and Telegram bot credentials are both required for the three-way lounge sync")
	}

	startupContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	databasePool, err := db.OpenPostgres(startupContext, cfg.DatabaseURL)
	cancel()
	if err != nil {
		return err
	}
	defer databasePool.Close()
	repository, err := chat.NewPostgresRepository(databasePool, cfg.LookupHMACKey)
	if err != nil {
		return err
	}
	senders := make(map[chat.Platform]chat.PlatformSender, 2)
	if cfg.DiscordChatBotToken != "" {
		discordSender, err := chat.NewDiscordSender(cfg.DiscordChatBotToken, cfg.DiscordChatChannelID)
		if err != nil {
			return err
		}
		senders[chat.PlatformDiscord] = discordSender
	}
	if cfg.TelegramBotToken != "" {
		telegramSender, err := chat.NewTelegramSender(cfg.TelegramBotToken, cfg.TelegramChatID)
		if err != nil {
			return err
		}
		senders[chat.PlatformTelegram] = telegramSender
	}
	worker := &chat.SyncWorker{
		Store:        repository,
		Senders:      senders,
		BatchSize:    20,
		PollInterval: 2 * time.Second,
		Logger:       logger,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := worker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}
