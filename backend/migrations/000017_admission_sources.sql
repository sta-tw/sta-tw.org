-- Official public admissions source registry. Source rows are a control-plane
-- allowlist; they never publish admissions data by themselves.

CREATE TABLE admissions_sources (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_code VARCHAR(3) NOT NULL REFERENCES schools(school_code) ON DELETE RESTRICT,
    academic_year SMALLINT NOT NULL CHECK (academic_year BETWEEN 100 AND 999),
    source_url TEXT NOT NULL,
    normalized_url TEXT NOT NULL,
    hostname TEXT NOT NULL,
    source_type VARCHAR(32) NOT NULL DEFAULT 'official_entry'
        CHECK (source_type IN ('official_entry', 'brochure', 'announcement', 'stage_notice', 'result', 'waitlist_notice', 'unknown')),
    status VARCHAR(16) NOT NULL DEFAULT 'candidate'
        CHECK (status IN ('candidate', 'active', 'rejected', 'expired')),
    decision_mode VARCHAR(16) NOT NULL DEFAULT 'agent'
        CHECK (decision_mode IN ('agent', 'manual')),
    affiliation_confidence VARCHAR(16) NOT NULL DEFAULT 'unknown'
        CHECK (affiliation_confidence IN ('high', 'medium', 'low', 'unknown')),
    discovery_method VARCHAR(32) NOT NULL DEFAULT 'manual'
        CHECK (discovery_method IN ('official_link', 'search_discovery', 'page_link', 'manual')),
    evidence JSONB NOT NULL DEFAULT '[]'::jsonb,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_crawled_at TIMESTAMPTZ,
    last_discovery_at TIMESTAMPTZ,
    discovery_needed BOOLEAN NOT NULL DEFAULT FALSE,
    discovery_reason TEXT,
    rejected_reason TEXT,
    manual_note TEXT,
    created_by UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    updated_by UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT admissions_sources_url_not_blank CHECK (length(btrim(source_url)) > 0),
    CONSTRAINT admissions_sources_normalized_url_not_blank CHECK (length(btrim(normalized_url)) > 0),
    CONSTRAINT admissions_sources_hostname_not_blank CHECK (length(btrim(hostname)) > 0),
    CONSTRAINT admissions_sources_evidence_array CHECK (jsonb_typeof(evidence) = 'array'),
    CONSTRAINT admissions_sources_rejected_reason_required CHECK (status <> 'rejected' OR length(btrim(COALESCE(rejected_reason, ''))) > 0)
);

CREATE UNIQUE INDEX admissions_sources_identity_idx
ON admissions_sources (school_code, academic_year, normalized_url);

CREATE INDEX admissions_sources_filter_idx
ON admissions_sources (academic_year, school_code, status, last_seen_at DESC);

CREATE TRIGGER admissions_sources_set_updated_at
BEFORE UPDATE ON admissions_sources
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

COMMENT ON TABLE admissions_sources IS '官方招生公開來源總清單；只控制可擷取來源，不直接發布招生資料。';
COMMENT ON COLUMN admissions_sources.evidence IS '來源與學校關聯的原文證據、URL 及頁面定位。';
