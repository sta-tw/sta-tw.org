-- Expand identity verification roles, document types, and revocation status.

ALTER TABLE accounts
    DROP CONSTRAINT accounts_identity_status_check,
    ADD CONSTRAINT accounts_identity_status_check
        CHECK (identity_status IN ('temporary', 'student', 'senior', 'parent', 'educator'));

ALTER TABLE verification_requests
    DROP CONSTRAINT verification_requests_identity_type_check,
    ADD CONSTRAINT verification_requests_identity_type_check
        CHECK (identity_type IN ('student', 'senior', 'parent', 'educator')),
    DROP CONSTRAINT verification_requests_document_type_check,
    ADD CONSTRAINT verification_requests_document_type_check
        CHECK (document_type IN (
            'student_id', 'admission_letter', 'alumni_proof',
            'parent_relationship', 'educator_credential', 'other'
        )),
    DROP CONSTRAINT verification_requests_status_check,
    ADD CONSTRAINT verification_requests_status_check
        CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled', 'expired', 'revoked'));

COMMENT ON COLUMN accounts.identity_status IS 'Account identity: temporary, student, senior, parent, or educator.';
