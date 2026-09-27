-- STA: 歷年招生資料 needs 備取人數 (waitlisted_count, the roster size) and
-- 最終遞補人數 (how many actually got promoted off that list) as two
-- separate numbers; only the former existed.

ALTER TABLE academic_programs
    ADD COLUMN promoted_count TEXT NOT NULL DEFAULT '-';

COMMENT ON COLUMN academic_programs.promoted_count IS '最終遞補人數（實際從備取名單遞補上的人數）；未提供時為 -。';
