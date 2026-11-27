package admissions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"sta-backend/internal/auth"
)

type ProgramLinkReview struct {
	AcademicYear                 int    `json:"academic_year"`
	ProgramIdentifier            string `json:"program_identifier"`
	SchoolCode                   string `json:"school_code"`
	SchoolName                   string `json:"school_name"`
	ProgramCode                  string `json:"program_code"`
	ProgramName                  string `json:"program_name"`
	CurrentSchoolURL             string `json:"current_school_url"`
	CurrentDepartmentURL         string `json:"current_department_url"`
	CurrentColleGoURL            string `json:"current_collego_url"`
	CandidateSchoolURL           string `json:"candidate_school_url"`
	CandidateSchoolSourceURL     string `json:"candidate_school_source_url"`
	CandidateDepartmentURL       string `json:"candidate_department_url"`
	CandidateDepartmentSourceURL string `json:"candidate_department_source_url"`
	CandidateColleGoURL          string `json:"candidate_collego_url"`
	CandidateColleGoSourceURL    string `json:"candidate_collego_source_url"`
	ColleGoMatchStatus           string `json:"collego_match_status"`
	Status                       string `json:"status"`
	Unpublished                  bool   `json:"unpublished"`
	UpdatedAt                    string `json:"updated_at"`
}

type ProgramLinkQuery struct {
	Status      string
	Search      string
	SchoolCode  string
	ProgramCode string
	Limit       int
	Offset      int
}

type ProgramLinkCandidateInput struct {
	SchoolURL           string `json:"school_url"`
	SchoolSourceURL     string `json:"school_source_url"`
	DepartmentURL       string `json:"department_url"`
	DepartmentSourceURL string `json:"department_source_url"`
	ColleGoURL          string `json:"collego_url"`
	ColleGoSourceURL    string `json:"collego_source_url"`
}

type ProgramLinksRepository interface {
	IsAdmin(context.Context, uuid.UUID) (bool, error)
	ListProgramLinkReviews(context.Context, uuid.UUID, ProgramLinkQuery) ([]ProgramLinkReview, error)
	SaveProgramLinkDraft(context.Context, uuid.UUID, string, string, ProgramLinkCandidateInput) (ProgramLinkReview, error)
	PublishProgramLinkDraft(context.Context, uuid.UUID, string, string, string) ([]string, error)
	UnpublishProgramLinks(context.Context, uuid.UUID, string, string) ([]string, error)
	RejectProgramLinkDraft(context.Context, uuid.UUID, string, string, string) error
}

type ProgramLinksHandler struct {
	authService  *auth.Service
	repository   ProgramLinksRepository
	invalidators []Invalidator
}

func NewProgramLinksHandler(authService *auth.Service, repository ProgramLinksRepository, invalidators ...Invalidator) (*ProgramLinksHandler, error) {
	if authService == nil || repository == nil {
		return nil, errors.New("admission program links handler dependencies are missing")
	}
	activeInvalidators := make([]Invalidator, 0, len(invalidators))
	for _, invalidator := range invalidators {
		if invalidator != nil {
			activeInvalidators = append(activeInvalidators, invalidator)
		}
	}
	return &ProgramLinksHandler{
		authService:  authService,
		repository:   repository,
		invalidators: activeInvalidators,
	}, nil
}

func (h *ProgramLinksHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/admin/admissions/program-links", h.list)
	mux.HandleFunc("PUT /api/v1/admin/admissions/program-links/{schoolCode}/{programCode}", h.saveDraft)
	mux.HandleFunc("POST /api/v1/admin/admissions/program-links/{schoolCode}/{programCode}/publish", h.publish)
	mux.HandleFunc("POST /api/v1/admin/admissions/program-links/{schoolCode}/{programCode}/unpublish", h.unpublish)
	mux.HandleFunc("POST /api/v1/admin/admissions/program-links/{schoolCode}/{programCode}/reject", h.reject)
}

