package admissions

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var brochureSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// normalizeTaiVariant folds 臺 onto 台 so search terms match either variant
// of a name regardless of which one the caller typed.
func normalizeTaiVariant(value string) string {
	return strings.ReplaceAll(value, "臺", "台")
}

// schoolAbbreviations covers common short forms of a school's name that
// AREN'T a contiguous substring of the official name (so ILIKE '%term%'
// alone can't find them) — e.g. "交大" doesn't literally appear in "國立陽
// 明交通大學" (the sequence there is 交-通-大, not 交-大). A name like "中興
// 大學", where the short form IS already a contiguous substring (中興 or
// 興大), needs no entry here. Kept in sync with the equivalent Meilisearch
// synonym list in internal/search/meili.go, which serves the separate
// /api/v1/search endpoint.
var schoolAbbreviations = map[string][]string{
	"政大": {"政治大學"}, "清大": {"清華大學"}, "台大": {"台灣大學"},
	"師大": {"台灣師範大學"}, "成大": {"成功大學"},
	"交大": {"陽明交通大學"}, "陽明交大": {"陽明交通大學"},
	"中大": {"中央大學", "中山大學", "中原大學"},
	"海大": {"台灣海洋大學"}, "高師大": {"高雄師範大學"}, "彰師大": {"彰化師範大學"},
	"嘉大": {"嘉義大學"}, "高大": {"高雄大學"}, "暨大": {"暨南國際大學"},
	"北藝大": {"台北藝術大學"}, "台藝大": {"台灣藝術大學"},
	"宜大": {"宜蘭大學"}, "南大": {"台南大學"},
	"北教大": {"台北教育大學"}, "中教大": {"台中教育大學"}, "屏大": {"屏東大學"},
	"台科大": {"台灣科技大學"}, "雲科大": {"雲林科技大學"}, "屏科大": {"屏東科技大學"},
	"北科大": {"台北科技大學"}, "虎科大": {"虎尾科技大學"},
	"高餐大": {"高雄餐旅大學"}, "高餐": {"高雄餐旅大學"},
	"台中科大": {"台中科技大學"}, "中科大": {"台中科技大學"},
	"北商大": {"台北商業大學"}, "高科大": {"高雄科技大學"},
	"輔大": {"輔仁大學"}, "淡大": {"淡江大學"},
	"高醫": {"高雄醫學大學"}, "高醫大": {"高雄醫學大學"},
	"北醫": {"台北醫學大學"}, "北醫大": {"台北醫學大學"},
	"亞大": {"亞洲大學"},
}

// departmentAbbreviations covers common short forms of a department name
// that aren't a contiguous substring of the full name (e.g. "資工" doesn't
// appear in "資訊工程學系" — the sequence there is 資-訊-工, not 資-工). Used
// together with departmentSuffixStripped so "資工系" / "資訊工程系" /
// "資訊工程學系" all resolve to the same "資訊工程" search candidate.
var departmentAbbreviations = map[string][]string{
	"資工": {"資訊工程"}, "資管": {"資訊管理"}, "電機": {"電機工程"},
	"機械": {"機械工程"}, "企管": {"企業管理"}, "財金": {"財務金融"},
	"會資": {"會計資訊"}, "外文": {"外國語文"}, "應外": {"應用外語"},
	"資傳": {"資訊傳播"}, "土木": {"土木工程"}, "化工": {"化學工程"},
	"環工": {"環境工程"}, "生科": {"生命科學"}, "生技": {"生物技術"},
	"餐旅": {"餐旅管理"}, "休運": {"休閒運動"}, "特教": {"特殊教育"},
	"幼教": {"幼兒教育"}, "社工": {"社會工作"}, "地政": {"地政學"},
	"心輔": {"心理輔導"}, "統計": {"統計學"}, "數學": {"數學系"},
	"物理": {"物理學"}, "化學": {"化學系"}, "光電": {"光電工程"},
	"通訊": {"通訊工程"}, "材料": {"材料科學"}, "工管": {"工業管理"},
	"國企": {"國際企業"}, "運管": {"運輸管理"}, "森林": {"森林學"},
}

// departmentSuffixStripped drops a trailing department-name suffix ("學系"
// checked before "系" so it isn't left with a stray "學") — "資訊工程學系"
// and "資訊工程系" both strip down to "資訊工程", so either form of a
// query matches either form of a stored name via plain substring ILIKE.
func departmentSuffixStripped(term string) (string, bool) {
	for _, suffix := range []string{"學系", "系所", "系"} {
		if stripped, ok := strings.CutSuffix(term, suffix); ok && stripped != "" {
			return stripped, true
		}
	}
	return "", false
}

