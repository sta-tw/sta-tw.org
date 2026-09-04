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
	"github.com/jackc/pgx/v5/pgxpool"

	"sta-backend/internal/pagination"
)

// userSummary is one row of the operator account list.
type userSummary struct {
	ID               string     `json:"id"`
	Username         string     `json:"username"`
	IdentityStatus   string     `json:"identity_status"`
	AccountStatus    string     `json:"account_status"`
	EmailVerified    bool       `json:"email_verified"`
	IsAdmin          bool       `json:"is_admin"`
	LastLoginAt      *time.Time `json:"last_login_at"`
	SuspendedAt      *time.Time `json:"suspended_at,omitempty"`
	SuspensionReason string     `json:"suspension_reason,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type userDetail struct {
	userSummary
	SuspendedBy    *string `json:"suspended_by,omitempty"`
	ActiveSessions int     `json:"active_sessions"`
	Applications   int     `json:"applications"`
	Experiences    int     `json:"experiences"`
}

func validAccountStatus(v string) bool {
	switch v {
	case "active", "suspended", "deleted":
		return true
	}
	return false
}

func validIdentityStatus(v string) bool {
	switch v {
	case "temporary", "student", "senior":
		return true
	}
	return false
}

// listUsers is a keyset page over accounts, newest first, with optional filters
// on account_status, identity_status, the admin role and a username prefix.
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
		if !validIdentityStatus(v) {
			writeError(w, http.StatusBadRequest, "invalid_query", "identity is invalid")
			return
		}
		add("a.identity_status = ?", v)
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
	sql := `SELECT a.id::text, a.username, a.identity_status, a.account_status,
	               a.email_verified_at IS NOT NULL,
	               EXISTS (SELECT 1 FROM account_roles ar WHERE ar.account_id = a.id AND ar.role = 'admin'),
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
		if err := rows.Scan(&u.ID, &u.Username, &u.IdentityStatus, &u.AccountStatus,
			&u.EmailVerified, &u.IsAdmin, &u.LastLoginAt, &u.SuspendedAt, &u.SuspensionReason, &u.CreatedAt); err != nil {
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
	err = h.pool.QueryRow(r.Context(), `
		SELECT a.id::text, a.username, a.identity_status, a.account_status,
		       a.email_verified_at IS NOT NULL,
		       EXISTS (SELECT 1 FROM account_roles ar WHERE ar.account_id = a.id AND ar.role = 'admin'),
		       a.last_login_at, a.suspended_at, COALESCE(a.suspension_reason, ''), a.created_at,
		       sb.username,
		       (SELECT count(*) FROM account_sessions s WHERE s.account_id = a.id AND s.revoked_at IS NULL AND s.expires_at > CURRENT_TIMESTAMP),
		       (SELECT count(*) FROM applications ap WHERE ap.account_id = a.id),
		       (SELECT count(*) FROM experiences e WHERE e.author_account_id = a.id)
		FROM accounts a
		LEFT JOIN accounts sb ON sb.id = a.suspended_by
		WHERE a.id = $1`, accountID).Scan(
		&d.ID, &d.Username, &d.IdentityStatus, &d.AccountStatus, &d.EmailVerified, &d.IsAdmin,
		&d.LastLoginAt, &d.SuspendedAt, &d.SuspensionReason, &d.CreatedAt,
		&d.SuspendedBy, &d.ActiveSessions, &d.Applications, &d.Experiences)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "account not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, d)
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
