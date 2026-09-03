package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sta-backend/internal/auth"
	"sta-backend/internal/config"
	"sta-backend/internal/db"
	"sta-backend/internal/email"
	"sta-backend/internal/httpapi"
	"sta-backend/internal/notifications"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("notification worker stopped with error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return errors.New("STA_DATABASE_URL is required for the notification worker")
	}
	if cfg.SMTPHost == "" || cfg.SMTPFrom == "" {
		return errors.New("STA_SMTP_HOST and STA_SMTP_FROM are required for the notification worker")
	}
	fieldCipher, err := auth.NewFieldCipher(cfg.EmailEncryptionKey)
	if err != nil {
		return err
	}
	mailer, err := email.NewSMTPSender(email.SMTPConfig{
		Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword, From: cfg.SMTPFrom, UseTLS: cfg.SMTPUseTLS,
	})
	if err != nil {
		return err
	}
	startupContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	databasePool, err := db.OpenPostgres(startupContext, cfg.DatabaseURL)
	cancel()
	if err != nil {
		return err
	}
	defer databasePool.Close()
	repository, err := notifications.NewPostgresRepository(databasePool, fieldCipher)
	if err != nil {
		return err
	}
	worker := &notifications.EmailWorker{
		Store: repository, InquiryStore: repository, Notifier: repository, Cipher: fieldCipher, Sender: mailer,
		BatchSize: 20, PollInterval: 2 * time.Second, Logger: logger,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		if err := httpapi.RunWorkerHealth(ctx, os.Getenv("STA_WORKER_HEALTH_ADDR"), logger, databasePool.Ping); err != nil {
			logger.Warn("worker health server stopped", "error", err)
		}
	}()
	if err := worker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}
