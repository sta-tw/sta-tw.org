package config

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHTTPAddr         = ":8080"
	defaultMaxJSONBodyBytes = int64(2 << 20)
	defaultShutdownTimeout  = 10 * time.Second
	defaultAllowedOrigin    = "http://localhost:3000"
	productionEnvironment   = "production"
	developmentEnvironment  = "development"
)

// Config contains process-level configuration. Secrets and provider credentials
// must be injected through a secret manager or environment variables, never
// committed to the repository.
type Config struct {
	Environment                             string
	HTTPAddr                                string
	AllowedOrigins                          []string
	DatabaseURL                             string
	RabbitMQURL                             string
	RabbitMQExchange                        string
	RabbitMQExtractQueue                    string
	RabbitMQResultQueue                     string
	ObjectStorageEndpoint                   string
	ObjectStorageAccessKey                  string
	ObjectStorageSecretKey                  string
	ObjectStorageBucket                     string
	ObjectStorageUseSSL                     bool
	ClamAVAddress                           string
	RequireFileScan                         bool
	DiscordChatWebhookSecret                string
	TelegramChatWebhookSecret               string
	DiscordChatBotToken                     string
	DiscordChatChannelID                    string
	SupportEmail                            string
	SupportEmailWebhookSecret               string
	DiscordSupportWebhookSecret             string
	DiscordSupportBotToken                  string
	DiscordSupportGuildID                   string
	DiscordSupportCategoryID                string
	DiscordSupportArchiveCategoryID         string
	DiscordSupportRoleID                    string
	TelegramBotToken                        string
	TelegramChatID                          string
	TelegramCrossCheckToken                 string
	TelegramCrossCheckAllowTestProvisioning bool
	BrochureDiscoveryAgentToken             string
	ExtractionServiceToken                  string
	SMTPHost                                string
	SMTPPort                                int
	SMTPUsername                            string
	SMTPPassword                            string
	SMTPFrom                                string
	SMTPUseTLS                              bool
	PublicBaseURL                           string
	MeilisearchURL                          string
	MeilisearchKey                          string
	RequireEduEmail                         bool
	RequireAdminMFA                         bool
	EmailEncryptionKey                      []byte
	LookupHMACKey                           []byte
	// FieldEncryptionKeys, when set, turns FieldCipher into a rotating keyring.
	// Format: STA_FIELD_ENCRYPTION_KEYS="1:<base64>,2:<base64>" and
	// STA_FIELD_ENCRYPTION_PRIMARY_VERSION="2". EmailEncryptionKey stays the
	// legacy (unversioned) read key.
	FieldEncryptionKeys           map[byte][]byte
	FieldEncryptionPrimaryVersion byte
	SessionTTL                    time.Duration
	GoogleOAuth                   OAuthProviderConfig
	DiscordOAuth                  OAuthProviderConfig
	MaxJSONBodyBytes              int64
	ShutdownTimeout               time.Duration
	EnableDebugResponses          bool
	CookieSecure                  bool
}

type OAuthProviderConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

