-- STA Phase 9: upload-only brochure intake.
-- A PDF can be received before its academic year or school code is known.
-- The source stays isolated here until an administrator confirms the extracted
-- metadata and program records.

CREATE TABLE brochure_uploads (
    id UUID PRIMARY KEY,
    ingestion_job_id UUID NOT NULL UNIQUE REFERENCES ingestion_jobs(id) ON DELETE RESTRICT,
    storage_key TEXT NOT NULL UNIQUE,
    original_file_name TEXT NOT NULL,
    mime_type VARCHAR(128) NOT NULL DEFAULT 'application/pdf',
    file_size_bytes BIGINT NOT NULL CHECK (file_size_bytes > 0),
    sha256_hex CHAR(64) NOT NULL CHECK (sha256_hex ~ '^[0-9a-f]{64}$'),
    source_url TEXT NOT NULL DEFAULT '-',
    detected_academic_year SMALLINT CHECK (detected_academic_year BETWEEN 100 AND 999),
    -- This is an extractor suggestion, not the canonical school identity;
    -- keep unknown OCR results available for human correction.
    detected_school_code VARCHAR(3),
    detected_school_name TEXT NOT NULL DEFAULT '-',
    status VARCHAR(16) NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'processing', 'pending_review', 'approved', 'rejected', 'failed')),
    raw_extraction JSONB,
    error_code VARCHAR(64),
    error_message TEXT,
    created_by UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    reviewed_by UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT brochure_uploads_file_name_not_blank CHECK (length(btrim(original_file_name)) > 0),
    CONSTRAINT brochure_uploads_school_code_format CHECK (
        detected_school_code IS NULL OR detected_school_code ~ '^[0-9]{3}$'
    )
);

CREATE INDEX brochure_uploads_review_idx
ON brochure_uploads (status, created_at DESC);

CREATE INDEX brochure_uploads_detected_identity_idx
ON brochure_uploads (detected_academic_year, detected_school_code);

CREATE TRIGGER brochure_uploads_set_updated_at
BEFORE UPDATE ON brochure_uploads
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

COMMENT ON TABLE brochure_uploads IS '只上傳 PDF 的簡章暫存；解析結果須人工確認後才建立正式簡章與招生資料。';
