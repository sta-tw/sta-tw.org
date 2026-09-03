-- Signed provider webhook for staff Email replies. Email is an input adapter
-- only; support_tickets/support_messages remain the canonical record.

ALTER TABLE support_messages
    DROP CONSTRAINT support_messages_source_platform_check;

ALTER TABLE support_messages
    ADD CONSTRAINT support_messages_source_platform_check
    CHECK (source_platform IN ('website', 'discord', 'email', 'system'));

CREATE TABLE support_email_inbound_dedup (
    external_message_id TEXT PRIMARY KEY,
    message_id UUID NOT NULL REFERENCES support_messages(id) ON DELETE RESTRICT,
    received_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT support_email_inbound_id_not_blank CHECK (length(btrim(external_message_id)) > 0)
);

COMMENT ON TABLE support_email_inbound_dedup IS '已驗證 Email provider webhook 的去重表；不保存原始郵件內容。';
