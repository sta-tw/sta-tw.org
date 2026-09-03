package admissions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const adminProgramSelect = `
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
		p.notes, COALESCE(p.source_locator, '-'), p.review_status,
		p.created_at, p.updated_at
	FROM academic_programs p
	JOIN schools s ON s.school_code = p.school_code`

func (r *PostgresRepository) ListAdminPrograms(ctx context.Context, adminID uuid.UUID, query ProgramAdminQuery) ([]AdminProgram, error) {
	if err := validateProgramAdminQuery(query); err != nil {
		return nil, err
	}
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrAdminRequired
	}
	conditions := []string{"TRUE"}
	args := make([]any, 0, 8)
	if query.AcademicYear > 0 {
		args = append(args, query.AcademicYear)
		conditions = append(conditions, fmt.Sprintf("p.academic_year = $%d", len(args)))
	}
	if query.SchoolCode != "" {
		args = append(args, query.SchoolCode)
		conditions = append(conditions, fmt.Sprintf("p.school_code = $%d", len(args)))
	}
	if query.ReviewStatus != "" {
		args = append(args, query.ReviewStatus)
		conditions = append(conditions, fmt.Sprintf("p.review_status = $%d", len(args)))
	}
	if query.Search != "" {
		args = append(args, "%"+escapeLike(query.Search)+"%")
		position := len(args)
		conditions = append(conditions, fmt.Sprintf("(s.school_name ILIKE $%d ESCAPE '\\' OR p.admission_program_name ILIKE $%d ESCAPE '\\' OR p.program_identifier = $%d)", position, position, position))
	}
	args = append(args, query.Limit, query.Offset)
	limitPosition, offsetPosition := len(args)-1, len(args)
	statement := fmt.Sprintf("%s WHERE %s ORDER BY p.academic_year DESC, p.school_code, p.program_code LIMIT $%d OFFSET $%d", adminProgramSelect, strings.Join(conditions, " AND "), limitPosition, offsetPosition)
	rows, err := r.pool.Query(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("list admin admission programs: %w", err)
	}
	defer rows.Close()
	result := make([]AdminProgram, 0, query.Limit)
	for rows.Next() {
		item, err := scanAdminProgram(rows)
		if err != nil {
			return nil, fmt.Errorf("scan admin admission program: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admin admission programs: %w", err)
	}
	rows.Close()
	for index := range result {
		result[index].Program.ExamItems, err = loadExamItems(ctx, r.pool, result[index].AcademicYear, result[index].SchoolCode, result[index].ProgramCode)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (r *PostgresRepository) GetAdminProgram(ctx context.Context, adminID uuid.UUID, identifier ProgramIdentifier) (AdminProgram, error) {
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return AdminProgram{}, err
	} else if !ok {
		return AdminProgram{}, ErrAdminRequired
	}
	return loadAdminProgram(ctx, r.pool, identifier, false)
}

func (r *PostgresRepository) UpsertPrograms(ctx context.Context, adminID uuid.UUID, input ProgramBatchInput) ([]AdminProgram, error) {
	input = normalizeProgramBatch(input)
	if err := input.Validate(); err != nil {
		return nil, err
	}
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrAdminRequired
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin admission program update: %w", err)
	}
	defer tx.Rollback(ctx)
	result, err := r.UpsertProgramsInTx(ctx, tx, adminID, input)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit admission program update: %w", err)
	}
	return result, nil
}

