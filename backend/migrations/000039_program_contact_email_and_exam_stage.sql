-- STA: split the program contact e-mail from the phone field, and record which
-- exam round (初試／複試／第一階段…) each exam item belongs to.
--
-- Brochures print a phone (with extension and contact person) and an e-mail
-- together; storing them apart lets the frontend render and validate each. The
-- exam-item list stays flat and the item names stay whatever the brochure uses
-- (書審／面試／筆試…), so the stage is an optional label, not a fixed grouping.

ALTER TABLE academic_programs
    ADD COLUMN consultation_email TEXT NOT NULL DEFAULT '-';

ALTER TABLE program_exam_items
    ADD COLUMN exam_stage TEXT NOT NULL DEFAULT '-';

COMMENT ON COLUMN academic_programs.consultation_email IS '諮詢信箱；與 consultation_phone 分開儲存，未提供時為 -。';
COMMENT ON COLUMN program_exam_items.exam_stage IS '該考試項目所屬階段（初試／複試／第一階段…）；未標示時為 -。';