// searchTermCandidates expands one whitespace-split search term into every
// string it should be ILIKE-matched against: itself, plus any school full
// name or department full name it's a known/derivable short form of. This
// is deliberately a curated normalization (suffix-stripping + a fixed
// abbreviation dictionary), not general fuzzy/typo matching — it lets
// "資工系"/"資訊工程系"/"資訊工程學系" all match each other without also
// matching on loose similarity the way trigram distance would.
func searchTermCandidates(term string) []string {
	normalized := normalizeTaiVariant(term)
	candidates := []string{normalized}
	if expansions, ok := schoolAbbreviations[normalized]; ok {
		candidates = append(candidates, expansions...)
	}
	stripped := normalized
	if s, ok := departmentSuffixStripped(normalized); ok {
		stripped = s
		candidates = append(candidates, stripped)
	}
	if expansions, ok := departmentAbbreviations[stripped]; ok {
		candidates = append(candidates, expansions...)
	}
	return candidates
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) (*PostgresRepository, error) {
	if pool == nil {
		return nil, errors.New("postgres pool is nil")
	}
	return &PostgresRepository{pool: pool}, nil
}

// ListPrograms requires admission_quota > 0 to hide empty placeholder slots
// from the public list; the admin list has no such filter.
func (r *PostgresRepository) ListPrograms(ctx context.Context, query ProgramQuery) ([]Program, error) {
	if query.Limit < 1 || query.Limit > 100 {
		query.Limit = 50
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	conditions := []string{"TRUE"}
	args := make([]any, 0, 5)
	if query.AcademicYear > 0 {
		args = append(args, query.AcademicYear)
		conditions = append(conditions, fmt.Sprintf("p.academic_year = $%d", len(args)))
	}
	if query.SchoolCode != "" {
		args = append(args, query.SchoolCode)
		conditions = append(conditions, fmt.Sprintf("p.school_code = $%d", len(args)))
	}
	if query.ProgramCode != "" {
		args = append(args, query.ProgramCode)
		conditions = append(conditions, fmt.Sprintf("p.program_code = $%d", len(args)))
	}
	if query.Search != "" {
		// Each whitespace-separated term must match somewhere (school name or
		// program name) — this lets a query like "淡江 經濟" require the
		// school AND the department to both be satisfied, even though they
		// live in different columns. Within a term, an abbreviation (e.g.
		// "交大") also matches the school's full name.
		for _, term := range strings.Fields(query.Search) {
			var orParts []string
			for _, candidate := range searchTermCandidates(term) {
				args = append(args, "%"+candidate+"%")
				orParts = append(orParts, fmt.Sprintf("REPLACE(s.school_name, '臺', '台') ILIKE $%d", len(args)))
				args = append(args, "%"+candidate+"%")
				orParts = append(orParts, fmt.Sprintf("REPLACE(p.admission_program_name, '臺', '台') ILIKE $%d", len(args)))
			}
			conditions = append(conditions, "("+strings.Join(orParts, " OR ")+")")
		}
	}
	// Catalogue-wide reads (no explicit academic_year, no single-program
	// lookup) collapse every school+program pair down to its newest
	// published academic_year — that's the only row anyone browsing/
	// searching the site should see. A specific academic_year, or a
	// school_code+program_code pair (getAdmissionProgramHistory, the 歷年
	// tab), still gets every matching row.
	dedupeToLatestYear := query.AcademicYear <= 0 && query.ProgramCode == ""
	args = append(args, dedupeToLatestYear, query.Limit, query.Offset)
	dedupePosition, limitPosition, offsetPosition := len(args)-2, len(args)-1, len(args)
	statement := fmt.Sprintf(`
		WITH ranked AS (
			SELECT
				p.academic_year, p.program_identifier, p.school_code, s.school_name,
				p.program_code, p.admission_program_name, p.admission_quota,
				p.willingness_values,
				p.brochure_is_tentative,
				p.consultation_phone, p.brochure_url, p.special_talent_target,
				p.different_education_backgrounds, p.different_education_other,
				p.notes, COALESCE(p.source_locator, '-') AS source_locator, p.consultation_email, p.consultation_contact,
				p.registration_fee, p.exam_location,
				COALESCE(to_char(p.recommendation_letter_deadline, 'YYYY-MM-DD'), '-') AS recommendation_letter_deadline,
				COALESCE(to_char(p.portfolio_deadline, 'YYYY-MM-DD'), '-') AS portfolio_deadline,
				p.checkin_waitlist_process, p.fee_reduction_eligibility,
				p.admission_group, p.cross_group, p.admission_category, p.priority_admission,
				p.portfolio_required, p.recommendation_letter_type, p.max_applicable_programs,
				p.applicant_count, p.interview_count, p.admitted_count, p.waitlisted_count, p.promoted_count,
				p.admission_rate, p.first_stage_pass_rate, p.competition_ratio,
				p.school_official_url, p.department_official_url,
				ROW_NUMBER() OVER (PARTITION BY p.school_code, p.program_code ORDER BY p.academic_year DESC) AS rn
			FROM academic_programs p
			JOIN schools s ON s.school_code = p.school_code
			WHERE %s
			  AND p.review_status = 'published'
			  AND p.admission_quota > 0
		)
		SELECT
			academic_year, program_identifier, school_code, school_name,
			program_code, admission_program_name, admission_quota,
			willingness_values,
			brochure_is_tentative,
			consultation_phone, brochure_url, special_talent_target,
			different_education_backgrounds, different_education_other,
			notes, source_locator, consultation_email, consultation_contact,
			registration_fee, exam_location,
			recommendation_letter_deadline,
			portfolio_deadline,
			checkin_waitlist_process, fee_reduction_eligibility,
			admission_group, cross_group, admission_category, priority_admission,
			portfolio_required, recommendation_letter_type, max_applicable_programs,
			applicant_count, interview_count, admitted_count, waitlisted_count, promoted_count,
			admission_rate, first_stage_pass_rate, competition_ratio,
			school_official_url, department_official_url
		FROM ranked
		WHERE rn = 1 OR NOT $%d
		ORDER BY academic_year DESC, school_code, program_code
		LIMIT $%d OFFSET $%d
	`, strings.Join(conditions, " AND "), dedupePosition, limitPosition, offsetPosition)
	rows, err := r.pool.Query(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("list admission programs: %w", err)
	}
	defer rows.Close()
	programs := make([]Program, 0, query.Limit)
	for rows.Next() {
		program, err := scanProgram(rows)
		if err != nil {
			return nil, fmt.Errorf("scan admission program: %w", err)
		}
		programs = append(programs, program)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admission programs: %w", err)
	}
	rows.Close()
	if err := loadBatchExamItems(ctx, r.pool, programs); err != nil {
		return nil, err
	}
	if err := loadBatchTimelineEvents(ctx, r.pool, programs); err != nil {
		return nil, err
	}
	return programs, nil
}

func (r *PostgresRepository) GetProgram(ctx context.Context, identifier ProgramIdentifier) (Program, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT
			p.academic_year, p.program_identifier, p.school_code, s.school_name,
			p.program_code, p.admission_program_name, p.admission_quota,
			p.willingness_values,
			p.brochure_is_tentative,
			p.consultation_phone, p.brochure_url, p.special_talent_target,
			p.different_education_backgrounds, p.different_education_other,
			p.notes, COALESCE(p.source_locator, '-'), p.consultation_email, p.consultation_contact,
			p.registration_fee, p.exam_location,
			COALESCE(to_char(p.recommendation_letter_deadline, 'YYYY-MM-DD'), '-'),
			COALESCE(to_char(p.portfolio_deadline, 'YYYY-MM-DD'), '-'),
			p.checkin_waitlist_process, p.fee_reduction_eligibility,
			p.admission_group, p.cross_group, p.admission_category, p.priority_admission,
			p.portfolio_required, p.recommendation_letter_type, p.max_applicable_programs,
			p.applicant_count, p.interview_count, p.admitted_count, p.waitlisted_count, p.promoted_count,
			p.admission_rate, p.first_stage_pass_rate, p.competition_ratio,
			p.school_official_url, p.department_official_url
		FROM academic_programs p
		JOIN schools s ON s.school_code = p.school_code
		WHERE p.academic_year = $1 AND p.school_code = $2 AND p.program_code = $3
		  AND p.review_status = 'published'
		  AND p.admission_quota > 0
	`, identifier.AcademicYear, identifier.SchoolCode, identifier.ProgramCode)
	program, err := scanProgram(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Program{}, ErrNotFound
	}
	if err != nil {
		return Program{}, fmt.Errorf("get admission program: %w", err)
	}
	program.ExamItems, err = r.loadExamItems(ctx, identifier.AcademicYear, identifier.SchoolCode, identifier.ProgramCode)
	if err != nil {
		return Program{}, err
	}
	program.TimelineEvents, err = r.loadTimelineEvents(ctx, identifier.AcademicYear, identifier.SchoolCode, identifier.ProgramCode)
	if err != nil {
		return Program{}, err
	}
	return program, nil
}

func (r *PostgresRepository) ListSchools(ctx context.Context, academicYear int) ([]School, error) {
	args := []any{}
	condition := "s.is_active = TRUE"
	yearCondition := ""
	if academicYear > 0 {
		args = append(args, academicYear)
		yearCondition = "p.academic_year = $1 AND "
	}
	condition += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM academic_programs p WHERE p.school_code = s.school_code AND %sp.review_status = 'published' AND p.admission_quota > 0)", yearCondition)
	rows, err := r.pool.Query(ctx, `SELECT s.school_code, s.school_name FROM schools s WHERE `+condition+` ORDER BY s.school_code`, args...)
	if err != nil {
		return nil, fmt.Errorf("list schools: %w", err)
	}
	defer rows.Close()
	result := make([]School, 0)
	for rows.Next() {
		var school School
		if err := rows.Scan(&school.SchoolCode, &school.SchoolName); err != nil {
			return nil, fmt.Errorf("scan school: %w", err)
		}
		result = append(result, school)
	}
	return result, rows.Err()
}

// IsAdmin, despite the name (kept for interface compatibility across this
// package's admin/brochure handlers), grants access to the whole admissions
// admin surface to either a full admin or the narrower admissions_moderator
// role — a brochure board moderator who should be able to manage admissions
// data but has no access to anything else under /admin. That's enforced
// entirely here: nothing outside this package's own IsAdmin checks accepts
// admissions_moderator.
func (r *PostgresRepository) IsAdmin(ctx context.Context, accountID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM account_roles
			WHERE account_id = $1 AND role IN ('admin', 'admissions_moderator')
		)
	`, accountID).Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) CreateBrochure(ctx context.Context, adminID uuid.UUID, input BrochureDocumentInput) (BrochureDocument, string, error) {
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return BrochureDocument{}, "", err
	} else if !ok {
		return BrochureDocument{}, "", ErrInvalidProgram
	}
	return r.createBrochure(ctx, &adminID, input)
}

