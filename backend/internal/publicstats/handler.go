// Package publicstats exposes the homepage's headline counts (brochures,
// registered accounts) via one unauthenticated endpoint. Discord member
// count has its own endpoint already (internal/community).
package publicstats

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Snapshot is the payload of GET /api/v1/stats/public.
type Snapshot struct {
	BrochureCount      int64 `json:"brochure_count"`
	RegisteredAccounts int64 `json:"registered_accounts"`
}

type Handler struct {
	pool *pgxpool.Pool
}

func NewHandler(pool *pgxpool.Pool) (*Handler, error) {
	if pool == nil {
		return nil, fmt.Errorf("publicstats: postgres pool is nil")
	}
	return &Handler{pool: pool}, nil
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/stats/public", h.get)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var snap Snapshot

	// Best-effort: one failed count shouldn't blank out the other.
	_ = h.pool.QueryRow(ctx, `SELECT count(*) FROM brochure_documents WHERE review_status = 'published'`).
		Scan(&snap.BrochureCount)
	_ = h.pool.QueryRow(ctx, `SELECT count(*) FROM accounts WHERE account_status = 'active'`).
		Scan(&snap.RegisteredAccounts)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": snap})
}