func (h *ProgramLinksHandler) list(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r, false)
	if !ok {
		return
	}
	query, err := parseProgramLinkQuery(r)
	if err != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_query", "program link query is invalid")
		return
	}
	items, err := h.repository.ListProgramLinkReviews(r.Context(), session.Session.Account.ID, query)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeAdmissionJSON(w, http.StatusOK, map[string]any{
		"data": items,
		"meta": map[string]any{"limit": query.Limit, "offset": query.Offset, "count": len(items)},
	})
}

func (h *ProgramLinksHandler) saveDraft(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r, true)
	if !ok {
		return
	}
	schoolCode, programCode, ok := parseProgramLinkPath(w, r)
	if !ok {
		return
	}
	var input ProgramLinkCandidateInput
	if err := decodeBrochureJSON(r, &input); err != nil || input.Validate() != nil {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_program_links", "program link candidate data is invalid")
		return
	}
	item, err := h.repository.SaveProgramLinkDraft(r.Context(), session.Session.Account.ID, schoolCode, programCode, input.normalize())
	if err != nil {
		h.writeError(w, err)
		return
	}
	writeAdmissionJSON(w, http.StatusOK, map[string]any{"data": item})
}

func (h *ProgramLinksHandler) publish(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r, true)
	if !ok {
		return
	}
	schoolCode, programCode, ok := parseProgramLinkPath(w, r)
	if !ok {
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	if err := decodeBrochureJSON(r, &input); err != nil || len([]rune(strings.TrimSpace(input.Reason))) > 2000 {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_program_links", "publish reason is invalid")
		return
	}
	identifiers, err := h.repository.PublishProgramLinkDraft(r.Context(), session.Session.Account.ID, schoolCode, programCode, input.Reason)
	if err != nil {
		h.writeError(w, err)
		return
	}
	for _, invalidator := range h.invalidators {
		invalidator.Invalidate(r.Context(), identifiers...)
	}
	writeAdmissionJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "published"}})
}

func (h *ProgramLinksHandler) unpublish(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r, true)
	if !ok {
		return
	}
	schoolCode, programCode, ok := parseProgramLinkPath(w, r)
	if !ok {
		return
	}
	identifiers, err := h.repository.UnpublishProgramLinks(r.Context(), session.Session.Account.ID, schoolCode, programCode)
	if err != nil {
		h.writeError(w, err)
		return
	}
	for _, invalidator := range h.invalidators {
		invalidator.Invalidate(r.Context(), identifiers...)
	}
	writeAdmissionJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "unpublished"}})
}

func (h *ProgramLinksHandler) reject(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdmin(w, r, true)
	if !ok {
		return
	}
	schoolCode, programCode, ok := parseProgramLinkPath(w, r)
	if !ok {
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	if err := decodeBrochureJSON(r, &input); err != nil || strings.TrimSpace(input.Reason) == "" || len([]rune(input.Reason)) > 2000 {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_program_links", "a reason is required to reject a candidate")
		return
	}
	if err := h.repository.RejectProgramLinkDraft(r.Context(), session.Session.Account.ID, schoolCode, programCode, input.Reason); err != nil {
		h.writeError(w, err)
		return
	}
	writeAdmissionJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "rejected"}})
}

