-- Telegram-first delivery for the cross-check willingness workflow.
--
-- Telegram is only a presentation/delivery adapter. The canonical application,
-- official-result, inquiry and willingness records continue to live in the
-- existing cross-check tables.

CREATE TABLE telegram_account_links (
    telegram_user_id BIGINT PRIMARY KEY CHECK (telegram_user_id > 0),
    account_id UUID NOT NULL UNIQUE REFERENCES accounts(id) ON DELETE RESTRICT,
    private_chat_id BIGINT UNIQUE CHECK (private_chat_id IS NULL OR private_chat_id > 0),
    notifications_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    provisioned_for_testing BOOLEAN NOT NULL DEFAULT FALSE,
    started_at TIMESTAMPTZ,
    last_seen_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT telegram_account_links_started_chat_pair CHECK (
        (started_at IS NULL AND private_chat_id IS NULL)
        OR (started_at IS NOT NULL AND private_chat_id IS NOT NULL)
    )
);

CREATE TRIGGER telegram_account_links_set_updated_at
BEFORE UPDATE ON telegram_account_links
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TABLE telegram_willingness_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    inquiry_id UUID NOT NULL UNIQUE REFERENCES willingness_inquiries(id) ON DELETE RESTRICT,
    application_id UUID NOT NULL REFERENCES applications(id) ON DELETE RESTRICT,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    telegram_user_id BIGINT NOT NULL REFERENCES telegram_account_links(telegram_user_id) ON DELETE RESTRICT,
    chat_id BIGINT CHECK (chat_id IS NULL OR chat_id > 0),
    status VARCHAR(24) NOT NULL DEFAULT 'waiting_binding'
        CHECK (status IN ('waiting_binding', 'pending', 'processing', 'sent', 'failed', 'blocked', 'responded')),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    available_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    leased_at TIMESTAMPTZ,
    telegram_message_id BIGINT,
    sent_at TIMESTAMPTZ,
    responded_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT telegram_willingness_outbox_chat_delivery CHECK (
        status IN ('waiting_binding', 'blocked') OR chat_id IS NOT NULL
    )
);

CREATE TRIGGER telegram_willingness_outbox_set_updated_at
BEFORE UPDATE ON telegram_willingness_outbox
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE INDEX telegram_willingness_outbox_due_idx
ON telegram_willingness_outbox (status, available_at, created_at)
WHERE status IN ('pending', 'failed', 'processing');

CREATE INDEX telegram_willingness_outbox_user_idx
ON telegram_willingness_outbox (telegram_user_id, created_at DESC);

ALTER TABLE willingness_response_events
ADD COLUMN response_source VARCHAR(16) NOT NULL DEFAULT 'web'
    CHECK (response_source IN ('web', 'telegram')),
ADD COLUMN external_event_id TEXT
    CHECK (external_event_id IS NULL OR length(external_event_id) BETWEEN 1 AND 200);

CREATE UNIQUE INDEX willingness_response_events_external_event_unique_idx
ON willingness_response_events (response_source, external_event_id)
WHERE external_event_id IS NOT NULL;

CREATE OR REPLACE FUNCTION enqueue_telegram_willingness_inquiry()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    INSERT INTO telegram_willingness_outbox
        (inquiry_id, application_id, account_id, telegram_user_id, chat_id, status)
    SELECT NEW.id,
           NEW.application_id,
           a.account_id,
           link.telegram_user_id,
           link.private_chat_id,
           CASE
               WHEN link.notifications_enabled AND link.private_chat_id IS NOT NULL THEN 'pending'
               WHEN link.started_at IS NOT NULL THEN 'blocked'
               ELSE 'waiting_binding'
           END
    FROM applications a
    JOIN telegram_account_links link ON link.account_id = a.account_id
    WHERE a.id = NEW.application_id
    ON CONFLICT (inquiry_id) DO NOTHING;
    RETURN NEW;
END;
$$;

CREATE TRIGGER willingness_inquiries_enqueue_telegram
AFTER INSERT ON willingness_inquiries
FOR EACH ROW
EXECUTE FUNCTION enqueue_telegram_willingness_inquiry();

