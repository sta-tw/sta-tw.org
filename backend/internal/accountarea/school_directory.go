package accountarea

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"sta-backend/internal/auth"
)

const otherSchoolStatusLabel = "其他（自學／休學／非臺灣學生）"

var errInvalidSchoolSelection = errors.New("invalid school selection")

type schoolDirectoryEntry struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	SchoolType string    `json:"school_type"`
	IsActive   bool      `json:"is_active"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type unlistedSchoolSuggestion struct {
	Name          string `json:"name"`
	ProfilesCount int    `json:"profiles_count"`
}

func (h *Handler) listPublicSchoolDirectory(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT id, school_name, school_type, is_active, updated_at
		FROM account_school_directory
		WHERE is_active
		ORDER BY CASE school_type WHEN 'university' THEN 0 ELSE 1 END, school_name`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	defer rows.Close()

	entries := make([]schoolDirectoryEntry, 0)
	for rows.Next() {
		var item schoolDirectoryEntry
		if err := rows.Scan(&item.ID, &item.Name, &item.SchoolType, &item.IsActive, &item.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		entries = append(entries, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": entries})
}

func (h *Handler) listAdminSchoolDirectory(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.schoolDirectoryAdmin(w, r, false); !ok {
		return
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT id, school_name, school_type, is_active, updated_at
		FROM account_school_directory
		ORDER BY CASE school_type WHEN 'university' THEN 0 ELSE 1 END, school_name`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	entries := make([]schoolDirectoryEntry, 0)
	for rows.Next() {
		var item schoolDirectoryEntry
		if err := rows.Scan(&item.ID, &item.Name, &item.SchoolType, &item.IsActive, &item.UpdatedAt); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		entries = append(entries, item)
	}
	rowsErr := rows.Err()
	rows.Close()
	if rowsErr != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	suggestionsRows, err := h.pool.Query(r.Context(), `
		SELECT MIN(btrim(p.school_name)), COUNT(*)::int
		FROM account_profiles p
		WHERE p.school_selection_type = 'unlisted'
		  AND btrim(p.school_name) <> ''
		  AND NOT EXISTS (
			SELECT 1 FROM account_school_directory d
			WHERE lower(btrim(d.school_name)) = lower(btrim(p.school_name))
		  )
		GROUP BY lower(btrim(p.school_name))
		ORDER BY COUNT(*) DESC, lower(MIN(btrim(p.school_name)))
		LIMIT 100`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	defer suggestionsRows.Close()
	suggestions := make([]unlistedSchoolSuggestion, 0)
	for suggestionsRows.Next() {
		var item unlistedSchoolSuggestion
		if err := suggestionsRows.Scan(&item.Name, &item.ProfilesCount); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		suggestions = append(suggestions, item)
	}
	if err := suggestionsRows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": entries, "suggestions": suggestions})
}

func (h *Handler) createSchoolDirectoryEntry(w http.ResponseWriter, r *http.Request) {
	session, ok := h.schoolDirectoryAdmin(w, r, true)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var input struct {
		Name       string `json:"name"`
		SchoolType string `json:"school_type"`
		Reason     string `json:"reason"`
	}
	if err := decode(r, &input); err != nil || !validDirectoryEntry(input.Name, input.SchoolType) || !validDirectoryReason(input.Reason) {
		writeError(w, http.StatusBadRequest, "invalid_school_directory_entry", "school name, type, or reason is invalid")
		return
	}
	input.Name, input.Reason = strings.TrimSpace(input.Name), strings.TrimSpace(input.Reason)
	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	defer tx.Rollback(ctx)
	var item schoolDirectoryEntry
	err = tx.QueryRow(ctx, `
		INSERT INTO account_school_directory (school_name, school_type)
		VALUES ($1, $2)
		RETURNING id, school_name, school_type, is_active, updated_at`, input.Name, input.SchoolType).
		Scan(&item.ID, &item.Name, &item.SchoolType, &item.IsActive, &item.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "school_already_exists", "this school is already in the directory")
		} else {
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		}
		return
	}
	profilesAssociated, err := tx.Exec(ctx, `
		UPDATE account_profiles
		SET school_selection_type = 'directory', school_directory_id = $1, school_name = $2
		WHERE school_selection_type = 'unlisted'
		  AND lower(btrim(school_name)) = lower(btrim($2))`, item.ID, item.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	after, _ := json.Marshal(map[string]any{
		"entry":               item,
		"profiles_associated": profilesAssociated.RowsAffected(),
	})
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_log (actor_account_id, action, entity_type, entity_key, after_data, reason)
		VALUES ($1, 'school_directory.create', 'account_school_directory', $2, $3::jsonb, $4)`,
		session.Session.Account.ID, item.ID.String(), after, input.Reason); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"data":                item,
		"profiles_associated": profilesAssociated.RowsAffected(),
	})
}

