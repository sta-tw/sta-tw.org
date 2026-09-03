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

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) (*PostgresRepository, error) {
	if pool == nil {
		return nil, errors.New("postgres pool is nil")
	}
	return &PostgresRepository{pool: pool}, nil
}

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
	if query.Search != "" {
		args = append(args, "%"+query.Search+"%")
		conditions = append(conditions, fmt.Sprintf("(s.school_name ILIKE $%d OR p.admission_program_name ILIKE $%d)", len(args), len(args)))
	}
	args = append(args, query.Limit, query.Offset)
	limitPosition, offsetPosition := len(args)-1, len(args)
	statement := fmt.Sprintf(`
		SELECT
			p.academic_year, p.program_identifier, p.school_code, s.school_name,
			p.program_code, p.admission_program_name, p.admission_quota,
			p.willingness_values,
			p.brochure_is_tentative,
			COALESCE(to_char(p.brochure_announcement_date, 'YYYY-MM-DD'), '-'),
			COALESCE(to_char(p.brochure_scheduled_date, 'YYYY-MM-DD'), '-'),
			COALESCE(to_char(p.registration_start_date, 'YYYY-MM-DD'), '-'),
			COALESCE(to_char(p.registration_end_date, 'YYYY-MM-DD'), '-'),
			COALESCE(to_char(p.exam_start_date, 'YYYY-MM-DD'), '-'),
			COALESCE(to_char(p.exam_end_date, 'YYYY-MM-DD'), '-'),
			COALESCE(to_char(p.result_date, 'YYYY-MM-DD'), '-'),
			p.consultation_phone, p.brochure_url, p.special_talent_target,
			p.different_education_backgrounds, p.different_education_other,
			p.notes, COALESCE(p.source_locator, '-')
		FROM academic_programs p
		JOIN schools s ON s.school_code = p.school_code
		WHERE %s
		  AND p.review_status = 'published'
		ORDER BY p.academic_year DESC, p.school_code, p.program_code
		LIMIT $%d OFFSET $%d
	`, strings.Join(conditions, " AND "), limitPosition, offsetPosition)
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
	for index := range programs {
		programs[index].ExamItems, err = r.loadExamItems(ctx, programs[index].AcademicYear, programs[index].SchoolCode, programs[index].ProgramCode)
		if err != nil {
			return nil, err
		}
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
			COALESCE(to_char(p.brochure_announcement_date, 'YYYY-MM-DD'), '-'),
			COALESCE(to_char(p.brochure_scheduled_date, 'YYYY-MM-DD'), '-'),
			COALESCE(to_char(p.registration_start_date, 'YYYY-MM-DD'), '-'),
			COALESCE(to_char(p.registration_end_date, 'YYYY-MM-DD'), '-'),
			COALESCE(to_char(p.exam_start_date, 'YYYY-MM-DD'), '-'),
			COALESCE(to_char(p.exam_end_date, 'YYYY-MM-DD'), '-'),
			COALESCE(to_char(p.result_date, 'YYYY-MM-DD'), '-'),
			p.consultation_phone, p.brochure_url, p.special_talent_target,
			p.different_education_backgrounds, p.different_education_other,
			p.notes, COALESCE(p.source_locator, '-')
		FROM academic_programs p
		JOIN schools s ON s.school_code = p.school_code
		WHERE p.academic_year = $1 AND p.school_code = $2 AND p.program_code = $3
		  AND p.review_status = 'published'
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
	return program, nil
}

func (r *PostgresRepository) ListSchools(ctx context.Context, academicYear int) ([]School, error) {
	args := []any{}
	condition := "s.is_active = TRUE"
	if academicYear > 0 {
		args = append(args, academicYear)
		condition += " AND EXISTS (SELECT 1 FROM academic_programs p WHERE p.school_code = s.school_code AND p.academic_year = $1 AND p.review_status = 'published')"
	}
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

func (r *PostgresRepository) IsAdmin(ctx context.Context, accountID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM account_roles WHERE account_id = $1 AND role = 'admin')`, accountID).Scan(&exists)
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
	err = tx.QueryRow(ctx, `
		UPDATE brochure_documents SET review_status = $3, published_at = CASE WHEN $3 = 'published' THEN CURRENT_TIMESTAMP ELSE NULL END, updated_at = CURRENT_TIMESTAMP
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
		&program.BrochureIsTentative, &program.BrochureAnnouncementDate,
		&program.BrochureScheduledDate, &program.RegistrationStartDate,
		&program.RegistrationEndDate, &program.ExamStartDate, &program.ExamEndDate,
		&program.ResultDate, &program.ConsultationPhone, &program.BrochureURL,
		&program.SpecialTalentTarget, &program.DifferentEducationBackgrounds,
		&program.DifferentEducationOther, &program.Notes, &program.SourceLocator,
	)
	return program, err
}

func (r *PostgresRepository) loadExamItems(ctx context.Context, academicYear int, schoolCode, programCode string) ([]ExamItem, error) {
	return loadExamItems(ctx, r.pool, academicYear, schoolCode, programCode)
}

type admissionQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func loadExamItems(ctx context.Context, queryer admissionQueryer, academicYear int, schoolCode, programCode string) ([]ExamItem, error) {
	rows, err := queryer.Query(ctx, `
		SELECT item_name, sort_order, weight_percent::float8, multiplier::float8, description,
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
		if err := rows.Scan(&item.Name, &item.SortOrder, &weight, &multiplier, &item.Description, &item.SourcePage); err != nil {
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
