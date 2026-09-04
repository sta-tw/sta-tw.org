-- STA Phase 17: short-lived admin MFA grant.
--
-- A code was required on every /api/v1/admin/* request. This records the last
-- time TOTP verification succeeded so that, within STA_ADMIN_MFA_GRANT_TTL,
-- further admin requests may omit X-MFA-Code. A blank/zero TTL keeps the
-- code-every-time behaviour.

ALTER TABLE account_admin_mfa ADD COLUMN last_verified_at TIMESTAMPTZ;

COMMENT ON COLUMN account_admin_mfa.last_verified_at IS
'最近一次 TOTP 驗證通過時間；STA_ADMIN_MFA_GRANT_TTL 內的 admin 請求可免帶 X-MFA-Code。';
