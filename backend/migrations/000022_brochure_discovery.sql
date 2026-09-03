-- 115 academic-year brochure discovery control plane.
-- Every school in the master is scanned, regardless of prior participation.

CREATE TABLE brochure_discovery_tasks (
    academic_year SMALLINT NOT NULL CHECK (academic_year BETWEEN 100 AND 999),
    school_code VARCHAR(3) NOT NULL REFERENCES schools(school_code) ON DELETE RESTRICT,
    status VARCHAR(24) NOT NULL DEFAULT 'pending_search'
        CHECK (status IN ('completed', 'under_review', 'searching', 'pending_search', 'needs_attention')),
    completion_method VARCHAR(24)
        CHECK (completion_method IS NULL OR completion_method IN ('ai_confirmed', 'manual_upload')),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    candidate_source_url TEXT,
    candidate_document_url TEXT,
    candidate_sha256_hex CHAR(64),
    candidate_confidence NUMERIC(5, 4) CHECK (candidate_confidence IS NULL OR candidate_confidence BETWEEN 0 AND 1),
    candidate_evidence JSONB,
    last_error_code VARCHAR(64),
    last_error_message TEXT,
    last_searched_at TIMESTAMPTZ,
    next_search_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    completed_by UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (academic_year, school_code),
    CONSTRAINT brochure_discovery_candidate_pair CHECK (
        (candidate_source_url IS NULL AND candidate_document_url IS NULL AND candidate_sha256_hex IS NULL)
        OR
        (length(btrim(candidate_source_url)) > 0 AND length(btrim(candidate_document_url)) > 0 AND candidate_sha256_hex ~ '^[0-9a-f]{64}$')
    ),
    CONSTRAINT brochure_discovery_completion_consistent CHECK (
        (status = 'completed' AND completion_method IS NOT NULL AND completed_at IS NOT NULL)
        OR
        (status <> 'completed' AND completion_method IS NULL AND completed_at IS NULL)
    ),
    CONSTRAINT brochure_discovery_evidence_object CHECK (
        candidate_evidence IS NULL OR jsonb_typeof(candidate_evidence) = 'object'
    )
);

CREATE INDEX brochure_discovery_queue_idx
ON brochure_discovery_tasks (academic_year, status, next_search_at, school_code);

CREATE TRIGGER brochure_discovery_tasks_set_updated_at
BEFORE UPDATE ON brochure_discovery_tasks
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

CREATE TABLE brochure_discovery_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    academic_year SMALLINT NOT NULL,
    school_code VARCHAR(3) NOT NULL,
    action VARCHAR(32) NOT NULL,
    from_status VARCHAR(24),
    to_status VARCHAR(24) NOT NULL,
    actor_account_id UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (academic_year, school_code)
        REFERENCES brochure_discovery_tasks(academic_year, school_code) ON DELETE RESTRICT,
    CONSTRAINT brochure_discovery_event_action_not_blank CHECK (length(btrim(action)) > 0),
    CONSTRAINT brochure_discovery_event_details_object CHECK (jsonb_typeof(details) = 'object')
);

CREATE INDEX brochure_discovery_events_task_idx
ON brochure_discovery_events (academic_year, school_code, created_at DESC);

CREATE OR REPLACE FUNCTION prevent_brochure_discovery_event_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'brochure_discovery_events is append-only';
END;
$$;

CREATE TRIGGER brochure_discovery_events_no_mutation
BEFORE UPDATE OR DELETE ON brochure_discovery_events
FOR EACH ROW
EXECUTE FUNCTION prevent_brochure_discovery_event_mutation();

INSERT INTO brochure_discovery_tasks (academic_year, school_code, status)
SELECT 115, school_code, 'pending_search'
FROM schools
ON CONFLICT (academic_year, school_code) DO NOTHING;

COMMENT ON TABLE brochure_discovery_tasks IS '逐校簡章探索狀態；115 學年度以完整學校母表為掃描範圍。';
COMMENT ON TABLE brochure_discovery_events IS '簡章探索、人工確認與人工補檔的 append-only 稽核紀錄。';
