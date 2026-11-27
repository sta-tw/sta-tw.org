ALTER TABLE admissions_program_link_candidates
    ADD COLUMN unpublished BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX admissions_program_link_candidates_unpublished_idx
    ON admissions_program_link_candidates (unpublished, updated_at DESC)
    WHERE unpublished = TRUE;
