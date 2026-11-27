CREATE TABLE admissions_school_official_links (
    school_code VARCHAR(3) PRIMARY KEY REFERENCES schools(school_code) ON DELETE RESTRICT,
    url TEXT NOT NULL CHECK (length(btrim(url)) > 0),
    updated_by UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE admissions_program_official_links (
    school_code VARCHAR(3) NOT NULL REFERENCES schools(school_code) ON DELETE RESTRICT,
    program_code VARCHAR(3) NOT NULL CHECK (program_code ~ '^[0-9]{3}$'),
    link_type VARCHAR(16) NOT NULL CHECK (link_type IN ('department', 'collegego')),
    url TEXT NOT NULL CHECK (length(btrim(url)) > 0),
    updated_by UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (school_code, program_code, link_type)
);

CREATE TABLE admissions_program_link_candidates (
    school_code VARCHAR(3) NOT NULL REFERENCES schools(school_code) ON DELETE RESTRICT,
    program_code VARCHAR(3) NOT NULL CHECK (program_code ~ '^[0-9]{3}$'),
    school_url TEXT NOT NULL DEFAULT '',
    school_source_url TEXT NOT NULL DEFAULT '',
    department_url TEXT NOT NULL DEFAULT '',
    department_source_url TEXT NOT NULL DEFAULT '',
    collego_url TEXT NOT NULL DEFAULT '',
    collego_source_url TEXT NOT NULL DEFAULT '',
    collego_match_status VARCHAR(32) NOT NULL DEFAULT ''
        CHECK (collego_match_status IN ('', 'exact_normalized_name', 'needs_manual_review', 'ambiguous_candidates', 'unmatched')),
    status VARCHAR(16) NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'published', 'rejected')),
    updated_by UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    reviewed_by UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    review_reason TEXT NOT NULL DEFAULT '',
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (school_code, program_code)
);

CREATE INDEX admissions_program_link_candidates_status_idx
    ON admissions_program_link_candidates (status, updated_at DESC);

COMMENT ON TABLE admissions_school_official_links IS
    'Published school website URLs; candidate data is kept separate until an administrator publishes it.';
COMMENT ON TABLE admissions_program_official_links IS
    'Published department and ColleGo! URLs; candidate data is kept separate until an administrator publishes it.';
COMMENT ON TABLE admissions_program_link_candidates IS
    'Unpublished school, department, and ColleGo! URL candidates awaiting administrator review.';
