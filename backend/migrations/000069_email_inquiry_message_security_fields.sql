-- Mail history was too thin for tracking/abuse investigation: only the body
-- and a free-text "actor" display name were kept per message, with no
-- record of the email's subject, the connecting SMTP client's IP, or its
-- SPF/DKIM/DMARC verdict (inbound), and no immutable identity for who
-- actually sent an outbound /re reply (a Discord display name can be
-- renamed at any time; the account's snowflake ID cannot).
ALTER TABLE email_inquiry_messages ADD COLUMN subject text NOT NULL DEFAULT '';
ALTER TABLE email_inquiry_messages ADD COLUMN sender_ip text NOT NULL DEFAULT '';
ALTER TABLE email_inquiry_messages ADD COLUMN auth_results text NOT NULL DEFAULT '';
ALTER TABLE email_inquiry_messages ADD COLUMN actor_discord_id text NOT NULL DEFAULT '';
