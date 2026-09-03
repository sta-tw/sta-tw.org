-- STA Phase 8: annual student verification, school-email whitelist, and
-- purgeable proof material. Verification proof is private object storage data;
-- the database keeps only review state and non-document account attributes.

CREATE TABLE school_email_domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_code VARCHAR(3) NOT NULL REFERENCES schools(school_code) ON DELETE RESTRICT,
    domain TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT school_email_domains_format CHECK (
        domain = lower(btrim(domain))
        AND domain ~ '^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$'
        AND length(domain) <= 253
    ),
    UNIQUE (school_code, domain)
);

CREATE INDEX school_email_domains_active_idx
ON school_email_domains (domain, school_code)
WHERE is_active = TRUE;

CREATE TRIGGER school_email_domains_set_updated_at
BEFORE UPDATE ON school_email_domains
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TABLE verification_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    academic_year SMALLINT NOT NULL CHECK (academic_year BETWEEN 100 AND 999),
    school_code VARCHAR(3) NOT NULL REFERENCES schools(school_code) ON DELETE RESTRICT,
    program_code VARCHAR(3),
    method VARCHAR(16) NOT NULL CHECK (method IN ('school_email', 'document')),
    status VARCHAR(16) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled', 'expired')),
    school_email_ciphertext BYTEA,
    school_email_lookup_hash BYTEA,
    rejection_reason TEXT,
    reviewed_by UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT verification_requests_program_fk
        FOREIGN KEY (academic_year, school_code, program_code)
        REFERENCES academic_programs(academic_year, school_code, program_code)
        ON DELETE RESTRICT,
    CONSTRAINT verification_requests_program_code_format CHECK (
        program_code IS NULL OR program_code ~ '^[0-9]{3}$'
    ),
    CONSTRAINT verification_requests_method_data CHECK (
        (method = 'school_email' AND school_email_ciphertext IS NOT NULL AND school_email_lookup_hash IS NOT NULL)
        OR (method = 'document' AND school_email_ciphertext IS NULL AND school_email_lookup_hash IS NULL)
    ),
    CONSTRAINT verification_requests_email_pair CHECK (
        (school_email_ciphertext IS NULL AND school_email_lookup_hash IS NULL)
        OR (school_email_ciphertext IS NOT NULL AND school_email_lookup_hash IS NOT NULL)
    )
);

CREATE INDEX verification_requests_account_idx
ON verification_requests (account_id, created_at DESC);

CREATE INDEX verification_requests_review_idx
ON verification_requests (status, created_at);

CREATE TRIGGER verification_requests_set_updated_at
BEFORE UPDATE ON verification_requests
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TABLE verification_documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id UUID NOT NULL REFERENCES verification_requests(id) ON DELETE CASCADE,
    original_file_name TEXT NOT NULL,
    storage_key TEXT NOT NULL,
    mime_type VARCHAR(128) NOT NULL,
    file_size_bytes BIGINT NOT NULL CHECK (file_size_bytes > 0),
    sha256_hex CHAR(64) NOT NULL CHECK (sha256_hex ~ '^[0-9a-f]{64}$'),
    status VARCHAR(16) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'approved', 'rejected')),
    rejection_reason TEXT,
    reviewed_by UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (storage_key)
);

CREATE INDEX verification_documents_request_idx
ON verification_documents (request_id, created_at);

CREATE TABLE verification_email_challenges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id UUID NOT NULL REFERENCES verification_requests(id) ON DELETE CASCADE,
    code_hash BYTEA NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    attempt_count SMALLINT NOT NULL DEFAULT 0 CHECK (attempt_count >= 0 AND attempt_count <= 5),
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT verification_email_challenge_hash_not_empty CHECK (octet_length(code_hash) > 0)
);

CREATE INDEX verification_email_challenges_due_idx
ON verification_email_challenges (request_id, expires_at)
WHERE consumed_at IS NULL;

CREATE TABLE student_verifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    source_request_id UUID REFERENCES verification_requests(id) ON DELETE SET NULL,
    academic_year SMALLINT NOT NULL CHECK (academic_year BETWEEN 100 AND 999),
    school_code VARCHAR(3) NOT NULL REFERENCES schools(school_code) ON DELETE RESTRICT,
    program_code VARCHAR(3),
    method VARCHAR(16) NOT NULL CHECK (method IN ('school_email', 'document', 'admin')),
    status VARCHAR(16) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'expired', 'revoked')),
    verified_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMPTZ NOT NULL,
    verified_by UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT student_verifications_program_fk
        FOREIGN KEY (academic_year, school_code, program_code)
        REFERENCES academic_programs(academic_year, school_code, program_code)
        ON DELETE RESTRICT,
    CONSTRAINT student_verifications_program_code_format CHECK (
        program_code IS NULL OR program_code ~ '^[0-9]{3}$'
    ),
    CONSTRAINT student_verifications_expiry CHECK (expires_at > verified_at)
);

CREATE UNIQUE INDEX student_verifications_identity_key
ON student_verifications (account_id, academic_year, school_code, COALESCE(program_code, '-'));

CREATE INDEX student_verifications_active_idx
ON student_verifications (account_id, status, expires_at DESC);

CREATE TRIGGER student_verifications_set_updated_at
BEFORE UPDATE ON student_verifications
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TABLE verification_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    request_id UUID REFERENCES verification_requests(id) ON DELETE SET NULL,
    verification_id UUID REFERENCES student_verifications(id) ON DELETE SET NULL,
    actor_account_id UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    action VARCHAR(32) NOT NULL,
    from_status VARCHAR(16),
    to_status VARCHAR(16),
    reason TEXT NOT NULL DEFAULT '-',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT verification_events_action_not_blank CHECK (length(btrim(action)) > 0)
);

CREATE INDEX verification_events_request_idx
ON verification_events (request_id, created_at DESC);

CREATE OR REPLACE FUNCTION prevent_verification_event_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'verification_events is append-only';
END;
$$;

CREATE TRIGGER verification_events_no_mutation
BEFORE UPDATE OR DELETE ON verification_events
FOR EACH ROW
EXECUTE FUNCTION prevent_verification_event_mutation();

CREATE TABLE annual_maintenance_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    academic_year SMALLINT NOT NULL UNIQUE CHECK (academic_year BETWEEN 100 AND 999),
    status VARCHAR(16) NOT NULL DEFAULT 'running'
        CHECK (status IN ('running', 'completed', 'failed')),
    verification_documents_removed INTEGER NOT NULL DEFAULT 0 CHECK (verification_documents_removed >= 0),
    verification_requests_removed INTEGER NOT NULL DEFAULT 0 CHECK (verification_requests_removed >= 0),
    accounts_promoted INTEGER NOT NULL DEFAULT 0 CHECK (accounts_promoted >= 0),
    error_message TEXT,
    started_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMPTZ
);

COMMENT ON TABLE school_email_domains IS '管理員維護的學校信箱網域白名單；不接受任意網域自稱校方信箱。';
COMMENT ON TABLE verification_requests IS '一次學生身份審核申請；學校信箱與證明文件在年度清理時移除。';
COMMENT ON TABLE verification_documents IS '在校證明的私有物件索引；只保留至年度清理，不進公開 API。';
COMMENT ON TABLE student_verifications IS '長期保留的驗證結果與年度／學校／科系欄位；不保存證明本體。';
COMMENT ON TABLE annual_maintenance_runs IS '六月年度切換與敏感學生驗證資料清理的冪等執行紀錄。';
