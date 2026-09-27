package security

import (
	"context"
	"time"
)

// CheckDistributed layers a distributed rate-limit check on top of an
// already-taken local Result, for HTTP handlers that need the local Result
// for X-RateLimit-* response headers (so they can't just call
// DistributedLimiter.Allow directly the way auth.Service.rateAllowed does).
// localAllowed should be the Result.Allowed from the handler's own
// FixedWindowLimiter.Take call — mirrors auth.Service.rateAllowed's two-tier
// short-circuit: a local rejection skips the distributed round trip
// entirely, and a nil distributed limiter (not configured) allows through.
func CheckDistributed(ctx context.Context, distributed DistributedLimiter, localAllowed bool, namespace, key string, limit int, window time.Duration, now time.Time) (bool, error) {
	if !localAllowed {
		return false, nil
	}
	if distributed == nil {
		return true, nil
	}
	allowed, err := distributed.Allow(ctx, namespace, key, limit, window, now.UTC())
	if err != nil {
		return false, err
	}
	return allowed, nil
}
