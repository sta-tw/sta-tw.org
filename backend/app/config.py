from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        extra="ignore",
    )

    # App
    app_name: str = "STA Platform"
    debug: bool = False
    secret_key: str = "dev-secret-key-change-in-production"

    # Database
    database_url: str = "postgresql+asyncpg://sta:stapassword@localhost:5432/sta"

    # Redis
    redis_url: str = "redis://localhost:6379"

    # JWT
    access_token_expire_minutes: int = 15
    refresh_token_expire_days: int = 30

    # Frontend URL (for CORS + OAuth redirects)
    frontend_url: str = "http://localhost:12003"

    # Extra CORS origins (JSON array, e.g. for LAN access: '["http://192.168.1.x:12003"]')
    extra_cors_origins: list[str] = []

    # Local file upload directory (dev mode fallback when R2 is not configured)
    local_upload_dir: str = "/tmp/sta_uploads"

    # OAuth
    google_client_id: str = ""
    google_client_secret: str = ""
    discord_client_id: str = ""
    discord_client_secret: str = ""

    # Meilisearch
    meili_url: str = "http://localhost:7700"
    meili_master_key: str = "meilidev"

    # Cloudflare R2 (leave empty to disable file upload in dev)
    r2_account_id: str = ""
    r2_access_key_id: str = ""
    r2_secret_access_key: str = ""
    r2_bucket: str = "sta-verification"

    # SMTP (empty = log to console)
    smtp_host: str = "smtp.gmail.com"
    smtp_port: int = 587
    smtp_user: str = ""
    smtp_pass: str = ""
    email_from: str = "noreply@sta.tw"


settings = Settings()
