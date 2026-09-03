-- STA Phase 10: forum scope uniqueness and the public global forum.
-- NULL values in a composite UNIQUE constraint do not prevent duplicate
-- global/annual rows in PostgreSQL, so each scope gets an explicit partial
-- unique index.

CREATE UNIQUE INDEX forum_spaces_global_unique_idx
ON forum_spaces (space_type)
WHERE space_type = 'global';

CREATE UNIQUE INDEX forum_spaces_annual_unique_idx
ON forum_spaces (academic_year)
WHERE space_type = 'annual';

CREATE UNIQUE INDEX forum_spaces_school_program_unique_idx
ON forum_spaces (academic_year, school_code, program_code)
WHERE space_type = 'school_program';

INSERT INTO forum_spaces (space_type, display_name)
VALUES ('global', '全域論壇')
ON CONFLICT DO NOTHING;
