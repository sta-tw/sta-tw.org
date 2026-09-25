-- The fixed date columns on academic_programs (registration_*, exam_*,
-- result_date, brochure_announcement_date) only cover a handful of generic
-- milestones. Real brochures publish a much longer, school-specific
-- schedule (網路登錄報名, 上網查詢報名狀態, 初試結果公告, 複試繳費期限,
-- 榜單公告, 正備取生報到, 備取遞補截止, ...) that doesn't fit a small set
-- of named columns without either losing detail or forcing every school
-- into the same vocabulary. This table stores that schedule as an ordered
-- list per program instead, mirroring the existing program_exam_items
-- pattern.
CREATE TABLE program_timeline_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    academic_year SMALLINT NOT NULL,
    school_code VARCHAR(3) NOT NULL,
    program_code VARCHAR(3) NOT NULL,
    event_name TEXT NOT NULL,
    -- Nullable: some milestones ("上網查詢報名狀態" etc.) are only ever
    -- given as a loose description in the brochure, not a hard date.
    event_date DATE,
    -- Free text for the part a plain DATE can't hold: a time-of-day, a
    -- half-open range ("9:00~17:00"), or "至...止" wording straight from
    -- the brochure. "-" when the brochure gives nothing beyond the date.
    event_time TEXT NOT NULL DEFAULT '-',
    sort_order SMALLINT NOT NULL CHECK (sort_order > 0),
    notes TEXT NOT NULL DEFAULT '-',
    source_page SMALLINT CHECK (source_page BETWEEN 1 AND 999),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT program_timeline_events_program_fk
        FOREIGN KEY (academic_year, school_code, program_code)
        REFERENCES academic_programs(academic_year, school_code, program_code)
        ON DELETE RESTRICT,
    CONSTRAINT program_timeline_events_name_not_blank CHECK (length(btrim(event_name)) > 0),
    UNIQUE (academic_year, school_code, program_code, sort_order)
);

CREATE INDEX program_timeline_events_program_idx
ON program_timeline_events (academic_year, school_code, program_code);

CREATE TRIGGER program_timeline_events_set_updated_at
BEFORE UPDATE ON program_timeline_events
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();
