-- STA Phase 8: official brochure file lifecycle and upload history.

CREATE TABLE brochure_document_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    academic_year SMALLINT NOT NULL,
    school_code VARCHAR(3) NOT NULL,
    storage_key TEXT NOT NULL,
    original_file_name TEXT NOT NULL,
    sha256_hex CHAR(64) NOT NULL CHECK (sha256_hex ~ '^[0-9a-f]{64}$'),
    action VARCHAR(32) NOT NULL,
    from_status VARCHAR(16),
    to_status VARCHAR(16),
    actor_account_id UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    reason TEXT NOT NULL DEFAULT '-',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT brochure_document_events_document_fk
        FOREIGN KEY (academic_year, school_code)
        REFERENCES brochure_documents(academic_year, school_code)
        ON DELETE RESTRICT,
    CONSTRAINT brochure_document_events_action_not_blank CHECK (length(btrim(action)) > 0)
);

CREATE INDEX brochure_document_events_document_idx
ON brochure_document_events (academic_year, school_code, created_at DESC);

CREATE OR REPLACE FUNCTION prevent_brochure_document_event_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'brochure_document_events is append-only';
END;
$$;

CREATE TRIGGER brochure_document_events_no_mutation
BEFORE UPDATE OR DELETE ON brochure_document_events
FOR EACH ROW
EXECUTE FUNCTION prevent_brochure_document_event_mutation();

COMMENT ON TABLE brochure_documents IS '每年度每校只保留最新上架檔；歷次上傳與狀態變更保留在 brochure_document_events。';
COMMENT ON TABLE brochure_document_events IS '簡章上傳、審核、上架與下架歷程；不可修改或刪除。';
