-- STA Phase 8: encrypted in-app notifications and durable email outbox.
-- Subject/body and recipient email ciphertext are decrypted only by the email
-- worker immediately before delivery; the API never sends SMTP synchronously.

CREATE TABLE notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    kind VARCHAR(64) NOT NULL,
    dedup_key VARCHAR(200) NOT NULL,
    title_ciphertext BYTEA NOT NULL,
    body_ciphertext BYTEA NOT NULL,
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT notifications_kind_not_blank CHECK (length(btrim(kind)) > 0),
    CONSTRAINT notifications_dedup_key_not_blank CHECK (length(btrim(dedup_key)) > 0),
    CONSTRAINT notifications_title_not_empty CHECK (octet_length(title_ciphertext) > 0),
    CONSTRAINT notifications_body_not_empty CHECK (octet_length(body_ciphertext) > 0),
    UNIQUE (account_id, dedup_key)
);

CREATE INDEX notifications_account_idx
ON notifications (account_id, created_at DESC);

CREATE TABLE email_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    notification_id UUID REFERENCES notifications(id) ON DELETE RESTRICT,
    dedup_key VARCHAR(200) NOT NULL,
    recipient_ciphertext BYTEA NOT NULL,
    payload_ciphertext BYTEA NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'sent', 'failed')),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    last_error TEXT,
    available_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT email_outbox_recipient_not_empty CHECK (octet_length(recipient_ciphertext) > 0),
    CONSTRAINT email_outbox_payload_not_empty CHECK (octet_length(payload_ciphertext) > 0),
    CONSTRAINT email_outbox_dedup_key_not_blank CHECK (length(btrim(dedup_key)) > 0),
    UNIQUE (account_id, dedup_key)
);

CREATE INDEX email_outbox_due_idx
ON email_outbox (status, available_at, created_at);

CREATE TRIGGER email_outbox_set_updated_at
BEFORE UPDATE ON email_outbox
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

COMMENT ON TABLE notifications IS '站內通知；標題與內容以欄位加密保存，前端只取得目前帳號的通知。';
COMMENT ON TABLE email_outbox IS '寄信 durable outbox；API 只入列，SMTP worker 負責重試與投遞。';
