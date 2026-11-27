ALTER TABLE admissions_contributions
    ADD COLUMN evidence_required BOOLEAN NOT NULL DEFAULT TRUE;

-- Existing contributions were collected before evidence was required. Keep
-- them reviewable while the default applies the requirement to new rows.
UPDATE admissions_contributions
SET evidence_required = FALSE;

COMMENT ON COLUMN admissions_contributions.evidence_required IS
    'Whether this contribution is subject to the required evidence rule introduced in migration 000091.';

ALTER TABLE admissions_contribution_attachments
    ADD COLUMN purpose TEXT NOT NULL DEFAULT 'legacy'
        CHECK (purpose IN ('data', 'evidence', 'brochure', 'legacy'));

COMMENT ON COLUMN admissions_contribution_attachments.purpose IS
    'Classifies new contribution attachments as submitted data, supporting evidence, brochure, or legacy unclassified file.';

CREATE FUNCTION enforce_admissions_contribution_evidence()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    target_contribution_id UUID;
    contribution_ids UUID[];
    contribution_category TEXT;
    contribution_evidence_required BOOLEAN;
BEGIN
    IF TG_TABLE_NAME = 'admissions_contributions' THEN
        IF TG_OP = 'UPDATE' THEN
            IF NEW.category = OLD.category THEN
                RETURN NULL;
            END IF;
        END IF;
        contribution_ids := ARRAY[NEW.id];
    ELSIF TG_OP = 'DELETE' THEN
        contribution_ids := ARRAY[OLD.contribution_id];
    ELSIF TG_OP = 'UPDATE' AND OLD.contribution_id <> NEW.contribution_id THEN
        contribution_ids := ARRAY[OLD.contribution_id, NEW.contribution_id];
    ELSE
        contribution_ids := ARRAY[NEW.contribution_id];
    END IF;

    FOREACH target_contribution_id IN ARRAY contribution_ids LOOP
        SELECT category, evidence_required
        INTO contribution_category, contribution_evidence_required
        FROM admissions_contributions
        WHERE id = target_contribution_id;

        IF FOUND
            AND contribution_evidence_required
            AND contribution_category IN ('applicants', 'second_stage', 'waitlist')
            AND NOT EXISTS (
                SELECT 1
                FROM admissions_contribution_attachments
                WHERE admissions_contribution_attachments.contribution_id = target_contribution_id
                  AND purpose = 'evidence'
            )
        THEN
            RAISE EXCEPTION 'count contributions require at least one evidence attachment'
                USING ERRCODE = '23514',
                      CONSTRAINT = 'admissions_contributions_evidence_required';
        END IF;
    END LOOP;

    RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER admissions_contributions_evidence_required
    AFTER INSERT OR UPDATE ON admissions_contributions
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW
    EXECUTE FUNCTION enforce_admissions_contribution_evidence();

CREATE CONSTRAINT TRIGGER admissions_contribution_attachments_evidence_required
    AFTER INSERT OR UPDATE OR DELETE ON admissions_contribution_attachments
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW
    EXECUTE FUNCTION enforce_admissions_contribution_evidence();
