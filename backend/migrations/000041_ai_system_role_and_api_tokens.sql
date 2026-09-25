-- STA: a distinct "ai_system" role + personal API tokens, so an external
-- brochure-submitting service authenticates as an attributable account
-- instead of a shared static secret, and its uploads go through the exact
-- same infer-then-confirm pipeline as an administrator's own upload — the
-- caller's claimed academic_year/school_code/source_url is never trusted.

ALTER TABLE account_roles DROP CONSTRAINT account_roles_role_check;
ALTER TABLE account_roles ADD CONSTRAINT account_roles_role_check
    CHECK (role IN ('admin', 'ai_system'));

CREATE TABLE account_api_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    label TEXT NOT NULL DEFAULT 'AI System',
    token_hash BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    CONSTRAINT account_api_tokens_label_not_blank CHECK (length(btrim(label)) > 0),
    CONSTRAINT account_api_tokens_hash_not_empty CHECK (octet_length(token_hash) > 0)
);

-- A token's hash must be globally unique only while active; a revoked token's
-- hash may collide with a fresh reissue.
CREATE UNIQUE INDEX account_api_tokens_hash_active_idx
ON account_api_tokens (token_hash) WHERE revoked_at IS NULL;

CREATE INDEX account_api_tokens_account_idx
ON account_api_tokens (account_id) WHERE revoked_at IS NULL;

COMMENT ON TABLE account_api_tokens IS '每個帳號可持有的個人 API token（例如 ai_system 角色的上傳帳號），與登入 session 分開，可個別撤銷並歸屬到單一帳號。';

ALTER TABLE brochure_uploads DROP CONSTRAINT brochure_uploads_intake_channel_check;
ALTER TABLE brochure_uploads ADD CONSTRAINT brochure_uploads_intake_channel_check
    CHECK (intake_channel IN ('admin_upload', 'external_api', 'ai_system'));

COMMENT ON COLUMN brochure_uploads.intake_channel IS
'admin_upload 是管理員自己上傳並立即確認的草稿；ai_system 是持有 ai_system 角色 API token 的帳號送出、進入待審核佇列的來源；external_api 是舊版共用密鑰＋呼叫方自報身分的送出方式，保留但不再是建議路徑。';
