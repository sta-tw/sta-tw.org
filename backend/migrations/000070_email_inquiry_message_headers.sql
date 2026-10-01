-- Rounding out the mail record toward a Gmail-style complete history:
-- to/in-reply-to/references/mailer were all available in the raw message
-- headers already but discarded after routing — keeping them lets a
-- future investigation actually reconstruct the real header trail instead
-- of just the parsed body.
ALTER TABLE email_inquiry_messages ADD COLUMN to_address text NOT NULL DEFAULT '';
ALTER TABLE email_inquiry_messages ADD COLUMN in_reply_to text NOT NULL DEFAULT '';
ALTER TABLE email_inquiry_messages ADD COLUMN email_references text NOT NULL DEFAULT '';
ALTER TABLE email_inquiry_messages ADD COLUMN mailer text NOT NULL DEFAULT '';
