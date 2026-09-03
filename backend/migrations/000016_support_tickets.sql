-- STA Phase 11: authenticated support tickets, canonical support messages,
-- durable email notifications, and per-ticket Discord synchronization.

CREATE TABLE support_tickets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_number BIGINT GENERATED ALWAYS AS IDENTITY UNIQUE,
    account_id UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    requester_email_ciphertext BYTEA,
    category VARCHAR(32) NOT NULL
        CHECK (category IN ('account', 'admissions', 'brochure', 'results', 'candidate_number', 'willingness', 'technical', 'other')),
    subject TEXT NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'waiting_staff'
        CHECK (status IN ('open', 'waiting_staff', 'waiting_user', 'closed', 'spam')),
    assigned_to UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    discord_channel_id TEXT,
    discord_sync_status VARCHAR(16) NOT NULL DEFAULT 'pending'
        CHECK (discord_sync_status IN ('disabled', 'pending', 'processing', 'synced', 'failed', 'archived')),
    latest_message_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    closed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT support_tickets_requester_present CHECK (
        account_id IS NOT NULL OR requester_email_ciphertext IS NOT NULL
    ),
    CONSTRAINT support_tickets_requester_email_not_empty CHECK (
        requester_email_ciphertext IS NULL OR octet_length(requester_email_ciphertext) > 0
    ),
    CONSTRAINT support_tickets_subject_not_blank CHECK (length(btrim(subject)) > 0),
    CONSTRAINT support_tickets_discord_channel_not_blank CHECK (
        discord_channel_id IS NULL OR length(btrim(discord_channel_id)) > 0
    ),
    CONSTRAINT support_tickets_closed_at_consistent CHECK (
        (status = 'closed') = (closed_at IS NOT NULL)
    )
);

CREATE INDEX support_tickets_account_idx
ON support_tickets (account_id, latest_message_at DESC)
WHERE account_id IS NOT NULL;

CREATE INDEX support_tickets_admin_idx
ON support_tickets (status, latest_message_at DESC);

CREATE INDEX support_tickets_discord_channel_idx
ON support_tickets (discord_channel_id)
WHERE discord_channel_id IS NOT NULL;

CREATE TRIGGER support_tickets_set_updated_at
BEFORE UPDATE ON support_tickets
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TABLE support_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id UUID NOT NULL REFERENCES support_tickets(id) ON DELETE RESTRICT,
    author_account_id UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    author_type VARCHAR(16) NOT NULL
        CHECK (author_type IN ('user', 'admin', 'system')),
    source_platform VARCHAR(16) NOT NULL
        CHECK (source_platform IN ('website', 'discord', 'system')),
    external_author_hash BYTEA,
    body TEXT NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'edited', 'deleted')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    edited_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    CONSTRAINT support_messages_body_not_blank CHECK (length(btrim(body)) > 0 OR status = 'deleted'),
    CONSTRAINT support_messages_external_author_not_empty CHECK (
        external_author_hash IS NULL OR octet_length(external_author_hash) > 0
    )
);

CREATE INDEX support_messages_ticket_idx
ON support_messages (ticket_id, created_at);

CREATE TABLE support_message_bridges (
    message_id UUID NOT NULL REFERENCES support_messages(id) ON DELETE RESTRICT,
    platform VARCHAR(16) NOT NULL CHECK (platform = 'discord'),
    external_message_id TEXT NOT NULL,
    sync_status VARCHAR(16) NOT NULL DEFAULT 'sent'
        CHECK (sync_status IN ('pending', 'sent', 'edited', 'deleted', 'failed')),
    last_error TEXT,
    synced_at TIMESTAMPTZ,
    PRIMARY KEY (message_id, platform),
    UNIQUE (platform, external_message_id)
);

CREATE TABLE support_discord_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id UUID NOT NULL REFERENCES support_tickets(id) ON DELETE RESTRICT,
    message_id UUID REFERENCES support_messages(id) ON DELETE RESTRICT,
    operation VARCHAR(24) NOT NULL
        CHECK (operation IN ('create_channel', 'create_message', 'edit_message', 'delete_message', 'archive_channel', 'reopen_channel')),
    external_message_id TEXT,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(16) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'sent', 'failed')),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    last_error TEXT,
    available_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT support_discord_outbox_message_requirement CHECK (
        (operation IN ('create_message', 'edit_message', 'delete_message')) = (message_id IS NOT NULL)
    )
);

CREATE INDEX support_discord_outbox_due_idx
ON support_discord_outbox (status, available_at, created_at);

CREATE TRIGGER support_discord_outbox_set_updated_at
BEFORE UPDATE ON support_discord_outbox
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TABLE support_discord_inbound_dedup (
    external_message_id TEXT PRIMARY KEY,
    message_id UUID NOT NULL REFERENCES support_messages(id) ON DELETE RESTRICT,
    received_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE support_ticket_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ticket_id UUID NOT NULL REFERENCES support_tickets(id) ON DELETE RESTRICT,
    actor_account_id UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    event_type VARCHAR(32) NOT NULL,
    data JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT support_ticket_events_type_not_blank CHECK (length(btrim(event_type)) > 0)
);

CREATE INDEX support_ticket_events_ticket_idx
ON support_ticket_events (ticket_id, created_at DESC);

CREATE OR REPLACE FUNCTION prevent_support_ticket_event_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'support_ticket_events is append-only';
END;
$$;

CREATE TRIGGER support_ticket_events_no_mutation
BEFORE UPDATE OR DELETE ON support_ticket_events
FOR EACH ROW
EXECUTE FUNCTION prevent_support_ticket_event_mutation();

-- PostgreSQL UNIQUE constraints treat NULL account IDs as distinct. Support
-- notifications addressed to a configured mailbox use NULL account_id, so
-- add a partial unique index to preserve idempotency for those messages too.
CREATE UNIQUE INDEX email_outbox_anonymous_dedup_idx
ON email_outbox (dedup_key)
WHERE account_id IS NULL;

COMMENT ON TABLE support_tickets IS '通用客服 Ticket；與追加申請用的 application_service_tickets 分開。';
COMMENT ON TABLE support_messages IS '客服對話的 canonical message；網站與 Discord 都回寫這裡。';
COMMENT ON TABLE support_discord_outbox IS '每個 Ticket 的 Discord 頻道／訊息 durable outbox。';
COMMENT ON TABLE support_ticket_events IS '客服狀態、指派與外部同步事件；append-only。';
