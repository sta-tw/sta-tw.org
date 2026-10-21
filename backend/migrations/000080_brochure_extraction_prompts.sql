CREATE TABLE brochure_extraction_prompts (
    school_code VARCHAR(3) PRIMARY KEY REFERENCES schools(school_code) ON DELETE RESTRICT,
    prompt TEXT NOT NULL,
    updated_by UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT brochure_extraction_prompts_school_code_format CHECK (school_code ~ '^[0-9]{3}$'),
    CONSTRAINT brochure_extraction_prompts_prompt_length CHECK (
        length(btrim(prompt)) > 0 AND length(prompt) <= 20000
    )
);

CREATE TRIGGER brochure_extraction_prompts_set_updated_at
BEFORE UPDATE ON brochure_extraction_prompts
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

COMMENT ON TABLE brochure_extraction_prompts IS '各校簡章版型的 Workers AI 擷取補充提示詞；校名辨識仍由本機規則處理。';
