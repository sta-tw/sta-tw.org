package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sta-backend/internal/auth"
	"sta-backend/internal/config"
	"sta-backend/internal/db"
	"sta-backend/internal/storage"
	"sta-backend/internal/verification"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	academicYear := flag.Int("academic-year", 0, "ROC academic year whose verification material is being closed")
	flag.Parse()
	if *academicYear < 100 || *academicYear > 999 {
		logger.Error("-academic-year must be a three-digit academic year")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, logger, *academicYear); err != nil {
		logger.Error("annual maintenance failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger, academicYear int) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return errors.New("STA_DATABASE_URL is required for annual maintenance")
	}
	if !objectStorageConfigured(cfg) {
		return errors.New("private object storage is required for annual verification cleanup")
	}
	cipher, err := auth.NewFieldCipher(cfg.EmailEncryptionKey)
	if err != nil {
		return err
	}
	blobStore, err := storage.NewMinioStore(cfg.ObjectStorageEndpoint, cfg.ObjectStorageAccessKey, cfg.ObjectStorageSecretKey, cfg.ObjectStorageBucket, cfg.ObjectStorageUseSSL)
	if err != nil {
		return err
	}
	startupContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	databasePool, err := db.OpenPostgres(startupContext, cfg.DatabaseURL)
	cancel()
	if err != nil {
		return err
	}
	defer databasePool.Close()
	repository, err := verification.NewPostgresRepository(databasePool)
	if err != nil {
		return err
	}
	service, err := verification.NewService(repository, cipher, cfg.LookupHMACKey, nil, blobStore)
	if err != nil {
		return err
	}
	report, err := service.PurgeAnnualData(ctx, academicYear)
	if err != nil {
		return err
	}
	logger.Info("annual verification maintenance completed", "academic_year", report.AcademicYear, "documents_removed", report.VerificationDocuments, "requests_removed", report.VerificationRequests, "accounts_promoted", report.AccountsPromotedToSenior)
	return nil
}

func objectStorageConfigured(cfg config.Config) bool {
	return cfg.ObjectStorageEndpoint != "" || cfg.ObjectStorageAccessKey != "" || cfg.ObjectStorageSecretKey != ""
}
