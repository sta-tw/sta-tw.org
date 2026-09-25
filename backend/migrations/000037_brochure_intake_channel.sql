-- Separate the administrator's immediate PDF confirmation flow from the
-- external brochure API queue. Only external API submissions belong in the
-- shared manual-review list.

ALTER TABLE brochure_uploads
ADD COLUMN intake_channel VARCHAR(16) NOT NULL DEFAULT 'external_api'
    CHECK (intake_channel IN ('admin_upload', 'external_api'));

CREATE INDEX brochure_uploads_channel_review_idx
ON brochure_uploads (intake_channel, status, created_at DESC);

COMMENT ON COLUMN brochure_uploads.intake_channel IS
'admin_upload is an administrator''s immediate PDF confirmation draft; external_api is a service submission for the manual-review queue.';
