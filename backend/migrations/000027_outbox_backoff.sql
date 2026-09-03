-- STA Phase 13: bound outbox retries.
--
-- The chat / email / support-Discord outboxes and the willingness-inquiry
-- notification queue previously retried a permanently failing row forever on a
-- fixed 30s delay. Give each a max_attempts cap, a terminal 'abandoned' state,
-- and let the worker apply graduated backoff (30s * attempt, capped).

-- chat_sync_outbox -----------------------------------------------------------
ALTER TABLE chat_sync_outbox
    ADD COLUMN max_attempts INTEGER NOT NULL DEFAULT 8
        CHECK (max_attempts BETWEEN 1 AND 100);
ALTER TABLE chat_sync_outbox DROP CONSTRAINT IF EXISTS chat_sync_outbox_status_check;
ALTER TABLE chat_sync_outbox ADD CONSTRAINT chat_sync_outbox_status_check
    CHECK (status IN ('pending', 'processing', 'sent', 'failed', 'abandoned'));

-- email_outbox -------------------------------------------------------------
ALTER TABLE email_outbox
    ADD COLUMN max_attempts INTEGER NOT NULL DEFAULT 8
        CHECK (max_attempts BETWEEN 1 AND 100);
ALTER TABLE email_outbox DROP CONSTRAINT IF EXISTS email_outbox_status_check;
ALTER TABLE email_outbox ADD CONSTRAINT email_outbox_status_check
    CHECK (status IN ('pending', 'processing', 'sent', 'failed', 'abandoned'));

-- support_discord_outbox ------------------------------------------------------
ALTER TABLE support_discord_outbox
    ADD COLUMN max_attempts INTEGER NOT NULL DEFAULT 8
        CHECK (max_attempts BETWEEN 1 AND 100);
ALTER TABLE support_discord_outbox DROP CONSTRAINT IF EXISTS support_discord_outbox_status_check;
ALTER TABLE support_discord_outbox ADD CONSTRAINT support_discord_outbox_status_check
    CHECK (status IN ('pending', 'processing', 'sent', 'failed', 'abandoned'));

-- willingness_inquiries notification queue --------------------------------
ALTER TABLE willingness_inquiries
    ADD COLUMN notification_max_attempts INTEGER NOT NULL DEFAULT 8
        CHECK (notification_max_attempts BETWEEN 1 AND 100);
ALTER TABLE willingness_inquiries DROP CONSTRAINT IF EXISTS willingness_inquiries_notification_status_check;
ALTER TABLE willingness_inquiries ADD CONSTRAINT willingness_inquiries_notification_status_check
    CHECK (notification_status IN ('pending', 'processing', 'enqueued', 'failed', 'abandoned'));

COMMENT ON COLUMN chat_sync_outbox.max_attempts IS '達到此嘗試次數後轉為 abandoned，不再重試。';
COMMENT ON COLUMN email_outbox.max_attempts IS '達到此嘗試次數後轉為 abandoned，不再重試。';
COMMENT ON COLUMN support_discord_outbox.max_attempts IS '達到此嘗試次數後轉為 abandoned，不再重試。';
COMMENT ON COLUMN willingness_inquiries.notification_max_attempts IS '達到此嘗試次數後通知轉為 abandoned，不再重試。';
