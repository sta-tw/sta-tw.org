-- STA: account applications (/apply form -> creates an STA account) and
-- email inquiries (someone emails account@ with a question) are different
-- things that shared one table/service by accident of history. Split them:
-- move every source='email' row (and its message thread) into new
-- email_inquiries/email_inquiry_messages tables, preserving id so an
-- already-sent reply's In-Reply-To/References still resolves.
-- account_applications becomes application-only; source is dropped.

CREATE TABLE email_inquiries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email_ciphertext BYTEA NOT NULL,
    email_lookup_hash BYTEA NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    telegram_chat_id BIGINT,
    telegram_message_id BIGINT,
    telegram_header_text TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX email_inquiries_email_lookup_idx ON email_inquiries (email_lookup_hash);

CREATE TABLE email_inquiry_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    inquiry_id UUID NOT NULL REFERENCES email_inquiries(id) ON DELETE CASCADE,
    direction TEXT NOT NULL CHECK (direction IN ('inbound', 'outbound')),
    body TEXT NOT NULL DEFAULT '',
    source_message_id TEXT NOT NULL DEFAULT '',
    actor TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX email_inquiry_messages_inquiry_idx ON email_inquiry_messages (inquiry_id, created_at);

CREATE TABLE email_inquiry_documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    inquiry_id UUID NOT NULL REFERENCES email_inquiries(id) ON DELETE CASCADE,
    object_storage_key TEXT NOT NULL,
    filename TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
    sha256_hex TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX email_inquiry_documents_inquiry_idx ON email_inquiry_documents (inquiry_id);

INSERT INTO email_inquiries (id, email_ciphertext, email_lookup_hash, note, telegram_chat_id, telegram_message_id, telegram_header_text, created_at)
SELECT id, email_ciphertext, email_lookup_hash, note, telegram_chat_id, telegram_message_id, telegram_header_text, created_at
FROM account_applications WHERE source = 'email';

INSERT INTO email_inquiry_messages (inquiry_id, direction, body, source_message_id, actor, created_at)
SELECT application_id, direction, body, source_message_id, actor, created_at
FROM account_application_messages
WHERE application_id IN (SELECT id FROM account_applications WHERE source = 'email');

DELETE FROM account_application_messages
WHERE application_id IN (SELECT id FROM account_applications WHERE source = 'email');

DELETE FROM account_application_documents
WHERE application_id IN (SELECT id FROM account_applications WHERE source = 'email');

DELETE FROM account_applications WHERE source = 'email';

ALTER TABLE account_applications DROP CONSTRAINT account_applications_source_check;
ALTER TABLE account_applications DROP COLUMN source;
