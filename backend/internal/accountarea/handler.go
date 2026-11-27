// Package accountarea contains the persistent account settings and public
// profile endpoints used by the account UI.
package accountarea

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"sta-backend/internal/auth"
)

type Handler struct {
	auth *auth.Service
	pool *pgxpool.Pool
}

func NewHandler(authService *auth.Service, pool *pgxpool.Pool) (*Handler, error) {
	if authService == nil || pool == nil {
		return nil, errors.New("account area dependencies are missing")
	}
	return &Handler{auth: authService, pool: pool}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/account-schools", h.listPublicSchoolDirectory)
	mux.HandleFunc("GET /api/v1/admin/account-schools", h.listAdminSchoolDirectory)
	mux.HandleFunc("POST /api/v1/admin/account-schools", h.createSchoolDirectoryEntry)
	mux.HandleFunc("PUT /api/v1/admin/account-schools/{entryID}", h.updateSchoolDirectoryEntry)
	mux.HandleFunc("GET /api/v1/profile/details", h.getMyDetails)
	mux.HandleFunc("PUT /api/v1/profile/details", h.putMyDetails)
	mux.HandleFunc("GET /api/v1/profile/privacy", h.getMyPrivacy)
	mux.HandleFunc("PUT /api/v1/profile/privacy", h.putMyPrivacy)
	mux.HandleFunc("GET /api/v1/users/resolve", h.resolveUser)
	mux.HandleFunc("GET /api/v1/users/{username}/details", h.getPublicDetails)
	mux.HandleFunc("GET /api/v1/users/{username}/portfolio", h.getPublicPortfolio)
	mux.HandleFunc("GET /api/v1/users/{username}/experiences", h.getPublicExperiences)
	mux.HandleFunc("GET /api/v1/me/experiences", h.getMyExperiences)
	mux.HandleFunc("GET /api/v1/experiences/tags", h.getExperienceTags)
	mux.HandleFunc("GET /api/v1/saved-programs", h.listSavedPrograms)
	mux.HandleFunc("GET /api/v1/saved-programs/{identifier}", h.getSavedProgram)
	mux.HandleFunc("PUT /api/v1/saved-programs/{identifier}", h.saveProgram)
	mux.HandleFunc("DELETE /api/v1/saved-programs/{identifier}", h.unsaveProgram)
	mux.HandleFunc("GET /api/v1/notifications/preferences", h.getNotificationPreferences)
	mux.HandleFunc("PUT /api/v1/notifications/preferences", h.putNotificationPreferences)
	mux.HandleFunc("GET /api/v1/auth/identities", h.listIdentities)
	mux.HandleFunc("DELETE /api/v1/verification/requests/{requestID}", h.cancelVerification)
	mux.HandleFunc("POST /api/v1/verification/requests/{requestID}/revoke", h.revokeVerification)
}