func (h *ProgramLinksHandler) requireAdmin(w http.ResponseWriter, r *http.Request, mutation bool) (auth.RequestSession, bool) {
	session, err := h.authService.Authenticate(r.Context(), r)
	if err != nil {
		if errors.Is(err, auth.ErrAdminMFARequired) || errors.Is(err, auth.ErrAdminMFAInvalid) {
			writeAdmissionError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
			return auth.RequestSession{}, false
		}
		writeAdmissionError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return auth.RequestSession{}, false
	}
	isAdmin, err := h.repository.IsAdmin(r.Context(), session.Session.Account.ID)
	if err != nil {
		writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return auth.RequestSession{}, false
	}
	if !isAdmin {
		writeAdmissionError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
		return auth.RequestSession{}, false
	}
	if mutation {
		if err := h.authService.AuthorizeMutation(r, session); err != nil {
			writeAdmissionError(w, http.StatusForbidden, "csrf_required", "request verification failed")
			return auth.RequestSession{}, false
		}
	}
	if err := h.authService.RequireAdminMFA(r.Context(), session.Session.Account.ID, r.Header.Get("X-MFA-Code")); err != nil {
		writeAdmissionError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *ProgramLinksHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidIdentifier), errors.Is(err, ErrInvalidProgram):
		writeAdmissionError(w, http.StatusBadRequest, "invalid_program_links", "program link data is invalid")
	case errors.Is(err, ErrAdminRequired):
		writeAdmissionError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
	case errors.Is(err, ErrInvalidStatus):
		writeAdmissionError(w, http.StatusConflict, "invalid_status", "program link candidate state does not allow this operation")
	case errors.Is(err, ErrNotFound):
		writeAdmissionError(w, http.StatusNotFound, "not_found", "program link candidate or program was not found")
	default:
		writeAdmissionError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func parseProgramLinkQuery(r *http.Request) (ProgramLinkQuery, error) {
	query := ProgramLinkQuery{
		Status: strings.TrimSpace(r.URL.Query().Get("status")),
		Search: strings.TrimSpace(r.URL.Query().Get("q")),
		Limit:  50,
	}
	if query.Status == "" {
		query.Status = "all"
	}
	if query.Status != "all" && query.Status != "draft" && query.Status != "published" && query.Status != "unpublished" && query.Status != "rejected" {
		return ProgramLinkQuery{}, ErrInvalidProgram
	}
	if len([]rune(query.Search)) > 100 {
		return ProgramLinkQuery{}, ErrInvalidProgram
	}
	var err error
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		query.Limit, err = strconv.Atoi(raw)
		if err != nil || query.Limit < 1 || query.Limit > 100 {
			return ProgramLinkQuery{}, ErrInvalidProgram
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		query.Offset, err = strconv.Atoi(raw)
		if err != nil || query.Offset < 0 || query.Offset > 1_000_000 {
			return ProgramLinkQuery{}, ErrInvalidProgram
		}
	}
	return query, nil
}

func parseProgramLinkPath(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	schoolCode := strings.TrimSpace(r.PathValue("schoolCode"))
	programCode := strings.TrimSpace(r.PathValue("programCode"))
	if !validSchoolCode(schoolCode) || !validProgramCode(programCode) {
		writeAdmissionError(w, http.StatusBadRequest, "invalid_program", "school or program code is invalid")
		return "", "", false
	}
	return schoolCode, programCode, true
}

func (input ProgramLinkCandidateInput) Validate() error {
	input = input.normalize()
	for _, raw := range []string{
		input.SchoolURL, input.SchoolSourceURL,
		input.DepartmentURL, input.DepartmentSourceURL,
		input.ColleGoURL, input.ColleGoSourceURL,
	} {
		if len(raw) > 2048 || ValidateOfficialURL(raw) != nil {
			return ErrInvalidProgram
		}
	}
	return nil
}

func (input ProgramLinkCandidateInput) normalize() ProgramLinkCandidateInput {
	input.SchoolURL = normalizeOptionalLink(input.SchoolURL)
	input.SchoolSourceURL = normalizeOptionalLink(input.SchoolSourceURL)
	input.DepartmentURL = normalizeOptionalLink(input.DepartmentURL)
	input.DepartmentSourceURL = normalizeOptionalLink(input.DepartmentSourceURL)
	input.ColleGoURL = normalizeOptionalLink(input.ColleGoURL)
	input.ColleGoSourceURL = normalizeOptionalLink(input.ColleGoSourceURL)
	return input
}

func normalizeOptionalLink(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == MissingValue {
		return ""
	}
	return raw
}

func (input ProgramLinkCandidateInput) asMap() map[string]any {
	return map[string]any{
		"school_url": input.SchoolURL, "school_source_url": input.SchoolSourceURL,
		"department_url": input.DepartmentURL, "department_source_url": input.DepartmentSourceURL,
		"collego_url": input.ColleGoURL, "collego_source_url": input.ColleGoSourceURL,
	}
}

func (r *PostgresRepository) ListProgramLinkReviews(ctx context.Context, adminID uuid.UUID, query ProgramLinkQuery) ([]ProgramLinkReview, error) {
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrAdminRequired
	}
	if query.Limit < 1 || query.Limit > 100 || query.Offset < 0 {
		return nil, ErrInvalidProgram
	}
	displayYear, _, err := r.GetDisplayLatestAcademicYear(ctx)
	if err != nil {
		return nil, err
	}
	args := []any{displayYear}
	conditions := []string{"TRUE"}
	if query.Status == "unpublished" {
		conditions = append(conditions, "l.unpublished")
	} else if query.Status == "published" {
		args = append(args, query.Status)
		conditions = append(conditions, fmt.Sprintf("l.status = $%d AND NOT l.unpublished", len(args)))
	} else if query.Status != "all" {
		args = append(args, query.Status)
		conditions = append(conditions, fmt.Sprintf("l.status = $%d", len(args)))
	}
	if query.SchoolCode != "" {
		args = append(args, query.SchoolCode)
		conditions = append(conditions, fmt.Sprintf("l.school_code = $%d", len(args)))
	}
	if query.ProgramCode != "" {
		args = append(args, query.ProgramCode)
		conditions = append(conditions, fmt.Sprintf("l.program_code = $%d", len(args)))
	}
	if query.Search != "" {
		args = append(args, "%"+escapeLike(normalizeTaiVariant(query.Search))+"%")
		position := len(args)
		conditions = append(conditions, fmt.Sprintf("(REPLACE(l.school_name, '臺', '台') ILIKE $%d ESCAPE '\\' OR REPLACE(l.program_name, '臺', '台') ILIKE $%d ESCAPE '\\' OR l.school_code ILIKE $%d OR l.program_code ILIKE $%d)", position, position, position, position))
	}
	args = append(args, query.Limit, query.Offset)
	limitPosition, offsetPosition := len(args)-1, len(args)
	statement := fmt.Sprintf(`
		WITH ranked AS (
			SELECT DISTINCT ON (p.school_code, p.program_code)
				p.academic_year, p.program_identifier, p.school_code, s.school_name,
				p.program_code, p.admission_program_name AS program_name,
				COALESCE(NULLIF(school_link.url, ''), NULLIF(p.school_official_url, '-'), '') AS current_school_url,
				COALESCE(NULLIF(department_link.url, ''), NULLIF(p.department_official_url, '-'), '') AS current_department_url,
				COALESCE(collego_link.url, '') AS current_collego_url,
				COALESCE(c.school_url, '') AS candidate_school_url,
				COALESCE(c.school_source_url, '') AS candidate_school_source_url,
				COALESCE(c.department_url, '') AS candidate_department_url,
				COALESCE(c.department_source_url, '') AS candidate_department_source_url,
				COALESCE(c.collego_url, '') AS candidate_collego_url,
				COALESCE(c.collego_source_url, '') AS candidate_collego_source_url,
				COALESCE(c.collego_match_status, '') AS collego_match_status,
				COALESCE(c.status, '') AS status,
				COALESCE(c.unpublished, FALSE) AS unpublished,
				COALESCE(c.updated_at::text, '') AS updated_at
			FROM academic_programs p
			JOIN schools s ON s.school_code = p.school_code
			LEFT JOIN admissions_school_official_links school_link ON school_link.school_code = p.school_code
			LEFT JOIN admissions_program_official_links department_link
				ON department_link.school_code = p.school_code AND department_link.program_code = p.program_code AND department_link.link_type = 'department'
			LEFT JOIN admissions_program_official_links collego_link
				ON collego_link.school_code = p.school_code AND collego_link.program_code = p.program_code AND collego_link.link_type = 'collegego'
			LEFT JOIN admissions_program_link_candidates c
				ON c.school_code = p.school_code AND c.program_code = p.program_code
			WHERE p.academic_year <= $1 AND p.review_status = 'published' AND p.admission_quota > 0
			ORDER BY p.school_code, p.program_code, p.academic_year DESC
		), filtered AS (
			SELECT * FROM ranked l WHERE %s
		)
		SELECT academic_year, program_identifier, school_code, school_name, program_code, program_name,
			current_school_url, current_department_url, current_collego_url,
			candidate_school_url, candidate_school_source_url, candidate_department_url,
			candidate_department_source_url, candidate_collego_url, candidate_collego_source_url,
			collego_match_status, status, unpublished, updated_at
		FROM filtered
		ORDER BY school_name, program_name, school_code, program_code
		LIMIT $%d OFFSET $%d
	`, strings.Join(conditions, " AND "), limitPosition, offsetPosition)
	rows, err := r.pool.Query(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("list admission program link reviews: %w", err)
	}
	defer rows.Close()
	items := make([]ProgramLinkReview, 0, query.Limit)
	for rows.Next() {
		var item ProgramLinkReview
		if err := rows.Scan(
			&item.AcademicYear, &item.ProgramIdentifier, &item.SchoolCode, &item.SchoolName,
			&item.ProgramCode, &item.ProgramName, &item.CurrentSchoolURL, &item.CurrentDepartmentURL,
			&item.CurrentColleGoURL, &item.CandidateSchoolURL, &item.CandidateSchoolSourceURL,
			&item.CandidateDepartmentURL, &item.CandidateDepartmentSourceURL, &item.CandidateColleGoURL,
			&item.CandidateColleGoSourceURL, &item.ColleGoMatchStatus, &item.Status, &item.Unpublished, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan admission program link review: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admission program link reviews: %w", err)
	}
	return items, nil
}

func (r *PostgresRepository) SaveProgramLinkDraft(ctx context.Context, adminID uuid.UUID, schoolCode, programCode string, input ProgramLinkCandidateInput) (ProgramLinkReview, error) {
	if !validSchoolCode(schoolCode) || !validProgramCode(programCode) || input.Validate() != nil {
		return ProgramLinkReview{}, ErrInvalidProgram
	}
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return ProgramLinkReview{}, err
	} else if !ok {
		return ProgramLinkReview{}, ErrAdminRequired
	}
	input = input.normalize()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ProgramLinkReview{}, fmt.Errorf("begin admission program link draft: %w", err)
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM academic_programs WHERE school_code = $1 AND program_code = $2 AND review_status = 'published'
	)`, schoolCode, programCode).Scan(&exists); err != nil {
		return ProgramLinkReview{}, fmt.Errorf("check admission program for link draft: %w", err)
	}
	if !exists {
		return ProgramLinkReview{}, ErrNotFound
	}
	var beforeJSON []byte
	err = tx.QueryRow(ctx, `SELECT jsonb_build_object(
		'school_url', school_url, 'school_source_url', school_source_url,
		'department_url', department_url, 'department_source_url', department_source_url,
		'collego_url', collego_url, 'collego_source_url', collego_source_url,
		'collego_match_status', collego_match_status, 'status', status
	) FROM admissions_program_link_candidates WHERE school_code = $1 AND program_code = $2 FOR UPDATE`, schoolCode, programCode).Scan(&beforeJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		beforeJSON = []byte("null")
	} else if err != nil {
		return ProgramLinkReview{}, fmt.Errorf("read existing admission program link draft: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO admissions_program_link_candidates (
			school_code, program_code, school_url, school_source_url, department_url,
			department_source_url, collego_url, collego_source_url,
			status, updated_by, reviewed_by, review_reason, reviewed_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'draft', $9, NULL, '', NULL, CURRENT_TIMESTAMP)
		ON CONFLICT (school_code, program_code) DO UPDATE SET
			school_url = EXCLUDED.school_url,
			school_source_url = EXCLUDED.school_source_url,
			department_url = EXCLUDED.department_url,
			department_source_url = EXCLUDED.department_source_url,
			collego_url = EXCLUDED.collego_url,
			collego_source_url = EXCLUDED.collego_source_url,
			status = 'draft', updated_by = EXCLUDED.updated_by,
			reviewed_by = NULL, review_reason = '', reviewed_at = NULL,
			updated_at = CURRENT_TIMESTAMP
	`, schoolCode, programCode, input.SchoolURL, input.SchoolSourceURL, input.DepartmentURL,
		input.DepartmentSourceURL, input.ColleGoURL, input.ColleGoSourceURL, adminID); err != nil {
		return ProgramLinkReview{}, fmt.Errorf("save admission program link draft: %w", err)
	}
	afterJSON, err := json.Marshal(input.asMap())
	if err != nil {
		return ProgramLinkReview{}, fmt.Errorf("encode admission program link draft audit: %w", err)
	}
	if err := insertProgramLinkAudit(ctx, tx, adminID, "program_links.draft", schoolCode+"-"+programCode, beforeJSON, afterJSON, "存入校系連結候選"); err != nil {
		return ProgramLinkReview{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ProgramLinkReview{}, fmt.Errorf("commit admission program link draft: %w", err)
	}
	items, err := r.ListProgramLinkReviews(ctx, adminID, ProgramLinkQuery{
		Status: "all", SchoolCode: schoolCode, ProgramCode: programCode, Limit: 100,
	})
	if err != nil {
		return ProgramLinkReview{}, err
	}
	for _, item := range items {
		if item.SchoolCode == schoolCode && item.ProgramCode == programCode {
			return item, nil
		}
	}
	return ProgramLinkReview{}, ErrNotFound
}

