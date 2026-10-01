-- Which Discord roles (besides the bot itself) can see a mail route's forum
-- channel. Empty array means "leave the channel's permissions alone" (admin
-- hasn't picked anything yet); the app only touches Discord's permission
-- overwrites when this is non-empty.
ALTER TABLE mail_routes ADD COLUMN visible_role_ids TEXT[] NOT NULL DEFAULT '{}';
