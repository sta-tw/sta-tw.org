-- STA Phase 17: forward a chat message into another channel.
--
-- A forward is a new message that copies the source body and points back at the
-- original. The reference is nulled if the source is later hard-deleted.

ALTER TABLE chat_messages
    ADD COLUMN forwarded_from_message_id UUID REFERENCES chat_messages(id) ON DELETE SET NULL;

CREATE INDEX chat_messages_forwarded_from_idx
ON chat_messages (forwarded_from_message_id)
WHERE forwarded_from_message_id IS NOT NULL;

COMMENT ON COLUMN chat_messages.forwarded_from_message_id IS '此訊息是轉發時，指向來源訊息。';
