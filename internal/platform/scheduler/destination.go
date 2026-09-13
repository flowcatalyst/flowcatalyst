package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatch"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/dispatchqueue"
)

// DestinationResolver resolves a claimed job's destination queue — the one
// place both publishers compute (tenant, priority) and compose it into a name.
//
// Extracted so the two cannot independently drift on where a job goes. The
// queue name a job is published to and the name the served router-config
// document advertises are composed by the same function from the same
// settings, so a dev/prod disagreement about queue TYPE can never become a
// disagreement about queue IDENTITY.
type DestinationResolver struct {
	tenants    *PoolCodeResolver
	priorities *SubscriptionPriorityCache
	settings   dispatch.Settings
}

// NewDestinationResolver wires the resolver.
func NewDestinationResolver(tenants *PoolCodeResolver, priorities *SubscriptionPriorityCache, settings dispatch.Settings) *DestinationResolver {
	return &DestinationResolver{tenants: tenants, priorities: priorities, settings: settings}
}

// Destination is the composed queue name item publishes to.
//
// The tenant is the job's client identifier, or the reserved platform tenant
// for a client-less or unresolved job. The priority comes from the
// subscription that raised the job: no subscription, an unresolvable one, a
// NULL stored value or unrecognised legacy text all read as DEFAULT, never an
// error and never a dropped job.
func (r *DestinationResolver) Destination(ctx context.Context, item PublishItem) (string, error) {
	tenant := dispatchqueue.TenantPlatform
	if identifier := r.tenants.ClientIdentifier(ctx, item.ClientID); identifier != nil {
		tenant = *identifier
	}
	priority := r.priorities.PriorityFor(ctx, item.SubscriptionID)
	return dispatchqueue.ComposeName(r.settings.Prefix, tenant, priority, r.settings.SQS)
}

// SubscriptionPriorityCache caches msg_subscriptions.queue — the raw stored
// dispatch priority — by subscription id, so a publish resolves a job's
// priority without a query per job. A claimed job carries only
// subscription_id, and the claim query is deliberately join-free.
//
// Refreshed on the same cadence as the pool-code cache: both cache a
// slow-moving configuration read consulted on every publish, where a stale
// read costs at most one TTL of misrouted priority, never a locked join.
type SubscriptionPriorityCache struct {
	pool *pgxpool.Pool
	ttl  time.Duration

	mu          sync.RWMutex
	stored      map[string]*string // subscription id → raw queue column
	lastRefresh time.Time
}

// NewSubscriptionPriorityCache wires the cache.
func NewSubscriptionPriorityCache(pool *pgxpool.Pool, ttl time.Duration) *SubscriptionPriorityCache {
	return &SubscriptionPriorityCache{
		pool:        pool,
		ttl:         ttl,
		stored:      make(map[string]*string),
		lastRefresh: time.Now().Add(-2 * ttl), // force an initial refresh
	}
}

// PriorityFor is the priority a job raised from subscriptionID publishes at.
// Never fails: a refresh failure serves the stale cache, and every unusable
// value reads as DEFAULT. Failing here would strand every job on the
// offending subscription rather than routing it to the normal lane.
func (c *SubscriptionPriorityCache) PriorityFor(ctx context.Context, subscriptionID string) dispatchqueue.Priority {
	if subscriptionID == "" {
		return dispatchqueue.PriorityDefault
	}
	if err := c.ensureFresh(ctx); err != nil {
		slog.Warn("subscription priority cache refresh failed; resolving from stale cache", "err", err)
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return dispatchqueue.ForPublishing(c.stored[subscriptionID])
}

func (c *SubscriptionPriorityCache) ensureFresh(ctx context.Context) error {
	c.mu.RLock()
	fresh := time.Since(c.lastRefresh) < c.ttl
	c.mu.RUnlock()
	if fresh {
		return nil
	}
	return c.refresh(ctx)
}

func (c *SubscriptionPriorityCache) refresh(ctx context.Context) error {
	rows, err := c.pool.Query(ctx, `SELECT id, queue FROM msg_subscriptions`)
	if err != nil {
		return err
	}
	defer rows.Close()
	stored := make(map[string]*string)
	for rows.Next() {
		var id string
		var queue *string
		if err := rows.Scan(&id, &queue); err != nil {
			return err
		}
		stored[id] = queue
	}
	if err := rows.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	c.stored = stored
	c.lastRefresh = time.Now()
	c.mu.Unlock()
	slog.Debug("subscription priority cache refreshed", "subscriptions", len(stored))
	return nil
}
