package schools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// CachedRepository decorates a Repository with a short-TTL Redis cache for
// List, the public school-listing read — schools are admin-edited rarely, so
// a few minutes of staleness is an acceptable trade-off for not re-scanning
// and fuzzy-searching the whole table on every request during a traffic
// surge. Every other method passes straight through uncached: IsAdmin/Upsert/
// ListHistory are either admin-only or need-fresh-data operations, not the
// hot public path this exists for.
//
// Caching here is purely an optimization, unlike the rate limiter: any Redis
// error or cache miss falls through to the inner repository rather than
// failing the request. No active invalidation on admin writes in this first
// pass — see the equivalent note in admissions.CachedRepository.
type CachedRepository struct {
	inner  Repository
	client *redis.Client
	ttl    time.Duration
}

func NewCachedRepository(inner Repository, client *redis.Client, ttl time.Duration) *CachedRepository {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &CachedRepository{inner: inner, client: client, ttl: ttl}
}

func (c *CachedRepository) List(ctx context.Context, includeInactive bool) ([]School, error) {
	key := fmt.Sprintf("cache:schools:list:%t", includeInactive)
	var list []School
	if c.getCached(ctx, key, &list) {
		return list, nil
	}
	list, err := c.inner.List(ctx, includeInactive)
	if err != nil {
		return nil, err
	}
	c.setCached(ctx, key, list)
	return list, nil
}

func (c *CachedRepository) IsAdmin(ctx context.Context, accountID uuid.UUID) (bool, error) {
	return c.inner.IsAdmin(ctx, accountID)
}

func (c *CachedRepository) Upsert(ctx context.Context, accountID uuid.UUID, input BatchInput) ([]School, error) {
	return c.inner.Upsert(ctx, accountID, input)
}

func (c *CachedRepository) ListHistory(ctx context.Context, accountID uuid.UUID, entityKey string) ([]AuditEvent, error) {
	return c.inner.ListHistory(ctx, accountID, entityKey)
}

func (c *CachedRepository) getCached(ctx context.Context, key string, dest any) bool {
	if c.client == nil {
		return false
	}
	raw, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		return false
	}
	return json.Unmarshal(raw, dest) == nil
}

func (c *CachedRepository) setCached(ctx context.Context, key string, value any) {
	if c.client == nil {
		return
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return
	}
	_ = c.client.Set(ctx, key, raw, c.ttl).Err()
}
