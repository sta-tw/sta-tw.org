-- Current applicants identify their high school until the admissions-cycle
-- rollover records actual university outcomes.
ALTER TABLE verification_requests
    ADD COLUMN school_directory_id UUID REFERENCES account_school_directory(id) ON DELETE RESTRICT;

ALTER TABLE verification_requests
    ALTER COLUMN school_code DROP NOT NULL,
    ADD CONSTRAINT verification_requests_one_school_source CHECK (
        (school_code IS NOT NULL AND school_directory_id IS NULL)
        OR (school_code IS NULL AND school_directory_id IS NOT NULL)
    ),
    ADD CONSTRAINT verification_requests_directory_school_type CHECK (
        school_directory_id IS NULL
        OR (identity_type = 'student' AND method = 'document' AND program_code IS NULL)
    );

ALTER TABLE student_verifications
    ADD COLUMN school_directory_id UUID REFERENCES account_school_directory(id) ON DELETE RESTRICT;

ALTER TABLE student_verifications
    ALTER COLUMN school_code DROP NOT NULL,
    ADD CONSTRAINT student_verifications_one_school_source CHECK (
        (school_code IS NOT NULL AND school_directory_id IS NULL)
        OR (school_code IS NULL AND school_directory_id IS NOT NULL)
    ),
    ADD CONSTRAINT student_verifications_directory_school_no_program CHECK (
        school_directory_id IS NULL OR program_code IS NULL
    );

DROP INDEX student_verifications_identity_key;
CREATE UNIQUE INDEX student_verifications_identity_key
    ON student_verifications (
        account_id,
        academic_year,
        COALESCE(school_code, '-'),
        COALESCE(program_code, '-'),
        COALESCE(school_directory_id::text, '-')
    );

COMMENT ON COLUMN verification_requests.school_directory_id IS
    'Selected senior-high-school directory entry for current student identity requests.';
COMMENT ON COLUMN student_verifications.school_directory_id IS
    'Senior-high-school directory entry for current student identity verifications.';
