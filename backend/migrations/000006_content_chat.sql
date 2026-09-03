-- STA Phase 7: website forums, experience revisions, and the single chat bridge.

CREATE TABLE forum_spaces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    space_type VARCHAR(20) NOT NULL
        CHECK (space_type IN ('global', 'school_program', 'annual')),
    display_name TEXT NOT NULL,
    academic_year SMALLINT CHECK (academic_year IS NULL OR academic_year BETWEEN 100 AND 999),
    school_code VARCHAR(3) REFERENCES schools(school_code) ON DELETE RESTRICT,
    program_code VARCHAR(3) CHECK (program_code IS NULL OR program_code ~ '^[0-9]{3}$'),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT forum_spaces_name_not_blank CHECK (length(btrim(display_name)) > 0),
    CONSTRAINT forum_spaces_scope CHECK (
        (space_type = 'global' AND academic_year IS NULL AND school_code IS NULL AND program_code IS NULL)
        OR (space_type = 'annual' AND academic_year IS NOT NULL AND school_code IS NULL AND program_code IS NULL)
        OR (space_type = 'school_program' AND academic_year IS NOT NULL AND school_code IS NOT NULL AND program_code IS NOT NULL)
    ),
    UNIQUE (space_type, academic_year, school_code, program_code)
);

CREATE TRIGGER forum_spaces_set_updated_at
BEFORE UPDATE ON forum_spaces
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TABLE forum_memberships (
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    space_id UUID NOT NULL REFERENCES forum_spaces(id) ON DELETE RESTRICT,
    status VARCHAR(16) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'removed')),
    joined_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    removed_at TIMESTAMPTZ,
    PRIMARY KEY (account_id, space_id)
);

CREATE INDEX forum_memberships_space_idx
ON forum_memberships (space_id, status);

CREATE TABLE forum_threads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    space_id UUID NOT NULL REFERENCES forum_spaces(id) ON DELETE RESTRICT,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    title TEXT NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'published'
        CHECK (status IN ('published', 'hidden', 'locked', 'removed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT forum_threads_title_not_blank CHECK (length(btrim(title)) > 0)
);

CREATE INDEX forum_threads_space_idx
ON forum_threads (space_id, status, updated_at DESC);

CREATE TRIGGER forum_threads_set_updated_at
BEFORE UPDATE ON forum_threads
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TABLE experiences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    author_account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    visibility VARCHAR(16) NOT NULL DEFAULT 'hidden'
        CHECK (visibility IN ('hidden', 'published', 'unpublished')),
    current_public_revision_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE experience_revisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    experience_id UUID NOT NULL REFERENCES experiences(id) ON DELETE RESTRICT,
    version_number INTEGER NOT NULL CHECK (version_number > 0),
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    author_type TEXT NOT NULL DEFAULT '-',
    admission_outcome TEXT NOT NULL DEFAULT '-',
    review_status VARCHAR(16) NOT NULL DEFAULT 'draft'
        CHECK (review_status IN ('draft', 'pending_review', 'approved', 'rejected')),
    rejection_reason TEXT,
    reviewed_by UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT experience_revisions_title_not_blank CHECK (length(btrim(title)) > 0),
    CONSTRAINT experience_revisions_body_not_blank CHECK (length(btrim(body)) > 0),
    UNIQUE (experience_id, version_number)
);

ALTER TABLE experiences
ADD CONSTRAINT experiences_public_revision_fk
FOREIGN KEY (current_public_revision_id) REFERENCES experience_revisions(id) ON DELETE RESTRICT;

CREATE INDEX experience_revisions_review_idx
ON experience_revisions (review_status, created_at);

CREATE INDEX experiences_public_idx
ON experiences (updated_at DESC)
WHERE visibility = 'published';

CREATE TRIGGER experiences_set_updated_at
BEFORE UPDATE ON experiences
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER experience_revisions_set_updated_at
BEFORE UPDATE ON experience_revisions
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TABLE experience_revision_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    revision_id UUID NOT NULL REFERENCES experience_revisions(id) ON DELETE RESTRICT,
    actor_account_id UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    action VARCHAR(32) NOT NULL,
    from_status VARCHAR(16),
    to_status VARCHAR(16),
    reason TEXT NOT NULL DEFAULT '-',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX experience_revision_events_revision_idx
