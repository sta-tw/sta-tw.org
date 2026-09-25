-- Existing brochure_uploads rows were created by the administrator upload
-- flow before intake_channel existed. Keep those rows out of the external API
-- review queue, while preserving explicitly-created external API rows.

ALTER TABLE brochure_uploads
ALTER COLUMN intake_channel SET DEFAULT 'admin_upload';

UPDATE brochure_uploads
SET intake_channel = 'admin_upload'
WHERE intake_channel = 'external_api'
  AND created_by IS NOT NULL;
