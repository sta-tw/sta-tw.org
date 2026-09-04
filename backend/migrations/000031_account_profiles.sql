-- STA Phase 16: opt-in public profiles with an avatar.
--
-- One row per account, created on first edit. display_name / bio / links are
-- shown on GET /api/v1/users/{username}; the avatar is stored privately in
-- object storage and streamed through the API (same handling as portfolio
-- files: ClamAV scan on upload, no direct-to-storage URL).

CREATE TABLE account_profiles (
    account_id UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    display_name TEXT,
    bio TEXT,
    links JSONB NOT NULL DEFAULT '[]'::jsonb,
    avatar_storage_key TEXT,
    avatar_content_type TEXT,
    avatar_updated_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT account_profiles_display_name_len CHECK (display_name IS NULL OR length(display_name) <= 80),
    CONSTRAINT account_profiles_bio_len CHECK (bio IS NULL OR length(bio) <= 500),
    CONSTRAINT account_profiles_links_is_array CHECK (jsonb_typeof(links) = 'array')
);

CREATE TRIGGER account_profiles_set_updated_at
BEFORE UPDATE ON account_profiles
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

COMMENT ON TABLE account_profiles IS '選擇性公開個人檔案；avatar 私有儲存、經 API 串流。';