func Load() (Config, error) {
	config := Config{
		Environment:                             valueOrDefault("STA_ENV", developmentEnvironment),
		HTTPAddr:                                valueOrDefault("STA_HTTP_ADDR", defaultHTTPAddr),
		AllowedOrigins:                          splitCSV(valueOrDefault("STA_ALLOWED_ORIGINS", defaultAllowedOrigin)),
		DatabaseURL:                             strings.TrimSpace(os.Getenv("STA_DATABASE_URL")),
		RabbitMQURL:                             strings.TrimSpace(os.Getenv("STA_RABBITMQ_URL")),
		RabbitMQExchange:                        valueOrDefault("STA_RABBITMQ_EXCHANGE", "sta.events"),
		RabbitMQExtractQueue:                    valueOrDefault("STA_RABBITMQ_EXTRACT_QUEUE", "sta.admissions.extract"),
		RabbitMQResultQueue:                     valueOrDefault("STA_RABBITMQ_RESULT_QUEUE", "sta.admissions.extracted"),
		ObjectStorageEndpoint:                   strings.TrimSpace(os.Getenv("STA_OBJECT_STORAGE_ENDPOINT")),
		ObjectStorageAccessKey:                  strings.TrimSpace(os.Getenv("STA_OBJECT_STORAGE_ACCESS_KEY")),
		ObjectStorageSecretKey:                  strings.TrimSpace(os.Getenv("STA_OBJECT_STORAGE_SECRET_KEY")),
		ObjectStorageBucket:                     valueOrDefault("STA_OBJECT_STORAGE_BUCKET", "sta-private"),
		ObjectStorageUseSSL:                     strings.EqualFold(strings.TrimSpace(os.Getenv("STA_OBJECT_STORAGE_USE_SSL")), "true"),
		ClamAVAddress:                           strings.TrimSpace(os.Getenv("STA_CLAMAV_ADDRESS")),
		RequireFileScan:                         false,
		DiscordChatWebhookSecret:                strings.TrimSpace(os.Getenv("STA_DISCORD_CHAT_WEBHOOK_SECRET")),
		TelegramChatWebhookSecret:               strings.TrimSpace(os.Getenv("STA_TELEGRAM_CHAT_WEBHOOK_SECRET")),
		DiscordChatBotToken:                     strings.TrimSpace(os.Getenv("STA_DISCORD_CHAT_BOT_TOKEN")),
		DiscordChatChannelID:                    strings.TrimSpace(os.Getenv("STA_DISCORD_CHAT_CHANNEL_ID")),
		SupportEmail:                            strings.TrimSpace(os.Getenv("STA_SUPPORT_EMAIL")),
		SupportEmailWebhookSecret:               strings.TrimSpace(os.Getenv("STA_SUPPORT_EMAIL_WEBHOOK_SECRET")),
		DiscordSupportWebhookSecret:             strings.TrimSpace(os.Getenv("STA_DISCORD_SUPPORT_WEBHOOK_SECRET")),
		DiscordSupportBotToken:                  strings.TrimSpace(os.Getenv("STA_DISCORD_SUPPORT_BOT_TOKEN")),
		DiscordSupportGuildID:                   strings.TrimSpace(os.Getenv("STA_DISCORD_SUPPORT_GUILD_ID")),
		DiscordSupportCategoryID:                strings.TrimSpace(os.Getenv("STA_DISCORD_SUPPORT_CATEGORY_ID")),
		DiscordSupportArchiveCategoryID:         strings.TrimSpace(os.Getenv("STA_DISCORD_SUPPORT_ARCHIVE_CATEGORY_ID")),
		DiscordSupportRoleID:                    strings.TrimSpace(os.Getenv("STA_DISCORD_SUPPORT_ROLE_ID")),
		TelegramBotToken:                        strings.TrimSpace(os.Getenv("STA_TELEGRAM_BOT_TOKEN")),
		TelegramChatID:                          strings.TrimSpace(os.Getenv("STA_TELEGRAM_CHAT_ID")),
		TelegramCrossCheckToken:                 strings.TrimSpace(os.Getenv("STA_TELEGRAM_CROSS_CHECK_TOKEN")),
		TelegramCrossCheckAllowTestProvisioning: strings.EqualFold(strings.TrimSpace(os.Getenv("STA_TELEGRAM_CROSS_CHECK_ALLOW_TEST_PROVISIONING")), "true"),
		BrochureDiscoveryAgentToken:             strings.TrimSpace(os.Getenv("STA_BROCHURE_DISCOVERY_AGENT_TOKEN")),
		ExtractionServiceToken:                  firstNonEmptyEnv("STA_EXTRACTION_SERVICE_TOKEN", "STA_EXTERNAL_INGESTION_TOKEN"),
		SMTPHost:                                strings.TrimSpace(os.Getenv("STA_SMTP_HOST")),
		SMTPUsername:                            strings.TrimSpace(os.Getenv("STA_SMTP_USERNAME")),
		SMTPPassword:                            os.Getenv("STA_SMTP_PASSWORD"),
		SMTPFrom:                                strings.TrimSpace(os.Getenv("STA_SMTP_FROM")),
		SMTPUseTLS:                              !strings.EqualFold(strings.TrimSpace(os.Getenv("STA_SMTP_USE_TLS")), "false"),
		PublicBaseURL:                           strings.TrimRight(strings.TrimSpace(os.Getenv("STA_PUBLIC_BASE_URL")), "/"),
		MeilisearchURL:                          strings.TrimSpace(os.Getenv("STA_MEILISEARCH_URL")),
		MeilisearchKey:                          strings.TrimSpace(os.Getenv("STA_MEILISEARCH_KEY")),
		RequireEduEmail:                         strings.EqualFold(valueOrDefault("STA_REQUIRE_EDU_EMAIL", "false"), "true"),
		RequireAdminMFA:                         false,
		MaxJSONBodyBytes:                        defaultMaxJSONBodyBytes,
		ShutdownTimeout:                         defaultShutdownTimeout,
		SessionTTL:                              30 * 24 * time.Hour,
		EnableDebugResponses:                    false,
		CookieSecure:                            false,
	}

	var err error
	if config.EmailEncryptionKey, err = decodeKey("STA_EMAIL_ENCRYPTION_KEY"); err != nil {
		return Config{}, err
	}
	if config.LookupHMACKey, err = decodeKey("STA_LOOKUP_HMAC_KEY"); err != nil {
		return Config{}, err
	}
	if config.FieldEncryptionKeys, config.FieldEncryptionPrimaryVersion, err = decodeKeyRing(); err != nil {
		return Config{}, err
	}
	config.GoogleOAuth = oauthProviderFromEnv("STA_GOOGLE_CLIENT_ID", "STA_GOOGLE_CLIENT_SECRET", "STA_GOOGLE_REDIRECT_URL")
	config.DiscordOAuth = oauthProviderFromEnv("STA_DISCORD_CLIENT_ID", "STA_DISCORD_CLIENT_SECRET", "STA_DISCORD_REDIRECT_URL")
	if config.DiscordSupportBotToken == "" {
		config.DiscordSupportBotToken = config.DiscordChatBotToken
	}

	if raw := strings.TrimSpace(os.Getenv("STA_MAX_JSON_BODY_BYTES")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed <= 0 {
			return Config{}, fmt.Errorf("STA_MAX_JSON_BODY_BYTES must be a positive integer")
		}
		config.MaxJSONBodyBytes = parsed
	}

	if raw := strings.TrimSpace(os.Getenv("STA_SHUTDOWN_TIMEOUT")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			return Config{}, fmt.Errorf("STA_SHUTDOWN_TIMEOUT must be a positive duration")
		}
		config.ShutdownTimeout = parsed
	}
	if raw := strings.TrimSpace(os.Getenv("STA_SESSION_TTL")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			return Config{}, fmt.Errorf("STA_SESSION_TTL must be a positive duration")
		}
		config.SessionTTL = parsed
	}
	if raw := strings.TrimSpace(os.Getenv("STA_SMTP_PORT")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 65535 {
			return Config{}, fmt.Errorf("STA_SMTP_PORT must be a valid TCP port")
		}
		config.SMTPPort = parsed
	}
	if config.Environment != developmentEnvironment && config.Environment != productionEnvironment && config.Environment != "test" {
		return Config{}, fmt.Errorf("STA_ENV must be development, test, or production")
	}
	if raw, exists := os.LookupEnv("STA_REQUIRE_ADMIN_MFA"); exists {
		parsed, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			return Config{}, fmt.Errorf("STA_REQUIRE_ADMIN_MFA must be true or false")
		}
		config.RequireAdminMFA = parsed
	} else if config.Environment == productionEnvironment {
		config.RequireAdminMFA = true
	}
	if raw, exists := os.LookupEnv("STA_REQUIRE_FILE_SCAN"); exists {
		parsed, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			return Config{}, fmt.Errorf("STA_REQUIRE_FILE_SCAN must be true or false")
		}
		config.RequireFileScan = parsed
	} else if config.Environment == productionEnvironment {
		config.RequireFileScan = true
	}
	if raw, exists := os.LookupEnv("STA_TELEGRAM_CROSS_CHECK_ALLOW_TEST_PROVISIONING"); exists {
		parsed, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			return Config{}, fmt.Errorf("STA_TELEGRAM_CROSS_CHECK_ALLOW_TEST_PROVISIONING must be true or false")
		}
		config.TelegramCrossCheckAllowTestProvisioning = parsed
	}
	if strings.TrimSpace(os.Getenv("STA_REQUIRE_EDU_EMAIL")) == "" && config.Environment == productionEnvironment {
		config.RequireEduEmail = true
	}
	if config.HTTPAddr == "" {
		return Config{}, fmt.Errorf("STA_HTTP_ADDR must not be empty")
	}
	if err := validateOrigins(config.Environment, config.AllowedOrigins); err != nil {
		return Config{}, err
	}
	if config.Environment == productionEnvironment {
		if config.DatabaseURL == "" {
			return Config{}, fmt.Errorf("STA_DATABASE_URL is required in production")
		}
		if len(config.EmailEncryptionKey) != 32 || len(config.LookupHMACKey) != 32 {
			return Config{}, fmt.Errorf("field encryption and lookup keys are required in production")
		}
		if config.TelegramCrossCheckAllowTestProvisioning {
			return Config{}, fmt.Errorf("Telegram cross-check test provisioning must be disabled in production")
		}
		if config.TelegramCrossCheckToken != "" && len([]rune(config.TelegramCrossCheckToken)) < 32 {
			return Config{}, fmt.Errorf("STA_TELEGRAM_CROSS_CHECK_TOKEN must contain at least 32 characters in production")
		}
		if config.BrochureDiscoveryAgentToken != "" && len(config.BrochureDiscoveryAgentToken) < 32 {
			return Config{}, fmt.Errorf("STA_BROCHURE_DISCOVERY_AGENT_TOKEN must contain at least 32 characters in production")
		}
		if config.ExtractionServiceToken != "" && len(config.ExtractionServiceToken) < 32 {
			return Config{}, fmt.Errorf("STA_EXTRACTION_SERVICE_TOKEN must contain at least 32 characters in production")
		}
	}
	config.EnableDebugResponses = config.Environment == developmentEnvironment
	config.CookieSecure = config.Environment == productionEnvironment

	return config, nil
}

