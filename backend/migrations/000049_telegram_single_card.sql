-- STA: redesign the account-application Telegram thread as a single
-- continuously-edited message instead of one message per turn (which
-- flooded the chat and made "which message do I reply to" ambiguous).
-- account_application_telegram_messages (000048) is superseded — with only
-- one message id per application, ever, the original telegram_chat_id/
-- telegram_message_id columns are sufficient again.

DROP TABLE account_application_telegram_messages;

-- The static header (寄件人/內容/附件 etc.) rendered once at creation, kept
-- so an edit can rebuild header + running transcript without re-deriving
-- document presign links or re-running the notification template.
ALTER TABLE account_applications
    ADD COLUMN telegram_header_text TEXT NOT NULL DEFAULT '';

-- Who said this turn, for the transcript rendering: "使用者" for inbound,
-- or the staff member's Telegram display name for an outbound reply typed
-- in Telegram. Empty for the system-generated rejection email (shown as
-- "系統" instead).
ALTER TABLE account_application_messages
    ADD COLUMN actor TEXT NOT NULL DEFAULT '';