// CreateBrochureSystem is the service-to-service equivalent used after an
// authenticated external discovery/extraction client uploads a source. The
// source remains pending and the audit event deliberately has no account
// actor; the caller's service token is recorded by the ingestion job.
func (r *PostgresRepository) CreateBrochureSystem(ctx context.Context, input BrochureDocumentInput) (BrochureDocument, string, error) {
	return r.createBrochure(ctx, nil, input)
}

// PublishBrochureInTx publishes a source file as part of the upload-only
// confirmation transaction. The caller has already authenticated the admin;
// keeping this operation on the admissions repository ensures the brochure
// row and its append-only event share the program transaction.
func (r *PostgresRepository) PublishBrochureInTx(ctx context.Context, tx pgx.Tx, actorID uuid.UUID, input BrochureDocumentInput, reason string) (BrochureDocument, string, error) {
	if input.AcademicYear < 100 || input.AcademicYear > 999 || !validSchoolCode(input.SchoolCode) ||
		strings.TrimSpace(input.StorageKey) == "" || strings.TrimSpace(input.OriginalFileName) == "" ||
		input.FileSizeBytes <= 0 || !brochureSHA256Pattern.MatchString(input.SHA256) ||
		ValidateOfficialURL(input.SourceURL) != nil {
		return BrochureDocument{}, "", ErrInvalidProgram
	}
	if strings.TrimSpace(input.MIMEType) == "" {
		input.MIMEType = "application/pdf"
	}
	if strings.TrimSpace(reason) == "" {
		reason = "人工確認簡章資料"
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('sta.brochure_files'))`); err != nil {
		return BrochureDocument{}, "", fmt.Errorf("lock brochure file master: %w", err)
	}
	var oldStorageKey, oldStatus string
	err := tx.QueryRow(ctx, `SELECT storage_key, review_status FROM brochure_documents WHERE academic_year = $1 AND school_code = $2 FOR UPDATE`, input.AcademicYear, input.SchoolCode).Scan(&oldStorageKey, &oldStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		oldStorageKey, oldStatus = "", "-"
	} else if err != nil {
		return BrochureDocument{}, "", err
	}
	var document BrochureDocument
	err = tx.QueryRow(ctx, `
		INSERT INTO brochure_documents (academic_year, school_code, storage_key, original_file_name, mime_type, file_size_bytes, sha256_hex, source_url, review_status, published_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'published', CURRENT_TIMESTAMP)
		ON CONFLICT (academic_year, school_code) DO UPDATE SET
			storage_key = EXCLUDED.storage_key,
			original_file_name = EXCLUDED.original_file_name,
			mime_type = EXCLUDED.mime_type,
			file_size_bytes = EXCLUDED.file_size_bytes,
			sha256_hex = EXCLUDED.sha256_hex,
			source_url = EXCLUDED.source_url,
			review_status = 'published',
			published_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
		RETURNING academic_year, school_code, storage_key, original_file_name, mime_type, file_size_bytes, sha256_hex, source_url, review_status, published_at, created_at, updated_at
	`, input.AcademicYear, input.SchoolCode, input.StorageKey, input.OriginalFileName, input.MIMEType, input.FileSizeBytes, input.SHA256, emptyBrochureValue(input.SourceURL)).Scan(
		&document.AcademicYear, &document.SchoolCode, &document.storageKey, &document.OriginalFileName, &document.MIMEType, &document.FileSizeBytes, &document.SHA256, &document.SourceURL, &document.ReviewStatus, &document.PublishedAt, &document.CreatedAt, &document.UpdatedAt,
	)
	if err != nil {
		return BrochureDocument{}, "", mapAdmissionRepositoryError(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO brochure_document_events (academic_year, school_code, storage_key, original_file_name, sha256_hex, action, from_status, to_status, actor_account_id, reason)
		VALUES ($1, $2, $3, $4, $5, 'published', $6, 'published', $7, $8)
	`, document.AcademicYear, document.SchoolCode, input.StorageKey, input.OriginalFileName, input.SHA256, oldStatus, actorID, emptyBrochureValue(reason)); err != nil {
		return BrochureDocument{}, "", err
	}
	return document, oldStorageKey, nil
}

