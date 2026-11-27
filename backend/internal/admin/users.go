package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"sta-backend/internal/auth"
	"sta-backend/internal/pagination"
	"sta-backend/internal/profile"
)

// userSummary is one row of the operator account list.
type userSummary struct {
	ID                     string     `json:"id"`
	Username               string     `json:"username"`
	IdentityStatus         string     `json:"identity_status"`
	SchoolIdentityVerified bool       `json:"school_identity_verified"`
	IdentityType           *string    `json:"identity_type"`
	AccountStatus          string     `json:"account_status"`
	EmailVerified          bool       `json:"email_verified"`
	IsAdmin                bool       `json:"is_admin"`
	IsAdmissionsModerator  bool       `json:"is_admissions_moderator"`
	LastLoginAt            *time.Time `json:"last_login_at"`
	SuspendedAt            *time.Time `json:"suspended_at,omitempty"`
	SuspensionReason       string     `json:"suspension_reason,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
}

type userDetail struct {
	userSummary
	SuspendedBy    *string `json:"suspended_by,omitempty"`
	ActiveSessions int     `json:"active_sessions"`
	Applications   int     `json:"applications"`
	Experiences    int     `json:"experiences"`
	// Email/SchoolEmail are decrypted only for this single-account detail
	// view, never for the list — see auth.Service.AdminAccountContact.
	// SchoolEmail is "" when the account has none on file (see
	// auth.AdminContactStore.GetAccountContact).
	Email       string         `json:"email"`
	SchoolEmail string         `json:"school_email"`
	DisplayName string         `json:"display_name"`
	Bio         string         `json:"bio"`
	Links       []profile.Link `json:"links"`
}

func validAccountStatus(v string) bool {
	switch v {
	case "active", "pending_verification", "suspended", "deleted":
		return true
	}
	return false
}

func validIdentityType(v string) bool {
	switch v {
	case "student", "senior", "parent", "educator":
		return true
	}
	return false
}

func validSchoolIdentityVerification(v string) bool {
	return v == "unverified" || v == "verified"
}

// listUsers is a keyset page over accounts, newest first, with optional filters
// on account_status, school identity verification, identity type, the admin
// role and a username prefix.
func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	q := r.URL.Query()

	limit := 50
	if raw := q.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			writeError(w, http.StatusBadRequest, "invalid_query", "limit must be 1-100")
			return
		}
		limit = parsed
	}

	cursor, err := pagination.Decode(q.Get("cursor"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_query", "cursor is invalid")
		return
	}

	args := []any{limit}
	where := []string{}
	add := func(clause string, value any) {
		args = append(args, value)
		where = append(where, strings.Replace(clause, "?", "$"+strconv.Itoa(len(args)), 1))
	}

	if !cursor.Zero() {
		args = append(args, cursor.Time)
		tsPos := len(args)
		args = append(args, cursor.UUID())
		idPos := len(args)
		where = append(where, fmt.Sprintf("(a.created_at, a.id) < ($%d::timestamptz, $%d::uuid)", tsPos, idPos))
	}
	if v := strings.TrimSpace(q.Get("status")); v != "" {
		if !validAccountStatus(v) {
			writeError(w, http.StatusBadRequest, "invalid_query", "status is invalid")
			return
		}
		add("a.account_status = ?", v)
	}
	if v := strings.TrimSpace(q.Get("identity")); v != "" {
		if !validIdentityType(v) {
			writeError(w, http.StatusBadRequest, "invalid_query", "identity is invalid")
			return
		}
		add("a.identity_type = ?", v)
	}
	if v := strings.TrimSpace(q.Get("verification")); v != "" {
		if !validSchoolIdentityVerification(v) {
			writeError(w, http.StatusBadRequest, "invalid_query", "verification is invalid")
			return
		}
		if v == "unverified" {
			add("a.school_identity_verified = ?", false)
		} else {
			where = append(where, "a.school_identity_verified = true")
		}
	}
	if strings.EqualFold(strings.TrimSpace(q.Get("role")), "admin") {
		where = append(where, "EXISTS (SELECT 1 FROM account_roles ar WHERE ar.account_id = a.id AND ar.role = 'admin')")
	}
	if v := strings.TrimSpace(q.Get("q")); v != "" {
		if len(v) > 64 {
			writeError(w, http.StatusBadRequest, "invalid_query", "q is too long")
			return
		}
		add("lower(a.username) LIKE ? || '%'", strings.ToLower(v))
	}

	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}
	sql := `SELECT a.id::text, a.username, a.identity_status, a.school_identity_verified, a.identity_type, a.account_status,
	               a.email_verified_at IS NOT NULL,
	               EXISTS (SELECT 1 FROM account_roles ar WHERE ar.account_id = a.id AND ar.role = 'admin'),
	               EXISTS (SELECT 1 FROM account_roles ar WHERE ar.account_id = a.id AND ar.role = 'admissions_moderator'),
	               a.last_login_at, a.suspended_at, COALESCE(a.suspension_reason, ''), a.created_at
	        FROM accounts a
	        ` + whereSQL + `
	        ORDER BY a.created_at DESC, a.id DESC
	        LIMIT $1`

	rows, err := h.pool.Query(r.Context(), sql, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	defer rows.Close()
	items := make([]userSummary, 0, limit)
	for rows.Next() {
		var u userSummary
		if err := rows.Scan(&u.ID, &u.Username, &u.IdentityStatus, &u.SchoolIdentityVerified, &u.IdentityType, &u.AccountStatus,
			&u.EmailVerified, &u.IsAdmin, &u.IsAdmissionsModerator, &u.LastLoginAt, &u.SuspendedAt, &u.SuspensionReason, &u.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		items = append(items, u)
	}
	if rows.Err() != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	next := ""
	if len(items) == limit {
		last := items[len(items)-1]
		next = pagination.Encode(pagination.Cursor{Time: last.CreatedAt, ID: last.ID})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "next_cursor": next})
}

func (h *Handler) getUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	accountID, err := uuid.Parse(r.PathValue("accountID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_account_id", "account id is invalid")
		return
	}
	var d userDetail
	var linksJSON []byte
	err = h.pool.QueryRow(r.Context(), `
		SELECT a.id::text, a.username, a.identity_status, a.school_identity_verified, a.identity_type, a.account_status,
		       a.email_verified_at IS NOT NULL,
		       EXISTS (SELECT 1 FROM account_roles ar WHERE ar.account_id = a.id AND ar.role = 'admin'),
		       EXISTS (SELECT 1 FROM account_roles ar WHERE ar.account_id = a.id AND ar.role = 'admissions_moderator'),
		       a.last_login_at, a.suspended_at, COALESCE(a.suspension_reason, ''), a.created_at,
		       sb.username,
		       (SELECT count(*) FROM account_sessions s WHERE s.account_id = a.id AND s.revoked_at IS NULL AND s.expires_at > CURRENT_TIMESTAMP),
		       (SELECT count(*) FROM applications ap WHERE ap.account_id = a.id),
		       (SELECT count(*) FROM experiences e WHERE e.author_account_id = a.id),
		       COALESCE(p.display_name, ''), COALESCE(p.bio, ''), COALESCE(p.links, '[]'::jsonb)
		FROM accounts a
		LEFT JOIN accounts sb ON sb.id = a.suspended_by
	LEFT JOIN account_profiles p ON p.account_id = a.id
		WHERE a.id = $1`, accountID).Scan(
		&d.ID, &d.Username, &d.IdentityStatus, &d.SchoolIdentityVerified, &d.IdentityType, &d.AccountStatus, &d.EmailVerified, &d.IsAdmin, &d.IsAdmissionsModerator,
		&d.LastLoginAt, &d.SuspendedAt, &d.SuspensionReason, &d.CreatedAt,
		&d.SuspendedBy, &d.ActiveSessions, &d.Applications, &d.Experiences,
		&d.DisplayName, &d.Bio, &linksJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "account not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	d.Links = []profile.Link{}
	if len(linksJSON) > 0 {
		if err := json.Unmarshal(linksJSON, &d.Links); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
	}
	d.Email, d.SchoolEmail, err = h.auth.AdminAccountContact(r.Context(), accountID)
	if err != nil && !errors.Is(err, auth.ErrNotConfigured) {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// updateUser edits username and contact addresses. Identity status can only
// change through a successful verification flow; password is deliberately
// never settable here — see resetPassword, the only supported path to change one.
func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	accountID, err := uuid.Parse(r.PathValue("accountID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_account_id", "account id is invalid")
		return
	}
	var body struct {
		Username       *string         `json:"username"`
		IdentityStatus *string         `json:"identity_status"`
		Email          *string         `json:"email"`
		SchoolEmail    *string         `json:"school_email"`
		DisplayName    *string         `json:"display_name"`
		Bio            *string         `json:"bio"`
		Links          *[]profile.Link `json:"links"`
		Reason         *string         `json:"reason"`
	}
	if err := decodeAdminJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is invalid")
		return
	}

	if body.IdentityStatus != nil {
		writeError(w, http.StatusUnprocessableEntity, "identity_verification_required", "identity status can only change after verification succeeds")
		return
	}
	requested := body.Username != nil || body.Email != nil || body.SchoolEmail != nil ||
		body.DisplayName != nil || body.Bio != nil || body.Links != nil
	if !requested {
		writeError(w, http.StatusBadRequest, "empty_update", "at least one account field is required")
		return
	}
	reason := "legacy admin update (client omitted reason)"
	if body.Reason != nil {
		reason = strings.TrimSpace(*body.Reason)
		if len([]rune(reason)) == 0 || len([]rune(reason)) > 500 {
			writeError(w, http.StatusBadRequest, "invalid_reason", "reason must be 1-500 characters when provided")
			return
		}
	}
	username := ""
	if body.Username != nil {
		username = strings.TrimSpace(*body.Username)
		if len([]rune(username)) < 3 || len([]rune(username)) > 64 {
			writeError(w, http.StatusBadRequest, "invalid_username", "username must be 3-64 characters")
			return
		}
	}
	requestedProfile := body.DisplayName != nil || body.Bio != nil || body.Links != nil
	if requestedProfile {
		candidate := profile.Input{}
		if body.DisplayName != nil {
			candidate.DisplayName = *body.DisplayName
		}
		if body.Bio != nil {
			candidate.Bio = *body.Bio
		}
		if body.Links != nil {
			candidate.Links = *body.Links
		}
		if err := candidate.Normalize(); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_profile", "profile fields or links are invalid")
			return
		}
	}

	// Correct encrypted contact fields through the auth service so its
	// normalization, hashing and uniqueness rules remain the single source of
	// truth. Every successful contact change is audited before the next starts.
	if body.Email != nil {
		if err := h.auth.AdminUpdateAccountEmail(r.Context(), accountID, *body.Email); err != nil {
			writeContactUpdateError(w, err)
			return
		}
		if err := writeAccountUpdateAudit(r.Context(), h.pool, session.Session.Account.ID, accountID, reason, []string{"email"}); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
	}
	if body.SchoolEmail != nil {
		var err error
		if strings.TrimSpace(*body.SchoolEmail) == "" {
			err = h.auth.AdminClearAccountSchoolEmail(r.Context(), accountID)
		} else {
			err = h.auth.AdminUpdateAccountSchoolEmail(r.Context(), accountID, *body.SchoolEmail)
		}
		if err != nil {
			writeContactUpdateError(w, err)
			return
		}
		if err := writeAccountUpdateAudit(r.Context(), h.pool, session.Session.Account.ID, accountID, reason, []string{"school_email"}); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	defer tx.Rollback(r.Context())
	var currentUsername string
	if err := tx.QueryRow(r.Context(), `SELECT username FROM accounts WHERE id = $1 FOR UPDATE`, accountID).
		Scan(&currentUsername); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "account not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	changedFields := make([]string, 0, 5)
	if body.Username != nil && username != currentUsername {
		if _, err := tx.Exec(r.Context(), `UPDATE accounts SET username = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $1`, accountID, username); err != nil {
			if isUniqueViolation(err) {
				writeError(w, http.StatusConflict, "username_taken", "username is already in use")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		changedFields = append(changedFields, "username")
	}
	if requestedProfile {
		var oldProfile profile.Input
		var linksJSON []byte
		err := tx.QueryRow(r.Context(), `
			SELECT COALESCE(display_name, ''), COALESCE(bio, ''), COALESCE(links, '[]'::jsonb)
			FROM account_profiles WHERE account_id = $1`, accountID).Scan(&oldProfile.DisplayName, &oldProfile.Bio, &linksJSON)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		oldProfile.Links = []profile.Link{}
		if len(linksJSON) > 0 {
			if err := json.Unmarshal(linksJSON, &oldProfile.Links); err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
				return
			}
		}
		newProfile := oldProfile
		if body.DisplayName != nil {
			newProfile.DisplayName = *body.DisplayName
		}
		if body.Bio != nil {
			newProfile.Bio = *body.Bio
		}
		if body.Links != nil {
			newProfile.Links = *body.Links
		}
		if err := newProfile.Normalize(); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_profile", "profile fields or links are invalid")
			return
		}
		profileChanged := false
		if newProfile.DisplayName != oldProfile.DisplayName {
			changedFields = append(changedFields, "display_name")
			profileChanged = true
		}
		if newProfile.Bio != oldProfile.Bio {
			changedFields = append(changedFields, "bio")
			profileChanged = true
		}
		if !sameProfileLinks(newProfile.Links, oldProfile.Links) {
			changedFields = append(changedFields, "links")
			profileChanged = true
		}
		if profileChanged {
			links, err := json.Marshal(newProfile.Links)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
				return
			}
			if _, err := tx.Exec(r.Context(), `
				INSERT INTO account_profiles (account_id, display_name, bio, links)
				VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), $4::jsonb)
				ON CONFLICT (account_id) DO UPDATE SET
					display_name = NULLIF($2, ''), bio = NULLIF($3, ''), links = $4::jsonb`,
				accountID, newProfile.DisplayName, newProfile.Bio, string(links)); err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
				return
			}
		}
	}
	if len(changedFields) > 0 {
		fieldsJSON, err := json.Marshal(map[string][]string{"changed_fields": changedFields})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO audit_log (actor_account_id, action, entity_type, entity_key, before_data, after_data, reason)
			VALUES ($1, 'account.updated', 'account', $2, '{}'::jsonb, $3::jsonb, $4)`,
			session.Session.Account.ID, accountID.String(), string(fieldsJSON), reason); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "updated"})
}

