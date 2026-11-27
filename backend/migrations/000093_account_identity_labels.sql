-- Keep school identity verification separate from the account's identity category.
ALTER TABLE accounts
    ADD COLUMN school_identity_verified boolean NOT NULL DEFAULT false,
    ADD COLUMN identity_type varchar(16);

ALTER TABLE accounts
    ADD CONSTRAINT accounts_identity_type_check
        CHECK (identity_type IS NULL OR identity_type IN ('student', 'senior', 'parent', 'educator')),
    ADD CONSTRAINT accounts_identity_type_requires_verification_check
        CHECK (identity_type IS NULL OR school_identity_verified);

COMMENT ON COLUMN accounts.school_identity_verified IS 'Whether school identity verification has succeeded; independent from identity_type.';
COMMENT ON COLUMN accounts.identity_type IS 'Optional identity category: student, senior, parent, or educator.';
