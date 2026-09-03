-- Support message attachments live in private object storage. The database
-- stores only metadata and the association with the canonical message.

CREATE TABLE support_attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id UUID NOT NULL REFERENCES support_tickets(id) ON DELETE RESTRICT,
    message_id UUID NOT NULL REFERENCES support_messages(id) ON DELETE RESTRICT,
    storage_key TEXT NOT NULL UNIQUE,
    original_file_name TEXT NOT NULL,
    mime_type VARCHAR(128) NOT NULL,
    file_size_bytes BIGINT NOT NULL CHECK (file_size_bytes > 0 AND file_size_bytes <= 10485760),
    sha256_hex CHAR(64) NOT NULL CHECK (sha256_hex ~ '^[0-9a-f]{64}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT support_attachments_file_name_not_blank CHECK (length(btrim(original_file_name)) > 0),
    CONSTRAINT support_attachments_storage_key_not_blank CHECK (length(btrim(storage_key)) > 0),
    CONSTRAINT support_attachments_mime_not_blank CHECK (length(btrim(mime_type)) > 0)
);

CREATE INDEX support_attachments_message_idx
ON support_attachments (message_id, created_at, id);

CREATE INDEX support_attachments_ticket_idx
ON support_attachments (ticket_id, created_at, id);

COMMENT ON TABLE support_attachments IS '客服附件 metadata；檔案本體只放 private object storage。';
