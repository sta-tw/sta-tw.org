package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"sta-backend/internal/admin"
	"sta-backend/internal/admissions"
	"sta-backend/internal/applications"
	"sta-backend/internal/auth"
	"sta-backend/internal/brochurediscovery"
	"sta-backend/internal/chat"
	"sta-backend/internal/config"
	"sta-backend/internal/content"
	"sta-backend/internal/db"
	"sta-backend/internal/events"
	"sta-backend/internal/httpapi"
	"sta-backend/internal/ingestion"
	"sta-backend/internal/jobs"
	"sta-backend/internal/notifications"
	"sta-backend/internal/obs"
	"sta-backend/internal/portfolio"
	"sta-backend/internal/results"
	"sta-backend/internal/schools"
	"sta-backend/internal/search"
	"sta-backend/internal/security"
	"sta-backend/internal/sources"
	"sta-backend/internal/sse"
	"sta-backend/internal/storage"
	"sta-backend/internal/support"
	"sta-backend/internal/telegramcrosscheck"
	"sta-backend/internal/verification"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	checkConfig := flag.Bool("check-config", false, "validate configuration from the environment and exit")
	flag.Parse()
	if *checkConfig {
		if _, err := config.Load(); err != nil {
			logger.Error("configuration is invalid", "error", err)
			os.Exit(1)
		}
		logger.Info("configuration is valid")
		return
	}
	if err := run(logger); err != nil {
		logger.Error("api stopped with error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	var databasePool *pgxpool.Pool
	var messageBroker *jobs.Broker
	var authService *auth.Service
	var fieldCipher *auth.FieldCipher
	var notificationRepository notifications.Repository
	var blobStore storage.BlobStore
	var fileScanner storage.Scanner
	var distributedLimiter security.DistributedLimiter
	var admissionRepository *admissions.PostgresRepository
	var resultRepository *results.PostgresRepository
	var discoveryHandler *brochurediscovery.Handler
	var ingestionService *ingestion.Service
	var eventHub *events.Hub
	registrars := make([]httpapi.RouteRegistrar, 0, 2)
	var readiness httpapi.ReadinessCheck
	var readinessChecks []httpapi.NamedCheck

	hubCtx, hubCancel := context.WithCancel(context.Background())
	defer hubCancel()

	if cfg.DatabaseURL != "" {
		startupContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		databasePool, err = db.OpenPostgres(startupContext, cfg.DatabaseURL)
		cancel()
		if err != nil {
			return err
		}
		defer databasePool.Close()
		eventHub = events.NewHub(hubCtx, databasePool, logger)
		if cfg.FieldEncryptionKeys != nil {
			fieldCipher, err = auth.NewFieldCipherRing(cfg.FieldEncryptionPrimaryVersion, cfg.FieldEncryptionKeys, cfg.EmailEncryptionKey)
		} else {
			fieldCipher, err = auth.NewFieldCipher(cfg.EmailEncryptionKey)
		}
		if err != nil {
			return err
		}
		store, err := auth.NewPostgresStore(databasePool)
		if err != nil {
			return err
		}
		authService, err = auth.NewService(store, fieldCipher, cfg.LookupHMACKey, cfg.SessionTTL, cfg.CookieSecure)
		if err != nil {
			return err
		}
		authService.ConfigureRegistrationPolicy(cfg.RequireEduEmail)
		authService.ConfigureAdminMFA(cfg.RequireAdminMFA)
		distributedLimiter, err = security.NewPostgresFixedWindowLimiter(databasePool)
		if err != nil {
			return err
		}
		authService.ConfigureDistributedLimiter(distributedLimiter)
		if providerConfigured(cfg.GoogleOAuth) {
			if err := authService.ConfigureOAuth("google", auth.OAuthProviderSettings{
				ClientID:     cfg.GoogleOAuth.ClientID,
				ClientSecret: cfg.GoogleOAuth.ClientSecret,
				RedirectURL:  cfg.GoogleOAuth.RedirectURL,
				AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
				TokenURL:     "https://oauth2.googleapis.com/token",
				UserInfoURL:  "https://openidconnect.googleapis.com/v1/userinfo",
			}); err != nil {
				return err
			}
		}
		if providerConfigured(cfg.DiscordOAuth) {
			if err := authService.ConfigureOAuth("discord", auth.OAuthProviderSettings{
				ClientID:     cfg.DiscordOAuth.ClientID,
				ClientSecret: cfg.DiscordOAuth.ClientSecret,
				RedirectURL:  cfg.DiscordOAuth.RedirectURL,
				AuthURL:      "https://discord.com/oauth2/authorize",
				TokenURL:     "https://discord.com/api/oauth2/token",
				UserInfoURL:  "https://discord.com/api/users/@me",
			}); err != nil {
				return err
			}
		}
		if objectStorageConfigured(cfg) {
			blobStore, err = storage.NewMinioStore(cfg.ObjectStorageEndpoint, cfg.ObjectStorageAccessKey, cfg.ObjectStorageSecretKey, cfg.ObjectStorageBucket, cfg.ObjectStorageUseSSL)
			if err != nil {
				return err
			}
		}
		if cfg.ClamAVAddress != "" {
			fileScanner, err = storage.NewClamAVScanner(cfg.ClamAVAddress)
			if err != nil {
				return err
			}
		}
		if cfg.RequireFileScan && objectStorageConfigured(cfg) && fileScanner == nil {
			return errors.New("STA_CLAMAV_ADDRESS is required when file scanning is enabled")
		}
		authHandler, err := auth.NewHandler(authService, logger)
		if err != nil {
			return err
		}
		registrars = append(registrars, authHandler.RegisterRoutes)
		admissionRepository, err = admissions.NewPostgresRepository(databasePool)
		if err != nil {
			return err
		}
		admissionHandler, err := admissions.NewHandler(admissionRepository)
		if err != nil {
			return err
		}
		registrars = append(registrars, admissionHandler.RegisterRoutes)
		admissionAdminHandler, err := admissions.NewAdminHandler(authService, admissionRepository)
		if err != nil {
			return err
		}
		registrars = append(registrars, admissionAdminHandler.RegisterRoutes)
		discoveryRepository, err := brochurediscovery.NewPostgresRepository(databasePool)
		if err != nil {
			return err
		}
		discoveryHandler, err = brochurediscovery.NewHandler(authService, discoveryRepository)
		if err != nil {
			return err
		}
		registrars = append(registrars, discoveryHandler.RegisterRoutes)
		sourceRepository, err := sources.NewPostgresRepository(databasePool)
		if err != nil {
			return err
		}
		sourceHandler, err := sources.NewHandler(authService, sourceRepository)
		if err != nil {
			return err
		}
		registrars = append(registrars, sourceHandler.RegisterRoutes)
		schoolRepository, err := schools.NewPostgresRepository(databasePool)
		if err != nil {
			return err
		}
		schoolHandler, err := schools.NewHandler(authService, schoolRepository)
		if err != nil {
			return err
		}
		registrars = append(registrars, schoolHandler.RegisterRoutes)
		applicationRepository, err := applications.NewPostgresRepository(databasePool)
		if err != nil {
			return err
		}
		applicationHandler, err := applications.NewHandler(authService, applicationRepository)
		if err != nil {
			return err
		}
		registrars = append(registrars, applicationHandler.RegisterRoutes)
		contentRepository, err := content.NewPostgresRepository(databasePool)
		if err != nil {
			return err
		}
		contentHandler, err := content.NewHandler(authService, contentRepository)
		if err != nil {
			return err
		}
		registrars = append(registrars, contentHandler.RegisterRoutes)
		resultRepository, err = results.NewPostgresRepository(databasePool, cfg.LookupHMACKey)
		if err != nil {
			return err
		}
		resultHandler, err := results.NewHandler(authService, resultRepository, fieldCipher, cfg.LookupHMACKey)
		if err != nil {
			return err
		}
		registrars = append(registrars, resultHandler.RegisterRoutes)
		resultImportHandler, err := results.NewImportHandler(authService, resultRepository)
		if err != nil {
			return err
		}
		registrars = append(registrars, resultImportHandler.RegisterRoutes)
		if cfg.TelegramCrossCheckToken != "" {
			telegramRepository, err := telegramcrosscheck.NewPostgresRepository(databasePool)
			if err != nil {
				return err
			}
			telegramHandler, err := telegramcrosscheck.NewHandler(
				authService,
				telegramRepository,
				resultRepository,
				fieldCipher,
				cfg.LookupHMACKey,
				cfg.TelegramCrossCheckToken,
				cfg.TelegramCrossCheckAllowTestProvisioning,
			)
			if err != nil {
				return err
			}
			telegramHandler.ConfigureLogger(logger)
			registrars = append(registrars, telegramHandler.RegisterRoutes)
			logger.Info("Telegram cross-check adapter enabled", "test_provisioning", cfg.TelegramCrossCheckAllowTestProvisioning)
		}
		notificationRepository, err = notifications.NewPostgresRepository(databasePool, fieldCipher)
		if err != nil {
			return err
		}
		authService.ConfigureEmailVerification(notificationRepository, cfg.PublicBaseURL)
		if pg, ok := notificationRepository.(*notifications.PostgresRepository); ok {
			pg.SetEventPublisher(eventHub)
		}
		notificationHandler, err := notifications.NewHandler(authService, notificationRepository)
		if err != nil {
			return err
		}
		registrars = append(registrars, notificationHandler.RegisterRoutes)
		supportRepository, err := support.NewPostgresRepository(databasePool, fieldCipher, cfg.LookupHMACKey, cfg.SupportEmail, cfg.PublicBaseURL)
		if err != nil {
			return err
		}
		supportHandler, err := support.NewHandlerWithConfigAndScanner(authService, supportRepository, cfg.DiscordSupportWebhookSecret, cfg.SupportEmailWebhookSecret, cfg.LookupHMACKey, blobStore, fileScanner)
		if err != nil {
			return err
		}
		supportHandler.ConfigureDistributedLimiter(distributedLimiter)
		registrars = append(registrars, supportHandler.RegisterRoutes)
		verificationRepository, err := verification.NewPostgresRepository(databasePool)
		if err != nil {
			return err
		}
		verificationService, err := verification.NewService(verificationRepository, fieldCipher, cfg.LookupHMACKey, notificationRepository, blobStore)
		if err != nil {
			return err
		}
		verificationService.ConfigureDistributedLimiter(distributedLimiter)
		verificationHandler, err := verification.NewHandlerWithScanner(authService, verificationService, verificationRepository, blobStore, fileScanner)
		if err != nil {
			return err
		}
		registrars = append(registrars, verificationHandler.RegisterRoutes)
		readiness = func(ctx context.Context) error {
			return databasePool.Ping(ctx)
		}
		readinessChecks = append(readinessChecks, httpapi.NamedCheck{
			Name:  "database",
			Check: func(ctx context.Context) error { return databasePool.Ping(ctx) },
		})
	} else {
		logger.Warn("database is not configured; only non-persistent health endpoints are enabled")
	}
	if authService != nil && blobStore != nil {
		portfolioRepository, err := portfolio.NewPostgresRepository(databasePool)
		if err != nil {
			return err
		}
		portfolioHandler, err := portfolio.NewHandlerWithScanner(authService, portfolioRepository, blobStore, fileScanner)
		if err != nil {
			return err
		}
		registrars = append(registrars, portfolioHandler.RegisterRoutes)
	} else if authService != nil {
		logger.Warn("object storage is not configured; portfolio file routes are disabled")
	}
	if authService != nil && databasePool != nil {
		chatRepository, err := chat.NewPostgresRepository(databasePool, cfg.LookupHMACKey)
		if err != nil {
			return err
		}
		chatRepository.SetEventPublisher(eventHub)
		chatHandler, err := chat.NewHandler(authService, chatRepository, cfg.DiscordChatWebhookSecret, cfg.TelegramChatWebhookSecret, cfg.LookupHMACKey)
		if err != nil {
			return err
		}
		chatHandler.ConfigureDistributedLimiter(distributedLimiter)
		registrars = append(registrars, chatHandler.RegisterRoutes)
	}
	if authService != nil && eventHub != nil {
		sseHandler, err := sse.NewHandler(authService, eventHub)
		if err != nil {
			return err
		}
		registrars = append(registrars, sseHandler.RegisterRoutes)
	}
	if authService != nil && databasePool != nil {
		adminHandler, err := admin.NewHandler(authService, databasePool)
		if err != nil {
			return err
		}
		registrars = append(registrars, adminHandler.RegisterRoutes)
	}
	if authService != nil && databasePool != nil && cfg.MeilisearchURL != "" {
		searchClient, err := search.NewClient(cfg.MeilisearchURL, cfg.MeilisearchKey)
		if err != nil {
			return err
		}
		if searchClient != nil {
			searchHandler, err := search.NewHandler(authService, searchClient, databasePool)
			if err != nil {
				return err
			}
			registrars = append(registrars, searchHandler.RegisterRoutes)
			logger.Info("Meilisearch search enabled")
		}
	}
	if cfg.RabbitMQURL != "" {
		messageBroker, err = jobs.OpenBroker(jobs.BrokerConfig{
			URL:          cfg.RabbitMQURL,
			Exchange:     cfg.RabbitMQExchange,
			ExtractQueue: cfg.RabbitMQExtractQueue,
			ResultQueue:  cfg.RabbitMQResultQueue,
			Logger:       logger,
		})
		if err != nil {
			return err
		}
		defer messageBroker.Close()
	} else {
		logger.Info("RabbitMQ is not configured; extraction jobs use the HTTP claim transport")
	}
	if authService != nil && databasePool != nil && admissionRepository != nil {
		ingestionRepository, err := ingestion.NewPostgresRepository(databasePool)
		if err != nil {
			return err
		}
		ingestionService, err = ingestion.NewService(ingestionRepository, messageBroker, logger)
		if err != nil {
			return err
		}
		ingestionHandler, err := ingestion.NewHandler(authService, ingestionRepository, ingestionService)
		if err != nil {
			return err
		}
		registrars = append(registrars, ingestionHandler.RegisterRoutes)
		brochureHandler, err := admissions.NewBrochureHandlerWithDispatcherAndScanner(authService, admissionRepository, blobStore, ingestionService, fileScanner)
		if err != nil {
			return err
		}
		registrars = append(registrars, brochureHandler.RegisterRoutes)
		externalExtractionHandler, err := ingestion.NewExternalHandler(
			authService, ingestionRepository, admissionRepository, resultRepository,
			ingestionService, blobStore, fileScanner, cfg.ExtractionServiceToken,
		)
		if err != nil {
			return err
		}
		registrars = append(registrars, externalExtractionHandler.RegisterRoutes)
	} else if authService != nil && admissionRepository != nil {
		brochureHandler, err := admissions.NewBrochureHandlerWithDispatcherAndScanner(authService, admissionRepository, blobStore, nil, fileScanner)
		if err != nil {
			return err
		}
		registrars = append(registrars, brochureHandler.RegisterRoutes)
	}
	if discoveryHandler != nil {
		discoveryHandler.ConfigureAgent(cfg.BrochureDiscoveryAgentToken, blobStore, fileScanner, ingestionService)
	}

	// Dependency probes for /readyz beyond the database. Each is optional and
	// added only when the corresponding integration is configured.
	if messageBroker != nil {
		readinessChecks = append(readinessChecks, httpapi.NamedCheck{Name: "broker", Check: messageBroker.Ping})
	}
	if pinger, ok := blobStore.(interface {
		Ping(context.Context) error
	}); ok && pinger != nil {
		readinessChecks = append(readinessChecks, httpapi.NamedCheck{Name: "object_storage", Check: pinger.Ping})
	}
	if pinger, ok := fileScanner.(interface {
		Ping(context.Context) error
	}); ok && pinger != nil {
		readinessChecks = append(readinessChecks, httpapi.NamedCheck{Name: "clamav", Check: pinger.Ping})
	}

	handlerOptions := []httpapi.Option{httpapi.WithObserver(obs.ObserveHTTP)}
	if len(readinessChecks) > 0 {
		handlerOptions = append(handlerOptions, httpapi.WithReadinessChecks(readinessChecks...))
	}
	registrars = append(registrars, func(mux *http.ServeMux) {
		mux.Handle("GET /metrics", obs.Handler())
	})

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewHandlerWithOptions(cfg, logger, readiness, handlerOptions, registrars...),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("api server listening", "addr", cfg.HTTPAddr, "environment", cfg.Environment)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	}
}

func providerConfigured(provider config.OAuthProviderConfig) bool {
	return provider.ClientID != "" || provider.ClientSecret != "" || provider.RedirectURL != ""
}

func objectStorageConfigured(cfg config.Config) bool {
	return cfg.ObjectStorageEndpoint != "" || cfg.ObjectStorageAccessKey != "" || cfg.ObjectStorageSecretKey != ""
}
