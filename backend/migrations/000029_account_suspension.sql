-- STA Phase 15: operator account management.
--
-- Admins can suspend an account and later reinstate it. Suspension relies on
-- the existing account_status = 'active' guard in the session and login paths:
-- flipping the status to 'suspended' and revoking live sessions locks the
-- account out immediately. These columns record why and by whom for the
-- operator console; audit_log keeps the full history.

ALTER TABLE accounts
    ADD COLUMN suspended_at TIMESTAMPTZ,
    ADD COLUMN suspension_reason TEXT,
    ADD COLUMN suspended_by UUID REFERENCES accounts(id) ON DELETE SET NULL;

-- The operator user list pages by status then recency.
CREATE INDEX accounts_status_created_idx
ON accounts (account_status, created_at DESC, id DESC);

-- Case-insensitive username prefix search for the same list.
CREATE INDEX accounts_username_lower_idx
ON accounts (lower(username) varchar_pattern_ops);

COMMENT ON COLUMN accounts.suspended_at IS '帳號停權時間；reinstate 後清空。';
COMMENT ON COLUMN accounts.suspension_reason IS '停權原因（操作者填寫）。';
COMMENT ON COLUMN accounts.suspended_by IS '執行停權的管理者帳號。';
