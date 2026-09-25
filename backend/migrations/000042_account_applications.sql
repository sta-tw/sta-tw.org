-- STA: manual account-application review for people without a school email
-- (self-taught applicants, or a domain not on the edu.tw allowlist). They
-- apply by emailing account@mail.sta-tw.org with proof attachments; the
-- inbound-mail intake creates a pending row here, an admin reviews it from a
-- Telegram notification, and approval creates the account and emails the
-- applicant a password-set link. Rejected/expired rows and their documents
-- are not accounts and carry no long-term identity data, so no annual
-- cleanup hook is needed here (unlike the school-email verification tables).

CREATE TABLE account_applications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    requested_username TEXT NOT NULL,
    email_ciphertext BYTEA NOT NULL,
    email_lookup_hash BYTEA NOT NULL,
    source TEXT NOT NULL CHECK (source IN ('email')),
    note TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    telegram_chat_id BIGINT,
    telegram_message_id BIGINT,
    reviewed_by UUID REFERENCES accounts(id) ON DELETE SET NULL,
    reviewed_at TIMESTAMPTZ,
    created_account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT account_applications_username_not_blank CHECK (length(btrim(requested_username)) > 0)
);

CREATE INDEX account_applications_status_created_idx
    ON account_applications (status, created_at);
CREATE INDEX account_applications_email_lookup_idx
    ON account_applications (email_lookup_hash);

CREATE TABLE account_application_documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID NOT NULL REFERENCES account_applications(id) ON DELETE CASCADE,
    object_storage_key TEXT NOT NULL,
    filename TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
    sha256_hex TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX account_application_documents_application_idx
    ON account_application_documents (application_id);
