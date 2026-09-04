-- STA Phase 16: chat channels, reactions, pins and threads.
--
-- The chat schema always had a chat_channels table and a channel_id on every
-- message, but the API only ever exposed one hard-coded 'lounge' channel. This
-- migration promotes the model to first class:
--   * channels gain a kind, a default flag and an archive timestamp;
--   * messages can be pinned and can reply to another message (one level);
--   * a reactions table records one emoji per account per message.
-- The Discord/Telegram bridge stays bound to the default channel only.

ALTER TABLE chat_channels
    ADD COLUMN kind VARCHAR(16) NOT NULL DEFAULT 'standard'
        CHECK (kind IN ('standard', 'announcement')),
    ADD COLUMN is_default BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN topic TEXT,
    ADD COLUMN archived_at TIMESTAMPTZ;

-- Exactly one default channel.
CREATE UNIQUE INDEX chat_channels_single_default_idx
ON chat_channels (is_default) WHERE is_default;

-- The lounge was created lazily on first message; make it a real seeded row so
-- the default channel always exists.
INSERT INTO chat_channels (channel_key, display_name, is_default)
VALUES ('lounge', '閒聊', TRUE)
ON CONFLICT (channel_key) DO UPDATE SET is_default = TRUE, is_active = TRUE;

ALTER TABLE chat_messages
    ADD COLUMN parent_message_id UUID REFERENCES chat_messages(id) ON DELETE SET NULL,
    ADD COLUMN pinned_at TIMESTAMPTZ,
    ADD COLUMN pinned_by UUID REFERENCES accounts(id) ON DELETE SET NULL;

CREATE INDEX chat_messages_parent_idx
ON chat_messages (parent_message_id, created_at)
WHERE parent_message_id IS NOT NULL;

CREATE INDEX chat_messages_pinned_idx
ON chat_messages (channel_id, pinned_at DESC)
WHERE pinned_at IS NOT NULL;

CREATE TABLE chat_message_reactions (
    message_id UUID NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    emoji VARCHAR(32) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (message_id, account_id, emoji),
    CONSTRAINT chat_reaction_emoji_not_blank CHECK (length(btrim(emoji)) > 0)
);

CREATE INDEX chat_message_reactions_message_idx
ON chat_message_reactions (message_id);

COMMENT ON COLUMN chat_channels.is_default IS '預設頻道（lounge）；跨平台橋接只綁定此頻道。';
COMMENT ON TABLE chat_message_reactions IS '每個帳號對一則訊息的每種 emoji 至多一次。';
