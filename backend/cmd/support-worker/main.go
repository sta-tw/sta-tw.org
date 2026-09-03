package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sta-backend/internal/auth"
	"sta-backend/internal/config"
	"sta-backend/internal/db"
	"sta-backend/internal/support"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("support worker stopped with error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return errors.New("STA_DATABASE_URL is required for the support worker")
	}
	sender, err := support.NewDiscordSender(support.DiscordConfig{
		Token:             cfg.DiscordSupportBotToken,
		GuildID:           cfg.DiscordSupportGuildID,
		CategoryID:        cfg.DiscordSupportCategoryID,
		ArchiveCategoryID: cfg.DiscordSupportArchiveCategoryID,
		SupportRoleID:     cfg.DiscordSupportRoleID,
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
	fieldCipher, err := supportFieldCipher(cfg)
	if err != nil {
		return err
	}
	repository, err := support.NewPostgresRepository(databasePool, fieldCipher, cfg.LookupHMACKey, cfg.SupportEmail, cfg.PublicBaseURL)
	if err != nil {
		return err
	}
	worker := &support.DiscordSyncWorker{
		Store:        repository,
		Sender:       sender,
		BatchSize:    20,
		PollInterval: 2 * time.Second,
		Logger:       logger,
	}
	gateway := &support.DiscordGateway{
		Token: cfg.DiscordSupportBotToken, GuildID: cfg.DiscordSupportGuildID,
		Store: repository, LookupKey: cfg.LookupHMACKey, Logger: logger,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Run the outbox sync worker and the Discord gateway together. If either
	// exits with a non-context error, cancel the other and surface it rather
	// than silently continuing degraded.
	errc := make(chan error, 2)
	go func() { errc <- annotate("outbox sync worker", worker.Run(ctx)) }()
	go func() { errc <- annotate("Discord gateway", gateway.Run(ctx)) }()

	firstErr := <-errc
	stop()
	<-errc
	return firstErr
}

// annotate drops context cancellation (clean shutdown) and labels anything else.
func annotate(component string, err error) error {
	if err == nil || errors.Is(err, context.Canceled) {
		return nil
	}
	return fmt.Errorf("support %s: %w", component, err)
}

func supportFieldCipher(cfg config.Config) (*auth.FieldCipher, error) {
	return auth.NewFieldCipher(cfg.EmailEncryptionKey)
}
