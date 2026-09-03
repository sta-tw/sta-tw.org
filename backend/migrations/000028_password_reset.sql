-- STA Phase 14: native-account password reset.
--
-- One-time reset tokens. The plaintext token only ever exists in the email
-- body; the row stores its SHA-256. Consuming a token updates the password and
-- revokes every existing session for the account in the same transaction.

CREATE TABLE password_reset_challenges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    token_hash BYTEA NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT password_reset_token_hash_not_empty CHECK (octet_length(token_hash) > 0),
    UNIQUE (token_hash)
);

CREATE INDEX password_reset_challenges_account_idx
ON password_reset_challenges (account_id, created_at DESC);

COMMENT ON TABLE password_reset_challenges IS
'密碼重設一次性 token；明文只在郵件內。消費 token 會同一交易更新密碼並撤銷全部 session。';
