-- STA Phase 9: cross-check willingness values live on the academic program.
-- The array contains only numeric values. Candidate numbers, applications and
-- inquiry identities remain in the official-result/inquiry tables.

ALTER TABLE academic_programs
ADD COLUMN willingness_values SMALLINT[] NOT NULL DEFAULT '{}'::smallint[];

ALTER TABLE academic_programs
ADD CONSTRAINT academic_programs_willingness_values_valid
CHECK (
    array_position(willingness_values, NULL::smallint) IS NULL
    AND willingness_values <@ ARRAY[
        0::smallint, 20::smallint, 40::smallint,
        60::smallint, 80::smallint, 100::smallint
    ]::smallint[]
);

ALTER TABLE official_results
ADD COLUMN willingness_index INTEGER;

ALTER TABLE official_results
ADD CONSTRAINT official_results_willingness_index_valid
CHECK (willingness_index IS NULL OR willingness_index >= 0);

CREATE UNIQUE INDEX official_results_batch_program_willingness_index_idx
ON official_results (batch_id, academic_year, school_code, program_code, willingness_index)
WHERE willingness_index IS NOT NULL;

-- Backfill the currently published data when this migration is applied to an
-- existing database. New publications run the same steps in the repository.
UPDATE official_results
SET willingness_index = NULL
WHERE batch_id IN (SELECT id FROM official_result_batches WHERE status = 'published');

WITH ordered AS (
    SELECT r.id,
           ((row_number() OVER (
                PARTITION BY r.batch_id, r.academic_year, r.school_code, r.program_code
                ORDER BY CASE WHEN r.result_status = 'admitted' THEN 0 ELSE 1 END,
                         r.official_rank, r.id
           ) - 1)::int) AS willingness_index
    FROM official_results r
    JOIN official_result_batches b ON b.id = r.batch_id AND b.status = 'published'
    WHERE r.result_status IN ('admitted', 'waitlisted')
)
UPDATE official_results r
SET willingness_index = ordered.willingness_index
FROM ordered
WHERE r.id = ordered.id;

UPDATE academic_programs p
SET willingness_values = COALESCE(
    (
        SELECT array_agg(COALESCE(w.value, 100::smallint) ORDER BY r.willingness_index)
        FROM official_results r
        JOIN official_result_batches b ON b.id = r.batch_id AND b.status = 'published'
        LEFT JOIN application_willingness w ON w.application_id = r.application_id
        WHERE r.academic_year = p.academic_year
          AND r.school_code = p.school_code
          AND r.program_code = p.program_code
          AND r.willingness_index IS NOT NULL
    ),
    ARRAY[]::smallint[]
),
updated_at = CURRENT_TIMESTAMP
WHERE EXISTS (
    SELECT 1
    FROM official_result_batches b
    WHERE b.status = 'published'
      AND b.academic_year = p.academic_year
      AND b.school_code = p.school_code
);

-- A willingness inquiry is tied to the exact official row and result batch.
-- The old application/round uniqueness rule cannot distinguish a replacement
-- official list for the same application, so it is replaced by result-row
-- uniqueness. Existing rows are backfilled to the latest published row when
-- possible and remain compatible if an old row cannot be matched.
ALTER TABLE willingness_inquiries
ADD COLUMN official_result_id UUID REFERENCES official_results(id) ON DELETE RESTRICT,
ADD COLUMN result_batch_id UUID REFERENCES official_result_batches(id) ON DELETE RESTRICT;

WITH matched AS (
    SELECT i.id AS inquiry_id, latest.result_id, latest.batch_id
    FROM willingness_inquiries i
    JOIN LATERAL (
        SELECT r.id AS result_id, r.batch_id
        FROM official_results r
        JOIN official_result_batches b ON b.id = r.batch_id
        WHERE r.application_id = i.application_id
          AND b.status = 'published'
        ORDER BY b.imported_at DESC, r.updated_at DESC, r.id DESC
        LIMIT 1
    ) latest ON TRUE
    WHERE i.official_result_id IS NULL
)
UPDATE willingness_inquiries i
SET official_result_id = matched.result_id,
    result_batch_id = matched.batch_id
FROM matched
WHERE i.id = matched.inquiry_id;

ALTER TABLE willingness_inquiries
DROP CONSTRAINT IF EXISTS willingness_inquiries_application_id_inquiry_round_key;

ALTER TABLE willingness_inquiries
ADD CONSTRAINT willingness_inquiries_result_round_key
UNIQUE (official_result_id, inquiry_round);

CREATE UNIQUE INDEX willingness_inquiries_application_batch_round_unique_idx
ON willingness_inquiries (application_id, result_batch_id, inquiry_round)
WHERE application_id IS NOT NULL AND result_batch_id IS NOT NULL;

ALTER TABLE willingness_response_events
ADD COLUMN official_result_id UUID REFERENCES official_results(id) ON DELETE RESTRICT;

UPDATE willingness_response_events e
SET official_result_id = i.official_result_id
FROM willingness_inquiries i
WHERE e.inquiry_id = i.id
  AND e.official_result_id IS NULL;

CREATE INDEX willingness_response_events_result_idx
ON willingness_response_events (official_result_id, created_at DESC)
WHERE official_result_id IS NOT NULL;

COMMENT ON COLUMN academic_programs.willingness_values IS
    '純數值意願陣列；索引由目前發布的官方錄取名冊 willingness_index 對應。';
COMMENT ON COLUMN official_results.willingness_index IS
    '目前發布名冊中的 0-based 意願陣列位置；正取先於備取排序。';
COMMENT ON COLUMN willingness_inquiries.official_result_id IS
    '詢問對應的官方錄取名冊資料列；application_id 只作暫時比對與授權。';
