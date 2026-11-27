ALTER TABLE academic_programs
    ADD COLUMN IF NOT EXISTS admission_quota_display TEXT NOT NULL DEFAULT '';
