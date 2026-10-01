package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// OpenPostgres opens a pool sized to maxConns. Every long-running service in
// this codebase — api, and each worker (notification-worker at 3 replicas)
// — calls this independently, each getting its OWN pool with its OWN
// MaxConns ceiling; PostgreSQL's max_connections (120, see
// docker-compose.yml) is shared across all of them at once. A caller must
// pick maxConns with that in mind, not just "what's fine for me alone" —
// this used to default every caller to 100, which is fine for any one
// service but sums to 700+ once every long-running service's pool is
// counted, against a shared ceiling of 120. A short-lived CLI tool (the
// bootstrap-*/reindex/reencrypt/*-tool commands) can safely pass a small
// value like 5 since it opens only briefly and never runs concurrently with
// itself.
func OpenPostgres(ctx context.Context, databaseURL string, maxConns int32) (*pgxpool.Pool, error) {
	if databaseURL == "" {
		return nil, errors.New("database URL is not configured")
	}
	if maxConns < 1 {
		maxConns = 1
	}
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	poolConfig.MaxConns = maxConns
	poolConfig.MinConns = 1
	poolConfig.MaxConnIdleTime = 5 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}