// UpsertProgramsInTx lets another reviewed admin workflow apply admission
// drafts atomically with its own state transition. The caller is responsible
// for authenticating adminID before opening the transaction.
func (r *PostgresRepository) UpsertProgramsInTx(ctx context.Context, tx pgx.Tx, adminID uuid.UUID, input ProgramBatchInput) ([]AdminProgram, error) {
	input = normalizeProgramBatch(input)
	if err := input.Validate(); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('sta.admission_programs'))`); err != nil {
		return nil, fmt.Errorf("lock admission program master: %w", err)
	}

	result := make([]AdminProgram, 0, len(input.Items))
	for _, item := range input.Items {
		identifier, err := item.identifier()
		if err != nil {
			return nil, err
		}
		var schoolName string
		if err := tx.QueryRow(ctx, `SELECT school_name FROM schools WHERE school_code = $1`, item.SchoolCode).Scan(&schoolName); errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		} else if err != nil {
			return nil, fmt.Errorf("load school %s: %w", item.SchoolCode, err)
		}
		program, err := item.MaterializeWithSchoolName(schoolName)
		if err != nil {
			return nil, err
		}
		before, err := loadAdminProgram(ctx, tx, identifier, true)
		exists := true
		if errors.Is(err, ErrNotFound) {
			exists = false
		} else if err != nil {
			return nil, err
		}
		changed := !exists || !programsEqual(before.Program, program)
		if exists && !changed {
			result = append(result, before)
			continue
		}
		status := ProgramStatusPending
		if _, err := tx.Exec(ctx, upsertProgramSQL,
			program.AcademicYear, program.SchoolCode, program.ProgramCode, program.AdmissionProgramName,
			program.AdmissionQuota, program.BrochureIsTentative,
			program.BrochureAnnouncementDate, program.BrochureScheduledDate,
			program.RegistrationStartDate, program.RegistrationEndDate,
			program.ExamStartDate, program.ExamEndDate, program.ResultDate,
			program.ConsultationPhone, program.BrochureURL, program.SpecialTalentTarget,
			program.DifferentEducationBackgrounds, program.DifferentEducationOther, program.Notes,
			sourcePageValue(item.SourcePage), status,
		); err != nil {
			return nil, mapAdmissionRepositoryError(err)
		}
		if changed {
			if err := replaceExamItems(ctx, tx, program); err != nil {
				return nil, err
			}
		}
		after, err := loadAdminProgram(ctx, tx, identifier, false)
		if err != nil {
			return nil, err
		}
		if changed {
			action := "update"
			beforeData := map[string]any(nil)
			if exists {
				beforeData = adminProgramSnapshot(before)
			} else {
				action = "create"
			}
			if err := insertProgramAudit(ctx, tx, adminID, action, identifier.String(), beforeData, adminProgramSnapshot(after), input.Reason); err != nil {
				return nil, err
			}
		}
		result = append(result, after)
	}
	return result, nil
}

func (r *PostgresRepository) ReviewProgram(ctx context.Context, adminID uuid.UUID, identifier ProgramIdentifier, input ProgramReviewInput) (AdminProgram, error) {
	if err := input.Validate(); err != nil {
		return AdminProgram{}, err
	}
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return AdminProgram{}, err
	} else if !ok {
		return AdminProgram{}, ErrAdminRequired
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return AdminProgram{}, fmt.Errorf("begin admission program review: %w", err)
	}
	defer tx.Rollback(ctx)
	before, err := loadAdminProgram(ctx, tx, identifier, true)
	if err != nil {
		return AdminProgram{}, err
	}
	if before.ReviewStatus != ProgramStatusPending {
		return AdminProgram{}, ErrInvalidStatus
	}
	newStatus := ProgramStatusRejected
	action := "reject"
	if input.Approved {
		newStatus = ProgramStatusPublished
		action = "publish"
	}
	if _, err := tx.Exec(ctx, `
		UPDATE academic_programs
		SET review_status = $4, updated_at = CURRENT_TIMESTAMP
		WHERE academic_year = $1 AND school_code = $2 AND program_code = $3
	`, identifier.AcademicYear, identifier.SchoolCode, identifier.ProgramCode, newStatus); err != nil {
		return AdminProgram{}, mapAdmissionRepositoryError(err)
	}
	after, err := loadAdminProgram(ctx, tx, identifier, false)
	if err != nil {
		return AdminProgram{}, err
	}
	if err := insertProgramAudit(ctx, tx, adminID, action, identifier.String(), adminProgramSnapshot(before), adminProgramSnapshot(after), strings.TrimSpace(input.Reason)); err != nil {
		return AdminProgram{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AdminProgram{}, fmt.Errorf("commit admission program review: %w", err)
	}
	return after, nil
}

func (r *PostgresRepository) ListProgramHistory(ctx context.Context, adminID uuid.UUID, identifier ProgramIdentifier) ([]ProgramAuditEvent, error) {
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrAdminRequired
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, action, entity_key, before_data, after_data, reason, created_at
		FROM audit_log
		WHERE entity_type = 'academic_program' AND entity_key = $1
		ORDER BY created_at DESC, id DESC
	`, identifier.String())
	if err != nil {
		return nil, fmt.Errorf("list admission program history: %w", err)
	}
	defer rows.Close()
	result := make([]ProgramAuditEvent, 0)
	for rows.Next() {
		var event ProgramAuditEvent
		var beforeData, afterData []byte
		if err := rows.Scan(&event.ID, &event.Action, &event.EntityKey, &beforeData, &afterData, &event.Reason, &event.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan admission program history: %w", err)
		}
		event.BeforeData = decodeProgramAuditData(beforeData)
		event.AfterData = decodeProgramAuditData(afterData)
		result = append(result, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admission program history: %w", err)
	}
	return result, nil
}

