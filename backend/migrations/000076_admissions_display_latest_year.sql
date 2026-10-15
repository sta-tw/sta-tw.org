-- Keep the admissions catalogue on its current newest published year until
-- an administrator explicitly advances the display year.
INSERT INTO app_settings (key, value)
SELECT 'admissions_display_latest_year', MAX(academic_year)::text
FROM academic_programs
WHERE review_status = 'published'
  AND admission_quota > 0
HAVING MAX(academic_year) IS NOT NULL
ON CONFLICT (key) DO NOTHING;
