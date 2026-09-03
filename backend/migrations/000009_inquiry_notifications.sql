-- STA Phase 8: durable notification dispatch for the two willingness inquiry rounds.

ALTER TABLE willingness_inquiries
ADD COLUMN notification_status VARCHAR(16) NOT NULL DEFAULT 'pending'
    CHECK (notification_status IN ('pending', 'processing', 'enqueued', 'failed'));

ALTER TABLE willingness_inquiries
ADD COLUMN notification_attempt_count INTEGER NOT NULL DEFAULT 0
    CHECK (notification_attempt_count >= 0);

ALTER TABLE willingness_inquiries
ADD COLUMN notification_error TEXT;

ALTER TABLE willingness_inquiries
ADD COLUMN notification_available_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;

ALTER TABLE willingness_inquiries
ADD COLUMN notification_enqueued_at TIMESTAMPTZ;

ALTER TABLE willingness_inquiries
ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;

CREATE TRIGGER willingness_inquiries_set_updated_at
BEFORE UPDATE ON willingness_inquiries
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE INDEX willingness_inquiries_notification_due_idx
ON willingness_inquiries (notification_status, notification_available_at, created_at);

COMMENT ON COLUMN willingness_inquiries.notification_status IS '通知 outbox 的獨立狀態；不等同於使用者是否已回覆意願。';
