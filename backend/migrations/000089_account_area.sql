-- Persistent account-area settings and profile/content metadata.

ALTER TABLE account_profiles
    ADD COLUMN school_name TEXT NOT NULL DEFAULT '',
    ADD COLUMN department TEXT NOT NULL DEFAULT '',
    ADD COLUMN hide_school BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN hide_department BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN profile_slug VARCHAR(30),
    ADD COLUMN profile_visibility VARCHAR(16) NOT NULL DEFAULT 'public'
        CHECK (profile_visibility IN ('public', 'members')),
    ADD COLUMN show_articles BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN show_portfolio BOOLEAN NOT NULL DEFAULT TRUE,
    ADD CONSTRAINT account_profiles_school_name_len CHECK (length(school_name) <= 120),
    ADD CONSTRAINT account_profiles_department_len CHECK (length(department) <= 120),
    ADD CONSTRAINT account_profiles_slug_format CHECK (
        profile_slug IS NULL OR profile_slug ~ '^[a-z0-9_-]{3,30}$'
    );

CREATE UNIQUE INDEX account_profiles_slug_unique
    ON account_profiles (lower(profile_slug)) WHERE profile_slug IS NOT NULL;

CREATE TABLE saved_programs (
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    academic_year SMALLINT NOT NULL,
    school_code VARCHAR(3) NOT NULL,
    program_code VARCHAR(3) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, academic_year, school_code, program_code),
    CONSTRAINT saved_programs_program_fk FOREIGN KEY (academic_year, school_code, program_code)
        REFERENCES academic_programs (academic_year, school_code, program_code) ON DELETE CASCADE
);

CREATE INDEX saved_programs_account_created_idx
    ON saved_programs (account_id, created_at DESC);

CREATE TABLE notification_preferences (
    account_id UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    email_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    kinds JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(kinds) = 'object'),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TRIGGER notification_preferences_set_updated_at
BEFORE UPDATE ON notification_preferences
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE verification_requests
    ADD COLUMN identity_type VARCHAR(16) NOT NULL DEFAULT 'student'
        CHECK (identity_type IN ('student', 'senior')),
    ADD COLUMN document_type VARCHAR(24) NOT NULL DEFAULT 'other'
        CHECK (document_type IN ('student_id', 'admission_letter', 'other')),
    ADD COLUMN academic_fields JSONB NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(academic_fields) = 'array');

CREATE TABLE experience_revision_tags (
    revision_id UUID NOT NULL REFERENCES experience_revisions(id) ON DELETE CASCADE,
    tag TEXT NOT NULL CHECK (length(tag) BETWEEN 1 AND 32 AND tag = lower(btrim(tag))),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (revision_id, tag)
);

CREATE INDEX experience_revision_tags_tag_idx ON experience_revision_tags (tag, revision_id);
