-- Keep the discovery population stable across academic years. The school
-- master can continue evolving for the rest of STA, but brochure discovery
-- always uses this explicitly seeded roster (149 schools in the initial data).

CREATE TABLE brochure_discovery_school_roster (
    school_code VARCHAR(3) PRIMARY KEY REFERENCES schools(school_code) ON DELETE RESTRICT,
    included_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO brochure_discovery_school_roster (school_code)
SELECT school_code FROM schools
ON CONFLICT (school_code) DO NOTHING;

COMMENT ON TABLE brochure_discovery_school_roster IS
'簡章探索固定學校名冊；新學年度只複製此名冊，不跟隨學校主檔臨時增減。';
