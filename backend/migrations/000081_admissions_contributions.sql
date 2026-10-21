CREATE TABLE admissions_contributions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email_ciphertext BYTEA NOT NULL,
    academic_year SMALLINT NOT NULL CHECK (academic_year BETWEEN 100 AND 999),
    category VARCHAR(32) NOT NULL CHECK (category IN (
        'applicants', 'second_stage', 'waitlist', 'historical_brochure'
    )),
    school_name TEXT NOT NULL DEFAULT '',
    data_text TEXT NOT NULL DEFAULT '',
    reference_url TEXT NOT NULL DEFAULT '',
    nickname TEXT NOT NULL DEFAULT '',
    publish_nickname BOOLEAN NOT NULL DEFAULT FALSE,
    attachment_storage_key TEXT,
    attachment_filename TEXT,
    attachment_mime_type TEXT,
    attachment_size_bytes BIGINT CHECK (attachment_size_bytes > 0),
    attachment_sha256 TEXT,
    status VARCHAR(16) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'accepted', 'rejected')),
    reviewed_by UUID REFERENCES accounts(id) ON DELETE SET NULL,
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT admissions_contributions_attachment_complete CHECK (
        (attachment_storage_key IS NULL AND attachment_filename IS NULL
            AND attachment_mime_type IS NULL AND attachment_size_bytes IS NULL
            AND attachment_sha256 IS NULL)
        OR
        (attachment_storage_key IS NOT NULL AND attachment_filename IS NOT NULL
            AND attachment_mime_type = 'application/pdf'
            AND attachment_size_bytes IS NOT NULL AND attachment_sha256 IS NOT NULL)
    ),
    CONSTRAINT admissions_contributions_review_consistent CHECK (
        (status = 'pending') = (reviewed_at IS NULL)
    ),
    CONSTRAINT admissions_contributions_nickname_consent CHECK (
        publish_nickname = FALSE OR length(btrim(nickname)) > 0
    ),
    CONSTRAINT admissions_contributions_category_data CHECK (
        (category = 'historical_brochure' AND academic_year <= 113
            AND (attachment_storage_key IS NOT NULL OR reference_url <> ''))
        OR
        (category IN ('applicants', 'second_stage', 'waitlist')
            AND length(btrim(school_name)) > 0 AND length(btrim(data_text)) > 0)
    )
);

CREATE INDEX admissions_contributions_review_idx
    ON admissions_contributions (status, created_at DESC);

CREATE INDEX admissions_contributions_public_thanks_idx
    ON admissions_contributions (reviewed_at DESC, created_at DESC)
    WHERE status = 'accepted' AND publish_nickname = TRUE;

COMMENT ON TABLE admissions_contributions IS
    'Public admissions data and historical brochure submissions, reviewed by admissions moderators.';
COMMENT ON COLUMN admissions_contributions.email_ciphertext IS
    'Contact email encrypted with the application field-encryption key.';
