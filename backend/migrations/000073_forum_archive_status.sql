-- Adds an 'archived' status for forum threads/posts, distinct from
-- 'removed': a moderator preserving a post as evidence (e.g. a possible
-- legal matter) rather than just taking down spam. Both statuses are
-- equally invisible to the public forum API — the only difference is
-- intent, surfaced to admins via a separate badge/action in /admin/forum.

ALTER TABLE forum_threads DROP CONSTRAINT forum_threads_status_check;
ALTER TABLE forum_threads ADD CONSTRAINT forum_threads_status_check
    CHECK (status IN ('published', 'hidden', 'locked', 'removed', 'archived'));

ALTER TABLE forum_posts DROP CONSTRAINT forum_posts_status_check;
ALTER TABLE forum_posts ADD CONSTRAINT forum_posts_status_check
    CHECK (status IN ('published', 'hidden', 'removed', 'archived'));
