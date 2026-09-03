-- Prevent an ambiguous official-result match when two confirmed applications
-- enter the same candidate number for the same academic program.

CREATE UNIQUE INDEX applications_confirmed_candidate_lookup_unique_idx
ON applications (academic_year, school_code, program_code, candidate_number_lookup_hash)
WHERE status = 'confirmed' AND candidate_number_lookup_hash IS NOT NULL;

COMMENT ON INDEX applications_confirmed_candidate_lookup_unique_idx IS
    '同年度同校系的已確認申請不得共用同一准考證 lookup hash，避免查榜結果任意匹配帳號。';
