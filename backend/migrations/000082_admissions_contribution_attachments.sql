ALTER TABLE admissions_contributions
    DROP CONSTRAINT admissions_contributions_attachment_complete,
    DROP CONSTRAINT admissions_contributions_category_data;

ALTER TABLE admissions_contributions
    ADD CONSTRAINT admissions_contributions_attachment_complete CHECK (
        (attachment_storage_key IS NULL AND attachment_filename IS NULL
            AND attachment_mime_type IS NULL AND attachment_size_bytes IS NULL
            AND attachment_sha256 IS NULL)
        OR
        (attachment_storage_key IS NOT NULL AND attachment_filename IS NOT NULL
            AND attachment_mime_type IN (
                'application/pdf', 'text/csv', 'text/tab-separated-values',
                'text/plain', 'application/json', 'application/msword',
                'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
                'application/vnd.ms-powerpoint',
                'application/vnd.openxmlformats-officedocument.presentationml.presentation',
                'application/vnd.ms-excel',
                'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
                'image/jpeg', 'image/png', 'image/gif', 'image/webp'
            )
            AND attachment_size_bytes IS NOT NULL AND attachment_sha256 IS NOT NULL)
    ),
    ADD CONSTRAINT admissions_contributions_category_data CHECK (
        (category = 'historical_brochure'
            AND (attachment_storage_key IS NOT NULL OR reference_url <> ''))
        OR
        (category IN ('applicants', 'second_stage', 'waitlist')
            AND length(btrim(school_name)) > 0
            AND (length(btrim(data_text)) > 0 OR attachment_storage_key IS NOT NULL))
    );

CREATE TABLE admissions_contribution_attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    contribution_id UUID NOT NULL REFERENCES admissions_contributions(id) ON DELETE CASCADE,
    storage_key TEXT NOT NULL UNIQUE,
    file_name TEXT NOT NULL,
    mime_type TEXT NOT NULL CHECK (mime_type IN (
        'application/pdf', 'text/csv', 'text/tab-separated-values',
        'text/plain', 'application/json', 'application/msword',
        'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
        'application/vnd.ms-powerpoint',
        'application/vnd.openxmlformats-officedocument.presentationml.presentation',
        'application/vnd.ms-excel',
        'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
        'image/jpeg', 'image/png', 'image/gif', 'image/webp'
    )),
    size_bytes BIGINT NOT NULL CHECK (size_bytes > 0),
    sha256_hex TEXT NOT NULL CHECK (sha256_hex ~ '^[0-9a-f]{64}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX admissions_contribution_attachments_order_idx
    ON admissions_contribution_attachments (contribution_id, created_at, id);

INSERT INTO admissions_contribution_attachments (
    contribution_id, storage_key, file_name, mime_type, size_bytes, sha256_hex
)
SELECT id, attachment_storage_key, attachment_filename, attachment_mime_type,
       attachment_size_bytes, attachment_sha256
FROM admissions_contributions
WHERE attachment_storage_key IS NOT NULL
ON CONFLICT (storage_key) DO NOTHING;

COMMENT ON TABLE admissions_contribution_attachments IS
    'Private, malware-scanned files submitted with admissions data contributions.';