func sameProfileLinks(a, b []profile.Link) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func writeAccountUpdateAudit(ctx context.Context, pool *pgxpool.Pool, actorID, accountID uuid.UUID, reason string, fields []string) error {
	fieldsJSON, err := json.Marshal(map[string][]string{"changed_fields": fields})
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO audit_log (actor_account_id, action, entity_type, entity_key, before_data, after_data, reason)
		VALUES ($1, 'account.updated', 'account', $2, '{}'::jsonb, $3::jsonb, $4)`,
		actorID, accountID.String(), string(fieldsJSON), reason)
	return err
}

func writeContactUpdateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrConflict):
		writeError(w, http.StatusConflict, "email_taken", "this email is already in use by another account")
	case errors.Is(err, auth.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_email", err.Error())
	case errors.Is(err, auth.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "account not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// resendActivation re-sends the school-email activation link for an account
// stuck 'pending_verification' — typically after correcting a mistyped
// school email via updateUser first. See auth.Service.AdminResendActivation.
func (h *Handler) resendActivation(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	accountID, err := uuid.Parse(r.PathValue("accountID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_account_id", "account id is invalid")
		return
	}
	expiresAt, err := h.auth.AdminResendActivation(r.Context(), accountID)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrNoSchoolEmailOnFile):
			writeError(w, http.StatusConflict, "no_school_email", "this account has no school email on file")
		case errors.Is(err, auth.ErrInvalidInput):
			writeError(w, http.StatusConflict, "not_pending", "account is not pending verification")
		case errors.Is(err, auth.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "account not found")
		case errors.Is(err, auth.ErrNotConfigured):
			writeError(w, http.StatusServiceUnavailable, "service_unavailable", "activation email is not configured")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		}
		return
	}
	if _, err := h.pool.Exec(r.Context(), `
		INSERT INTO audit_log (actor_account_id, action, entity_type, entity_key, reason)
		VALUES ($1, 'account.activation_resent', 'account', $2, '-')`, session.Session.Account.ID, accountID.String()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "sent", "expires_at": expiresAt})
}

// resendEmailVerification re-sends the contact-email verification link for
// an active account whose email_verified_at is still null — separate from
// resendActivation, which is for the pre-activation school-email link.
func (h *Handler) resendEmailVerification(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	accountID, err := uuid.Parse(r.PathValue("accountID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_account_id", "account id is invalid")
		return
	}
	expiresAt, err := h.auth.IssueEmailVerification(r.Context(), accountID)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrRateLimited):
			writeError(w, http.StatusTooManyRequests, "rate_limited", "too many verification emails sent recently")
		case errors.Is(err, auth.ErrNotConfigured):
			writeError(w, http.StatusServiceUnavailable, "service_unavailable", "email verification is not configured")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		}
		return
	}
	if _, err := h.pool.Exec(r.Context(), `
		INSERT INTO audit_log (actor_account_id, action, entity_type, entity_key, reason)
		VALUES ($1, 'account.email_verification_resent', 'account', $2, '-')`, session.Session.Account.ID, accountID.String()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "sent", "expires_at": expiresAt})
}

type suspendRequest struct {
	Reason string `json:"reason"`
}

func decodeAdminJSON(r *http.Request, dst any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<16))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return nil // empty body is allowed; caller validates required fields
		}
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("multiple JSON values")
	}
	return nil
}

func (h *Handler) suspendUser(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	accountID, err := uuid.Parse(r.PathValue("accountID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_account_id", "account id is invalid")
		return
	}
	if accountID == session.Session.Account.ID {
		writeError(w, http.StatusConflict, "cannot_suspend_self", "an administrator cannot suspend their own account")
		return
	}
	var body suspendRequest
	if err := decodeAdminJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is invalid")
		return
	}
	reason := strings.TrimSpace(body.Reason)
	if reason == "" || len(reason) > 500 {
		writeError(w, http.StatusBadRequest, "reason_required", "a suspension reason of 1-500 characters is required")
		return
	}

	revoked, status, err := suspendAccount(r.Context(), h.pool, accountID, session.Session.Account.ID, reason)
	switch {
	case errors.Is(err, errAccountNotFound):
		writeError(w, http.StatusNotFound, "not_found", "account not found")
		return
	case errors.Is(err, errAccountIsAdmin):
		writeError(w, http.StatusConflict, "cannot_suspend_admin", "an administrator account cannot be suspended via this endpoint")
		return
	case errors.Is(err, errAccountNotActive):
		writeError(w, http.StatusConflict, "not_active", "account is not active")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": status, "sessions_revoked": revoked})
}

func (h *Handler) reinstateUser(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	accountID, err := uuid.Parse(r.PathValue("accountID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_account_id", "account id is invalid")
		return
	}
	var body suspendRequest
	if err := decodeAdminJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is invalid")
		return
	}
	reason := strings.TrimSpace(body.Reason)
	if len(reason) > 500 {
		writeError(w, http.StatusBadRequest, "invalid_body", "reason is too long")
		return
	}
	if reason == "" {
		reason = "-"
	}

	err = reinstateAccount(r.Context(), h.pool, accountID, session.Session.Account.ID, reason)
	switch {
	case errors.Is(err, errAccountNotFound):
		writeError(w, http.StatusNotFound, "not_found", "account not found")
		return
	case errors.Is(err, errAccountNotSuspended):
		writeError(w, http.StatusConflict, "not_suspended", "account is not suspended")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "active"})
}

func (h *Handler) forceLogoutUser(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	accountID, err := uuid.Parse(r.PathValue("accountID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_account_id", "account id is invalid")
		return
	}
	var body suspendRequest
	if err := decodeAdminJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is invalid")
		return
	}
	reason := strings.TrimSpace(body.Reason)
	if len(reason) > 500 {
		writeError(w, http.StatusBadRequest, "invalid_body", "reason is too long")
		return
	}
	if reason == "" {
		reason = "-"
	}

	revoked, err := forceLogoutAccount(r.Context(), h.pool, accountID, session.Session.Account.ID, reason)
	switch {
	case errors.Is(err, errAccountNotFound):
		writeError(w, http.StatusNotFound, "not_found", "account not found")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions_revoked": revoked})
}

// setAdmissionsModerator grants or revokes the admissions_moderator role —
// content-only access to /admin/admissions for a brochure board moderator,
// with no visibility into anything else under /admin. Deliberately narrower
// than the generic role-grant path used for 'admin'/'service'/'ai_system'
// (see createServiceUser): this endpoint can only ever touch this one role.
func (h *Handler) setAdmissionsModerator(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	accountID, err := uuid.Parse(r.PathValue("accountID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_account_id", "account id is invalid")
		return
	}
	var body struct {
		Grant  bool   `json:"grant"`
		Reason string `json:"reason"`
	}
	if err := decodeAdminJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "request body is invalid")
		return
	}
	reason := strings.TrimSpace(body.Reason)
	if len(reason) > 500 {
		writeError(w, http.StatusBadRequest, "invalid_body", "reason is too long")
		return
	}
	if reason == "" {
		reason = "-"
	}

	err = setAdmissionsModeratorRole(r.Context(), h.pool, accountID, session.Session.Account.ID, body.Grant, reason)
	switch {
	case errors.Is(err, errAccountNotFound):
		writeError(w, http.StatusNotFound, "not_found", "account not found")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"admissions_moderator": body.Grant})
}

// resetPassword sends the same "set a new password" email the self-service
// forgot-password flow sends — for a user who's locked out and can't trigger
// it themselves. Never sees the account's plaintext email; see
// auth.Service.RequestPasswordResetForAccount.
func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	session, ok := h.requireAdminMutation(w, r)
	if !ok {
		return
	}
	accountID, err := uuid.Parse(r.PathValue("accountID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_account_id", "account id is invalid")
		return
	}
	if err := h.auth.RequestPasswordResetForAccount(r.Context(), accountID); err != nil {
		if errors.Is(err, auth.ErrNotConfigured) {
			writeError(w, http.StatusServiceUnavailable, "service_unavailable", "password reset is not configured")
			return
		}
		writeError(w, http.StatusNotFound, "not_found", "account not found or not active")
		return
	}
	if _, err := h.pool.Exec(r.Context(), `
		INSERT INTO audit_log (actor_account_id, action, entity_type, entity_key, reason)
		VALUES ($1, 'account.password_reset_sent', 'account', $2, '-')`, session.Session.Account.ID, accountID.String()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "sent"})
}

// --- data-layer helpers ---

var (
	errAccountNotFound     = errors.New("account not found")
	errAccountIsAdmin      = errors.New("account is admin")
	errAccountNotActive    = errors.New("account not active")
	errAccountNotSuspended = errors.New("account not suspended")
)

func suspendAccount(ctx context.Context, pool *pgxpool.Pool, accountID, actorID uuid.UUID, reason string) (int64, string, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, "", err
	}
	defer tx.Rollback(ctx)

	var status string
	var isAdmin bool
	err = tx.QueryRow(ctx, `
		SELECT a.account_status,
		       EXISTS (SELECT 1 FROM account_roles ar WHERE ar.account_id = a.id AND ar.role = 'admin')
		FROM accounts a WHERE a.id = $1 FOR UPDATE`, accountID).Scan(&status, &isAdmin)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", errAccountNotFound
	}
	if err != nil {
		return 0, "", err
	}
	if isAdmin {
		return 0, "", errAccountIsAdmin
	}
	if status != "active" {
		return 0, "", errAccountNotActive
	}

	if _, err := tx.Exec(ctx, `
		UPDATE accounts
		SET account_status = 'suspended', suspended_at = CURRENT_TIMESTAMP,
		    suspension_reason = $2, suspended_by = $3, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`, accountID, reason, actorID); err != nil {
		return 0, "", err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE account_sessions SET revoked_at = COALESCE(revoked_at, CURRENT_TIMESTAMP)
		WHERE account_id = $1 AND revoked_at IS NULL`, accountID)
	if err != nil {
		return 0, "", err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_log (actor_account_id, action, entity_type, entity_key, reason)
		VALUES ($1, 'account.suspended', 'account', $2, $3)`, actorID, accountID.String(), reason); err != nil {
		return 0, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, "", err
	}
	return tag.RowsAffected(), "suspended", nil
}

func reinstateAccount(ctx context.Context, pool *pgxpool.Pool, accountID, actorID uuid.UUID, reason string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `SELECT account_status FROM accounts WHERE id = $1 FOR UPDATE`, accountID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return errAccountNotFound
	}
	if err != nil {
		return err
	}
	if status != "suspended" {
		return errAccountNotSuspended
	}
	if _, err := tx.Exec(ctx, `
		UPDATE accounts
		SET account_status = 'active', suspended_at = NULL, suspension_reason = NULL,
		    suspended_by = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`, accountID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_log (actor_account_id, action, entity_type, entity_key, reason)
		VALUES ($1, 'account.reinstated', 'account', $2, $3)`, actorID, accountID.String(), reason); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func forceLogoutAccount(ctx context.Context, pool *pgxpool.Pool, accountID, actorID uuid.UUID, reason string) (int64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM accounts WHERE id = $1)`, accountID).Scan(&exists); err != nil {
		return 0, err
	}
	if !exists {
		return 0, errAccountNotFound
	}
	tag, err := tx.Exec(ctx, `
		UPDATE account_sessions SET revoked_at = COALESCE(revoked_at, CURRENT_TIMESTAMP)
		WHERE account_id = $1 AND revoked_at IS NULL`, accountID)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_log (actor_account_id, action, entity_type, entity_key, reason)
		VALUES ($1, 'account.force_logout', 'account', $2, $3)`, actorID, accountID.String(), reason); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func setAdmissionsModeratorRole(ctx context.Context, pool *pgxpool.Pool, accountID, actorID uuid.UUID, grant bool, reason string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM accounts WHERE id = $1)`, accountID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errAccountNotFound
	}

	action := "account.admissions_moderator_revoked"
	if grant {
		action = "account.admissions_moderator_granted"
		if _, err := tx.Exec(ctx, `
			INSERT INTO account_roles (account_id, role)
			VALUES ($1, 'admissions_moderator')
			ON CONFLICT (account_id, role) DO NOTHING`, accountID); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(ctx, `
			DELETE FROM account_roles WHERE account_id = $1 AND role = 'admissions_moderator'`, accountID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_log (actor_account_id, action, entity_type, entity_key, reason)
		VALUES ($1, $2, 'account', $3, $4)`, actorID, action, accountID.String(), reason); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
