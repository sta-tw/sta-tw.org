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
		p.consultation_phone, p.consultation_email, p.consultation_contact, p.brochure_url, p.special_talent_target,
		p.different_education_backgrounds, p.different_education_other,
		p.notes, COALESCE(p.source_locator, '-'), p.review_status,
		p.registration_fee, p.exam_location,
		COALESCE(to_char(p.recommendation_letter_deadline, 'YYYY-MM-DD'), '-'),
		COALESCE(to_char(p.portfolio_deadline, 'YYYY-MM-DD'), '-'),
		p.checkin_waitlist_process, p.fee_reduction_eligibility,
		p.admission_group, p.cross_group, p.admission_category, p.priority_admission,
		p.portfolio_required, p.recommendation_letter_type, p.max_applicable_programs,
		p.applicant_count, p.interview_count, p.admitted_count, p.waitlisted_count,
		p.admission_rate, p.first_stage_pass_rate, p.competition_ratio,
		p.school_official_url, p.department_official_url,
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
	if query.ProgramCode != "" {
		args = append(args, query.ProgramCode)
		conditions = append(conditions, fmt.Sprintf("p.program_code = $%d", len(args)))
	}
	if query.ReviewStatus != "" {
		args = append(args, query.ReviewStatus)
		conditions = append(conditions, fmt.Sprintf("p.review_status = $%d", len(args)))
	}
	if query.Search != "" {
		args = append(args, "%"+escapeLike(normalizeTaiVariant(query.Search))+"%")
		position := len(args)
		conditions = append(conditions, fmt.Sprintf("(REPLACE(s.school_name, '臺', '台') ILIKE $%d ESCAPE '\\' OR REPLACE(p.admission_program_name, '臺', '台') ILIKE $%d ESCAPE '\\' OR p.program_identifier = $%d)", position, position, position))
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
		result[index].Program.TimelineEvents, err = loadTimelineEvents(ctx, r.pool, result[index].AcademicYear, result[index].SchoolCode, result[index].ProgramCode)
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

// UpsertProgramsInTx applies admission drafts within a caller-owned transaction.
// The caller is responsible for authenticating adminID beforehand.
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
			program.ConsultationPhone, program.BrochureURL, program.SpecialTalentTarget,
			program.DifferentEducationBackgrounds, program.DifferentEducationOther, program.Notes,
			sourcePageValue(item.SourcePage), status, program.ConsultationEmail, program.ConsultationContact,
			program.RegistrationFee, program.ExamLocation,
			program.RecommendationLetterDeadline, program.PortfolioDeadline,
			program.CheckinWaitlistProcess, program.FeeReductionEligibility,
			program.AdmissionGroup, program.CrossGroup, program.AdmissionCategory, program.PriorityAdmission,
			program.PortfolioRequired, program.RecommendationLetterType, program.MaxApplicablePrograms,
			program.ApplicantCount, program.InterviewCount, program.AdmittedCount, program.WaitlistedCount,
			program.AdmissionRate, program.FirstStagePassRate, program.CompetitionRatio,
			program.SchoolOfficialURL, program.DepartmentOfficialURL,
		); err != nil {
			return nil, mapAdmissionRepositoryError(err)
		}
		if changed {
			if err := replaceExamItems(ctx, tx, program); err != nil {
				return nil, err
			}
			if err := replaceTimelineEvents(ctx, tx, program); err != nil {
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

// PublishProgramsInTx publishes rows after an admin confirms extracted PDF fields
// (the ordinary sync/update path leaves programs pending instead).
func (r *PostgresRepository) PublishProgramsInTx(ctx context.Context, tx pgx.Tx, adminID uuid.UUID, programs []AdminProgram, reason string) ([]AdminProgram, error) {
	if len(programs) == 0 || len(programs) > 500 {
		return nil, ErrInvalidProgram
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "人工確認簡章資料"
	}
	result := make([]AdminProgram, 0, len(programs))
	for _, program := range programs {
		identifier, err := ParseProgramIdentifier(program.ProgramIdentifier)
		if err != nil {
			return nil, ErrInvalidProgram
		}
		before, err := loadAdminProgram(ctx, tx, identifier, true)
		if err != nil {
			return nil, err
		}
		if before.ReviewStatus == ProgramStatusPublished {
			result = append(result, before)
			continue
		}
		if before.ReviewStatus != ProgramStatusPending {
			return nil, ErrInvalidStatus
		}
		if _, err := tx.Exec(ctx, `
			UPDATE academic_programs
			SET review_status = $4, updated_at = CURRENT_TIMESTAMP
			WHERE academic_year = $1 AND school_code = $2 AND program_code = $3
		`, identifier.AcademicYear, identifier.SchoolCode, identifier.ProgramCode, ProgramStatusPublished); err != nil {
			return nil, mapAdmissionRepositoryError(err)
		}
		after, err := loadAdminProgram(ctx, tx, identifier, false)
		if err != nil {
			return nil, err
		}
		if err := insertProgramAudit(ctx, tx, adminID, "publish", identifier.String(), adminProgramSnapshot(before), adminProgramSnapshot(after), reason); err != nil {
			return nil, err
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

// DeleteProgram removes an empty placeholder program (admission_quota == 0) and its
// exam-item/timeline-event rows; ON DELETE RESTRICT elsewhere guards real applicant data.
func (r *PostgresRepository) DeleteProgram(ctx context.Context, adminID uuid.UUID, identifier ProgramIdentifier, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" || len([]rune(reason)) > 2000 {
		return ErrInvalidProgram
	}
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return err
	} else if !ok {
		return ErrAdminRequired
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin admission program delete: %w", err)
	}
	defer tx.Rollback(ctx)
	before, err := loadAdminProgram(ctx, tx, identifier, true)
	if err != nil {
		return err
	}
	if before.AdmissionQuota != 0 {
		return ErrProgramNotDeletable
	}
	if _, err := tx.Exec(ctx, `DELETE FROM program_exam_items WHERE academic_year = $1 AND school_code = $2 AND program_code = $3`,
		identifier.AcademicYear, identifier.SchoolCode, identifier.ProgramCode); err != nil {
		return mapAdmissionRepositoryError(err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM program_timeline_events WHERE academic_year = $1 AND school_code = $2 AND program_code = $3`,
		identifier.AcademicYear, identifier.SchoolCode, identifier.ProgramCode); err != nil {
		return mapAdmissionRepositoryError(err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM academic_programs WHERE academic_year = $1 AND school_code = $2 AND program_code = $3`,
		identifier.AcademicYear, identifier.SchoolCode, identifier.ProgramCode); err != nil {
		return mapAdmissionRepositoryError(err)
	}
	if err := insertProgramAudit(ctx, tx, adminID, "delete", identifier.String(), adminProgramSnapshot(before), nil, reason); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit admission program delete: %w", err)
	}
	return nil
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
	item.Program.TimelineEvents, err = loadTimelineEvents(ctx, queryer, identifier.AcademicYear, identifier.SchoolCode, identifier.ProgramCode)
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
		&item.BrochureIsTentative,
		&item.ConsultationPhone, &item.ConsultationEmail, &item.ConsultationContact, &item.BrochureURL,
		&item.SpecialTalentTarget, &item.DifferentEducationBackgrounds,
		&item.DifferentEducationOther, &item.Notes, &item.SourceLocator,
		&item.ReviewStatus,
		&item.RegistrationFee, &item.ExamLocation,
		&item.RecommendationLetterDeadline, &item.PortfolioDeadline,
		&item.CheckinWaitlistProcess, &item.FeeReductionEligibility,
		&item.AdmissionGroup, &item.CrossGroup, &item.AdmissionCategory, &item.PriorityAdmission,
		&item.PortfolioRequired, &item.RecommendationLetterType, &item.MaxApplicablePrograms,
		&item.ApplicantCount, &item.InterviewCount, &item.AdmittedCount, &item.WaitlistedCount,
		&item.AdmissionRate, &item.FirstStagePassRate, &item.CompetitionRatio,
		&item.SchoolOfficialURL, &item.DepartmentOfficialURL,
		&item.CreatedAt, &item.UpdatedAt,
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
				(academic_year, school_code, program_code, item_name, exam_stage, sort_order, weight_percent, multiplier, description, source_page)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, program.AcademicYear, program.SchoolCode, program.ProgramCode, item.Name, normalizeDash(item.Stage), item.SortOrder, item.WeightPercent, item.Multiplier, item.Description, page); err != nil {
			return mapAdmissionRepositoryError(err)
		}
	}
	return nil
}

func replaceTimelineEvents(ctx context.Context, tx pgx.Tx, program Program) error {
	if _, err := tx.Exec(ctx, `DELETE FROM program_timeline_events WHERE academic_year = $1 AND school_code = $2 AND program_code = $3`, program.AcademicYear, program.SchoolCode, program.ProgramCode); err != nil {
		return fmt.Errorf("replace program timeline events: %w", err)
	}
	for _, event := range program.TimelineEvents {
		if _, err := tx.Exec(ctx, `
			INSERT INTO program_timeline_events
				(academic_year, school_code, program_code, event_name, start_date, start_time, end_date, end_time, sort_order, notes)
			VALUES ($1, $2, $3, $4, NULLIF($5, '-')::date, $6, NULLIF($7, '-')::date, $8, $9, $10)
		`, program.AcademicYear, program.SchoolCode, program.ProgramCode, event.Name, event.StartDate, normalizeDash(event.StartTime), event.EndDate, normalizeDash(event.EndTime), event.SortOrder, normalizeDash(event.Notes)); err != nil {
			return mapAdmissionRepositoryError(err)
		}
	}
	return nil
}

func programsEqual(before, after Program) bool {
	// willingness_values is result-derived; ignore it for the modified-check.
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
		"timeline_events":                 item.TimelineEvents,
		"brochure_is_tentative":           item.BrochureIsTentative,
		"consultation_phone":              item.ConsultationPhone,
		"consultation_email":              item.ConsultationEmail,
		"consultation_contact":            item.ConsultationContact,
		"brochure_url":                    item.BrochureURL,
		"special_talent_target":           item.SpecialTalentTarget,
		"different_education_backgrounds": item.DifferentEducationBackgrounds,
		"different_education_other":       item.DifferentEducationOther,
		"notes":                           item.Notes,
		"source_locator":                  item.SourceLocator,
		"review_status":                   item.ReviewStatus,
		"registration_fee":                item.RegistrationFee,
		"exam_location":                   item.ExamLocation,
		"recommendation_letter_deadline":  item.RecommendationLetterDeadline,
		"portfolio_deadline":              item.PortfolioDeadline,
		"checkin_waitlist_process":        item.CheckinWaitlistProcess,
		"fee_reduction_eligibility":       item.FeeReductionEligibility,
		"admission_group":                 item.AdmissionGroup,
		"cross_group":                     item.CrossGroup,
		"admission_category":              item.AdmissionCategory,
		"priority_admission":              item.PriorityAdmission,
		"portfolio_required":              item.PortfolioRequired,
		"recommendation_letter_type":      item.RecommendationLetterType,
		"max_applicable_programs":         item.MaxApplicablePrograms,
		"applicant_count":                 item.ApplicantCount,
		"interview_count":                 item.InterviewCount,
		"admitted_count":                  item.AdmittedCount,
		"waitlisted_count":                item.WaitlistedCount,
		"admission_rate":                  item.AdmissionRate,
		"first_stage_pass_rate":           item.FirstStagePassRate,
		"competition_ratio":               item.CompetitionRatio,
		"school_official_url":             item.SchoolOfficialURL,
		"department_official_url":         item.DepartmentOfficialURL,
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
		admission_quota, brochure_is_tentative,
		consultation_phone,
		brochure_url, special_talent_target, different_education_backgrounds,
		different_education_other, notes, source_page, review_status,
		consultation_email, consultation_contact,
		registration_fee, exam_location, recommendation_letter_deadline, portfolio_deadline,
		checkin_waitlist_process, fee_reduction_eligibility,
		admission_group, cross_group, admission_category, priority_admission,
		portfolio_required, recommendation_letter_type, max_applicable_programs,
		applicant_count, interview_count, admitted_count, waitlisted_count,
		admission_rate, first_stage_pass_rate, competition_ratio,
		school_official_url, department_official_url
	)
	VALUES (
		$1, $2, $3, $4, $5, $6,
		$7, $8, $9, $10, $11, $12, $13, $14, $15, $16,
		$17, $18, NULLIF($19, '-')::date, NULLIF($20, '-')::date, $21, $22,
		$23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33, $34, $35, $36,
		$37, $38
	)
	ON CONFLICT (academic_year, school_code, program_code) DO UPDATE SET
		admission_program_name = EXCLUDED.admission_program_name,
		admission_quota = EXCLUDED.admission_quota,
		brochure_is_tentative = EXCLUDED.brochure_is_tentative,
		consultation_phone = EXCLUDED.consultation_phone,
		brochure_url = EXCLUDED.brochure_url,
		special_talent_target = EXCLUDED.special_talent_target,
		different_education_backgrounds = EXCLUDED.different_education_backgrounds,
		different_education_other = EXCLUDED.different_education_other,
		notes = EXCLUDED.notes,
		source_page = EXCLUDED.source_page,
		review_status = EXCLUDED.review_status,
		consultation_email = EXCLUDED.consultation_email,
		consultation_contact = EXCLUDED.consultation_contact,
		registration_fee = EXCLUDED.registration_fee,
		exam_location = EXCLUDED.exam_location,
		recommendation_letter_deadline = EXCLUDED.recommendation_letter_deadline,
		portfolio_deadline = EXCLUDED.portfolio_deadline,
		checkin_waitlist_process = EXCLUDED.checkin_waitlist_process,
		fee_reduction_eligibility = EXCLUDED.fee_reduction_eligibility,
		admission_group = EXCLUDED.admission_group,
		cross_group = EXCLUDED.cross_group,
		admission_category = EXCLUDED.admission_category,
		priority_admission = EXCLUDED.priority_admission,
		portfolio_required = EXCLUDED.portfolio_required,
		recommendation_letter_type = EXCLUDED.recommendation_letter_type,
		max_applicable_programs = EXCLUDED.max_applicable_programs,
		applicant_count = EXCLUDED.applicant_count,
		interview_count = EXCLUDED.interview_count,
		admitted_count = EXCLUDED.admitted_count,
		waitlisted_count = EXCLUDED.waitlisted_count,
		admission_rate = EXCLUDED.admission_rate,
		first_stage_pass_rate = EXCLUDED.first_stage_pass_rate,
		competition_ratio = EXCLUDED.competition_ratio,
		school_official_url = EXCLUDED.school_official_url,
		department_official_url = EXCLUDED.department_official_url,
		updated_at = CURRENT_TIMESTAMP`
