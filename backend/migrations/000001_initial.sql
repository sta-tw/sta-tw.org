-- STA Phase 2: core relational model.
--
-- This migration deliberately stores sensitive lookup values as application-
-- produced hashes/ciphertext. Plaintext verification documents, OAuth subject
-- values, candidate numbers, and uploaded files must not be put in these tables.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$;

CREATE TABLE accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username VARCHAR(64) NOT NULL,
    email_ciphertext BYTEA NOT NULL,
    email_lookup_hash BYTEA NOT NULL,
    password_hash TEXT NOT NULL,
    identity_status VARCHAR(16) NOT NULL DEFAULT 'temporary'
        CHECK (identity_status IN ('temporary', 'student', 'senior')),
    account_status VARCHAR(16) NOT NULL DEFAULT 'active'
        CHECK (account_status IN ('active', 'suspended', 'deleted')),
    email_verified_at TIMESTAMPTZ,
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT accounts_username_not_blank CHECK (length(btrim(username)) > 0),
    CONSTRAINT accounts_email_ciphertext_not_empty CHECK (octet_length(email_ciphertext) > 0),
    CONSTRAINT accounts_email_lookup_hash_not_empty CHECK (octet_length(email_lookup_hash) > 0),
    CONSTRAINT accounts_password_hash_not_empty CHECK (length(password_hash) > 0),
    UNIQUE (username),
    UNIQUE (email_lookup_hash)
);

CREATE TRIGGER accounts_set_updated_at
BEFORE UPDATE ON accounts
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TABLE oauth_identities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    provider VARCHAR(16) NOT NULL
        CHECK (provider IN ('google', 'discord')),
    provider_subject_hash BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMPTZ,
    CONSTRAINT oauth_subject_hash_not_empty CHECK (octet_length(provider_subject_hash) > 0),
    UNIQUE (provider, provider_subject_hash),
    UNIQUE (account_id, provider)
);

CREATE TABLE schools (
    school_code VARCHAR(3) PRIMARY KEY
        CHECK (school_code ~ '^[0-9]{3}$'),
    school_name TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT schools_name_not_blank CHECK (length(btrim(school_name)) > 0)
);

