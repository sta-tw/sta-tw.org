ALTER TABLE academic_programs
    ADD COLUMN admission_group TEXT NOT NULL DEFAULT '-',
    ADD COLUMN cross_group TEXT NOT NULL DEFAULT '-',
    ADD COLUMN admission_category TEXT NOT NULL DEFAULT '-',
    ADD COLUMN priority_admission TEXT NOT NULL DEFAULT '-',
    ADD COLUMN portfolio_required TEXT NOT NULL DEFAULT '-',
    ADD COLUMN recommendation_letter_type TEXT NOT NULL DEFAULT '-',
    ADD COLUMN max_applicable_programs TEXT NOT NULL DEFAULT '-',
    ADD COLUMN applicant_count TEXT NOT NULL DEFAULT '-',
    ADD COLUMN interview_count TEXT NOT NULL DEFAULT '-',
    ADD COLUMN admitted_count TEXT NOT NULL DEFAULT '-',
    ADD COLUMN waitlisted_count TEXT NOT NULL DEFAULT '-',
    ADD COLUMN admission_rate TEXT NOT NULL DEFAULT '-',
    ADD COLUMN first_stage_pass_rate TEXT NOT NULL DEFAULT '-',
    ADD COLUMN competition_ratio TEXT NOT NULL DEFAULT '-';

COMMENT ON COLUMN academic_programs.admission_group IS '學群（依大表分類）；未提供時為 -。';
COMMENT ON COLUMN academic_programs.cross_group IS '跨學群；未提供時為 -。';
COMMENT ON COLUMN academic_programs.admission_category IS '學類；未提供時為 -。';
COMMENT ON COLUMN academic_programs.priority_admission IS '是否優先錄取（含適用對象說明）；未提供時為 -。';
COMMENT ON COLUMN academic_programs.portfolio_required IS '是否需要作品集（Y/N 或說明文字）；未提供時為 -。';
COMMENT ON COLUMN academic_programs.recommendation_letter_type IS '推薦函形式：暗函/明函/不需要等；未提供時為 -。';
COMMENT ON COLUMN academic_programs.max_applicable_programs IS '可報名學系數量限制；未提供時為 -。';
COMMENT ON COLUMN academic_programs.applicant_count IS '報名人數（招生進度統計，公告前為「尚未公告」）；未提供時為 -。';
COMMENT ON COLUMN academic_programs.interview_count IS '面試人數；未提供時為 -。';
COMMENT ON COLUMN academic_programs.admitted_count IS '正取人數；未提供時為 -。';
COMMENT ON COLUMN academic_programs.waitlisted_count IS '備取人數；未提供時為 -。';
COMMENT ON COLUMN academic_programs.admission_rate IS '錄取率；未提供時為 -。';
COMMENT ON COLUMN academic_programs.first_stage_pass_rate IS '初試通過率；未提供時為 -。';
COMMENT ON COLUMN academic_programs.competition_ratio IS '競爭倍率；未提供時為 -。';