func decodeKey(key string) ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil, nil
	}
	decoded, err := base64.RawStdEncoding.DecodeString(raw)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(raw)
	}
	if err != nil || len(decoded) != 32 {
		return nil, fmt.Errorf("%s must be a base64-encoded 32-byte key", key)
	}
	return decoded, nil
}

// decodeKeyRing parses STA_FIELD_ENCRYPTION_KEYS ("1:<b64>,2:<b64>") plus
// STA_FIELD_ENCRYPTION_PRIMARY_VERSION. Both empty means "no keyring" (the
// single-key path stays in effect); a partial config is a hard error.
func decodeKeyRing() (map[byte][]byte, byte, error) {
	raw := strings.TrimSpace(os.Getenv("STA_FIELD_ENCRYPTION_KEYS"))
	primaryRaw := strings.TrimSpace(os.Getenv("STA_FIELD_ENCRYPTION_PRIMARY_VERSION"))
	if raw == "" && primaryRaw == "" {
		return nil, 0, nil
	}
	if raw == "" || primaryRaw == "" {
		return nil, 0, fmt.Errorf("STA_FIELD_ENCRYPTION_KEYS and STA_FIELD_ENCRYPTION_PRIMARY_VERSION must be set together")
	}
	primary, err := strconv.Atoi(primaryRaw)
	if err != nil || primary < 1 || primary > 255 {
		return nil, 0, fmt.Errorf("STA_FIELD_ENCRYPTION_PRIMARY_VERSION must be an integer 1-255")
	}
	keys := make(map[byte][]byte)
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		version, encoded, ok := strings.Cut(pair, ":")
		if !ok {
			return nil, 0, fmt.Errorf("STA_FIELD_ENCRYPTION_KEYS entries must be \"<version>:<base64 key>\"")
		}
		v, err := strconv.Atoi(strings.TrimSpace(version))
		if err != nil || v < 1 || v > 255 {
			return nil, 0, fmt.Errorf("STA_FIELD_ENCRYPTION_KEYS version %q must be an integer 1-255", version)
		}
		decoded, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(encoded))
		if err != nil {
			decoded, err = base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
		}
		if err != nil || len(decoded) != 32 {
			return nil, 0, fmt.Errorf("STA_FIELD_ENCRYPTION_KEYS version %d must be a base64-encoded 32-byte key", v)
		}
		keys[byte(v)] = decoded
	}
	if len(keys[byte(primary)]) != 32 {
		return nil, 0, fmt.Errorf("STA_FIELD_ENCRYPTION_PRIMARY_VERSION %d has no matching key in STA_FIELD_ENCRYPTION_KEYS", primary)
	}
	return keys, byte(primary), nil
}

