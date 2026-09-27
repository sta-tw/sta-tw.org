package security

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// rateLimitScript atomically increments the window counter and, only on the
// first hit of a fresh window, sets its expiry — the standard Redis
// fixed-window pattern. Slightly more lenient at window boundaries than the
// Postgres implementation's explicit window-alignment logic, which is the
// normal trade-off for O(1) Redis ops instead of a table write.
var rateLimitScript = redis.NewScript(`
local current = redis.call("INCR", KEYS[1])
if current == 1 then
    redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
return current
`)

// RedisFixedWindowLimiter is a drop-in replacement for
// PostgresFixedWindowLimiter — same DistributedLimiter interface, so none of
// the existing call sites (auth, chat, support, verification) need to change.
type RedisFixedWindowLimiter struct {
	client *redis.Client
}

func NewRedisFixedWindowLimiter(client *redis.Client) (*RedisFixedWindowLimiter, error) {
	if client == nil {
		return nil, ErrDistributedLimiterUnavailable
	}
	return &RedisFixedWindowLimiter{client: client}, nil
}

func (l *RedisFixedWindowLimiter) Allow(ctx context.Context, namespace, key string, limit int, window time.Duration, now time.Time) (bool, error) {
	if l == nil || l.client == nil {
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
	bucketKey := "ratelimit:" + namespace + ":" + key
	count, err := rateLimitScript.Run(ctx, l.client, []string{bucketKey}, window.Milliseconds()).Int64()
	if err != nil {
		return false, ErrDistributedLimiterUnavailable
	}
	return count <= int64(limit), nil
}
