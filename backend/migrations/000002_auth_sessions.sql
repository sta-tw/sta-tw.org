-- STA Phase 3: native sessions, email verification challenges, and OAuth flow state.
-- Raw session tokens, CSRF tokens, OAuth state values, and PKCE verifiers are
-- never stored. The application stores only hashes or encrypted ciphertext.

CREATE TABLE account_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    token_hash BYTEA NOT NULL,
    csrf_token_hash BYTEA NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    last_seen_at TIMESTAMPTZ,
    ip_hash BYTEA,
    user_agent_hash BYTEA,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT account_sessions_token_hash_not_empty CHECK (octet_length(token_hash) > 0),
    CONSTRAINT account_sessions_csrf_hash_not_empty CHECK (octet_length(csrf_token_hash) > 0),
    UNIQUE (token_hash)
);

CREATE INDEX account_sessions_account_idx
ON account_sessions (account_id, expires_at DESC)
WHERE revoked_at IS NULL;

CREATE INDEX account_sessions_expiry_idx
ON account_sessions (expires_at)
WHERE revoked_at IS NULL;

CREATE TABLE email_verification_challenges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    token_hash BYTEA NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT email_challenge_token_hash_not_empty CHECK (octet_length(token_hash) > 0),
    UNIQUE (token_hash)
);

CREATE INDEX email_verification_challenges_account_idx
ON email_verification_challenges (account_id, created_at DESC);

CREATE TABLE oauth_states (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider VARCHAR(16) NOT NULL
        CHECK (provider IN ('google', 'discord')),
    account_id UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    state_hash BYTEA NOT NULL,
    code_verifier_ciphertext BYTEA NOT NULL,
    redirect_url TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT oauth_state_hash_not_empty CHECK (octet_length(state_hash) > 0),
    CONSTRAINT oauth_code_verifier_not_empty CHECK (octet_length(code_verifier_ciphertext) > 0),
    CONSTRAINT oauth_redirect_url_not_blank CHECK (length(btrim(redirect_url)) > 0),
    UNIQUE (state_hash)
);

CREATE INDEX oauth_states_expiry_idx
ON oauth_states (expires_at)
WHERE consumed_at IS NULL;

CREATE INDEX oauth_states_account_idx
ON oauth_states (account_id, created_at DESC)
WHERE account_id IS NOT NULL AND consumed_at IS NULL;

COMMENT ON TABLE account_sessions IS 'Opaque cookie session；token 與 CSRF 值只保存 hash。';
COMMENT ON TABLE email_verification_challenges IS 'Email 驗證一次性 challenge；明文 token 只存在郵件內容。';
COMMENT ON TABLE oauth_states IS 'OAuth state + PKCE verifier 的一次性伺服器端狀態。';
