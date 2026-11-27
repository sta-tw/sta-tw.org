-- Preserve why Discord registration was accepted without claiming either email
-- address has been verified.
CREATE TABLE oauth_registration_verifications (
    account_id UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    provider VARCHAR(16) NOT NULL CHECK (provider = 'discord'),
    guild_id TEXT NOT NULL,
    role_id TEXT NOT NULL,
    verified_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE oauth_registration_consumptions (
    nonce_hash BYTEA PRIMARY KEY,
    expires_at TIMESTAMPTZ NOT NULL
);
