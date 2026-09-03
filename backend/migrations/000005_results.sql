-- STA Phase 6: official results, candidate matching, willingness, and inquiry rounds.

CREATE TABLE official_result_batches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    academic_year SMALLINT NOT NULL CHECK (academic_year BETWEEN 100 AND 999),
    school_code VARCHAR(3) NOT NULL REFERENCES schools(school_code) ON DELETE RESTRICT,
    source_url TEXT NOT NULL DEFAULT '-',
    source_sha256_hex CHAR(64) NOT NULL CHECK (source_sha256_hex ~ '^[0-9a-f]{64}$'),
    status VARCHAR(16) NOT NULL DEFAULT 'pending_review'
        CHECK (status IN ('processing', 'pending_review', 'approved', 'rejected', 'published', 'superseded')),
    imported_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    reviewed_by UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    reviewed_at TIMESTAMPTZ,
    UNIQUE (academic_year, school_code, source_sha256_hex)
);

CREATE INDEX official_result_batches_status_idx
ON official_result_batches (status, imported_at DESC);

CREATE TABLE official_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id UUID NOT NULL REFERENCES official_result_batches(id) ON DELETE RESTRICT,
    academic_year SMALLINT NOT NULL CHECK (academic_year BETWEEN 100 AND 999),
    school_code VARCHAR(3) NOT NULL REFERENCES schools(school_code) ON DELETE RESTRICT,
    program_code VARCHAR(3) NOT NULL CHECK (program_code ~ '^[0-9]{3}$'),
    application_id UUID REFERENCES applications(id) ON DELETE RESTRICT,
    candidate_number_lookup_hash BYTEA NOT NULL,
    candidate_number_last4 VARCHAR(4) NOT NULL CHECK (candidate_number_last4 ~ '^[[:alnum:]]{4}$'),
    masked_name TEXT NOT NULL DEFAULT '-',
    result_status VARCHAR(16) NOT NULL
        CHECK (result_status IN ('admitted', 'waitlisted', 'rejected', 'unknown')),
    official_rank INTEGER CHECK (official_rank IS NULL OR official_rank > 0),
    quota INTEGER CHECK (quota IS NULL OR quota >= 0),
    source_page SMALLINT CHECK (source_page BETWEEN 1 AND 999),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT official_results_candidate_hash_not_empty CHECK (octet_length(candidate_number_lookup_hash) > 0),
    CONSTRAINT official_results_program_fk
        FOREIGN KEY (academic_year, school_code, program_code)
        REFERENCES academic_programs(academic_year, school_code, program_code)
        ON DELETE RESTRICT,
    UNIQUE (batch_id, academic_year, school_code, program_code, candidate_number_lookup_hash)
);

CREATE INDEX official_results_program_rank_idx
ON official_results (academic_year, school_code, program_code, official_rank)
WHERE result_status = 'waitlisted';

CREATE INDEX official_results_application_idx
ON official_results (application_id)
WHERE application_id IS NOT NULL;

CREATE TRIGGER official_results_set_updated_at
BEFORE UPDATE ON official_results
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TABLE application_willingness (
    application_id UUID PRIMARY KEY REFERENCES applications(id) ON DELETE RESTRICT,
    value SMALLINT CHECK (value IS NULL OR (value BETWEEN 0 AND 100 AND value % 20 = 0)),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE willingness_inquiries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id UUID NOT NULL REFERENCES applications(id) ON DELETE RESTRICT,
    inquiry_round VARCHAR(32) NOT NULL
        CHECK (inquiry_round IN ('result_released', 'acceptance_deadline')),
    sent_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    response_deadline TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (application_id, inquiry_round)
);

CREATE INDEX willingness_inquiries_due_idx
ON willingness_inquiries (response_deadline)
WHERE response_deadline IS NOT NULL;

CREATE TABLE willingness_response_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    application_id UUID NOT NULL REFERENCES applications(id) ON DELETE RESTRICT,
    inquiry_id UUID REFERENCES willingness_inquiries(id) ON DELETE RESTRICT,
    value SMALLINT NOT NULL CHECK (value BETWEEN 0 AND 100 AND value % 20 = 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX willingness_response_events_application_idx
ON willingness_response_events (application_id, created_at DESC);

CREATE OR REPLACE FUNCTION prevent_willingness_event_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'willingness_response_events is append-only';
END;
$$;

CREATE TRIGGER willingness_response_events_no_mutation
BEFORE UPDATE OR DELETE ON willingness_response_events
FOR EACH ROW
EXECUTE FUNCTION prevent_willingness_event_mutation();

CREATE INDEX official_results_batch_application_idx
ON official_results (batch_id, application_id)
WHERE application_id IS NOT NULL;

COMMENT ON TABLE official_results IS '官方結果與使用者意願完全分開；candidate number 只保存 hash／末四碼。';
COMMENT ON COLUMN official_results.masked_name IS '官方公開的去識別化名稱，只作內部簡單核對，不進個人結果 API。';
COMMENT ON TABLE application_willingness IS '目前意願值；NULL 代表未回覆，不等於 0。';
COMMENT ON TABLE willingness_response_events IS '意願修改歷程；0/20/40/60/80/100 均為合法值。';