func loadAdminProgram(ctx context.Context, queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, identifier ProgramIdentifier, forUpdate bool) (AdminProgram, error) {
	statement := adminProgramSelect + ` WHERE p.academic_year = $1 AND p.school_code = $2 AND p.program_code = $3`
	if forUpdate {
		statement += ` FOR UPDATE`
	}
	item, err := scanAdminProgram(queryer.QueryRow(ctx, statement, identifier.AcademicYear, identifier.SchoolCode, identifier.ProgramCode))
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminProgram{}, ErrNotFound
	}
	if err != nil {
		return AdminProgram{}, fmt.Errorf("load admin admission program: %w", err)
	}
	item.Program.ExamItems, err = loadExamItems(ctx, queryer, identifier.AcademicYear, identifier.SchoolCode, identifier.ProgramCode)
	if err != nil {
		return AdminProgram{}, err
	}
	return item, nil
}

func scanAdminProgram(row interface{ Scan(...any) error }) (AdminProgram, error) {
	var item AdminProgram
	err := row.Scan(
		&item.AcademicYear, &item.ProgramIdentifier, &item.SchoolCode, &item.SchoolName,
		&item.ProgramCode, &item.AdmissionProgramName, &item.AdmissionQuota,
		&item.WillingnessValues,
		&item.BrochureIsTentative, &item.BrochureAnnouncementDate,
		&item.BrochureScheduledDate, &item.RegistrationStartDate,
		&item.RegistrationEndDate, &item.ExamStartDate, &item.ExamEndDate,
		&item.ResultDate, &item.ConsultationPhone, &item.BrochureURL,
		&item.SpecialTalentTarget, &item.DifferentEducationBackgrounds,
		&item.DifferentEducationOther, &item.Notes, &item.SourceLocator,
		&item.ReviewStatus, &item.CreatedAt, &item.UpdatedAt,
	)
	return item, err
}

func replaceExamItems(ctx context.Context, tx pgx.Tx, program Program) error {
	if _, err := tx.Exec(ctx, `DELETE FROM program_exam_items WHERE academic_year = $1 AND school_code = $2 AND program_code = $3`, program.AcademicYear, program.SchoolCode, program.ProgramCode); err != nil {
		return fmt.Errorf("replace admission exam items: %w", err)
	}
	for _, item := range program.ExamItems {
		page, err := parseExamItemPage(item.SourcePage)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO program_exam_items
				(academic_year, school_code, program_code, item_name, sort_order, weight_percent, multiplier, description, source_page)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, program.AcademicYear, program.SchoolCode, program.ProgramCode, item.Name, item.SortOrder, item.WeightPercent, item.Multiplier, item.Description, page); err != nil {
			return mapAdmissionRepositoryError(err)
		}
	}
	return nil
}

func programsEqual(before, after Program) bool {
	// willingness_values is result-derived data. Manual program edits must not
	// reset it or make an otherwise unchanged program appear modified.
	before.WillingnessValues = nil
	after.WillingnessValues = nil
	return reflect.DeepEqual(before, after)
}

func adminProgramSnapshot(item AdminProgram) map[string]any {
	return map[string]any{
		"academic_year":                   item.AcademicYear,
		"program_identifier":              item.ProgramIdentifier,
		"school_code":                     item.SchoolCode,
		"school_name":                     item.SchoolName,
		"program_code":                    item.ProgramCode,
		"admission_program_name":          item.AdmissionProgramName,
		"admission_quota":                 item.AdmissionQuota,
		"exam_items":                      item.ExamItems,
		"brochure_is_tentative":           item.BrochureIsTentative,
		"brochure_announcement_date":      item.BrochureAnnouncementDate,
		"brochure_scheduled_date":         item.BrochureScheduledDate,
		"registration_start_date":         item.RegistrationStartDate,
		"registration_end_date":           item.RegistrationEndDate,
		"exam_start_date":                 item.ExamStartDate,
		"exam_end_date":                   item.ExamEndDate,
		"result_date":                     item.ResultDate,
		"consultation_phone":              item.ConsultationPhone,
		"brochure_url":                    item.BrochureURL,
		"special_talent_target":           item.SpecialTalentTarget,
		"different_education_backgrounds": item.DifferentEducationBackgrounds,
		"different_education_other":       item.DifferentEducationOther,
		"notes":                           item.Notes,
		"source_locator":                  item.SourceLocator,
		"review_status":                   item.ReviewStatus,
	}
}

