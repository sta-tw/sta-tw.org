package security

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrDistributedLimiterUnavailable = errors.New("distributed rate limiter is unavailable")

// DistributedLimiter is deliberately small so application services can keep a
// process-local fast path while PostgreSQL provides the cross-replica guard.
type DistributedLimiter interface {
	Allow(context.Context, string, string, int, time.Duration, time.Time) (bool, error)
}

type PostgresFixedWindowLimiter struct {
	pool *pgxpool.Pool
}

func NewPostgresFixedWindowLimiter(pool *pgxpool.Pool) (*PostgresFixedWindowLimiter, error) {
	if pool == nil {
		return nil, ErrDistributedLimiterUnavailable
	}
	return &PostgresFixedWindowLimiter{pool: pool}, nil
}

func (l *PostgresFixedWindowLimiter) Allow(ctx context.Context, namespace, key string, limit int, window time.Duration, now time.Time) (bool, error) {
	if l == nil || l.pool == nil {
		return false, ErrDistributedLimiterUnavailable
	}
	if limit < 1 {
		limit = 1
	}
	if window <= 0 || window > 24*time.Hour {
		window = time.Minute
	}
	namespace = strings.TrimSpace(namespace)
	key = strings.TrimSpace(key)
	if namespace == "" || key == "" || len(namespace) > 64 || len(key) > 512 || strings.ContainsAny(namespace+key, "\x00\r\n") {
		return false, errors.New("rate limit key is invalid")
	}
	started := now.UTC()
	expires := started.Add(window)
	var allowed bool
	err := l.pool.QueryRow(ctx, `
		INSERT INTO rate_limit_buckets (bucket_key, window_started, request_count, expires_at)
		VALUES ($1 || ':' || $2, $3, 1, $4)
		ON CONFLICT (bucket_key) DO UPDATE
		SET window_started = CASE
		        WHEN rate_limit_buckets.expires_at <= EXCLUDED.window_started
		          OR rate_limit_buckets.window_started > EXCLUDED.window_started
		        THEN EXCLUDED.window_started
		        ELSE rate_limit_buckets.window_started
		    END,
		    request_count = CASE
		        WHEN rate_limit_buckets.expires_at <= EXCLUDED.window_started
		          OR rate_limit_buckets.window_started > EXCLUDED.window_started
		        THEN 1
		        WHEN rate_limit_buckets.request_count < $5
		        THEN rate_limit_buckets.request_count + 1
		        ELSE rate_limit_buckets.request_count
		    END,
		    expires_at = CASE
		        WHEN rate_limit_buckets.expires_at <= EXCLUDED.window_started
		          OR rate_limit_buckets.window_started > EXCLUDED.window_started
		        THEN EXCLUDED.expires_at
		        ELSE rate_limit_buckets.expires_at
		    END,
		    updated_at = CURRENT_TIMESTAMP
		RETURNING request_count <= $5
	`, namespace, key, started, expires, limit).Scan(&allowed)
	if err != nil {
		return false, ErrDistributedLimiterUnavailable
	}
	return allowed, nil
}