func (h *Handler) session(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, err := h.auth.Authenticate(r.Context(), r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *Handler) mutationSession(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, ok := h.session(w, r)
	if !ok {
		return auth.RequestSession{}, false
	}
	if err := h.auth.AuthorizeMutation(r, session); err != nil {
		writeError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return auth.RequestSession{}, false
	}
	return session, true
}

type profileDetails struct {
	SchoolName          string     `json:"school_name"`
	SchoolSelectionType string     `json:"school_selection_type"`
	SchoolDirectoryID   *uuid.UUID `json:"school_directory_id"`
	Department          string     `json:"department"`
	HideSchool          bool       `json:"hide_school"`
	HideDepartment      bool       `json:"hide_department"`
	ProfileSlug         *string    `json:"profile_slug"`
}

type privacySettings struct {
	ProfileVisibility string `json:"profile_visibility"`
	ShowArticles      bool   `json:"show_articles"`
	ShowPortfolio     bool   `json:"show_portfolio"`
}

func (h *Handler) getMyDetails(w http.ResponseWriter, r *http.Request) {
	session, ok := h.session(w, r)
	if !ok {
		return
	}
	item, err := h.myDetails(r, session.Session.Account.ID)
	if err != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	writeJSON(w, 200, map[string]any{"data": item})
}

func (h *Handler) myDetails(r *http.Request, accountID uuid.UUID) (profileDetails, error) {
	var item profileDetails
	err := h.pool.QueryRow(r.Context(), `
		SELECT COALESCE(d.school_name, p.school_name, ''), p.school_selection_type, p.school_directory_id,
			COALESCE(p.department,''), p.hide_school, p.hide_department, p.profile_slug
		FROM account_profiles p
		LEFT JOIN account_school_directory d ON d.id = p.school_directory_id
		WHERE p.account_id = $1`, accountID).Scan(
		&item.SchoolName, &item.SchoolSelectionType, &item.SchoolDirectoryID,
		&item.Department, &item.HideSchool, &item.HideDepartment, &item.ProfileSlug)
	if errors.Is(err, pgx.ErrNoRows) {
		return profileDetails{SchoolSelectionType: "none", ProfileSlug: nil}, nil
	}
	return item, err
}

var slugPattern = regexp.MustCompile(`^[a-z0-9_-]{3,30}$`)

func (h *Handler) putMyDetails(w http.ResponseWriter, r *http.Request) {
	session, ok := h.mutationSession(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var item profileDetails
	if err := decode(r, &item); err != nil || len([]rune(item.SchoolName)) > 120 || len([]rune(item.Department)) > 120 {
		writeError(w, 400, "invalid_profile", "profile details are invalid")
		return
	}
	item.SchoolName, item.Department = strings.TrimSpace(item.SchoolName), strings.TrimSpace(item.Department)
	accountID := session.Session.Account.ID
	if err := h.normalizeSchoolSelection(r, accountID, &item); err != nil {
		if errors.Is(err, errInvalidSchoolSelection) {
			writeError(w, 400, "invalid_school_selection", "school selection is invalid")
			return
		}
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	if len([]rune(item.SchoolName)) > 120 {
		writeError(w, 400, "invalid_school_selection", "school selection is invalid")
		return
	}
	if item.ProfileSlug != nil {
		slug := strings.ToLower(strings.TrimSpace(*item.ProfileSlug))
		if slug == "" {
			item.ProfileSlug = nil
		} else if !slugPattern.MatchString(slug) {
			writeError(w, 400, "invalid_slug", "profile URL is invalid")
			return
		} else {
			item.ProfileSlug = &slug
		}
	}
	ctx := r.Context()
	var current *string
	err := h.pool.QueryRow(ctx, `SELECT profile_slug FROM account_profiles WHERE account_id = $1`, accountID).Scan(&current)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	if current != nil && item.ProfileSlug != nil && *current != *item.ProfileSlug {
		writeError(w, 409, "slug_locked", "profile URL can only be set once")
		return
	}
	if item.ProfileSlug != nil && current == nil {
		var collision bool
		if err := h.pool.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM accounts WHERE id <> $2 AND lower(username) = lower($1)
			UNION ALL
			SELECT 1 FROM account_profiles WHERE lower(profile_slug) = lower($1) AND account_id <> $2
		)`, *item.ProfileSlug, accountID).Scan(&collision); err != nil {
			writeError(w, 500, "internal_error", "internal server error")
			return
		}
		if collision {
			writeError(w, 409, "slug_taken", "profile URL is already in use")
			return
		}
	}
	_, err = h.pool.Exec(ctx, `
		INSERT INTO account_profiles (account_id, school_name, school_selection_type, school_directory_id,
			department, hide_school, hide_department, profile_slug)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (account_id) DO UPDATE SET school_name=EXCLUDED.school_name,
		 school_selection_type=EXCLUDED.school_selection_type,
		 school_directory_id=EXCLUDED.school_directory_id,
		 department=EXCLUDED.department, hide_school=EXCLUDED.hide_school,
		 hide_department=EXCLUDED.hide_department,
		 profile_slug=COALESCE(account_profiles.profile_slug, EXCLUDED.profile_slug)`,
		accountID, item.SchoolName, item.SchoolSelectionType, item.SchoolDirectoryID,
		item.Department, item.HideSchool, item.HideDepartment, item.ProfileSlug)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, 409, "slug_taken", "profile URL is already in use")
		} else {
			writeError(w, 500, "internal_error", "internal server error")
		}
		return
	}
	updated, err := h.myDetails(r, accountID)
	if err != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	if item.ProfileSlug != nil && (updated.ProfileSlug == nil || *updated.ProfileSlug != *item.ProfileSlug) {
		writeError(w, 409, "slug_locked", "profile URL can only be set once")
		return
	}
	writeJSON(w, 200, map[string]any{"data": updated})
}

func (h *Handler) getMyPrivacy(w http.ResponseWriter, r *http.Request) {
	session, ok := h.session(w, r)
	if !ok {
		return
	}
	item, err := h.myPrivacy(r, session.Session.Account.ID)
	if err != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	writeJSON(w, 200, map[string]any{"data": item})
}

func (h *Handler) myPrivacy(r *http.Request, accountID uuid.UUID) (privacySettings, error) {
	var item privacySettings
	err := h.pool.QueryRow(r.Context(), `SELECT profile_visibility, show_articles, show_portfolio FROM account_profiles WHERE account_id=$1`, accountID).Scan(
		&item.ProfileVisibility, &item.ShowArticles, &item.ShowPortfolio)
	if errors.Is(err, pgx.ErrNoRows) {
		return privacySettings{ProfileVisibility: "public", ShowArticles: true, ShowPortfolio: true}, nil
	}
	return item, err
}

func (h *Handler) putMyPrivacy(w http.ResponseWriter, r *http.Request) {
	session, ok := h.mutationSession(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var item privacySettings
	if err := decode(r, &item); err != nil || (item.ProfileVisibility != "public" && item.ProfileVisibility != "members") {
		writeError(w, 400, "invalid_privacy", "privacy settings are invalid")
		return
	}
	_, err := h.pool.Exec(r.Context(), `INSERT INTO account_profiles (account_id, profile_visibility, show_articles, show_portfolio)
		VALUES ($1,$2,$3,$4) ON CONFLICT (account_id) DO UPDATE SET profile_visibility=EXCLUDED.profile_visibility,
		show_articles=EXCLUDED.show_articles, show_portfolio=EXCLUDED.show_portfolio`,
		session.Session.Account.ID, item.ProfileVisibility, item.ShowArticles, item.ShowPortfolio)
	if err != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	writeJSON(w, 200, map[string]any{"data": item})
}

type publicAccount struct {
	ID            uuid.UUID
	Username      string
	Visibility    string
	ShowArticles  bool
	ShowPortfolio bool
}

func (h *Handler) publicAccount(r *http.Request, username string) (publicAccount, error) {
	var account publicAccount
	err := h.pool.QueryRow(r.Context(), `SELECT a.id, a.username, COALESCE(p.profile_visibility,'public'),
		COALESCE(p.show_articles,TRUE), COALESCE(p.show_portfolio,TRUE)
		FROM accounts a LEFT JOIN account_profiles p ON p.account_id=a.id
		WHERE lower(a.username)=lower($1) AND a.account_status='active'`, username).Scan(
		&account.ID, &account.Username, &account.Visibility, &account.ShowArticles, &account.ShowPortfolio)
	if errors.Is(err, pgx.ErrNoRows) {
		return publicAccount{}, pgx.ErrNoRows
	}
	return account, err
}

func (h *Handler) allowPublicAccount(w http.ResponseWriter, r *http.Request, username string) (publicAccount, bool) {
	account, err := h.publicAccount(r, username)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "not_found", "account not found")
		return publicAccount{}, false
	}
	if err != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return publicAccount{}, false
	}
	if account.Visibility == "members" {
		if _, err := h.auth.Authenticate(r.Context(), r); err != nil {
			writeError(w, 401, "unauthorized", "sign in to view this profile")
			return publicAccount{}, false
		}
	}
	return account, true
}

func (h *Handler) resolveUser(w http.ResponseWriter, r *http.Request) {
	segment := strings.TrimSpace(r.URL.Query().Get("segment"))
	if segment == "" || len(segment) > 64 {
		writeError(w, 400, "invalid_segment", "profile URL is invalid")
		return
	}
	var username, visibility string
	err := h.pool.QueryRow(r.Context(), `SELECT a.username, COALESCE(p.profile_visibility,'public') FROM accounts a
		JOIN account_profiles p ON p.account_id=a.id
		WHERE a.account_status='active' AND lower(p.profile_slug)=lower($1)
		LIMIT 1`, segment).Scan(&username, &visibility)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "not_found", "account not found")
		return
	}
	if err != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	if visibility == "members" {
		if _, err := h.auth.Authenticate(r.Context(), r); err != nil {
			writeError(w, 401, "unauthorized", "sign in to view this profile")
			return
		}
	}
	writeJSON(w, 200, map[string]any{"data": map[string]string{"username": username}})
}

func (h *Handler) getPublicDetails(w http.ResponseWriter, r *http.Request) {
	account, ok := h.allowPublicAccount(w, r, r.PathValue("username"))
	if !ok {
		return
	}
	var school, department string
	var hideSchool, hideDepartment bool
	err := h.pool.QueryRow(r.Context(), `SELECT COALESCE(d.school_name, p.school_name, ''), COALESCE(p.department,''), p.hide_school, p.hide_department
		FROM account_profiles p LEFT JOIN account_school_directory d ON d.id=p.school_directory_id
		WHERE p.account_id=$1`, account.ID).Scan(&school, &department, &hideSchool, &hideDepartment)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	data := map[string]any{"show_articles": account.ShowArticles, "show_portfolio": account.ShowPortfolio}
	if !hideSchool && school != "" {
		data["school_name"] = school
	}
	if !hideDepartment && department != "" {
		data["department"] = department
	}
	writeJSON(w, 200, map[string]any{"data": data})
}

func (h *Handler) getPublicPortfolio(w http.ResponseWriter, r *http.Request) {
	account, ok := h.allowPublicAccount(w, r, r.PathValue("username"))
	if !ok {
		return
	}
	items := make([]map[string]any, 0)
	if account.ShowPortfolio {
		rows, err := h.pool.Query(r.Context(), `SELECT pf.id::text, pp.title, s.school_name, ap.admission_program_name,
			a.academic_year, pf.updated_at
			FROM portfolio_files pf JOIN portfolio_projects pp ON pp.id=pf.project_id
			JOIN applications a ON a.id=pp.application_id
			JOIN academic_programs ap ON (ap.academic_year,ap.school_code,ap.program_code)=(a.academic_year,a.school_code,a.program_code)
			JOIN schools s ON s.school_code=a.school_code
			WHERE pp.account_id=$1 AND pf.status='published' ORDER BY pf.updated_at DESC, pf.id`, account.ID)
		if err != nil {
			writeError(w, 500, "internal_error", "internal server error")
			return
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			var project, school, program string
			var year int
			var publishedAt any
			if err := rows.Scan(&id, &project, &school, &program, &year, &publishedAt); err != nil {
				writeError(w, 500, "internal_error", "internal server error")
				return
			}
			items = append(items, map[string]any{"file_id": id, "project_title": project, "school_name": school, "program_name": program, "academic_year": year, "published_at": publishedAt})
		}
		if rows.Err() != nil {
			writeError(w, 500, "internal_error", "internal server error")
			return
		}
	}
	writeJSON(w, 200, map[string]any{"data": items})
}

func (h *Handler) getPublicExperiences(w http.ResponseWriter, r *http.Request) {
	account, ok := h.allowPublicAccount(w, r, r.PathValue("username"))
	if !ok {
		return
	}
	items := make([]map[string]any, 0)
	if account.ShowArticles {
		rows, err := h.pool.Query(r.Context(), `SELECT e.id::text, e.visibility, r.id::text, r.title, r.body,
			r.author_type, r.admission_outcome, r.version_number, e.created_at, e.updated_at,
			COALESCE((SELECT jsonb_agg(t.tag ORDER BY t.tag) FROM experience_revision_tags t WHERE t.revision_id=r.id),'[]'::jsonb)
			FROM experiences e JOIN experience_revisions r ON r.id=e.current_public_revision_id
			WHERE e.author_account_id=$1 AND e.visibility='published' AND r.review_status='approved'
			ORDER BY e.updated_at DESC, e.id DESC`, account.ID)
		if err != nil {
			writeError(w, 500, "internal_error", "internal server error")
			return
		}
		defer rows.Close()
		for rows.Next() {
			var id, revisionID, visibility, title, body, authorType, outcome string
			var number int
			var createdAt, updatedAt any
			var tags []byte
			if err := rows.Scan(&id, &visibility, &revisionID, &title, &body, &authorType, &outcome, &number, &createdAt, &updatedAt, &tags); err != nil {
				writeError(w, 500, "internal_error", "internal server error")
				return
			}
			var decodedTags []string
			_ = json.Unmarshal(tags, &decodedTags)
			items = append(items, map[string]any{"id": id, "visibility": visibility, "current_revision_id": revisionID, "title": title, "body": body, "author_type": authorType, "admission_outcome": outcome, "revision_number": number, "created_at": createdAt, "updated_at": updatedAt, "tags": decodedTags})
		}
		if rows.Err() != nil {
			writeError(w, 500, "internal_error", "internal server error")
			return
		}
	}
	writeJSON(w, 200, map[string]any{"data": items})
}

func (h *Handler) getMyExperiences(w http.ResponseWriter, r *http.Request) {
	session, ok := h.session(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT e.id::text, e.visibility, r.id::text, r.title, r.body,
		r.author_type,r.admission_outcome,r.version_number,e.created_at,e.updated_at,r.review_status,
		COALESCE((SELECT jsonb_agg(t.tag ORDER BY t.tag) FROM experience_revision_tags t WHERE t.revision_id=r.id),'[]'::jsonb)
		FROM experiences e JOIN LATERAL (SELECT * FROM experience_revisions x WHERE x.experience_id=e.id ORDER BY x.version_number DESC LIMIT 1) r ON TRUE
		WHERE e.author_account_id=$1 ORDER BY e.updated_at DESC,e.id DESC`, session.Session.Account.ID)
	if err != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, revisionID, visibility, title, body, authorType, outcome, status string
		var number int
		var createdAt, updatedAt any
		var tags []byte
		if err := rows.Scan(&id, &visibility, &revisionID, &title, &body, &authorType, &outcome, &number, &createdAt, &updatedAt, &status, &tags); err != nil {
			writeError(w, 500, "internal_error", "internal server error")
			return
		}
		var decodedTags []string
		_ = json.Unmarshal(tags, &decodedTags)
		items = append(items, map[string]any{"id": id, "visibility": visibility, "current_revision_id": revisionID, "title": title, "body": body, "author_type": authorType, "admission_outcome": outcome, "revision_number": number, "created_at": createdAt, "updated_at": updatedAt, "review_status": status, "tags": decodedTags})
	}
	if rows.Err() != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	writeJSON(w, 200, map[string]any{"data": items})
}

func (h *Handler) getExperienceTags(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `SELECT t.tag, COUNT(*)::int FROM experience_revision_tags t
		JOIN experience_revisions r ON r.id=t.revision_id JOIN experiences e ON e.current_public_revision_id=r.id
		WHERE e.visibility='published' AND r.review_status='approved' GROUP BY t.tag ORDER BY COUNT(*) DESC,t.tag`)
	if err != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	defer rows.Close()
	type tagCount struct {
		Tag   string `json:"tag"`
		Count int    `json:"count"`
	}
	items := make([]tagCount, 0)
	for rows.Next() {
		var item tagCount
		if err := rows.Scan(&item.Tag, &item.Count); err != nil {
			writeError(w, 500, "internal_error", "internal server error")
			return
		}
		items = append(items, item)
	}
	if rows.Err() != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	writeJSON(w, 200, map[string]any{"data": items})
}

