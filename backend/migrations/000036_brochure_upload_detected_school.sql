-- Detected school codes are OCR suggestions and may be unknown until an
-- administrator corrects them during review. Existing installations created
-- with 000035 must therefore drop the optional foreign key as well.

ALTER TABLE brochure_uploads
DROP CONSTRAINT IF EXISTS brochure_uploads_detected_school_code_fkey;
