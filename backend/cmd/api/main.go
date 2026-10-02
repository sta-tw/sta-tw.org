package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"sta-backend/internal/accountapplications"
	"sta-backend/internal/accountmail"
	"sta-backend/internal/admin"
	"sta-backend/internal/admissions"
	"sta-backend/internal/applications"
	"sta-backend/internal/auth"
	"sta-backend/internal/brochurediscovery"
	"sta-backend/internal/calendar"
	"sta-backend/internal/chat"
	"sta-backend/internal/community"
	"sta-backend/internal/config"
	"sta-backend/internal/content"
	"sta-backend/internal/db"
	"sta-backend/internal/discordmail"
	"sta-backend/internal/email"
	"sta-backend/internal/emailinquiries"
	"sta-backend/internal/events"
	"sta-backend/internal/httpapi"
	"sta-backend/internal/ingestion"
	"sta-backend/internal/jobs"
	"sta-backend/internal/mailroutes"
	"sta-backend/internal/notifications"
	"sta-backend/internal/obs"
	"sta-backend/internal/portfolio"
	"sta-backend/internal/profile"
	"sta-backend/internal/publicstats"
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

// timelineChangedNotification decodes the payload of an
// "admissions.timeline_changed" event — see migrations/000074's
// notify_timeline_event_change trigger, which fires this whenever a direct
// SQL UPDATE changes a program_timeline_events row's date, bypassing the
// admin API entirely (so admissions.PostgresRepository's own before/after
// diff never runs). StartDate/EndDate are pointers because the trigger's
// json_build_object emits JSON null for a NULL date column.
type timelineChangedNotification struct {
	AcademicYear int     `json:"academic_year"`
	SchoolCode   string  `json:"school_code"`
	ProgramCode  string  `json:"program_code"`
	SortOrder    int     `json:"sort_order"`
	StartDate    *string `json:"start_date"`
	EndDate      *string `json:"end_date"`
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
	var redisClient *redis.Client
	var admissionRepository *admissions.PostgresRepository
	var resultRepository *results.PostgresRepository
	var discoveryHandler *brochurediscovery.Handler
	var ingestionService *ingestion.Service
	var eventHub *events.Hub
	registrars := make([]httpapi.RouteRegistrar, 0, 2)
	var readiness httpapi.ReadinessCheck
	var readinessChecks []httpapi.NamedCheck

	communityHandler := community.NewHandler(cfg.DiscordCommunityInviteCode)
	registrars = append(registrars, communityHandler.RegisterRoutes)

	hubCtx, hubCancel := context.WithCancel(context.Background())
	defer hubCancel()

	tracingShutdown, err := obs.InitTracing(context.Background(), obs.TracingConfig{
		Endpoint:    cfg.OTelExporterEndpoint,
		Insecure:    cfg.OTelExporterInsecure,
		ServiceName: cfg.OTelServiceName,
		Environment: cfg.Environment,
		SampleRatio: cfg.OTelSampleRatio,
	})
	if err != nil {
		return err
	}
	if obs.TracingEnabled() {
		logger.Info("OTLP trace exporter enabled", "endpoint", cfg.OTelExporterEndpoint, "sample_ratio", cfg.OTelSampleRatio)
	}
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tracingShutdown(flushCtx); err != nil {
			logger.Warn("trace exporter shutdown", "error", err)
		}
	}()

	if cfg.RedisURL != "" {
		redisOpts, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			return fmt.Errorf("parse STA_REDIS_URL: %w", err)
		}
		redisClient = redis.NewClient(redisOpts)
		pingCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err = redisClient.Ping(pingCtx).Err()
		cancel()
		if err != nil {
			return fmt.Errorf("connect to redis: %w", err)
		}
		defer redisClient.Close()
		readinessChecks = append(readinessChecks, httpapi.NamedCheck{
			Name:  "redis",
			Check: func(ctx context.Context) error { return redisClient.Ping(ctx).Err() },
		})
	}

	if cfg.DatabaseURL != "" {
		startupContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		databasePool, err = db.OpenPostgres(startupContext, cfg.DatabaseURL, 40)
		cancel()
		if err != nil {
			return err
		}
		defer databasePool.Close()
		eventHub = events.NewHub(hubCtx, databasePool, logger)
		publicStatsHandler, err := publicstats.NewHandler(databasePool)
		if err != nil {
			return err
		}
		registrars = append(registrars, publicStatsHandler.RegisterRoutes)
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
		if cfg.CookieDomain != "" {
			authService.SetCookieDomain(cfg.CookieDomain)
		}
		authService.ConfigureAdminMFA(cfg.RequireAdminMFA)
		authService.ConfigureAdminMFAGrant(cfg.AdminMFAGrantTTL)
		if err := authService.ConfigureLookupKeyRotation(cfg.LookupHMACSecondaryKeys); err != nil {
			return err
		}
		// Redis is preferred when configured; Postgres stays as the fallback
		// path for local dev/test without a Redis instance running.
		if redisClient != nil {
			distributedLimiter, err = security.NewRedisFixedWindowLimiter(redisClient)
		} else {
			distributedLimiter, err = security.NewPostgresFixedWindowLimiter(databasePool)
		}
		if err != nil {
			return err
		}
		authService.ConfigureDistributedLimiter(distributedLimiter)
		if providerConfigured(cfg.GoogleOAuth) {
			// Calendar scope rides along with login — see auth.CalendarScope.
			if err := authService.ConfigureOAuth("google", auth.OAuthProviderSettings{
				ClientID:     cfg.GoogleOAuth.ClientID,
				ClientSecret: cfg.GoogleOAuth.ClientSecret,
				RedirectURL:  cfg.GoogleOAuth.RedirectURL,
				AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
				TokenURL:     "https://oauth2.googleapis.com/token",
				UserInfoURL:  "https://openidconnect.googleapis.com/v1/userinfo",
				Scopes:       []string{"openid", "email", "profile", auth.CalendarScope},
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
			minioStore, err := storage.NewMinioStore(cfg.ObjectStorageEndpoint, cfg.ObjectStorageAccessKey, cfg.ObjectStorageSecretKey, cfg.ObjectStorageBucket, cfg.ObjectStorageUseSSL)
			if err != nil {
				return err
			}
			if cfg.ObjectStoragePublicEndpoint != "" {
				if err := minioStore.UsePublicEndpointForPresign(cfg.ObjectStoragePublicEndpoint, cfg.ObjectStorageAccessKey, cfg.ObjectStorageSecretKey, cfg.ObjectStoragePublicUseSSL); err != nil {
					return err
				}
			}
			blobStore = minioStore
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
		if cfg.Environment == "production" || cfg.TurnstileSecret != "" {
			verifier, err := auth.NewTurnstileVerifier(cfg.TurnstileSecret, cfg.TurnstileHostname)
			if err != nil {
				return fmt.Errorf("configure Turnstile: %w", err)
			}
			authHandler.ConfigureTurnstile(verifier)
		}
		registrars = append(registrars, authHandler.RegisterRoutes)
		var calendarService *calendar.Service
		if providerConfigured(cfg.GoogleOAuth) {
			calendarEventLinks, err := calendar.NewPostgresEventLinkStore(databasePool)
			if err != nil {
				return err
			}
			calendarService, err = calendar.NewService(store, calendarEventLinks, fieldCipher, cfg.GoogleOAuth.ClientID, cfg.GoogleOAuth.ClientSecret)
			if err != nil {
				return err
			}
			calendarHandler, err := calendar.NewHandler(calendarService, authService)
			if err != nil {
				return err
			}
			registrars = append(registrars, calendarHandler.RegisterRoutes)
		}
		admissionRepository, err = admissions.NewPostgresRepository(databasePool)
		if err != nil {
			return err
		}
		if calendarService != nil {
			// syncTimelineDates pushes one corrected date/time to every Google
			// Calendar event linked to externalID. Shared by both trigger
			// paths below — the app-level save hook and the DB-trigger
			// listener — so a slow or partially-failing Google sync is
			// logged the same way regardless of which path caught the edit.
			syncTimelineDates := func(externalID, isoStart, isoEnd string) {
				if isoStart == "" {
					return
				}
				syncCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				updated, err := calendarService.SyncEventDates(syncCtx, externalID, isoStart, isoEnd)
				cancel()
				if err != nil {
					logger.Error("calendar timeline sync failed", "external_id", externalID, "updated", updated, "error", err)
				} else if updated > 0 {
					logger.Info("calendar timeline sync applied", "external_id", externalID, "updated", updated)
				}
			}
			// Path 1: an admin correcting a timeline date through the normal
			// admin UI save. This catches it synchronously (replaceTimelineEvents'
			// before/after diff), before the DB trigger below would even see it,
			// since that trigger only fires on UPDATE and this path is a
			// DELETE+INSERT.
			admissionRepository.SetTimelineChangeHook(func(_ context.Context, changes []admissions.TimelineChange) {
				go func() {
					for _, change := range changes {
						externalID := fmt.Sprintf("%s:timeline-%d", change.ProgramIdentifier, change.SortOrder)
						syncTimelineDates(externalID, change.ISOStart, change.ISOEnd)
					}
				}()
			})
			// Path 2: a timeline row corrected by a direct SQL UPDATE (e.g. an
			// admin fixing a data-entry mistake over psql, bypassing the admin
			// API entirely) — program_timeline_events_notify_change (see
			// migrations/000074) fires a Postgres NOTIFY the app-level hook
			// above can never see, since no Go code ran. eventHub already
			// has the LISTEN connection open for SSE; this just adds another
			// subscriber on a different topic.
			if eventHub != nil {
				go func() {
					for ev := range eventHub.Subscribe(hubCtx, "admissions.timeline_changed") {
						var change timelineChangedNotification
						if err := json.Unmarshal(ev.Data, &change); err != nil {
							logger.Warn("dropping malformed timeline change notification", "error", err)
							continue
						}
						externalID := fmt.Sprintf("%03d-%s-%s:timeline-%d", change.AcademicYear, change.SchoolCode, change.ProgramCode, change.SortOrder)
						isoStart, isoEnd := "", ""
						if change.StartDate != nil {
							isoStart = *change.StartDate
							isoEnd = isoStart
							if change.EndDate != nil {
								isoEnd = *change.EndDate
							}
						}
						syncTimelineDates(externalID, isoStart, isoEnd)
					}
				}()
			}
		}
		// Cached only for the public catalogue handler — admin/ingestion/results
		// consumers below keep using the uncached admissionRepository directly
		// so they never see stale reads.
		var publicAdmissionRepository admissions.Repository = admissionRepository
		var admissionsInvalidator admissions.Invalidator
		if redisClient != nil {
			cachedRepo := admissions.NewCachedRepository(admissionRepository, redisClient, 5*time.Minute)
			publicAdmissionRepository = cachedRepo
			admissionsInvalidator = cachedRepo
		}
		admissionHandler, err := admissions.NewHandler(publicAdmissionRepository)
		if err != nil {
			return err
		}
		registrars = append(registrars, admissionHandler.RegisterRoutes)
		admissionAdminHandler, err := admissions.NewAdminHandler(authService, admissionRepository, admissionsInvalidator)
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
		var publicSchoolRepository schools.Repository = schoolRepository
		if redisClient != nil {
			publicSchoolRepository = schools.NewCachedRepository(schoolRepository, redisClient, 10*time.Minute)
		}
		schoolHandler, err := schools.NewHandler(authService, publicSchoolRepository)
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
		applicationHandler.ConfigureDistributedLimiter(distributedLimiter)
		registrars = append(registrars, applicationHandler.RegisterRoutes)
		contentRepository, err := content.NewPostgresRepository(databasePool)
		if err != nil {
			return err
		}
		contentHandler, err := content.NewHandler(authService, contentRepository)
		if err != nil {
			return err
		}
		contentHandler.ConfigureDistributedLimiter(distributedLimiter)
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
		verificationHandler.ConfigureDistributedLimiter(distributedLimiter)
		registrars = append(registrars, verificationHandler.RegisterRoutes)
		if authService != nil && blobStore != nil {
			var accountApplicationMailer email.Sender
			if cfg.SMTPHost != "" && cfg.SMTPFrom != "" {
				// account@, not noreply@: these emails invite a reply and only account@ receives one.
				mailer, err := email.NewSMTPSender(email.SMTPConfig{
					Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername,
					Password: cfg.SMTPPassword, From: "account@" + cfg.MailDomain, UseTLS: cfg.SMTPUseTLS,
					AllowInsecure: cfg.SMTPAllowInsecure,
				})
				if err != nil {
					return err
				}
				accountApplicationMailer = mailer
			}
			// Telegram notification is retired (moving to Discord); nil is a
			// supported "no notifier configured" state throughout this service.
			accountApplicationRepository, err := accountapplications.NewPostgresRepository(databasePool)
			if err != nil {
				return err
			}
			accountApplicationService, err := accountapplications.NewService(
				accountApplicationRepository, blobStore, fileScanner, fieldCipher, cfg.LookupHMACKey,
				authService, nil, accountApplicationMailer, cfg.MailDomain, logger,
			)
			if err != nil {
				return err
			}
			accountApplicationHandler, err := accountapplications.NewHandler(
				accountApplicationService, cfg.AccountApplicationReviewToken,
			)
			if err != nil {
				return err
			}
			accountApplicationHandler.ConfigureDistributedLimiter(distributedLimiter)
			registrars = append(registrars, accountApplicationHandler.RegisterRoutes)

			mailRoutesRepository, err := mailroutes.NewPostgresRepository(databasePool)
			if err != nil {
				return err
			}
			var mailPermissions mailroutes.PermissionsClient
			if cfg.DiscordMailBotToken != "" && cfg.DiscordMailGuildID != "" && cfg.DiscordMailForumCategoryID != "" {
				mailPermissions, err = mailroutes.NewHTTPPermissionsClient(cfg.DiscordMailBotToken, cfg.DiscordMailGuildID, cfg.DiscordMailForumCategoryID)
				if err != nil {
					return err
				}
			} else {
				logger.Warn("STA_DISCORD_MAIL_BOT_TOKEN/STA_DISCORD_MAIL_GUILD_ID/STA_DISCORD_MAIL_FORUM_CATEGORY_ID are not all set; creating new mail routes is disabled")
			}
			mailRoutesHandler, err := mailroutes.NewHandler(authService, mailRoutesRepository, mailPermissions, logger)
			if err != nil {
				return err
			}
			registrars = append(registrars, mailRoutesHandler.RegisterRoutes)

			var forumNotifier emailinquiries.ForumNotifier
			if cfg.DiscordMailBotToken != "" {
				forumNotifier, err = emailinquiries.NewHTTPDiscordNotifier(cfg.DiscordMailBotToken)
				if err != nil {
					return err
				}
			} else {
				logger.Warn("STA_DISCORD_MAIL_BOT_TOKEN is not set; inbound mail inquiries will not be posted to Discord")
			}

			inquiryRepository, err := emailinquiries.NewPostgresRepository(databasePool)
			if err != nil {
				return err
			}
			inquiryService, err := emailinquiries.NewService(
				inquiryRepository, blobStore, fileScanner, fieldCipher, cfg.LookupHMACKey,
				forumNotifier, mailRoutesRepository, accountApplicationMailer, cfg.MailDomain, logger,
			)
			if err != nil {
				return err
			}
			inquiryAdminHandler, err := emailinquiries.NewHandler(authService, inquiryService, mailRoutesRepository.IsAdmin)
			if err != nil {
				return err
			}
			registrars = append(registrars, inquiryAdminHandler.RegisterRoutes)

			if cfg.DiscordMailApplicationPublicKey != "" {
				discordMailHandler, err := discordmail.NewHandler(cfg.DiscordMailApplicationPublicKey, inquiryService)
				if err != nil {
					return err
				}
				if redisClient != nil {
					draftStore, err := discordmail.NewRedisDraftStore(redisClient)
					if err != nil {
						return err
					}
					discordMailHandler.ConfigureDraftStore(draftStore)
				} else {
					logger.Warn("STA_REDIS_URL is not set; /re skips the 送出/編輯/刪除 preview and sends immediately on submit")
				}
				registrars = append(registrars, discordMailHandler.RegisterRoutes)
			} else {
				logger.Warn("STA_DISCORD_MAIL_APPLICATION_PUBLIC_KEY is not set; the /re slash command endpoint is disabled")
			}

			if cfg.DiscordMailBotToken != "" {
				// Purely cosmetic: keeps the bot showing "online" in Discord
				// so a human has a quick at-a-glance health signal. Every
				// actual feature is plain HTTPS and doesn't depend on this
				// connection at all — see discordmail.RunPresence's doc
				// comment.
				go discordmail.RunPresence(hubCtx, cfg.DiscordMailBotToken, logger)
			}

			mailIntakeHandler, err := accountmail.NewHandler(cfg.AccountApplicationMailToken, accountApplicationService, inquiryService, mailRoutesRepository)
			if err != nil {
				return err
			}
			registrars = append(registrars, mailIntakeHandler.RegisterRoutes)
		}
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
		portfolioHandler.ConfigureDistributedLimiter(distributedLimiter)
		registrars = append(registrars, portfolioHandler.RegisterRoutes)

		profileRepository, err := profile.NewPostgresRepository(databasePool)
		if err != nil {
			return err
		}
		profileHandler, err := profile.NewHandler(authService, profileRepository, blobStore, fileScanner)
		if err != nil {
			return err
		}
		profileHandler.ConfigureDistributedLimiter(distributedLimiter)
		registrars = append(registrars, profileHandler.RegisterRoutes)
	} else if authService != nil {
		logger.Warn("object storage is not configured; portfolio and profile file routes are disabled")
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
			searchHandler.ConfigureDistributedLimiter(distributedLimiter)
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
		ingestionHandler, err := ingestion.NewHandlerWithBlobStore(authService, ingestionRepository, ingestionService, blobStore)
		if err != nil {
			return err
		}
		registrars = append(registrars, ingestionHandler.RegisterRoutes)
		brochureHandler, err := admissions.NewBrochureHandlerWithDispatcherAndScanner(authService, admissionRepository, blobStore, ingestionService, fileScanner)
		if err != nil {
			return err
		}
		brochureHandler.ConfigureDistributedLimiter(distributedLimiter)
		registrars = append(registrars, brochureHandler.RegisterRoutes)
		externalExtractionHandler, err := ingestion.NewExternalHandler(
			authService, ingestionRepository, admissionRepository, resultRepository,
			ingestionService, blobStore, fileScanner, cfg.ExtractionServiceToken,
		)
		if err != nil {
			return err
		}
		externalExtractionHandler.ConfigureDistributedLimiter(distributedLimiter)
		registrars = append(registrars, externalExtractionHandler.RegisterRoutes)
	} else if authService != nil && admissionRepository != nil {
		brochureHandler, err := admissions.NewBrochureHandlerWithDispatcherAndScanner(authService, admissionRepository, blobStore, nil, fileScanner)
		if err != nil {
			return err
		}
		brochureHandler.ConfigureDistributedLimiter(distributedLimiter)
		registrars = append(registrars, brochureHandler.RegisterRoutes)
	}
	if discoveryHandler != nil {
		discoveryHandler.ConfigureAgent(cfg.BrochureDiscoveryAgentToken, blobStore, fileScanner, ingestionService)
	}

	// Optional /readyz probes, added only when configured.
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

	server.RegisterOnShutdown(hubCancel)

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
		hubCancel()
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		shutdownErr := server.Shutdown(shutdownCtx)
		hubCancel()
		return shutdownErr
	}
}

func providerConfigured(provider config.OAuthProviderConfig) bool {
	return provider.ClientID != "" || provider.ClientSecret != "" || provider.RedirectURL != ""
}

func objectStorageConfigured(cfg config.Config) bool {
	return cfg.ObjectStorageEndpoint != "" || cfg.ObjectStorageAccessKey != "" || cfg.ObjectStorageSecretKey != ""
}
