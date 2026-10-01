-- Admin-managed inbound mail categories: each row is one local-part at the
-- existing mail domain (e.g. "brochure" -> brochure@mail.sta-tw.org) wired
-- to its own Discord forum channel. Postfix looks this table up directly
-- (pgsql maps) so a new category needs no container restart.
CREATE TABLE mail_routes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    local_part TEXT NOT NULL UNIQUE,
    label TEXT NOT NULL,
    discord_forum_channel_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by UUID REFERENCES accounts(id)
);
