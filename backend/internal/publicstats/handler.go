// Package publicstats exposes the homepage's headline counts (participating
// programmes, published brochures, and registered accounts) via one
// unauthenticated endpoint. Discord member count has its own endpoint already
// (internal/community).
package publicstats

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Snapshot is the payload of GET /api/v1/stats/public.
type Snapshot struct {
	ProgramCount       int64 `json:"program_count"`
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

	// Match the public catalogue: only published programmes with a positive
	// quota count, and collapse historical rows to the newest year for each
	// school/programme pair.
	_ = h.pool.QueryRow(ctx, `
		WITH ranked AS (
			SELECT row_number() OVER (
				PARTITION BY school_code, program_code
				ORDER BY academic_year DESC
			) AS rn
			FROM academic_programs
			WHERE review_status = 'published' AND admission_quota > 0
		)
		SELECT count(*) FROM ranked WHERE rn = 1
	`).Scan(&snap.ProgramCount)

	// Keep the existing brochure count in the response for API compatibility.
	_ = h.pool.QueryRow(ctx, `SELECT count(*) FROM brochure_documents WHERE review_status = 'published'`).
		Scan(&snap.BrochureCount)
	_ = h.pool.QueryRow(ctx, `SELECT count(*) FROM accounts WHERE account_status = 'active'`).
		Scan(&snap.RegisteredAccounts)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": snap})
}