func insertProgramAudit(ctx context.Context, tx pgx.Tx, adminID uuid.UUID, action, entityKey string, before, after map[string]any, reason string) error {
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		return fmt.Errorf("marshal admission program audit before data: %w", err)
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		return fmt.Errorf("marshal admission program audit after data: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_log (actor_account_id, action, entity_type, entity_key, before_data, after_data, reason)
		VALUES ($1, $2, 'academic_program', $3, NULLIF($4, 'null')::jsonb, $5::jsonb, $6)
	`, adminID, action, entityKey, string(beforeJSON), string(afterJSON), strings.TrimSpace(reason)); err != nil {
		return fmt.Errorf("record admission program audit event: %w", err)
	}
	return nil
}

func decodeProgramAuditData(value []byte) map[string]any {
	if len(value) == 0 || string(value) == "null" {
		return nil
	}
	var data map[string]any
	if json.Unmarshal(value, &data) != nil {
		return nil
	}
	return data
}

func sourcePageValue(page *int) any {
	if page == nil {
		return nil
	}
	return *page
}

func validateProgramAdminQuery(query ProgramAdminQuery) error {
	if query.AcademicYear < 0 || query.AcademicYear > 999 || (query.AcademicYear > 0 && query.AcademicYear < 100) {
		return ErrInvalidProgram
	}
	if query.SchoolCode != "" && !validSchoolCode(query.SchoolCode) {
		return ErrInvalidProgram
	}
	if query.ReviewStatus != "" && !validProgramReviewStatus(query.ReviewStatus) {
		return ErrInvalidProgram
	}
	if query.Limit < 1 || query.Limit > 100 || query.Offset < 0 || query.Offset > 10000 || len([]rune(query.Search)) > 100 {
		return ErrInvalidProgram
	}
	return nil
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	return strings.ReplaceAll(value, `_`, `\_`)
}

var _ AdminRepository = (*PostgresRepository)(nil)

const upsertProgramSQL = `
	INSERT INTO academic_programs (
		academic_year, school_code, program_code, admission_program_name,
		admission_quota, brochure_is_tentative, brochure_announcement_date,
		brochure_scheduled_date, registration_start_date, registration_end_date,
		exam_start_date, exam_end_date, result_date, consultation_phone,
		brochure_url, special_talent_target, different_education_backgrounds,
		different_education_other, notes, source_page, review_status
	)
	VALUES (
		$1, $2, $3, $4, $5, $6,
		NULLIF($7, '-')::date, NULLIF($8, '-')::date,
		NULLIF($9, '-')::date, NULLIF($10, '-')::date,
		NULLIF($11, '-')::date, NULLIF($12, '-')::date,
		NULLIF($13, '-')::date, $14, $15, $16, $17, $18, $19, $20, $21
	)
	ON CONFLICT (academic_year, school_code, program_code) DO UPDATE SET
		admission_program_name = EXCLUDED.admission_program_name,
		admission_quota = EXCLUDED.admission_quota,
		brochure_is_tentative = EXCLUDED.brochure_is_tentative,
		brochure_announcement_date = EXCLUDED.brochure_announcement_date,
		brochure_scheduled_date = EXCLUDED.brochure_scheduled_date,
		registration_start_date = EXCLUDED.registration_start_date,
		registration_end_date = EXCLUDED.registration_end_date,
		exam_start_date = EXCLUDED.exam_start_date,
		exam_end_date = EXCLUDED.exam_end_date,
		result_date = EXCLUDED.result_date,
		consultation_phone = EXCLUDED.consultation_phone,
		brochure_url = EXCLUDED.brochure_url,
		special_talent_target = EXCLUDED.special_talent_target,
		different_education_backgrounds = EXCLUDED.different_education_backgrounds,
		different_education_other = EXCLUDED.different_education_other,
		notes = EXCLUDED.notes,
		source_page = EXCLUDED.source_page,
		review_status = EXCLUDED.review_status,
		updated_at = CURRENT_TIMESTAMP`