CREATE TRIGGER schools_set_updated_at
BEFORE UPDATE ON schools
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TABLE academic_programs (
    academic_year SMALLINT NOT NULL
        CHECK (academic_year BETWEEN 100 AND 999),
    school_code VARCHAR(3) NOT NULL REFERENCES schools(school_code) ON DELETE RESTRICT,
    program_code VARCHAR(3) NOT NULL
        CHECK (program_code ~ '^[0-9]{3}$'),
    admission_program_name TEXT NOT NULL,
    admission_quota INTEGER NOT NULL CHECK (admission_quota >= 0),
    brochure_is_tentative BOOLEAN NOT NULL DEFAULT FALSE,
    brochure_announcement_date DATE,
    brochure_scheduled_date DATE,
    registration_start_date DATE,
    registration_end_date DATE,
    exam_start_date DATE,
    exam_end_date DATE,
    result_date DATE,
    consultation_phone TEXT NOT NULL DEFAULT '-',
    brochure_url TEXT NOT NULL DEFAULT '-',
    special_talent_target TEXT NOT NULL DEFAULT '-',
    different_education_backgrounds TEXT NOT NULL DEFAULT '-',
    different_education_other TEXT NOT NULL DEFAULT '-',
    notes TEXT NOT NULL DEFAULT '-',
    source_page SMALLINT CHECK (source_page BETWEEN 1 AND 999),
    program_identifier VARCHAR(11) GENERATED ALWAYS AS (
        lpad(academic_year::TEXT, 3, '0') || '-' || school_code || '-' || program_code
    ) STORED,
    source_locator VARCHAR(7) GENERATED ALWAYS AS (
        CASE
            WHEN source_page IS NULL THEN NULL
            ELSE school_code || '-' || lpad(source_page::TEXT, 3, '0')
        END
    ) STORED,
    review_status VARCHAR(16) NOT NULL DEFAULT 'draft'
        CHECK (review_status IN ('draft', 'pending', 'approved', 'published', 'rejected', 'archived')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (academic_year, school_code, program_code),
    UNIQUE (program_identifier),
    CONSTRAINT academic_programs_name_not_blank CHECK (length(btrim(admission_program_name)) > 0),
    CONSTRAINT academic_programs_registration_range CHECK (
        registration_start_date IS NULL
        OR registration_end_date IS NULL
        OR registration_start_date <= registration_end_date
    ),
    CONSTRAINT academic_programs_exam_range CHECK (
        exam_start_date IS NULL
        OR exam_end_date IS NULL
        OR exam_start_date <= exam_end_date
    )
);

CREATE INDEX academic_programs_school_year_idx
ON academic_programs (school_code, academic_year);

CREATE TRIGGER academic_programs_set_updated_at
BEFORE UPDATE ON academic_programs
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TABLE brochure_documents (
    academic_year SMALLINT NOT NULL
        CHECK (academic_year BETWEEN 100 AND 999),
    school_code VARCHAR(3) NOT NULL REFERENCES schools(school_code) ON DELETE RESTRICT,
    storage_key TEXT NOT NULL,
    original_file_name TEXT NOT NULL,
    mime_type VARCHAR(128) NOT NULL DEFAULT 'application/pdf',
    file_size_bytes BIGINT NOT NULL CHECK (file_size_bytes > 0),
    sha256_hex CHAR(64) NOT NULL CHECK (sha256_hex ~ '^[0-9a-f]{64}$'),
    source_url TEXT NOT NULL DEFAULT '-',
    review_status VARCHAR(16) NOT NULL DEFAULT 'draft'
        CHECK (review_status IN ('draft', 'pending', 'approved', 'published', 'rejected', 'archived')),
    published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (academic_year, school_code),
    UNIQUE (storage_key)
);

CREATE INDEX brochure_documents_status_idx
ON brochure_documents (academic_year, review_status);

CREATE TRIGGER brochure_documents_set_updated_at
BEFORE UPDATE ON brochure_documents
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TABLE program_exam_items (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    academic_year SMALLINT NOT NULL,
    school_code VARCHAR(3) NOT NULL,
    program_code VARCHAR(3) NOT NULL,
    item_name TEXT NOT NULL,
    sort_order SMALLINT NOT NULL CHECK (sort_order > 0),
    weight_percent NUMERIC(5, 2) CHECK (weight_percent BETWEEN 0 AND 100),
    multiplier NUMERIC(8, 3) CHECK (multiplier >= 0),
    description TEXT NOT NULL DEFAULT '-',
    source_page SMALLINT CHECK (source_page BETWEEN 1 AND 999),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT program_exam_items_program_fk
        FOREIGN KEY (academic_year, school_code, program_code)
        REFERENCES academic_programs(academic_year, school_code, program_code)
        ON DELETE RESTRICT,
    CONSTRAINT program_exam_items_name_not_blank CHECK (length(btrim(item_name)) > 0),
    CONSTRAINT program_exam_items_has_ratio CHECK (weight_percent IS NOT NULL OR multiplier IS NOT NULL),
    UNIQUE (academic_year, school_code, program_code, sort_order)
);

CREATE INDEX program_exam_items_program_idx
ON program_exam_items (academic_year, school_code, program_code);

CREATE TRIGGER program_exam_items_set_updated_at
BEFORE UPDATE ON program_exam_items
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TABLE applications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    academic_year SMALLINT NOT NULL,
    school_code VARCHAR(3) NOT NULL,
    program_code VARCHAR(3) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'confirmed', 'withdrawn', 'archived')),
    locked_at TIMESTAMPTZ,
    candidate_number_ciphertext BYTEA,
    candidate_number_lookup_hash BYTEA,
    candidate_number_last4 VARCHAR(4),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT applications_program_fk
        FOREIGN KEY (academic_year, school_code, program_code)
        REFERENCES academic_programs(academic_year, school_code, program_code)
        ON DELETE RESTRICT,
    CONSTRAINT applications_candidate_number_pair CHECK (
        (candidate_number_ciphertext IS NULL AND candidate_number_lookup_hash IS NULL)
        OR (candidate_number_ciphertext IS NOT NULL AND candidate_number_lookup_hash IS NOT NULL)
    ),
    CONSTRAINT applications_candidate_number_last4 CHECK (
        candidate_number_last4 IS NULL OR candidate_number_last4 ~ '^[[:alnum:]]{4}$'
    ),
    UNIQUE (account_id, academic_year, school_code, program_code)
);

