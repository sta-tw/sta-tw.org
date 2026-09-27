package admissions

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// CachedRepository decorates a Repository with a short-TTL Redis cache for
// the public catalogue reads (ListPrograms/GetProgram/ListSchools). This data
// is admin-edited and low-churn (the annual admissions cycle, occasional
// proofreading fixes), so a few minutes of staleness is an acceptable
// trade-off for not re-running ListPrograms' per-row N+1 query
// (loadExamItems/loadTimelineEvents once per program in the result set) on
// every request during a traffic surge.
//
// Caching here is purely an optimization, unlike the rate limiter: any Redis
// error or cache miss falls through to the inner repository rather than
// failing the request, so a Redis outage degrades performance, not
// correctness. There is no active invalidation on admin writes in this first
// pass — deliberately simple; add SCAN+DEL from the admin CRUD handlers later
// if the TTL window turns out to matter in practice.
type CachedRepository struct {
	inner  Repository
	client *redis.Client
	ttl    time.Duration
}

func NewCachedRepository(inner Repository, client *redis.Client, ttl time.Duration) *CachedRepository {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &CachedRepository{inner: inner, client: client, ttl: ttl}
}

func (c *CachedRepository) ListPrograms(ctx context.Context, query ProgramQuery) ([]Program, error) {
	key := fmt.Sprintf("cache:admissions:programs:list:%d:%s:%s:%s:%d:%d",
		query.AcademicYear, query.SchoolCode, query.ProgramCode, query.Search, query.Limit, query.Offset)
	var programs []Program
	if c.getCached(ctx, key, &programs) {
		return programs, nil
	}
	programs, err := c.inner.ListPrograms(ctx, query)
	if err != nil {
		return nil, err
	}
	c.setCached(ctx, key, programs)
	return programs, nil
}

func (c *CachedRepository) GetProgram(ctx context.Context, identifier ProgramIdentifier) (Program, error) {
	key := "cache:admissions:programs:get:" + identifier.String()
	var program Program
	if c.getCached(ctx, key, &program) {
		return program, nil
	}
	program, err := c.inner.GetProgram(ctx, identifier)
	if err != nil {
		return Program{}, err
	}
	c.setCached(ctx, key, program)
	return program, nil
}

func (c *CachedRepository) ListSchools(ctx context.Context, academicYear int) ([]School, error) {
	key := fmt.Sprintf("cache:admissions:schools:list:%d", academicYear)
	var schools []School
	if c.getCached(ctx, key, &schools) {
		return schools, nil
	}
	schools, err := c.inner.ListSchools(ctx, academicYear)
	if err != nil {
		return nil, err
	}
	c.setCached(ctx, key, schools)
	return schools, nil
}

type Invalidator interface {
	Invalidate(ctx context.Context)
}

func (c *CachedRepository) Invalidate(ctx context.Context) {
	if c.client == nil {
		return
	}
	var cursor uint64
	for {
		keys, nextCursor, err := c.client.Scan(ctx, cursor, "cache:admissions:*", 100).Result()
		if err != nil {
			break
		}
		if len(keys) > 0 {
			_ = c.client.Del(ctx, keys...).Err()
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
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
