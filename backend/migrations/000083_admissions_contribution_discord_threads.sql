ALTER TABLE admissions_contributions
    ADD COLUMN discord_thread_id TEXT;

CREATE UNIQUE INDEX admissions_contributions_discord_thread_idx
    ON admissions_contributions (discord_thread_id)
    WHERE discord_thread_id IS NOT NULL;

COMMENT ON COLUMN admissions_contributions.discord_thread_id IS
    'Discord forum thread for this report; used to route staff clarification replies.';
