-- STA: give the program a dedicated 聯絡人 field, split out of consultation_phone.
--
-- consultation_phone now keeps only the number(s) and extension (multiple lines
-- separated by 「；」). consultation_contact is free text — it may name a person
-- (「吳助教」) or a unit (「語文與創作學系系辦」); the extractor does not force a
-- distinction and the reviewer edits it like any other field.

ALTER TABLE academic_programs
    ADD COLUMN consultation_contact TEXT NOT NULL DEFAULT '-';

COMMENT ON COLUMN academic_programs.consultation_contact IS '諮詢聯絡人或單位（自由文字）；與電話、信箱分開儲存，未提供時為 -。';