var programIdentifierPattern = regexp.MustCompile(`^([0-9]{3})-([0-9]{3})-([0-9]{3})$`)

func parseProgramIdentifier(value string) (int, string, string, bool) {
	m := programIdentifierPattern.FindStringSubmatch(strings.TrimSpace(value))
	if m == nil {
		return 0, "", "", false
	}
	year := 0
	_, _ = fmt.Sscanf(m[1], "%d", &year)
	return year, m[2], m[3], true
}

func (h *Handler) listSavedPrograms(w http.ResponseWriter, r *http.Request) {
	session, ok := h.session(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT s.academic_year,s.school_code,s.program_code,s.created_at,
		sc.school_name,p.admission_program_name FROM saved_programs s
		JOIN schools sc ON sc.school_code=s.school_code JOIN academic_programs p
		ON (p.academic_year,p.school_code,p.program_code)=(s.academic_year,s.school_code,s.program_code)
		WHERE s.account_id=$1 ORDER BY s.created_at DESC,s.academic_year DESC,s.school_code,s.program_code`, session.Session.Account.ID)
	if err != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	defer rows.Close()
	type saved struct {
		Identifier string `json:"program_identifier"`
		Year       int    `json:"academic_year"`
		School     string `json:"school_name"`
		Program    string `json:"admission_program_name"`
		SavedAt    any    `json:"saved_at"`
	}
	items := make([]saved, 0)
	for rows.Next() {
		var item saved
		var schoolCode, programCode string
		if err := rows.Scan(&item.Year, &schoolCode, &programCode, &item.SavedAt, &item.School, &item.Program); err != nil {
			writeError(w, 500, "internal_error", "internal server error")
			return
		}
		item.Identifier = fmt.Sprintf("%03d-%s-%s", item.Year, schoolCode, programCode)
		items = append(items, item)
	}
	if rows.Err() != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	writeJSON(w, 200, map[string]any{"data": items})
}

func (h *Handler) getSavedProgram(w http.ResponseWriter, r *http.Request) {
	session, ok := h.session(w, r)
	if !ok {
		return
	}
	year, school, program, valid := parseProgramIdentifier(r.PathValue("identifier"))
	if !valid {
		writeError(w, 400, "invalid_identifier", "program identifier is invalid")
		return
	}
	var saved bool
	err := h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM saved_programs WHERE account_id=$1 AND academic_year=$2 AND school_code=$3 AND program_code=$4)`, session.Session.Account.ID, year, school, program).Scan(&saved)
	if err != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	writeJSON(w, 200, map[string]any{"data": map[string]bool{"saved": saved}})
}

