ALTER TABLE applications
    ADD COLUMN candidate_name_ciphertext BYTEA;

ALTER TABLE applications
    ADD CONSTRAINT applications_candidate_name_requires_number CHECK (
        candidate_name_ciphertext IS NULL OR candidate_number_ciphertext IS NOT NULL
    );

COMMENT ON COLUMN applications.candidate_name_ciphertext IS
    '考生姓名的欄位加密值；不回傳 API，也不得寫入稽核紀錄。';