ON experience_revision_events (revision_id, created_at DESC);

CREATE OR REPLACE FUNCTION prevent_experience_event_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'experience_revision_events is append-only';
END;
$$;

CREATE TRIGGER experience_revision_events_no_mutation
BEFORE UPDATE OR DELETE ON experience_revision_events
FOR EACH ROW
EXECUTE FUNCTION prevent_experience_event_mutation();

CREATE TABLE forum_posts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id UUID NOT NULL REFERENCES forum_threads(id) ON DELETE RESTRICT,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    body TEXT NOT NULL,
    quoted_experience_id UUID REFERENCES experiences(id) ON DELETE RESTRICT,
    status VARCHAR(16) NOT NULL DEFAULT 'published'
        CHECK (status IN ('published', 'hidden', 'removed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT forum_posts_body_not_blank CHECK (length(btrim(body)) > 0)
);

CREATE INDEX forum_posts_thread_idx
ON forum_posts (thread_id, status, created_at);

CREATE TABLE chat_channels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_key VARCHAR(32) NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE chat_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id UUID NOT NULL REFERENCES chat_channels(id) ON DELETE RESTRICT,
    author_account_id UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    source_platform VARCHAR(16) NOT NULL
        CHECK (source_platform IN ('website', 'discord', 'telegram')),
    external_author_hash BYTEA,
    body TEXT NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'edited', 'deleted')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    edited_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    CONSTRAINT chat_messages_body_not_blank CHECK (length(btrim(body)) > 0 OR status = 'deleted')
);

CREATE INDEX chat_messages_channel_idx
ON chat_messages (channel_id, created_at DESC);

CREATE TABLE chat_message_bridges (
    message_id UUID NOT NULL REFERENCES chat_messages(id) ON DELETE RESTRICT,
    platform VARCHAR(16) NOT NULL CHECK (platform IN ('discord', 'telegram')),
    external_message_id TEXT NOT NULL,
    sync_status VARCHAR(16) NOT NULL DEFAULT 'pending'
        CHECK (sync_status IN ('pending', 'sent', 'edited', 'deleted', 'failed')),
    last_error TEXT,
    synced_at TIMESTAMPTZ,
    PRIMARY KEY (message_id, platform),
    UNIQUE (platform, external_message_id)
);

CREATE TABLE chat_inbound_dedup (
    platform VARCHAR(16) NOT NULL CHECK (platform IN ('discord', 'telegram')),
    external_message_id TEXT NOT NULL,
    message_id UUID NOT NULL REFERENCES chat_messages(id) ON DELETE RESTRICT,
    received_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (platform, external_message_id)
);

CREATE TABLE chat_sync_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL REFERENCES chat_messages(id) ON DELETE RESTRICT,
    target_platform VARCHAR(16) NOT NULL CHECK (target_platform IN ('discord', 'telegram')),
    operation VARCHAR(16) NOT NULL CHECK (operation IN ('create', 'edit', 'delete')),
    payload JSONB NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'sent', 'failed')),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    last_error TEXT,
    available_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX chat_sync_outbox_due_idx
ON chat_sync_outbox (status, available_at, created_at);

CREATE TRIGGER chat_sync_outbox_set_updated_at
BEFORE UPDATE ON chat_sync_outbox
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

COMMENT ON TABLE forum_memberships IS '身份組概念；加入／移除不會改寫 applications，加入資格由 API 依申請與身份判斷。';
COMMENT ON TABLE experiences IS '心得文章本體；沒有留言表，論壇可透過 forum_posts.quoted_experience_id 引用。';
COMMENT ON TABLE experience_revisions IS '作者編輯產生新 revision；舊 approved/public revision 可繼續公開直到新 revision 通過。';
COMMENT ON TABLE chat_messages IS 'Discord／網站／Telegram 單一閒聊頻道的網站 canonical message。';
COMMENT ON TABLE chat_message_bridges IS '每則 canonical message 在外部平台的訊息對照與同步狀態。';
COMMENT ON TABLE chat_sync_outbox IS '網站 canonical message 的跨平台同步 outbox；平台故障時可重試。';