func (h *Handler) saveProgram(w http.ResponseWriter, r *http.Request) {
	session, ok := h.mutationSession(w, r)
	if !ok {
		return
	}
	year, school, program, valid := parseProgramIdentifier(r.PathValue("identifier"))
	if !valid {
		writeError(w, 400, "invalid_identifier", "program identifier is invalid")
		return
	}
	tag, err := h.pool.Exec(r.Context(), `INSERT INTO saved_programs(account_id,academic_year,school_code,program_code) SELECT $1,$2,$3,$4 WHERE EXISTS(SELECT 1 FROM academic_programs WHERE academic_year=$2 AND school_code=$3 AND program_code=$4) ON CONFLICT(account_id,academic_year,school_code,program_code) DO UPDATE SET created_at=saved_programs.created_at`, session.Session.Account.ID, year, school, program)
	if err != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, 404, "program_not_found", "program was not found")
		return
	}
	var result struct {
		Year       int    `json:"academic_year"`
		School     string `json:"school_name"`
		Program    string `json:"admission_program_name"`
		SavedAt    any    `json:"saved_at"`
		Identifier string `json:"program_identifier"`
	}
	err = h.pool.QueryRow(r.Context(), `SELECT s.academic_year,sc.school_name,p.admission_program_name,s.created_at FROM saved_programs s JOIN schools sc ON sc.school_code=s.school_code JOIN academic_programs p ON (p.academic_year,p.school_code,p.program_code)=(s.academic_year,s.school_code,s.program_code) WHERE s.account_id=$1 AND s.academic_year=$2 AND s.school_code=$3 AND s.program_code=$4`, session.Session.Account.ID, year, school, program).Scan(&result.Year, &result.School, &result.Program, &result.SavedAt)
	if err != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	result.Identifier = fmt.Sprintf("%03d-%s-%s", year, school, program)
	writeJSON(w, 200, map[string]any{"data": result})
}

