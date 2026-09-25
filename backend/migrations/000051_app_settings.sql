-- STA: a small key/value table for admin-toggleable runtime settings —
-- starting with whether admin MFA is enforced. STA_REQUIRE_ADMIN_MFA (the
-- env var) stays as the *default* when no row exists yet; once set here,
-- this overrides it without needing a redeploy.

CREATE TABLE app_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by UUID REFERENCES accounts(id) ON DELETE SET NULL
);
