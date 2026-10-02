-- Public downloads serve a watermarked copy built once at publish time
-- (hidden OCG layer, see internal/admissions/pdfwatermark.go), separate
-- from the admin-facing original at storage_key. NULL means the brochure
-- was published before this existed, or the one-time watermarking step
-- failed; callers fall back to storage_key in that case.
ALTER TABLE brochure_documents
    ADD COLUMN watermarked_storage_key TEXT;