CREATE OR REPLACE FUNCTION sync_telegram_link_outbox()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    INSERT INTO telegram_willingness_outbox
        (inquiry_id, application_id, account_id, telegram_user_id, chat_id, status)
    SELECT i.id,
           i.application_id,
           NEW.account_id,
           NEW.telegram_user_id,
           NEW.private_chat_id,
           CASE
               WHEN NEW.notifications_enabled AND NEW.private_chat_id IS NOT NULL THEN 'pending'
               WHEN NEW.started_at IS NOT NULL THEN 'blocked'
               ELSE 'waiting_binding'
           END
    FROM willingness_inquiries i
    JOIN applications a ON a.id = i.application_id AND a.account_id = NEW.account_id
    WHERE NOT EXISTS (
        SELECT 1
        FROM willingness_response_events event
        WHERE event.inquiry_id = i.id
    )
    ON CONFLICT (inquiry_id) DO NOTHING;

    UPDATE telegram_willingness_outbox outbox
    SET chat_id = NEW.private_chat_id,
        status = CASE
            WHEN NEW.notifications_enabled
                 AND NEW.private_chat_id IS NOT NULL
                 AND outbox.status IN ('waiting_binding', 'blocked', 'failed') THEN 'pending'
            WHEN NOT NEW.notifications_enabled
                 AND NEW.started_at IS NOT NULL
                 AND outbox.status IN ('waiting_binding', 'pending', 'failed') THEN 'blocked'
            ELSE outbox.status
        END,
        available_at = CASE
            WHEN NEW.notifications_enabled
                 AND NEW.private_chat_id IS NOT NULL
                 AND outbox.status IN ('waiting_binding', 'blocked', 'failed') THEN CURRENT_TIMESTAMP
            ELSE outbox.available_at
        END,
        leased_at = CASE
            WHEN outbox.status IN ('waiting_binding', 'blocked', 'failed') THEN NULL
            ELSE outbox.leased_at
        END,
        last_error = CASE
            WHEN NEW.notifications_enabled AND NEW.private_chat_id IS NOT NULL THEN NULL
            ELSE outbox.last_error
        END
    WHERE outbox.telegram_user_id = NEW.telegram_user_id
      AND outbox.status NOT IN ('sent', 'responded', 'processing');
    RETURN NEW;
END;
$$;

CREATE TRIGGER telegram_account_links_sync_outbox
AFTER INSERT OR UPDATE OF private_chat_id, notifications_enabled, started_at
ON telegram_account_links
FOR EACH ROW
EXECUTE FUNCTION sync_telegram_link_outbox();

INSERT INTO telegram_willingness_outbox
    (inquiry_id, application_id, account_id, telegram_user_id, chat_id, status)
SELECT i.id,
       i.application_id,
       a.account_id,
       link.telegram_user_id,
       link.private_chat_id,
       CASE
           WHEN link.notifications_enabled AND link.private_chat_id IS NOT NULL THEN 'pending'
           WHEN link.started_at IS NOT NULL THEN 'blocked'
           ELSE 'waiting_binding'
       END
FROM willingness_inquiries i
JOIN applications a ON a.id = i.application_id
JOIN telegram_account_links link ON link.account_id = a.account_id
WHERE NOT EXISTS (
    SELECT 1 FROM willingness_response_events event WHERE event.inquiry_id = i.id
)
ON CONFLICT (inquiry_id) DO NOTHING;

COMMENT ON TABLE telegram_account_links IS
    'Telegram numeric user ID to canonical STA account mapping; private chat is activated only after /start.';
COMMENT ON TABLE telegram_willingness_outbox IS
    'Durable Telegram delivery state for willingness inquiries; waiting_binding is distinct from responded.';
COMMENT ON COLUMN willingness_response_events.response_source IS
    'Originating frontend. Telegram stores labels externally and converts them to canonical numeric values internally.';
COMMENT ON COLUMN willingness_response_events.external_event_id IS
    'Frontend delivery/callback idempotency key; never contains the willingness value.';