func (h *Handler) unsaveProgram(w http.ResponseWriter, r *http.Request) {
	session, ok := h.mutationSession(w, r)
	if !ok {
		return
	}
	year, school, program, valid := parseProgramIdentifier(r.PathValue("identifier"))
	if !valid {
		writeError(w, 400, "invalid_identifier", "program identifier is invalid")
		return
	}
	_, err := h.pool.Exec(r.Context(), `DELETE FROM saved_programs WHERE account_id=$1 AND academic_year=$2 AND school_code=$3 AND program_code=$4`, session.Session.Account.ID, year, school, program)
	if err != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type notificationPreferences struct {
	EmailEnabled bool            `json:"email_enabled"`
	Kinds        map[string]bool `json:"kinds"`
}

func (h *Handler) getNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	session, ok := h.session(w, r)
	if !ok {
		return
	}
	item := notificationPreferences{EmailEnabled: true, Kinds: map[string]bool{}}
	var kinds []byte
	err := h.pool.QueryRow(r.Context(), `SELECT email_enabled,kinds FROM notification_preferences WHERE account_id=$1`, session.Session.Account.ID).Scan(&item.EmailEnabled, &kinds)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	if len(kinds) > 0 {
		_ = json.Unmarshal(kinds, &item.Kinds)
	}
	if item.Kinds == nil {
		item.Kinds = map[string]bool{}
	}
	writeJSON(w, 200, map[string]any{"data": item})
}

