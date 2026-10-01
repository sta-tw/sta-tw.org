-- email_inquiries moves from a single hardcoded Telegram-notified inbox to
-- a category (mail_routes) notified via a Discord forum thread.
--
-- Bootstrap a "account" route for existing rows (the only category that
-- existed before this migration was account@mail.sta-tw.org) so the
-- NOT NULL foreign key below has something to point at; its
-- discord_forum_channel_id is a placeholder the admin must replace via the
-- new /admin/mail-routes page before relying on it.
INSERT INTO mail_routes (local_part, label, discord_forum_channel_id)
VALUES ('account', '帳號問題', 'REPLACE_ME');

ALTER TABLE email_inquiries ADD COLUMN mail_route_id UUID REFERENCES mail_routes(id);
UPDATE email_inquiries SET mail_route_id = (SELECT id FROM mail_routes WHERE local_part = 'account');
ALTER TABLE email_inquiries ALTER COLUMN mail_route_id SET NOT NULL;

ALTER TABLE email_inquiries ADD COLUMN discord_thread_id TEXT NOT NULL DEFAULT '';
ALTER TABLE email_inquiries DROP COLUMN telegram_chat_id;
ALTER TABLE email_inquiries DROP COLUMN telegram_message_id;
ALTER TABLE email_inquiries DROP COLUMN telegram_header_text;