func (r *PostgresRepository) createBrochure(ctx context.Context, actorID *uuid.UUID, input BrochureDocumentInput) (BrochureDocument, string, error) {
	if input.AcademicYear < 100 || input.AcademicYear > 999 || !validSchoolCode(input.SchoolCode) || input.StorageKey == "" || input.FileSizeBytes <= 0 || !brochureSHA256Pattern.MatchString(input.SHA256) || ValidateOfficialURL(input.SourceURL) != nil {
		return BrochureDocument{}, "", ErrInvalidProgram
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return BrochureDocument{}, "", err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('sta.brochure_files'))`); err != nil {
		return BrochureDocument{}, "", fmt.Errorf("lock brochure file master: %w", err)
	}
	var oldStorageKey, oldStatus string
	err = tx.QueryRow(ctx, `SELECT storage_key, review_status FROM brochure_documents WHERE academic_year = $1 AND school_code = $2 FOR UPDATE`, input.AcademicYear, input.SchoolCode).Scan(&oldStorageKey, &oldStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		oldStorageKey, oldStatus = "", "-"
	} else if err != nil {
		return BrochureDocument{}, "", err
	}
	var document BrochureDocument
	err = tx.QueryRow(ctx, `
		INSERT INTO brochure_documents (academic_year, school_code, storage_key, original_file_name, mime_type, file_size_bytes, sha256_hex, source_url, review_status, published_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'pending', NULL)
		ON CONFLICT (academic_year, school_code) DO UPDATE SET
			storage_key = EXCLUDED.storage_key,
			original_file_name = EXCLUDED.original_file_name,
			mime_type = EXCLUDED.mime_type,
			file_size_bytes = EXCLUDED.file_size_bytes,
			sha256_hex = EXCLUDED.sha256_hex,
			source_url = EXCLUDED.source_url,
			review_status = 'pending',
			published_at = NULL,
			updated_at = CURRENT_TIMESTAMP
		RETURNING academic_year, school_code, storage_key, original_file_name, mime_type, file_size_bytes, sha256_hex, source_url, review_status, published_at, created_at, updated_at
	`, input.AcademicYear, input.SchoolCode, input.StorageKey, input.OriginalFileName, input.MIMEType, input.FileSizeBytes, input.SHA256, emptyBrochureValue(input.SourceURL)).Scan(
		&document.AcademicYear, &document.SchoolCode, &document.storageKey, &document.OriginalFileName, &document.MIMEType, &document.FileSizeBytes, &document.SHA256, &document.SourceURL, &document.ReviewStatus, &document.PublishedAt, &document.CreatedAt, &document.UpdatedAt,
	)
	if err != nil {
		return BrochureDocument{}, "", mapAdmissionRepositoryError(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO brochure_document_events (academic_year, school_code, storage_key, original_file_name, sha256_hex, action, from_status, to_status, actor_account_id)
		VALUES ($1, $2, $3, $4, $5, 'uploaded', $6, 'pending', $7)
	`, document.AcademicYear, document.SchoolCode, input.StorageKey, input.OriginalFileName, input.SHA256, oldStatus, actorID); err != nil {
		return BrochureDocument{}, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return BrochureDocument{}, "", err
	}
	return document, oldStorageKey, nil
}

