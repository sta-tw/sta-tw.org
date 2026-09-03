package search

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"sta-backend/internal/auth"
	"sta-backend/internal/security"
)

// Handler serves GET /api/v1/search (public) and
// POST /api/v1/admin/search/reindex (admin).
type Handler struct {
	auth    *auth.Service
	client  *Client
	pool    *pgxpool.Pool
	limiter *security.FixedWindowLimiter
}

func NewHandler(authService *auth.Service, client *Client, pool *pgxpool.Pool) (*Handler, error) {
	if authService == nil || client == nil || pool == nil {
		return nil, errors.New("search handler dependencies are missing")
	}
	return &Handler{
		auth:    authService,
		client:  client,
		pool:    pool,
		limiter: security.NewFixedWindowLimiter(30, time.Minute, 4096),
	}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/search", h.search)
	mux.HandleFunc("POST /api/v1/admin/search/reindex", h.reindex)
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" || len(query) > 200 {
		writeError(w, http.StatusBadRequest, "invalid_query", "q must be 1-200 characters")
		return
	}
	if !h.limiter.Allow(clientKey(r), time.Now()) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many search requests")
		return
	}
	indexes := AllIndexes
	if raw := strings.TrimSpace(r.URL.Query().Get("types")); raw != "" {
		indexes = nil
		for _, t := range strings.Split(raw, ",") {
			switch strings.TrimSpace(t) {
			case IndexSchools, IndexPrograms, IndexExperiences:
				indexes = append(indexes, strings.TrimSpace(t))
			}
		}
		if len(indexes) == 0 {
			writeError(w, http.StatusBadRequest, "invalid_query", "types must be schools, programs and/or experiences")
			return
		}
	}
	results, err := h.client.Search(r.Context(), query, indexes, 10)
	if err != nil {
		writeError(w, http.StatusBadGateway, "search_unavailable", "search backend is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"query": query, "results": results})
}

func (h *Handler) reindex(w http.ResponseWriter, r *http.Request) {
	session, err := h.auth.Authenticate(r.Context(), r)
	if err != nil {
		if errors.Is(err, auth.ErrAdminMFARequired) || errors.Is(err, auth.ErrAdminMFAInvalid) {
			writeError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
			return
		}
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	if err := h.auth.AuthorizeMutation(r, session); err != nil {
		writeError(w, http.StatusForbidden, "csrf_required", "request verification failed")
		return
	}
	var isAdmin bool
	if err := h.pool.QueryRow(r.Context(),
		`SELECT EXISTS (SELECT 1 FROM account_roles WHERE account_id = $1 AND role = 'admin')`,
		session.Session.Account.ID).Scan(&isAdmin); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if !isAdmin {
		writeError(w, http.StatusForbidden, "admin_required", "administrator permission is required")
		return
	}
	if err := h.auth.RequireAdminMFA(r.Context(), session.Session.Account.ID, r.Header.Get("X-MFA-Code")); err != nil {
		writeError(w, http.StatusPreconditionRequired, "admin_mfa_required", "administrator MFA verification is required")
		return
	}
	counts, err := Reindex(r.Context(), h.client, h.pool)
	if err != nil {
		writeError(w, http.StatusBadGateway, "reindex_failed", "search reindex failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"indexed": counts})
}

func clientKey(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		if i := strings.IndexByte(v, ','); i > 0 {
			return strings.TrimSpace(v[:i])
		}
		return strings.TrimSpace(v)
	}
	return r.RemoteAddr
}

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
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
