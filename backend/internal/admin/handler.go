// Package admin serves cross-cutting operator endpoints that do not belong to a
// single domain: a platform statistics snapshot and a global audit-log query.
//
// Every other domain writes to the shared audit_log table but only exposes it
// filtered to one entity ("GET .../{id}/history"). This package is the one place
// an operator can query it across entity types and actors.
package admin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"sta-backend/internal/auth"
)

type Handler struct {
	auth *auth.Service
	pool *pgxpool.Pool
}

func NewHandler(authService *auth.Service, pool *pgxpool.Pool) (*Handler, error) {
	if authService == nil || pool == nil {
		return nil, errors.New("admin handler dependencies are missing")
	}
	return &Handler{auth: authService, pool: pool}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/admin/stats", h.stats)
	mux.HandleFunc("GET /api/v1/admin/audit-log", h.auditLog)
	mux.HandleFunc("GET /api/v1/admin/users", h.listUsers)
	mux.HandleFunc("GET /api/v1/admin/users/{accountID}", h.getUser)
	mux.HandleFunc("POST /api/v1/admin/users/{accountID}/suspend", h.suspendUser)
	mux.HandleFunc("POST /api/v1/admin/users/{accountID}/reinstate", h.reinstateUser)
	mux.HandleFunc("POST /api/v1/admin/users/{accountID}/force-logout", h.forceLogoutUser)
}

// requireAdmin authenticates the caller, confirms the admin role and enforces
// admin MFA. Both endpoints are read-only, so no CSRF check is applied.
func (h *Handler) requireAdmin(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, err := h.auth.Authenticate(r.Context(), r)
	if err != nil {
		if errors.Is(err, auth.ErrAdminMFARequired) || errors.Is(err, auth.ErrAdminMFAInvalid) {
			writeError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
			return auth.RequestSession{}, false
		}
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return auth.RequestSession{}, false
	}
	var isAdmin bool
	if err := h.pool.QueryRow(r.Context(),
		`SELECT EXISTS (SELECT 1 FROM account_roles WHERE account_id = $1 AND role = 'admin')`,
		session.Session.Account.ID).Scan(&isAdmin); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return auth.RequestSession{}, false
	}
	if !isAdmin {
		writeError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
		return auth.RequestSession{}, false
	}
	if err := h.auth.RequireAdminMFA(r.Context(), session.Session.Account.ID, r.Header.Get("X-MFA-Code")); err != nil {
		writeError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
		return auth.RequestSession{}, false
	}
	return session, true
}

// requireAdminMutation is requireAdmin plus the CSRF check that state-changing
// operator endpoints need. Cookie-authenticated callers must present the CSRF
// token; bearer-token callers are exempt (AuthorizeMutation handles both).
func (h *Handler) requireAdminMutation(w http.ResponseWriter, r *http.Request) (auth.RequestSession, bool) {
	session, ok := h.requireAdmin(w, r)
	if !ok {
		return auth.RequestSession{}, false
	}
	if err := h.auth.AuthorizeMutation(r, session); err != nil {
		writeError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return auth.RequestSession{}, false
	}
	return session, true
}

func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAdmin(w, r); !ok {
		return
	}
	snapshot, err := collectStats(r.Context(), h.pool)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

// --- audit log query ---

type auditRow struct {
	ID             int64           `json:"id"`
	ActorAccountID *string         `json:"actor_account_id"`
	Action         string          `json:"action"`
	EntityType     string          `json:"entity_type"`
	EntityKey      string          `json:"entity_key"`
	BeforeData     json.RawMessage `json:"before_data,omitempty"`
	AfterData      json.RawMessage `json:"after_data,omitempty"`
	Reason         string          `json:"reason"`
	RequestID      *string         `json:"request_id"`
	CreatedAt      time.Time       `json:"created_at"`
}

func (h *Handler) auditLog(w http.ResponseWriter, r *http.Request) {
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

	afterID, err := decodeAuditCursor(q.Get("cursor"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_query", "cursor is invalid")
		return
	}

	var f auditFilters
	f.EntityType = strings.TrimSpace(q.Get("entity_type"))
	f.EntityKey = strings.TrimSpace(q.Get("entity_key"))
	f.Action = strings.TrimSpace(q.Get("action"))
	if v := strings.TrimSpace(q.Get("actor")); v != "" {
		actor, parseErr := uuid.Parse(v)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "invalid_query", "actor must be a UUID")
			return
		}
		f.Actor = &actor
	}
	if v := strings.TrimSpace(q.Get("since")); v != "" {
		ts, parseErr := time.Parse(time.RFC3339, v)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "invalid_query", "since must be RFC3339")
			return
		}
		f.Since = &ts
	}
	if v := strings.TrimSpace(q.Get("until")); v != "" {
		ts, parseErr := time.Parse(time.RFC3339, v)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "invalid_query", "until must be RFC3339")
			return
		}
		f.Until = &ts
	}

	entries, err := queryAuditLog(r.Context(), h.pool, f, limit, afterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	var next string
	if len(entries) == limit && limit > 0 {
		next = encodeAuditCursor(entries[len(entries)-1].ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": entries, "next_cursor": next})
}

type auditFilters struct {
	EntityType string
	EntityKey  string
	Action     string
	Actor      *uuid.UUID
	Since      *time.Time
	Until      *time.Time
}

// queryAuditLog runs a keyset page over audit_log newest-first. afterID nil
// starts from the top; otherwise only rows with id < *afterID are returned.
func queryAuditLog(ctx context.Context, pool *pgxpool.Pool, f auditFilters, limit int, afterID *int64) ([]auditRow, error) {
	// $1 limit, $2 after-id (nullable bigint). Optional filters append from $3.
	args := []any{limit, afterID}
	where := []string{"($2::bigint IS NULL OR id < $2::bigint)"}
	add := func(clause string, value any) {
		args = append(args, value)
		where = append(where, strings.Replace(clause, "?", "$"+strconv.Itoa(len(args)), 1))
	}
	if f.EntityType != "" {
		add("entity_type = ?", f.EntityType)
	}
	if f.EntityKey != "" {
		add("entity_key = ?", f.EntityKey)
	}
	if f.Action != "" {
		add("action = ?", f.Action)
	}
	if f.Actor != nil {
		add("actor_account_id = ?", *f.Actor)
	}
	if f.Since != nil {
		add("created_at >= ?", *f.Since)
	}
	if f.Until != nil {
		add("created_at < ?", *f.Until)
	}

	sql := `SELECT id, actor_account_id::text, action, entity_type, entity_key,
	               before_data, after_data, reason, request_id, created_at
	        FROM audit_log
	        WHERE ` + strings.Join(where, " AND ") + `
	        ORDER BY id DESC
	        LIMIT $1`

	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]auditRow, 0, limit)
	for rows.Next() {
		var e auditRow
		if err := rows.Scan(&e.ID, &e.ActorAccountID, &e.Action, &e.EntityType, &e.EntityKey,
			&e.BeforeData, &e.AfterData, &e.Reason, &e.RequestID, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// The audit cursor is just the last row id; base64 keeps it opaque so clients
// treat it as a token rather than a row number to arithmetic on.
func encodeAuditCursor(id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(id, 10)))
}

func decodeAuditCursor(token string) (*int64, error) {
	if token == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil || id < 0 {
		return nil, errors.New("invalid cursor")
	}
	return &id, nil
}

// --- shared JSON helpers ---

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	var body errorBody
	body.Error.Code = code
	body.Error.Message = message
	writeJSON(w, status, body)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
