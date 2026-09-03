-- STA Phase 12: encrypted administrator TOTP seeds.
-- The application encrypts the base32 seed before it reaches this table.

CREATE TABLE account_admin_mfa (
    account_id UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE RESTRICT,
    secret_ciphertext BYTEA NOT NULL,
    enabled_at TIMESTAMPTZ,
    pending_expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT account_admin_mfa_secret_not_empty CHECK (octet_length(secret_ciphertext) > 0)
);

CREATE TRIGGER account_admin_mfa_set_updated_at
BEFORE UPDATE ON account_admin_mfa
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

COMMENT ON TABLE account_admin_mfa IS '管理員 TOTP seed；只保存應用層加密後的值。';