func (r *PostgresRepository) ListBrochures(ctx context.Context, adminID uuid.UUID, academicYear int) ([]BrochureDocument, error) {
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrInvalidProgram
	}
	condition := "TRUE"
	args := []any{}
	if academicYear > 0 {
		condition = "academic_year = $1"
		args = append(args, academicYear)
	}
	rows, err := r.pool.Query(ctx, `SELECT academic_year, school_code, storage_key, original_file_name, mime_type, file_size_bytes, sha256_hex, source_url, review_status, published_at, created_at, updated_at FROM brochure_documents WHERE `+condition+` ORDER BY academic_year DESC, school_code`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]BrochureDocument, 0)
	for rows.Next() {
		item, err := scanBrochureDocument(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) ListBrochureEvents(ctx context.Context, adminID uuid.UUID, academicYear int, schoolCode string) ([]BrochureEvent, error) {
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrInvalidProgram
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, academic_year, school_code, action, COALESCE(from_status, ''), COALESCE(to_status, ''), original_file_name, sha256_hex, reason, created_at
		FROM brochure_document_events
		WHERE academic_year = $1 AND school_code = $2
		ORDER BY created_at DESC
	`, academicYear, schoolCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]BrochureEvent, 0)
	for rows.Next() {
		var item BrochureEvent
		if err := rows.Scan(&item.ID, &item.AcademicYear, &item.SchoolCode, &item.Action, &item.FromStatus, &item.ToStatus, &item.OriginalName, &item.SHA256, &item.Reason, &item.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) ReviewBrochure(ctx context.Context, adminID uuid.UUID, academicYear int, schoolCode string, approved bool, reason string) (BrochureDocument, error) {
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return BrochureDocument{}, err
	} else if !ok {
		return BrochureDocument{}, ErrInvalidProgram
	}
	status := "rejected"
	action := "rejected"
	if approved {
		status, action = "published", "published"
	}
	return r.changeBrochureStatus(ctx, adminID, academicYear, schoolCode, "pending", status, action, reason)
}

func (r *PostgresRepository) SetBrochurePublished(ctx context.Context, adminID uuid.UUID, academicYear int, schoolCode string, published bool, reason string) (BrochureDocument, error) {
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return BrochureDocument{}, err
	} else if !ok {
		return BrochureDocument{}, ErrInvalidProgram
	}
	fromStatus, toStatus, action := "published", "archived", "unpublished"
	if published {
		fromStatus, toStatus, action = "archived", "published", "republished"
	}
	return r.changeBrochureStatus(ctx, adminID, academicYear, schoolCode, fromStatus, toStatus, action, reason)
}

func (r *PostgresRepository) changeBrochureStatus(ctx context.Context, adminID uuid.UUID, academicYear int, schoolCode, expectedStatus, newStatus, action, reason string) (BrochureDocument, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return BrochureDocument{}, err
	}
	defer tx.Rollback(ctx)
	var oldStatus string
	if err := tx.QueryRow(ctx, `SELECT review_status FROM brochure_documents WHERE academic_year = $1 AND school_code = $2 FOR UPDATE`, academicYear, schoolCode).Scan(&oldStatus); errors.Is(err, pgx.ErrNoRows) {
		return BrochureDocument{}, ErrNotFound
	} else if err != nil {
		return BrochureDocument{}, err
	} else if oldStatus != expectedStatus {
		return BrochureDocument{}, ErrInvalidProgram
	}
	var document BrochureDocument
	// $3 is used twice below (SET target and inside the CASE), and pgx's
	// extended query protocol infers a type per occurrence — one comes back
	// `text`, the other `character varying`, and Postgres refuses the
	// mismatch ("inconsistent types deduced for parameter $3"). psql's
	// simple query protocol doesn't hit this, which is why manual testing
	// never caught it; this whole publish path had never actually been
	// exercised through the real Go call path before. Casting pins both
	// occurrences to the same type.
	err = tx.QueryRow(ctx, `
		UPDATE brochure_documents SET review_status = $3::varchar, published_at = CASE WHEN $3::varchar = 'published' THEN CURRENT_TIMESTAMP ELSE NULL END, updated_at = CURRENT_TIMESTAMP
		WHERE academic_year = $1 AND school_code = $2
		RETURNING academic_year, school_code, storage_key, original_file_name, mime_type, file_size_bytes, sha256_hex, source_url, review_status, published_at, created_at, updated_at
	`, academicYear, schoolCode, newStatus).Scan(
		&document.AcademicYear, &document.SchoolCode, &document.storageKey, &document.OriginalFileName, &document.MIMEType, &document.FileSizeBytes, &document.SHA256, &document.SourceURL, &document.ReviewStatus, &document.PublishedAt, &document.CreatedAt, &document.UpdatedAt,
	)
	if err != nil {
		return BrochureDocument{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO brochure_document_events (academic_year, school_code, storage_key, original_file_name, sha256_hex, action, from_status, to_status, actor_account_id, reason)
		SELECT academic_year, school_code, storage_key, original_file_name, sha256_hex, $3, $4, $5, $6, $7
		FROM brochure_documents WHERE academic_year = $1 AND school_code = $2
	`, academicYear, schoolCode, action, oldStatus, newStatus, adminID, emptyBrochureValue(reason)); err != nil {
		return BrochureDocument{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BrochureDocument{}, err
	}
	return document, nil
}

func (r *PostgresRepository) GetPublishedBrochure(ctx context.Context, academicYear int, schoolCode string) (BrochureDocument, error) {
	row := r.pool.QueryRow(ctx, `SELECT academic_year, school_code, storage_key, original_file_name, mime_type, file_size_bytes, sha256_hex, source_url, review_status, published_at, created_at, updated_at FROM brochure_documents WHERE academic_year = $1 AND school_code = $2 AND review_status = 'published'`, academicYear, schoolCode)
	item, err := scanBrochureDocument(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return BrochureDocument{}, ErrNotFound
	}
	return item, err
}

// ListPublishedBrochures is the public "歷史簡章" listing for a school — every
// academic_year that has a published brochure PDF, newest first. Unlike
// GetPublishedBrochure it doesn't need a presigned download URL up front;
// callers fetch that per-year, on demand, via the existing download
// endpoint, since a presigned URL is only valid for 5 minutes.
func (r *PostgresRepository) ListPublishedBrochures(ctx context.Context, schoolCode string) ([]BrochureDocument, error) {
	rows, err := r.pool.Query(ctx, `SELECT academic_year, school_code, storage_key, original_file_name, mime_type, file_size_bytes, sha256_hex, source_url, review_status, published_at, created_at, updated_at FROM brochure_documents WHERE school_code = $1 AND review_status = 'published' ORDER BY academic_year DESC`, schoolCode)
	if err != nil {
		return nil, fmt.Errorf("list published brochures: %w", err)
	}
	defer rows.Close()
	items := make([]BrochureDocument, 0)
	for rows.Next() {
		item, err := scanBrochureDocument(rows)
		if err != nil {
			return nil, fmt.Errorf("scan published brochure: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate published brochures: %w", err)
	}
	return items, nil
}

func (r *PostgresRepository) GetBrochure(ctx context.Context, adminID uuid.UUID, academicYear int, schoolCode string) (BrochureDocument, error) {
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return BrochureDocument{}, err
	} else if !ok {
		return BrochureDocument{}, ErrInvalidProgram
	}
	row := r.pool.QueryRow(ctx, `SELECT academic_year, school_code, storage_key, original_file_name, mime_type, file_size_bytes, sha256_hex, source_url, review_status, published_at, created_at, updated_at FROM brochure_documents WHERE academic_year = $1 AND school_code = $2`, academicYear, schoolCode)
	item, err := scanBrochureDocument(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return BrochureDocument{}, ErrNotFound
	}
	return item, err
}

func scanBrochureDocument(row rowScanner) (BrochureDocument, error) {
	var item BrochureDocument
	err := row.Scan(&item.AcademicYear, &item.SchoolCode, &item.storageKey, &item.OriginalFileName, &item.MIMEType, &item.FileSizeBytes, &item.SHA256, &item.SourceURL, &item.ReviewStatus, &item.PublishedAt, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func emptyBrochureValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}

func mapAdmissionRepositoryError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503":
			return ErrNotFound
		case "23505", "23514", "22001":
			return ErrInvalidProgram
		}
	}
	return err
}

