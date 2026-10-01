-- ListPrograms (internal/admissions/postgres_repository.go) does a
-- ROW_NUMBER() OVER (PARTITION BY school_code, program_code ORDER BY
-- academic_year DESC) to collapse the catalogue down to each program's
-- latest year — the existing primary key is (academic_year, school_code,
-- program_code), which orders by the WRONG column first for this access
-- pattern, forcing an explicit Sort of every matching row before the window
-- function can run. This index matches the partition/order exactly, turning
-- that Sort into an index-order scan.
CREATE INDEX academic_programs_dedupe_idx
    ON academic_programs (school_code, program_code, academic_year DESC);

-- The same query's free-text search does
-- REPLACE(school_name, '臺', '台') ILIKE '%term%' (and the equivalent on
-- admission_program_name) so a 台/臺 variant always matches regardless of
-- which one was typed — a plain index on either column can't serve that
-- (leading wildcard, and the column is wrapped in an expression), so every
-- search term currently forces a sequential scan. These expression trigram
-- indexes match the exact expression the query already uses, and support
-- ILIKE '%...%' instead of just prefix matches.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX academic_programs_name_trgm_idx
    ON academic_programs USING gin ((REPLACE(admission_program_name, '臺', '台')) gin_trgm_ops);

CREATE INDEX schools_name_trgm_idx
    ON schools USING gin ((REPLACE(school_name, '臺', '台')) gin_trgm_ops);
