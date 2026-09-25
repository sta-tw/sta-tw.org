package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	clearConfigEnv(t)

	config, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if config.Environment != developmentEnvironment {
		t.Fatalf("Environment = %q, want %q", config.Environment, developmentEnvironment)
	}
	if config.HTTPAddr != defaultHTTPAddr {
		t.Fatalf("HTTPAddr = %q, want %q", config.HTTPAddr, defaultHTTPAddr)
	}
	if len(config.AllowedOrigins) != 1 || config.AllowedOrigins[0] != defaultAllowedOrigin {
		t.Fatalf("AllowedOrigins = %#v, want [%q]", config.AllowedOrigins, defaultAllowedOrigin)
	}
	if config.RequireAdminMFA {
		t.Fatal("development should not require administrator MFA by default")
	}
}

func TestProductionRequiresAdminMFAByDefault(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("STA_ENV", productionEnvironment)
	t.Setenv("STA_ALLOWED_ORIGINS", "https://app.example.test")
	setProductionSecrets(t)

	config, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !config.RequireAdminMFA {
		t.Fatal("production should require administrator MFA by default")
	}
}

func TestLoadAdminMFAOverride(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("STA_ENV", productionEnvironment)
	t.Setenv("STA_ALLOWED_ORIGINS", "https://app.example.test")
	t.Setenv("STA_REQUIRE_ADMIN_MFA", "false")
	setProductionSecrets(t)

	config, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if config.RequireAdminMFA {
		t.Fatal("explicit administrator MFA override was ignored")
	}
}

func TestLoadTelegramCrossCheckAdapterSettings(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("STA_TELEGRAM_CROSS_CHECK_TOKEN", "test-telegram-cross-check-service-token")
	t.Setenv("STA_TELEGRAM_CROSS_CHECK_ALLOW_TEST_PROVISIONING", "true")

	config, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if config.TelegramCrossCheckToken != "test-telegram-cross-check-service-token" {
		t.Fatalf("TelegramCrossCheckToken = %q", config.TelegramCrossCheckToken)
	}
	if !config.TelegramCrossCheckAllowTestProvisioning {
		t.Fatal("Telegram cross-check test provisioning was not enabled")
	}
}

func TestLoadExtractionServiceToken(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("STA_EXTRACTION_SERVICE_TOKEN", "local-extraction-service-token-123456")

	config, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if config.ExtractionServiceToken != "local-extraction-service-token-123456" {
		t.Fatalf("ExtractionServiceToken = %q", config.ExtractionServiceToken)
	}
}

func TestLoadRejectsTelegramTestProvisioningInProduction(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("STA_ENV", productionEnvironment)
	t.Setenv("STA_ALLOWED_ORIGINS", "https://app.example.test")
	t.Setenv("STA_TELEGRAM_CROSS_CHECK_ALLOW_TEST_PROVISIONING", "true")
	setProductionSecrets(t)

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want production Telegram provisioning error")
	}
}

func TestLoadRejectsWildcardOrigin(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("STA_ALLOWED_ORIGINS", "*")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want wildcard origin error")
	}
}

func TestLoadRejectsHTTPProductionOrigin(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("STA_ENV", productionEnvironment)
	t.Setenv("STA_ALLOWED_ORIGINS", "http://app.example.test")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want HTTPS origin error")
	}
}

func TestLoadRejectsLookalikeLocalhostProductionOrigin(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("STA_ENV", productionEnvironment)
	t.Setenv("STA_ALLOWED_ORIGINS", "http://localhost.attacker.example")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want lookalike localhost origin error")
	}
}

func TestLoadRejectsOriginPath(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("STA_ALLOWED_ORIGINS", "https://app.example.test/frontend")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want origin path error")
	}
}

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"STA_ENV",
		"STA_HTTP_ADDR",
		"STA_ALLOWED_ORIGINS",
		"STA_MAX_JSON_BODY_BYTES",
		"STA_SHUTDOWN_TIMEOUT",
		"STA_SESSION_TTL",
		"STA_DATABASE_URL",
		"STA_RABBITMQ_URL",
		"STA_RABBITMQ_EXCHANGE",
		"STA_RABBITMQ_EXTRACT_QUEUE",
		"STA_RABBITMQ_RESULT_QUEUE",
		"STA_OBJECT_STORAGE_ENDPOINT",
		"STA_OBJECT_STORAGE_ACCESS_KEY",
		"STA_OBJECT_STORAGE_SECRET_KEY",
		"STA_OBJECT_STORAGE_BUCKET",
		"STA_OBJECT_STORAGE_USE_SSL",
		"STA_CLAMAV_ADDRESS",
		"STA_REQUIRE_FILE_SCAN",
		"STA_DISCORD_CHAT_WEBHOOK_SECRET",
		"STA_TELEGRAM_CHAT_WEBHOOK_SECRET",
		"STA_DISCORD_CHAT_BOT_TOKEN",
		"STA_DISCORD_CHAT_CHANNEL_ID",
		"STA_SUPPORT_EMAIL",
		"STA_SUPPORT_EMAIL_WEBHOOK_SECRET",
		"STA_DISCORD_SUPPORT_WEBHOOK_SECRET",
		"STA_DISCORD_SUPPORT_BOT_TOKEN",
		"STA_DISCORD_SUPPORT_GUILD_ID",
		"STA_DISCORD_SUPPORT_CATEGORY_ID",
		"STA_DISCORD_SUPPORT_ARCHIVE_CATEGORY_ID",
		"STA_DISCORD_SUPPORT_ROLE_ID",
		"STA_TELEGRAM_BOT_TOKEN",
		"STA_TELEGRAM_CHAT_ID",
		"STA_TELEGRAM_CROSS_CHECK_TOKEN",
		"STA_TELEGRAM_CROSS_CHECK_ALLOW_TEST_PROVISIONING",
		"STA_SMTP_HOST",
		"STA_SMTP_PORT",
		"STA_SMTP_USERNAME",
		"STA_SMTP_PASSWORD",
		"STA_SMTP_FROM",
		"STA_SMTP_USE_TLS",
		"STA_PUBLIC_BASE_URL",
		"STA_EXTRACTION_SERVICE_TOKEN",
		"STA_EXTERNAL_INGESTION_TOKEN",
		"STA_REQUIRE_ADMIN_MFA",
		"STA_EMAIL_ENCRYPTION_KEY",
		"STA_LOOKUP_HMAC_KEY",
		"STA_GOOGLE_CLIENT_ID",
		"STA_GOOGLE_CLIENT_SECRET",
		"STA_GOOGLE_REDIRECT_URL",
		"STA_DISCORD_CLIENT_ID",
		"STA_DISCORD_CLIENT_SECRET",
		"STA_DISCORD_REDIRECT_URL",
	} {
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("Unsetenv(%q): %v", key, err)
		}
	}
}

func setProductionSecrets(t *testing.T) {
	t.Helper()
	t.Setenv("STA_DATABASE_URL", "postgres://localhost/sta")
	t.Setenv("STA_EMAIL_ENCRYPTION_KEY", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	t.Setenv("STA_LOOKUP_HMAC_KEY", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
}