func oauthProviderFromEnv(clientIDKey, clientSecretKey, redirectURLKey string) OAuthProviderConfig {
	return OAuthProviderConfig{
		ClientID:     strings.TrimSpace(os.Getenv(clientIDKey)),
		ClientSecret: strings.TrimSpace(os.Getenv(clientSecretKey)),
		RedirectURL:  strings.TrimSpace(os.Getenv(redirectURLKey)),
	}
}

func valueOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, exists := seen[part]; exists {
			continue
		}
		seen[part] = struct{}{}
		result = append(result, part)
	}
	return result
}

func validateOrigins(environment string, origins []string) error {
	if len(origins) == 0 {
		return fmt.Errorf("STA_ALLOWED_ORIGINS must contain at least one exact origin")
	}
	for _, origin := range origins {
		if origin == "*" {
			return fmt.Errorf("STA_ALLOWED_ORIGINS must not use wildcard origins")
		}
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("STA_ALLOWED_ORIGINS contains an invalid origin: %q", origin)
		}
		if parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return fmt.Errorf("STA_ALLOWED_ORIGINS must contain only scheme and authority: %q", origin)
		}
		if environment == productionEnvironment && parsed.Scheme == "http" && parsed.Hostname() != "localhost" {
			return fmt.Errorf("production origins must use HTTPS: %q", origin)
		}
	}
	return nil
}
