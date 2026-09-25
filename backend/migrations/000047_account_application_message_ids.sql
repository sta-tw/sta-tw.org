-- STA: an outbound reply (staff typing in Telegram, or a rejection notice)
-- never referenced the sender's own Message-ID, so it showed up in Gmail as
-- an unrelated new email instead of threading under what they sent.
-- source_message_id records each inbound message's RFC 5322 Message-ID so a
-- later outbound reply can set In-Reply-To/References against it.

ALTER TABLE account_application_messages
    ADD COLUMN source_message_id TEXT NOT NULL DEFAULT '';