CREATE INDEX applications_program_idx
ON applications (academic_year, school_code, program_code, status);

CREATE INDEX applications_candidate_lookup_idx
ON applications (academic_year, school_code, program_code, candidate_number_lookup_hash)
WHERE candidate_number_lookup_hash IS NOT NULL;

CREATE TRIGGER applications_set_updated_at
BEFORE UPDATE ON applications
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TABLE application_service_tickets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    requested_academic_year SMALLINT NOT NULL CHECK (requested_academic_year BETWEEN 100 AND 999),
    requested_school_code VARCHAR(3) NOT NULL REFERENCES schools(school_code) ON DELETE RESTRICT,
    requested_program_code VARCHAR(3) NOT NULL,
    reason TEXT NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'open'
        CHECK (status IN ('open', 'approved', 'rejected', 'closed')),
    reviewed_by UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT application_tickets_program_fk
        FOREIGN KEY (requested_academic_year, requested_school_code, requested_program_code)
        REFERENCES academic_programs(academic_year, school_code, program_code)
        ON DELETE RESTRICT,
    CONSTRAINT application_tickets_reason_not_blank CHECK (length(btrim(reason)) > 0),
    CONSTRAINT application_tickets_program_code_format CHECK (requested_program_code ~ '^[0-9]{3}$')
);

CREATE INDEX application_service_tickets_status_idx
ON application_service_tickets (status, created_at);

CREATE TRIGGER application_service_tickets_set_updated_at
BEFORE UPDATE ON application_service_tickets
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TABLE audit_log (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_account_id UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    action VARCHAR(64) NOT NULL,
    entity_type VARCHAR(64) NOT NULL,
    entity_key TEXT NOT NULL,
    before_data JSONB,
    after_data JSONB,
    reason TEXT NOT NULL DEFAULT '-',
    request_id VARCHAR(128),
    ip_hash BYTEA,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT audit_log_action_not_blank CHECK (length(btrim(action)) > 0),
    CONSTRAINT audit_log_entity_type_not_blank CHECK (length(btrim(entity_type)) > 0),
    CONSTRAINT audit_log_entity_key_not_blank CHECK (length(btrim(entity_key)) > 0)
);

CREATE INDEX audit_log_entity_idx
ON audit_log (entity_type, entity_key, created_at DESC);

CREATE INDEX audit_log_actor_idx
ON audit_log (actor_account_id, created_at DESC);

CREATE OR REPLACE FUNCTION prevent_audit_log_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is append-only';
END;
$$;

CREATE TRIGGER audit_log_no_update
BEFORE UPDATE OR DELETE ON audit_log
FOR EACH ROW
EXECUTE FUNCTION prevent_audit_log_mutation();

COMMENT ON TABLE accounts IS '原生 STA 帳號；email 與 OAuth subject 不以明文保存。';
COMMENT ON TABLE oauth_identities IS '第三方 OAuth 輔助綁定；每個 provider 每個帳號最多一筆。';
COMMENT ON TABLE academic_programs IS '年度-學校-科系的招生資料；甲乙丙組使用不同 program_code。';
COMMENT ON COLUMN academic_programs.program_identifier IS '對外資料序號，格式為 000-000-000；年度封存後以新年度建立新序號。';
COMMENT ON COLUMN academic_programs.source_locator IS '由 school_code 與 source_page 產生的 000-000 定位編號；定位頁碼可空。';
COMMENT ON TABLE brochure_documents IS '每年度每校只保存最新簡章；歷次替換由 audit_log 留下異動紀錄。';
COMMENT ON TABLE applications IS '使用者正式申請；confirmed 後由 API 鎖定，追加申請走 service ticket。';
COMMENT ON TABLE audit_log IS '只允許新增，不允許一般更新或刪除；before/after 必須由應用層先去識別化。';
