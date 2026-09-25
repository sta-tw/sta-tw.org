ALTER TABLE academic_programs
    ADD COLUMN school_official_url TEXT NOT NULL DEFAULT '-',
    ADD COLUMN department_official_url TEXT NOT NULL DEFAULT '-';

COMMENT ON COLUMN academic_programs.school_official_url IS '學校官網（非簡章連結）；未提供時為 -。';
COMMENT ON COLUMN academic_programs.department_official_url IS '學系官網（非簡章連結）；未提供時為 -。';
