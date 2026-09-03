-- STA Phase 4: durable ingestion jobs and isolated brochure extraction results.
-- Python workers may create job results, but public academic_programs rows are
-- changed only by a Go/admin review transaction.

CREATE TABLE ingestion_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_type VARCHAR(64) NOT NULL,
    idempotency_key VARCHAR(255) NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'running', 'succeeded', 'retrying', 'failed', 'dead_letter')),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    max_attempts INTEGER NOT NULL DEFAULT 5 CHECK (max_attempts BETWEEN 1 AND 20),
    next_attempt_at TIMESTAMPTZ,
    locked_at TIMESTAMPTZ,
    last_error_code VARCHAR(64),
    last_error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT ingestion_jobs_type_not_blank CHECK (length(btrim(job_type)) > 0),
    CONSTRAINT ingestion_jobs_idempotency_not_blank CHECK (length(btrim(idempotency_key)) > 0),
    UNIQUE (idempotency_key)
);

CREATE INDEX ingestion_jobs_status_idx
ON ingestion_jobs (status, next_attempt_at, created_at);

CREATE TRIGGER ingestion_jobs_set_updated_at
BEFORE UPDATE ON ingestion_jobs
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TABLE brochure_extraction_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ingestion_job_id UUID REFERENCES ingestion_jobs(id) ON DELETE RESTRICT,
    academic_year SMALLINT NOT NULL CHECK (academic_year BETWEEN 100 AND 999),
    school_code VARCHAR(3) NOT NULL REFERENCES schools(school_code) ON DELETE RESTRICT,
    source_sha256_hex CHAR(64) NOT NULL CHECK (source_sha256_hex ~ '^[0-9a-f]{64}$'),
    processor_version VARCHAR(64) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending_review'
        CHECK (status IN ('processing', 'pending_review', 'approved', 'rejected', 'failed')),
    raw_extraction JSONB,
    error_code VARCHAR(64),
    error_message TEXT,
    reviewed_by UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT brochure_extraction_runs_processor_not_blank CHECK (length(btrim(processor_version)) > 0),
    UNIQUE (academic_year, school_code, source_sha256_hex, processor_version)
);

CREATE INDEX brochure_extraction_runs_review_idx
ON brochure_extraction_runs (status, created_at);

CREATE TRIGGER brochure_extraction_runs_set_updated_at
BEFORE UPDATE ON brochure_extraction_runs
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TABLE brochure_extraction_candidates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES brochure_extraction_runs(id) ON DELETE RESTRICT,
    program_code VARCHAR(3) NOT NULL CHECK (program_code ~ '^[0-9]{3}$'),
    extracted_data JSONB NOT NULL,
    source_page SMALLINT CHECK (source_page BETWEEN 1 AND 999),
    confidence NUMERIC(5, 4) CHECK (confidence BETWEEN 0 AND 1),
    review_status VARCHAR(16) NOT NULL DEFAULT 'pending'
        CHECK (review_status IN ('pending', 'approved', 'rejected')),
    reviewed_by UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (run_id, program_code)
);

CREATE INDEX brochure_extraction_candidates_review_idx
ON brochure_extraction_candidates (review_status, created_at);

CREATE TRIGGER brochure_extraction_candidates_set_updated_at
BEFORE UPDATE ON brochure_extraction_candidates
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

COMMENT ON TABLE ingestion_jobs IS 'Durable queue metadata；idempotency key prevents duplicate ingestion。';
COMMENT ON TABLE brochure_extraction_runs IS 'Python 解析結果的隔離區；未審核不會成為公開招生資料。';
COMMENT ON TABLE brochure_extraction_candidates IS '逐科系候選資料與頁碼／信心分數；Go/admin 審核後才套用。';