func (h *Handler) putNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	session, ok := h.mutationSession(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var item notificationPreferences
	if err := decode(r, &item); err != nil || item.Kinds == nil || len(item.Kinds) > 100 {
		writeError(w, 400, "invalid_preferences", "notification preferences are invalid")
		return
	}
	for key := range item.Kinds {
		if strings.TrimSpace(key) == "" || len(key) > 80 {
			writeError(w, 400, "invalid_preferences", "notification preferences are invalid")
			return
		}
	}
	blob, err := json.Marshal(item.Kinds)
	if err != nil {
		writeError(w, 400, "invalid_preferences", "notification preferences are invalid")
		return
	}
	_, err = h.pool.Exec(r.Context(), `INSERT INTO notification_preferences(account_id,email_enabled,kinds) VALUES($1,$2,$3::jsonb) ON CONFLICT(account_id) DO UPDATE SET email_enabled=EXCLUDED.email_enabled,kinds=EXCLUDED.kinds`, session.Session.Account.ID, item.EmailEnabled, string(blob))
	if err != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	writeJSON(w, 200, map[string]any{"data": item})
}

func (h *Handler) listIdentities(w http.ResponseWriter, r *http.Request) {
	session, ok := h.session(w, r)
	if !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT provider,created_at FROM oauth_identities WHERE account_id=$1 ORDER BY created_at,provider`, session.Session.Account.ID)
	if err != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var provider string
		var linkedAt any
		if err := rows.Scan(&provider, &linkedAt); err != nil {
			writeError(w, 500, "internal_error", "internal server error")
			return
		}
		items = append(items, map[string]any{"provider": provider, "linked_at": linkedAt})
	}
	if rows.Err() != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	writeJSON(w, 200, map[string]any{"data": items})
}

func (h *Handler) cancelVerification(w http.ResponseWriter, r *http.Request) {
	session, ok := h.mutationSession(w, r)
	if !ok {
		return
	}
	requestID, err := uuid.Parse(r.PathValue("requestID"))
	if err != nil {
		writeError(w, 400, "invalid_request_id", "request id is invalid")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	defer tx.Rollback(r.Context())
	var status string
	err = tx.QueryRow(r.Context(), `SELECT status FROM verification_requests WHERE id=$1 AND account_id=$2 FOR UPDATE`, requestID, session.Session.Account.ID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "not_found", "verification request not found")
		return
	}
	if err != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	if status != "pending" {
		writeError(w, 409, "invalid_status", "only pending requests can be cancelled")
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE verification_requests SET status='cancelled' WHERE id=$1`, requestID); err != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO verification_events(request_id,actor_account_id,action,from_status,to_status,reason) VALUES($1,$2,'owner_cancelled','pending','cancelled','owner cancelled request')`, requestID, session.Session.Account.ID); err != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) revokeVerification(w http.ResponseWriter, r *http.Request) {
	session, ok := h.mutationSession(w, r)
	if !ok {
		return
	}
	requestID, err := uuid.Parse(r.PathValue("requestID"))
	if err != nil {
		writeError(w, 400, "invalid_request_id", "request id is invalid")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	defer tx.Rollback(r.Context())
	var status, identityType string
	err = tx.QueryRow(r.Context(), `SELECT status, identity_type FROM verification_requests WHERE id=$1 AND account_id=$2 FOR UPDATE`, requestID, session.Session.Account.ID).Scan(&status, &identityType)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "not_found", "verification request not found")
		return
	}
	if err != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	if status != "approved" {
		writeError(w, 409, "invalid_status", "only approved requests can be revoked")
		return
	}
	if identityType == "parent" || identityType == "educator" {
		if _, err = tx.Exec(r.Context(), `UPDATE verification_requests SET status='revoked' WHERE id=$1 AND account_id=$2`, requestID, session.Session.Account.ID); err != nil {
			writeError(w, 500, "internal_error", "internal server error")
			return
		}
		if _, err = tx.Exec(r.Context(), `INSERT INTO verification_events(request_id,actor_account_id,action,from_status,to_status,reason) VALUES($1,$2,'owner_revoked','approved','revoked','owner revoked verification')`, requestID, session.Session.Account.ID); err != nil {
			writeError(w, 500, "internal_error", "internal server error")
			return
		}
	} else {
		rows, queryErr := tx.Query(r.Context(), `UPDATE student_verifications SET status='revoked' WHERE source_request_id=$1 AND account_id=$2 AND status='active' RETURNING id`, requestID, session.Session.Account.ID)
		if queryErr != nil {
			writeError(w, 500, "internal_error", "internal server error")
			return
		}
		ids := make([]uuid.UUID, 0)
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				writeError(w, 500, "internal_error", "internal server error")
				return
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			writeError(w, 500, "internal_error", "internal server error")
			return
		}
		rows.Close()
		if len(ids) == 0 {
			writeError(w, 409, "already_revoked", "verification is no longer active")
			return
		}
		if _, err = tx.Exec(r.Context(), `UPDATE verification_requests SET status='revoked' WHERE id=$1 AND account_id=$2`, requestID, session.Session.Account.ID); err != nil {
			writeError(w, 500, "internal_error", "internal server error")
			return
		}
		for _, id := range ids {
			if _, err = tx.Exec(r.Context(), `INSERT INTO verification_events(request_id,verification_id,actor_account_id,action,from_status,to_status,reason) VALUES($1,$2,$3,'owner_revoked','active','revoked','owner revoked verification')`, requestID, id, session.Session.Account.ID); err != nil {
				writeError(w, 500, "internal_error", "internal server error")
				return
			}
		}
	}
	var lockedAccountID uuid.UUID
	if err := tx.QueryRow(r.Context(), `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, session.Session.Account.ID).Scan(&lockedAccountID); err != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	var fallbackIdentity string
	err = tx.QueryRow(r.Context(), `
		SELECT COALESCE((
			SELECT CASE
				WHEN vr.identity_type = 'student'
				 AND NOT EXISTS (
					SELECT 1 FROM student_verifications v
					WHERE v.source_request_id = vr.id AND v.status = 'active' AND v.expires_at > CURRENT_TIMESTAMP
				 )
				 AND EXISTS (
					SELECT 1 FROM student_verifications v
					WHERE v.source_request_id = vr.id
					  AND (v.status = 'expired' OR (v.status = 'active' AND v.expires_at <= CURRENT_TIMESTAMP))
				 ) THEN 'senior'
				ELSE vr.identity_type
			END
			FROM verification_requests vr
			WHERE vr.account_id = $1 AND vr.status = 'approved'
			  AND (
				vr.identity_type IN ('parent', 'educator')
				OR EXISTS (
					SELECT 1 FROM student_verifications v
					WHERE v.source_request_id = vr.id AND v.status = 'active' AND v.expires_at > CURRENT_TIMESTAMP
				)
				OR EXISTS (
					SELECT 1 FROM student_verifications v
					WHERE v.source_request_id = vr.id
					  AND (v.status = 'expired' OR (v.status = 'active' AND v.expires_at <= CURRENT_TIMESTAMP))
				)
			  )
			ORDER BY CASE
				WHEN vr.identity_type IN ('parent', 'educator') OR EXISTS (
					SELECT 1 FROM student_verifications v
					WHERE v.source_request_id = vr.id AND v.status = 'active' AND v.expires_at > CURRENT_TIMESTAMP
				) THEN 0 ELSE 1
			END,
			vr.reviewed_at DESC NULLS LAST, vr.created_at DESC
			LIMIT 1
		), 'temporary')
	`, session.Session.Account.ID).Scan(&fallbackIdentity)
	if err != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE accounts
		SET identity_status=$2, school_identity_verified=($2 <> 'temporary'),
		    identity_type=NULLIF($2, 'temporary'), updated_at=CURRENT_TIMESTAMP
		WHERE id=$1 AND identity_status=$3`, session.Session.Account.ID, fallbackIdentity, identityType); err != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "internal_error", "internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decode(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
