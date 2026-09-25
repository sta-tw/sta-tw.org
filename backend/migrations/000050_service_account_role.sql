-- STA: a generic 'service' role for bot/automation accounts — distinct from
-- 'ai_system' (which is specifically for the AI extraction pipeline and
-- always routes uploads through infer-then-confirm; a 'service' account is
-- for direct, admin-trusted machine access, e.g. a scripted brochure
-- upload with a known-correct academic_year/school_code, no AI involved).
-- Reuses the existing account_api_tokens table (migration 000041) — same
-- per-account opaque-bearer-token mechanism ai_system already uses,
-- entirely separate from account_sessions/password/MFA.

ALTER TABLE account_roles DROP CONSTRAINT account_roles_role_check;
ALTER TABLE account_roles ADD CONSTRAINT account_roles_role_check
    CHECK (role IN ('admin', 'ai_system', 'service'));
