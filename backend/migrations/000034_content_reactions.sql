-- STA Phase 17: emoji reactions on forum posts and experiences.
--
-- One polymorphic table keyed by (target_type, target_id). No FK on target_id
-- because it spans two tables; the API checks the target exists on insert and
-- ON DELETE CASCADE on account_id cleans up when an account is removed.

CREATE TABLE content_reactions (
    target_type VARCHAR(16) NOT NULL CHECK (target_type IN ('forum_post', 'experience')),
    target_id UUID NOT NULL,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    emoji VARCHAR(32) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (target_type, target_id, account_id, emoji),
    CONSTRAINT content_reaction_emoji_not_blank CHECK (length(btrim(emoji)) > 0)
);

CREATE INDEX content_reactions_target_idx
ON content_reactions (target_type, target_id);

COMMENT ON TABLE content_reactions IS '論壇貼文／心得文章的 emoji 反應；每帳號每目標每種 emoji 至多一次。';
