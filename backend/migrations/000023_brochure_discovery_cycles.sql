-- Configurable academic-year control plane for brochure discovery.

CREATE TABLE brochure_discovery_cycles (
    academic_year SMALLINT PRIMARY KEY CHECK (academic_year BETWEEN 100 AND 999),
    status VARCHAR(16) NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'active', 'closed')),
    created_by UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    started_by UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    closed_by UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    started_at TIMESTAMPTZ,
    closed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT brochure_discovery_cycle_dates_valid CHECK (
        (status = 'draft' AND started_at IS NULL AND closed_at IS NULL)
        OR (status = 'active' AND started_at IS NOT NULL AND closed_at IS NULL)
        OR (status = 'closed' AND started_at IS NOT NULL AND closed_at IS NOT NULL)
    )
);

CREATE UNIQUE INDEX brochure_discovery_one_active_cycle_idx
ON brochure_discovery_cycles ((status))
WHERE status = 'active';

CREATE TRIGGER brochure_discovery_cycles_set_updated_at
BEFORE UPDATE ON brochure_discovery_cycles
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- Migration 22 created the initial 115 task set. It becomes a draft cycle so
-- the operator must explicitly start scheduling from the control panel.
INSERT INTO brochure_discovery_cycles (academic_year, status)
VALUES (115, 'draft')
ON CONFLICT (academic_year) DO NOTHING;

ALTER TABLE brochure_discovery_tasks
ADD CONSTRAINT brochure_discovery_tasks_cycle_fk
FOREIGN KEY (academic_year) REFERENCES brochure_discovery_cycles(academic_year) ON DELETE RESTRICT;

ALTER TABLE brochure_discovery_tasks
DROP CONSTRAINT brochure_discovery_tasks_completion_method_check;

ALTER TABLE brochure_discovery_tasks
ADD CONSTRAINT brochure_discovery_tasks_completion_method_check
CHECK (completion_method IS NULL OR completion_method IN ('ai_confirmed', 'manual_upload', 'no_brochure_confirmed'));

CREATE TABLE brochure_discovery_cycle_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    academic_year SMALLINT NOT NULL REFERENCES brochure_discovery_cycles(academic_year) ON DELETE RESTRICT,
    action VARCHAR(32) NOT NULL,
    from_status VARCHAR(16),
    to_status VARCHAR(16) NOT NULL,
    actor_account_id UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT brochure_discovery_cycle_event_action_not_blank CHECK (length(btrim(action)) > 0)
);

CREATE INDEX brochure_discovery_cycle_events_year_idx
ON brochure_discovery_cycle_events (academic_year, created_at DESC);

CREATE TRIGGER brochure_discovery_cycle_events_no_mutation
BEFORE UPDATE OR DELETE ON brochure_discovery_cycle_events
FOR EACH ROW
EXECUTE FUNCTION prevent_brochure_discovery_event_mutation();

COMMENT ON TABLE brochure_discovery_cycles IS '簡章探索學年度控制面；同一時間最多一個 active 排程。';
COMMENT ON COLUMN brochure_discovery_tasks.completion_method IS '完成原因：外部 discovery agent 找到並經人工確認、人工補檔，或人工確認該校本年度無簡章。';
