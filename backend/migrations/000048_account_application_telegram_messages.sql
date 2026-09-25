-- STA: every message id that belongs to an application's Telegram thread —
-- the root notification, follow-up notifications, the bot's own reply
-- confirmations, and a staff member's own typed reply — so a staff reply to
-- ANY of them (not just the root card) can be resolved back to the
-- application. Without this, replying to anything but the very first
-- message silently failed (FindByTelegramMessage only matched the one
-- root id stored on account_applications).

CREATE TABLE account_application_telegram_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID NOT NULL REFERENCES account_applications(id) ON DELETE CASCADE,
    chat_id BIGINT NOT NULL,
    message_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (chat_id, message_id)
);

CREATE INDEX account_application_telegram_messages_application_idx
    ON account_application_telegram_messages (application_id, created_at DESC);