func (r *PostgresRepository) PublishProgramLinkDraft(ctx context.Context, adminID uuid.UUID, schoolCode, programCode, reason string) ([]string, error) {
	if !validSchoolCode(schoolCode) || !validProgramCode(programCode) || len([]rune(strings.TrimSpace(reason))) > 2000 {
		return nil, ErrInvalidProgram
	}
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrAdminRequired
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin publish admission program links: %w", err)
	}
	defer tx.Rollback(ctx)
	var input ProgramLinkCandidateInput
	var status string
	var unpublished bool
	if err := tx.QueryRow(ctx, `SELECT school_url, school_source_url, department_url, department_source_url,
		collego_url, collego_source_url, status, unpublished
		FROM admissions_program_link_candidates WHERE school_code = $1 AND program_code = $2 FOR UPDATE`,
		schoolCode, programCode).Scan(&input.SchoolURL, &input.SchoolSourceURL, &input.DepartmentURL,
		&input.DepartmentSourceURL, &input.ColleGoURL, &input.ColleGoSourceURL, &status, &unpublished); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("load admission program link draft: %w", err)
	}
	if status != "draft" && status != "published" {
		return nil, ErrInvalidStatus
	}
	input = input.normalize()
	if input.SchoolURL == "" && input.DepartmentURL == "" && input.ColleGoURL == "" && !unpublished {
		return nil, ErrInvalidProgram
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT program_identifier FROM academic_programs
		WHERE school_code = $1 AND program_code = $2 AND review_status = 'published'`, schoolCode, programCode)
	if err != nil {
		return nil, fmt.Errorf("resolve published admission program identifiers: %w", err)
	}
	programIdentifiers := make([]string, 0)
	for rows.Next() {
		var identifier string
		if err := rows.Scan(&identifier); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan published admission program identifier: %w", err)
		}
		programIdentifiers = append(programIdentifiers, identifier)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate published admission program identifiers: %w", err)
	}
	rows.Close()
	if len(programIdentifiers) == 0 {
		return nil, ErrNotFound
	}
	before, err := loadCurrentProgramLinks(ctx, tx, schoolCode, programCode)
	if err != nil {
		return nil, err
	}
	if input.SchoolURL != "" {
		if _, err := tx.Exec(ctx, `INSERT INTO admissions_school_official_links (school_code, url, updated_by, updated_at)
			VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
			ON CONFLICT (school_code) DO UPDATE SET url = EXCLUDED.url, updated_by = EXCLUDED.updated_by, updated_at = CURRENT_TIMESTAMP`, schoolCode, input.SchoolURL, adminID); err != nil {
			return nil, fmt.Errorf("publish school official link: %w", err)
		}
	}
	for _, link := range []struct{ kind, value string }{{"department", input.DepartmentURL}, {"collegego", input.ColleGoURL}} {
		if link.value == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO admissions_program_official_links (school_code, program_code, link_type, url, updated_by, updated_at)
			VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP)
			ON CONFLICT (school_code, program_code, link_type) DO UPDATE SET url = EXCLUDED.url, updated_by = EXCLUDED.updated_by, updated_at = CURRENT_TIMESTAMP`, schoolCode, programCode, link.kind, link.value, adminID); err != nil {
			return nil, fmt.Errorf("publish program official link: %w", err)
		}
	}
	after := input.asMap()
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		return nil, fmt.Errorf("encode current program links for audit: %w", err)
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		return nil, fmt.Errorf("encode published program links for audit: %w", err)
	}
	if err := insertProgramLinkAudit(ctx, tx, adminID, "program_links.publish", schoolCode+"-"+programCode, beforeJSON, afterJSON, strings.TrimSpace(reason)); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE admissions_program_link_candidates SET status = 'published', unpublished = FALSE, updated_by = $3,
		reviewed_by = $3, review_reason = $4, reviewed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE school_code = $1 AND program_code = $2`, schoolCode, programCode, adminID, strings.TrimSpace(reason)); err != nil {
		return nil, fmt.Errorf("mark admission program links published: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit published admission program links: %w", err)
	}
	return programIdentifiers, nil
}

func (r *PostgresRepository) UnpublishProgramLinks(ctx context.Context, adminID uuid.UUID, schoolCode, programCode string) ([]string, error) {
	if !validSchoolCode(schoolCode) || !validProgramCode(programCode) {
		return nil, ErrInvalidProgram
	}
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrAdminRequired
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin unpublish admission program links: %w", err)
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `SELECT DISTINCT program_identifier FROM academic_programs
		WHERE school_code = $1 AND program_code = $2 AND review_status = 'published'`, schoolCode, programCode)
	if err != nil {
		return nil, fmt.Errorf("resolve published admission program identifiers for unpublish: %w", err)
	}
	programIdentifiers := make([]string, 0)
	for rows.Next() {
		var identifier string
		if err := rows.Scan(&identifier); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan published admission program identifier for unpublish: %w", err)
		}
		programIdentifiers = append(programIdentifiers, identifier)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate published admission program identifiers for unpublish: %w", err)
	}
	rows.Close()
	if len(programIdentifiers) == 0 {
		return nil, ErrNotFound
	}

	before, err := loadCurrentProgramLinks(ctx, tx, schoolCode, programCode)
	if err != nil {
		return nil, err
	}
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		return nil, fmt.Errorf("encode current program links for unpublish audit: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO admissions_program_link_candidates (
		school_code, program_code, status, unpublished, updated_by, reviewed_by, review_reason, reviewed_at, updated_at
	) VALUES ($1, $2, 'published', TRUE, $3, $3, '管理員下架校系網站連結', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	ON CONFLICT (school_code, program_code) DO UPDATE SET
		unpublished = TRUE, updated_by = EXCLUDED.updated_by, reviewed_by = EXCLUDED.reviewed_by,
		review_reason = EXCLUDED.review_reason, reviewed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP`,
		schoolCode, programCode, adminID); err != nil {
		return nil, fmt.Errorf("mark admission program links unpublished: %w", err)
	}
	afterJSON, err := json.Marshal(map[string]any{"unpublished": true})
	if err != nil {
		return nil, fmt.Errorf("encode unpublished program links audit: %w", err)
	}
	if err := insertProgramLinkAudit(ctx, tx, adminID, "program_links.unpublish", schoolCode+"-"+programCode, beforeJSON, afterJSON, "管理員下架校系網站連結"); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit unpublished admission program links: %w", err)
	}
	return programIdentifiers, nil
}