type rowScanner interface {
	Scan(...any) error
}

func scanProgram(row rowScanner) (Program, error) {
	var program Program
	err := row.Scan(
		&program.AcademicYear, &program.ProgramIdentifier, &program.SchoolCode, &program.SchoolName,
		&program.ProgramCode, &program.AdmissionProgramName, &program.AdmissionQuota,
		&program.WillingnessValues,
		&program.BrochureIsTentative,
		&program.ConsultationPhone, &program.BrochureURL,
		&program.SpecialTalentTarget, &program.DifferentEducationBackgrounds,
		&program.DifferentEducationOther, &program.Notes, &program.SourceLocator,
		&program.ConsultationEmail, &program.ConsultationContact,
		&program.RegistrationFee, &program.ExamLocation,
		&program.RecommendationLetterDeadline, &program.PortfolioDeadline,
		&program.CheckinWaitlistProcess, &program.FeeReductionEligibility,
		&program.AdmissionGroup, &program.CrossGroup, &program.AdmissionCategory, &program.PriorityAdmission,
		&program.PortfolioRequired, &program.RecommendationLetterType, &program.MaxApplicablePrograms,
		&program.ApplicantCount, &program.InterviewCount, &program.AdmittedCount, &program.WaitlistedCount, &program.PromotedCount,
		&program.AdmissionRate, &program.FirstStagePassRate, &program.CompetitionRatio,
		&program.SchoolOfficialURL, &program.DepartmentOfficialURL,
	)
	return program, err
}

