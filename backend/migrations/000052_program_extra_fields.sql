ALTER TABLE academic_programs
    ADD COLUMN registration_fee TEXT NOT NULL DEFAULT '-',
    ADD COLUMN exam_location TEXT NOT NULL DEFAULT '-',
    ADD COLUMN recommendation_letter_deadline DATE,
    ADD COLUMN portfolio_deadline DATE,
    ADD COLUMN checkin_waitlist_process TEXT NOT NULL DEFAULT '-',
    ADD COLUMN fee_reduction_eligibility TEXT NOT NULL DEFAULT '-';

COMMENT ON COLUMN academic_programs.registration_fee IS '報名費，含金額或減免說明摘要；未提供時為 -。';
COMMENT ON COLUMN academic_programs.exam_location IS '甄試/面試地點，與日期分開儲存；未提供時為 -。';
COMMENT ON COLUMN academic_programs.recommendation_letter_deadline IS '推薦函截止日期；NULL 代表不需要推薦函，有值代表需要且該日期為截止期限。';
COMMENT ON COLUMN academic_programs.portfolio_deadline IS '作品集上傳截止日期；NULL 代表不需要作品集，有值代表需要且該日期為截止期限。';
COMMENT ON COLUMN academic_programs.checkin_waitlist_process IS '報到與備取遞補流程及各項期限的摘要文字；未提供時為 -。';
COMMENT ON COLUMN academic_programs.fee_reduction_eligibility IS '報名費減免資格說明；未提供時為 -。';