func (h *Handler) updateSchoolDirectoryEntry(w http.ResponseWriter, r *http.Request) {
	session, ok := h.schoolDirectoryAdmin(w, r, true)
	if !ok {
		return
	}
	entryID, err := uuid.Parse(r.PathValue("entryID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_school_id", "school id is invalid")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var input struct {
		Name       string `json:"name"`
		SchoolType string `json:"school_type"`
		IsActive   bool   `json:"is_active"`
		Reason     string `json:"reason"`
	}
	if err := decode(r, &input); err != nil || !validDirectoryEntry(input.Name, input.SchoolType) || !validDirectoryReason(input.Reason) {
		writeError(w, http.StatusBadRequest, "invalid_school_directory_entry", "school name, type, or reason is invalid")
		return
	}
	input.Name, input.Reason = strings.TrimSpace(input.Name), strings.TrimSpace(input.Reason)
	ctx := r.Context()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	defer tx.Rollback(ctx)
	var before schoolDirectoryEntry
	err = tx.QueryRow(ctx, `
		SELECT id, school_name, school_type, is_active, updated_at
		FROM account_school_directory WHERE id = $1 FOR UPDATE`, entryID).
		Scan(&before.ID, &before.Name, &before.SchoolType, &before.IsActive, &before.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "school_not_found", "school was not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	var after schoolDirectoryEntry
	err = tx.QueryRow(ctx, `
		UPDATE account_school_directory
		SET school_name = $2, school_type = $3, is_active = $4
		WHERE id = $1
		RETURNING id, school_name, school_type, is_active, updated_at`,
		entryID, input.Name, input.SchoolType, input.IsActive).
		Scan(&after.ID, &after.Name, &after.SchoolType, &after.IsActive, &after.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "school_already_exists", "this school is already in the directory")
		} else {
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		}
		return
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_log (actor_account_id, action, entity_type, entity_key, before_data, after_data, reason)
		VALUES ($1, 'school_directory.update', 'account_school_directory', $2, $3::jsonb, $4::jsonb, $5)`,
		session.Session.Account.ID, entryID.String(), beforeJSON, afterJSON, input.Reason); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": after})
}

func (h *Handler) schoolDirectoryAdmin(w http.ResponseWriter, r *http.Request, mutation bool) (auth.RequestSession, bool) {
	// Authenticate enforces the configured admin MFA grant for /api/v1/admin/*.
	session, err := h.auth.Authenticate(r.Context(), r)
	if err != nil {
		if errors.Is(err, auth.ErrAdminMFARateLimited) {
			writeError(w, http.StatusTooManyRequests, "rate_limited", "too many MFA attempts; try again later")
		} else if errors.Is(err, auth.ErrAdminMFARequired) || errors.Is(err, auth.ErrAdminMFAInvalid) {
			writeError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
		} else {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		}
		return auth.RequestSession{}, false
	}
	isAdmin, err := h.auth.IsAdmin(r.Context(), session.Session.Account.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return auth.RequestSession{}, false
	}
	if !isAdmin {
		writeError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
		return auth.RequestSession{}, false
	}
	if mutation {
		if err := h.auth.AuthorizeMutation(r, session); err != nil {
			writeError(w, http.StatusForbidden, "csrf_required", "request verification failed")
			return auth.RequestSession{}, false
		}
	}
	return session, true
}

func (h *Handler) normalizeSchoolSelection(r *http.Request, accountID uuid.UUID, item *profileDetails) error {
	item.SchoolName = strings.TrimSpace(item.SchoolName)
	switch item.SchoolSelectionType {
	case "", "none":
		if item.SchoolName != "" || item.SchoolDirectoryID != nil {
			return errInvalidSchoolSelection
		}
		item.SchoolSelectionType = "none"
		item.SchoolDirectoryID = nil
		return nil
	case "other_status":
		if item.SchoolDirectoryID != nil {
			return errInvalidSchoolSelection
		}
		item.SchoolName = otherSchoolStatusLabel
		return nil
	case "unlisted":
		if item.SchoolDirectoryID != nil || item.SchoolName == "" || len([]rune(item.SchoolName)) > 120 {
			return errInvalidSchoolSelection
		}
		return nil
	case "directory":
		if item.SchoolDirectoryID == nil {
			return errInvalidSchoolSelection
		}
		var name string
		var active bool
		err := h.pool.QueryRow(r.Context(), `SELECT school_name, is_active FROM account_school_directory WHERE id = $1`, *item.SchoolDirectoryID).Scan(&name, &active)
		if errors.Is(err, pgx.ErrNoRows) {
			return errInvalidSchoolSelection
		}
		if err != nil {
			return err
		}
		if !active {
			var unchanged bool
			err := h.pool.QueryRow(r.Context(), `SELECT EXISTS (
				SELECT 1 FROM account_profiles WHERE account_id = $1
				AND school_selection_type = 'directory' AND school_directory_id = $2
			)`, accountID, *item.SchoolDirectoryID).Scan(&unchanged)
			if err != nil {
				return err
			}
			if !unchanged {
				return errInvalidSchoolSelection
			}
		}
		item.SchoolName = name
		return nil
	default:
		return errInvalidSchoolSelection
	}
}

func validDirectoryEntry(name, schoolType string) bool {
	name = strings.TrimSpace(name)
	return name != "" && len([]rune(name)) <= 120 &&
		(schoolType == "university" || schoolType == "senior_high")
}

func validDirectoryReason(reason string) bool {
	reason = strings.TrimSpace(reason)
	return len([]rune(reason)) >= 3 && len([]rune(reason)) <= 500
}
