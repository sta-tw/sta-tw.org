package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"sta-backend/internal/config"
	"sta-backend/internal/db"
	"sta-backend/internal/ingestion"
	"sta-backend/internal/jobs"
	"sta-backend/internal/results"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("ingestion worker stopped with error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return errors.New("STA_DATABASE_URL is required for the ingestion worker")
	}
	if cfg.RabbitMQURL == "" {
		return errors.New("STA_RABBITMQ_URL is required for the ingestion worker")
	}
	startupContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	databasePool, err := db.OpenPostgres(startupContext, cfg.DatabaseURL)
	cancel()
	if err != nil {
		return err
	}
	defer databasePool.Close()
	broker, err := jobs.OpenBroker(jobs.BrokerConfig{
		URL: cfg.RabbitMQURL, Exchange: cfg.RabbitMQExchange,
		ExtractQueue: cfg.RabbitMQExtractQueue, ResultQueue: cfg.RabbitMQResultQueue,
		Logger: logger,
	})
	if err != nil {
		return err
	}
	defer broker.Close()
	repository, err := ingestion.NewPostgresRepository(databasePool)
	if err != nil {
		return err
	}
	resultRepository, err := results.NewPostgresRepository(databasePool, cfg.LookupHMACKey)
	if err != nil {
		return err
	}
	worker := &ingestion.ResultWorker{
		Repository:  repository,
		ListApplier: resultRepository,
		Broker:      broker,
		Queue:       cfg.RabbitMQResultQueue,
		Consumer:    "sta-ingestion-result-" + uuid.NewString(),
		Logger:      logger,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return worker.Run(ctx)
}
