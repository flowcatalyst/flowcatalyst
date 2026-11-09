package dispatchpool

import (
	"context"
	"sync"
	"time"
)

// A pool's status governs delivery (owner ruling 2026-11-09):
//
//   - SUSPENDED: nothing in the pool is delivered; its jobs WAIT (they are not
//     dropped). The scheduler leaves them PENDING, unclaimed, and the delivery
//     callback hands back, to PENDING, any already on the queue when the pool
//     was suspended. Resuming the pool lets them flow again, in order.
//   - ARCHIVED: no new job may name the pool (refused where a job or a
//     subscription is created); jobs that already name it wait like a
//     suspended pool's.
//
// Both are "held": HeldStatuses. The router is told nothing new — the
// router-config document still lists every pool, so codes a job already
// carries keep resolving to their own concurrency and rate limit.

// HeldStatuses are the pool statuses whose jobs are not delivered.
var HeldStatuses = []Status{StatusSuspended, StatusArchived}

// IsHeld reports whether a pool in status s delivers nothing.
func (s Status) IsHeld() bool { return s == StatusSuspended || s == StatusArchived }

// HeldPoolCache caches the ids of the held pools (SUSPENDED or ARCHIVED),
// refreshed every ttl. Read on every scheduler claim and every delivery
// callback, so a status change takes effect within ttl.
type HeldPoolCache struct {
	repo *Repository
	ttl  time.Duration

	mu          sync.RWMutex
	ids         map[string]struct{}
	lastRefresh time.Time
}

// NewHeldPoolCache wires the cache. ttl <= 0 refreshes on every read.
func NewHeldPoolCache(repo *Repository, ttl time.Duration) *HeldPoolCache {
	return &HeldPoolCache{repo: repo, ttl: ttl, ids: map[string]struct{}{}}
}

// IDs returns a copy of the held pool ids, refreshing if stale. A refresh
// failure is returned (the caller decides; the scheduler skips the claim, the
// callback answers 503 so the router keeps the message).
func (c *HeldPoolCache) IDs(ctx context.Context) (map[string]struct{}, error) {
	if err := c.refreshIfStale(ctx); err != nil {
		return nil, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]struct{}, len(c.ids))
	for k := range c.ids {
		out[k] = struct{}{}
	}
	return out, nil
}

// IsHeld reports whether the pool with this id is SUSPENDED or ARCHIVED.
func (c *HeldPoolCache) IsHeld(ctx context.Context, poolID string) (bool, error) {
	if err := c.refreshIfStale(ctx); err != nil {
		return false, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, held := c.ids[poolID]
	return held, nil
}

func (c *HeldPoolCache) refreshIfStale(ctx context.Context) error {
	c.mu.RLock()
	fresh := !c.lastRefresh.IsZero() && time.Since(c.lastRefresh) < c.ttl
	c.mu.RUnlock()
	if fresh {
		return nil
	}
	ids, err := c.repo.HeldIDs(ctx)
	if err != nil {
		return err
	}
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	c.mu.Lock()
	c.ids = set
	c.lastRefresh = time.Now()
	c.mu.Unlock()
	return nil
}
