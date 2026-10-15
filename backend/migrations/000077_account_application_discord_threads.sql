-- Account applications now live in Discord forum threads so reviewers can
-- approve/reject from message buttons and staff can use /re for corrections.
ALTER TABLE account_applications
    ADD COLUMN discord_thread_id TEXT;

ALTER TABLE account_application_messages
    ADD COLUMN actor_discord_id TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX account_applications_discord_thread_idx
    ON account_applications (discord_thread_id)
    WHERE discord_thread_id IS NOT NULL;