func (r *PostgresRepository) loadExamItems(ctx context.Context, academicYear int, schoolCode, programCode string) ([]ExamItem, error) {
	return loadExamItems(ctx, r.pool, academicYear, schoolCode, programCode)
}

func (r *PostgresRepository) loadTimelineEvents(ctx context.Context, academicYear int, schoolCode, programCode string) ([]TimelineEvent, error) {
	return loadTimelineEvents(ctx, r.pool, academicYear, schoolCode, programCode)
}

type admissionQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func batchKey(academicYear int, schoolCode, programCode string) string {
	return fmt.Sprintf("%d|%s|%s", academicYear, schoolCode, programCode)
}

// loadBatchExamItems and loadBatchTimelineEvents populate every program's
// ExamItems/TimelineEvents in a single query each, instead of the N+1 pattern
// of calling loadExamItems/loadTimelineEvents once per row — ListPrograms can
// return dozens of programs per page, so this avoids dozens of extra
// round-trips.
func loadBatchExamItems(ctx context.Context, queryer admissionQueryer, programs []Program) error {
	if len(programs) == 0 {
		return nil
	}
	years := make([]int32, len(programs))
	schoolCodes := make([]string, len(programs))
	programCodes := make([]string, len(programs))
	index := make(map[string]int, len(programs))
	for i, p := range programs {
		years[i] = int32(p.AcademicYear)
		schoolCodes[i] = p.SchoolCode
		programCodes[i] = p.ProgramCode
		index[batchKey(p.AcademicYear, p.SchoolCode, p.ProgramCode)] = i
		programs[i].ExamItems = make([]ExamItem, 0)
	}
	rows, err := queryer.Query(ctx, `
		SELECT academic_year, school_code, program_code, item_name, exam_stage, sort_order,
		       weight_percent::float8, multiplier::float8, description, COALESCE(source_page::text, '-')
		FROM program_exam_items
		WHERE (academic_year, school_code, program_code) IN (
			SELECT * FROM unnest($1::int[], $2::text[], $3::text[])
		)
		ORDER BY academic_year, school_code, program_code, sort_order
	`, years, schoolCodes, programCodes)
	if err != nil {
		return fmt.Errorf("load exam items: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var year int
		var schoolCode, programCode string
		var item ExamItem
		var weight, multiplier *float64
		if err := rows.Scan(&year, &schoolCode, &programCode, &item.Name, &item.Stage, &item.SortOrder, &weight, &multiplier, &item.Description, &item.SourcePage); err != nil {
			return fmt.Errorf("scan exam item: %w", err)
		}
		item.WeightPercent = weight
		item.Multiplier = multiplier
		if i, ok := index[batchKey(year, schoolCode, programCode)]; ok {
			programs[i].ExamItems = append(programs[i].ExamItems, item)
		}
	}
	return rows.Err()
}

func loadBatchTimelineEvents(ctx context.Context, queryer admissionQueryer, programs []Program) error {
	if len(programs) == 0 {
		return nil
	}
	years := make([]int32, len(programs))
	schoolCodes := make([]string, len(programs))
	programCodes := make([]string, len(programs))
	index := make(map[string]int, len(programs))
	for i, p := range programs {
		years[i] = int32(p.AcademicYear)
		schoolCodes[i] = p.SchoolCode
		programCodes[i] = p.ProgramCode
		index[batchKey(p.AcademicYear, p.SchoolCode, p.ProgramCode)] = i
		programs[i].TimelineEvents = make([]TimelineEvent, 0)
	}
	rows, err := queryer.Query(ctx, `
		SELECT academic_year, school_code, program_code, event_name,
		       COALESCE(to_char(start_date, 'YYYY-MM-DD'), '-'), start_time,
		       COALESCE(to_char(end_date, 'YYYY-MM-DD'), '-'), end_time, sort_order, notes
		FROM program_timeline_events
		WHERE (academic_year, school_code, program_code) IN (
			SELECT * FROM unnest($1::int[], $2::text[], $3::text[])
		)
		ORDER BY academic_year, school_code, program_code, sort_order
	`, years, schoolCodes, programCodes)
	if err != nil {
		return fmt.Errorf("load timeline events: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var year int
		var schoolCode, programCode string
		var event TimelineEvent
		if err := rows.Scan(&year, &schoolCode, &programCode, &event.Name, &event.StartDate, &event.StartTime, &event.EndDate, &event.EndTime, &event.SortOrder, &event.Notes); err != nil {
			return fmt.Errorf("scan timeline event: %w", err)
		}
		if i, ok := index[batchKey(year, schoolCode, programCode)]; ok {
			programs[i].TimelineEvents = append(programs[i].TimelineEvents, event)
		}
	}
	return rows.Err()
}

func loadExamItems(ctx context.Context, queryer admissionQueryer, academicYear int, schoolCode, programCode string) ([]ExamItem, error) {
	rows, err := queryer.Query(ctx, `
		SELECT item_name, exam_stage, sort_order, weight_percent::float8, multiplier::float8, description,
		       COALESCE(source_page::text, '-')
		FROM program_exam_items
		WHERE academic_year = $1 AND school_code = $2 AND program_code = $3
		ORDER BY sort_order
	`, academicYear, schoolCode, programCode)
	if err != nil {
		return nil, fmt.Errorf("load exam items: %w", err)
	}
	defer rows.Close()
	items := make([]ExamItem, 0)
	for rows.Next() {
		var item ExamItem
		var weight, multiplier *float64
		if err := rows.Scan(&item.Name, &item.Stage, &item.SortOrder, &weight, &multiplier, &item.Description, &item.SourcePage); err != nil {
			return nil, fmt.Errorf("scan exam item: %w", err)
		}
		item.WeightPercent = weight
		item.Multiplier = multiplier
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate exam items: %w", err)
	}
	return items, nil
}

func loadTimelineEvents(ctx context.Context, queryer admissionQueryer, academicYear int, schoolCode, programCode string) ([]TimelineEvent, error) {
	rows, err := queryer.Query(ctx, `
		SELECT event_name, COALESCE(to_char(start_date, 'YYYY-MM-DD'), '-'), start_time,
		       COALESCE(to_char(end_date, 'YYYY-MM-DD'), '-'), end_time, sort_order, notes
		FROM program_timeline_events
		WHERE academic_year = $1 AND school_code = $2 AND program_code = $3
		ORDER BY sort_order
	`, academicYear, schoolCode, programCode)
	if err != nil {
		return nil, fmt.Errorf("load timeline events: %w", err)
	}
	defer rows.Close()
	events := make([]TimelineEvent, 0)
	for rows.Next() {
		var event TimelineEvent
		if err := rows.Scan(&event.Name, &event.StartDate, &event.StartTime, &event.EndDate, &event.EndTime, &event.SortOrder, &event.Notes); err != nil {
			return nil, fmt.Errorf("scan timeline event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate timeline events: %w", err)
	}
	return events, nil
}
