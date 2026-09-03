-- STA Phase 5: minimal administrator role and portfolio file lifecycle.

CREATE TABLE account_roles (
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    role VARCHAR(32) NOT NULL CHECK (role IN ('admin')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, role)
);

CREATE TABLE portfolio_projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    application_id UUID NOT NULL REFERENCES applications(id) ON DELETE RESTRICT,
    title TEXT NOT NULL DEFAULT '-',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT portfolio_projects_title_not_blank CHECK (length(btrim(title)) > 0),
    UNIQUE (application_id)
);

CREATE INDEX portfolio_projects_owner_idx
ON portfolio_projects (account_id, updated_at DESC);

CREATE TRIGGER portfolio_projects_set_updated_at
BEFORE UPDATE ON portfolio_projects
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TABLE portfolio_files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES portfolio_projects(id) ON DELETE RESTRICT,
    version_number INTEGER NOT NULL CHECK (version_number > 0),
    original_file_name TEXT NOT NULL,
    storage_key TEXT NOT NULL,
    mime_type VARCHAR(128) NOT NULL,
    file_size_bytes BIGINT NOT NULL CHECK (file_size_bytes > 0),
    sha256_hex CHAR(64) NOT NULL CHECK (sha256_hex ~ '^[0-9a-f]{64}$'),
    status VARCHAR(20) NOT NULL DEFAULT 'hidden'
        CHECK (status IN ('hidden', 'pending_review', 'published', 'unpublished', 'rejected')),
    rejection_reason TEXT,
    reviewed_by UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT portfolio_files_name_not_blank CHECK (length(btrim(original_file_name)) > 0),
    CONSTRAINT portfolio_files_storage_key_not_blank CHECK (length(btrim(storage_key)) > 0),
    CONSTRAINT portfolio_files_version_unique UNIQUE (project_id, version_number),
    CONSTRAINT portfolio_files_storage_unique UNIQUE (storage_key)
);

CREATE INDEX portfolio_files_project_status_idx
ON portfolio_files (project_id, status, version_number DESC);

CREATE INDEX portfolio_files_public_idx
ON portfolio_files (status, created_at DESC)
WHERE status = 'published';

CREATE TRIGGER portfolio_files_set_updated_at
BEFORE UPDATE ON portfolio_files
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TABLE portfolio_file_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    file_id UUID NOT NULL REFERENCES portfolio_files(id) ON DELETE RESTRICT,
    actor_account_id UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    action VARCHAR(32) NOT NULL,
    from_status VARCHAR(20),
    to_status VARCHAR(20),
    reason TEXT NOT NULL DEFAULT '-',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT portfolio_file_events_action_not_blank CHECK (length(btrim(action)) > 0)
);

CREATE INDEX portfolio_file_events_file_idx
ON portfolio_file_events (file_id, created_at DESC);

CREATE OR REPLACE FUNCTION prevent_portfolio_file_event_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'portfolio_file_events is append-only';
END;
$$;

CREATE TRIGGER portfolio_file_events_no_mutation
BEFORE UPDATE OR DELETE ON portfolio_file_events
FOR EACH ROW
EXECUTE FUNCTION prevent_portfolio_file_event_mutation();

COMMENT ON TABLE account_roles IS '目前只保留單一 admin role；細分管理權限留待後續階段。';
COMMENT ON TABLE portfolio_projects IS '每筆正式申請最多一個備審資料專案。';
COMMENT ON TABLE portfolio_files IS '檔案本體在 private object storage；此表保存版本、狀態與 checksum。';
COMMENT ON COLUMN portfolio_files.status IS 'hidden 僅本人、pending_review 待審、published 公開、unpublished 下架、rejected 退回。';
COMMENT ON TABLE portfolio_file_events IS '所有上傳、送審、審核、上下架與退回紀錄；append-only。';
