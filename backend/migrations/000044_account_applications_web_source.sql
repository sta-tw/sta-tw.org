-- STA: the public application form (POST /api/v1/account-applications) is
-- now the primary way to apply without a school email; email intake is a
-- fallback plus a reply channel. Allow source = 'web' alongside 'email'.

ALTER TABLE account_applications DROP CONSTRAINT account_applications_source_check;
ALTER TABLE account_applications ADD CONSTRAINT account_applications_source_check
    CHECK (source IN ('email', 'web'));