func (r *PostgresRepository) RejectProgramLinkDraft(ctx context.Context, adminID uuid.UUID, schoolCode, programCode, reason string) error {
	if !validSchoolCode(schoolCode) || !validProgramCode(programCode) || strings.TrimSpace(reason) == "" || len([]rune(reason)) > 2000 {
		return ErrInvalidProgram
	}
	if ok, err := r.IsAdmin(ctx, adminID); err != nil {
		return err
	} else if !ok {
		return ErrAdminRequired
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin reject admission program links: %w", err)
	}
	defer tx.Rollback(ctx)
	var before []byte
	if err := tx.QueryRow(ctx, `SELECT jsonb_build_object('school_url', school_url, 'school_source_url', school_source_url,
		'department_url', department_url, 'department_source_url', department_source_url,
		'collego_url', collego_url, 'collego_source_url', collego_source_url, 'status', status)
		FROM admissions_program_link_candidates WHERE school_code = $1 AND program_code = $2 AND status = 'draft' FOR UPDATE`,
		schoolCode, programCode).Scan(&before); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("load admission program link draft for rejection: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE admissions_program_link_candidates SET status = 'rejected', updated_by = $3,
		reviewed_by = $3, review_reason = $4, reviewed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE school_code = $1 AND program_code = $2`, schoolCode, programCode, adminID, strings.TrimSpace(reason)); err != nil {
		return fmt.Errorf("reject admission program link draft: %w", err)
	}
	if err := insertProgramLinkAudit(ctx, tx, adminID, "program_links.reject", schoolCode+"-"+programCode, before, nil, strings.TrimSpace(reason)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit rejected admission program links: %w", err)
	}
	return nil
}

func loadCurrentProgramLinks(ctx context.Context, tx pgx.Tx, schoolCode, programCode string) (map[string]string, error) {
	var links map[string]string
	if err := tx.QueryRow(ctx, `SELECT jsonb_build_object(
		'school_url', COALESCE(NULLIF(school_link.url, ''), NULLIF(p.school_official_url, '-'), ''),
		'department_url', COALESCE(NULLIF(department_link.url, ''), NULLIF(p.department_official_url, '-'), ''),
		'collego_url', COALESCE(collego_link.url, '')
	) FROM academic_programs p
	LEFT JOIN admissions_school_official_links school_link ON school_link.school_code = p.school_code
	LEFT JOIN admissions_program_official_links department_link ON department_link.school_code = p.school_code AND department_link.program_code = p.program_code AND department_link.link_type = 'department'
	LEFT JOIN admissions_program_official_links collego_link ON collego_link.school_code = p.school_code AND collego_link.program_code = p.program_code AND collego_link.link_type = 'collegego'
	WHERE p.school_code = $1 AND p.program_code = $2 AND p.review_status = 'published'
	ORDER BY p.academic_year DESC LIMIT 1`, schoolCode, programCode).Scan(&links); err != nil {
		return nil, fmt.Errorf("load current official links: %w", err)
	}
	return links, nil
}

func insertProgramLinkAudit(ctx context.Context, tx pgx.Tx, adminID uuid.UUID, action, entityKey string, before, after []byte, reason string) error {
	if len(before) == 0 {
		before = []byte("null")
	}
	if len(after) == 0 {
		after = []byte("null")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_log (actor_account_id, action, entity_type, entity_key, before_data, after_data, reason)
		VALUES ($1, $2, 'admission_program_link', $3, NULLIF($4, 'null')::jsonb, NULLIF($5, 'null')::jsonb, $6)`,
		adminID, action, entityKey, string(before), string(after), reason); err != nil {
		return fmt.Errorf("record admission program link audit event: %w", err)
	}
	return nil
}

var _ ProgramLinksRepository = (*PostgresRepository)(nil)
