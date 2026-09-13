package dispatch

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/dispatchqueue"
)

// DocumentBuilder builds the {processingPools, queues} document the router
// polls. It describes what should exist; it creates and checks nothing.
//
// It reads the two tables directly rather than through the aggregate
// repositories, for the same reason PoolCodeResolver does: this is an
// infrastructure read of a few slow-moving columns, served on every router
// config poll, and loading subscriptions through their repository would
// hydrate every row's event-type bindings and custom config — none of which
// the document uses.
type DocumentBuilder struct {
	pool     *pgxpool.Pool
	settings Settings
}

// NewDocumentBuilder wires the builder.
func NewDocumentBuilder(pool *pgxpool.Pool, settings Settings) *DocumentBuilder {
	return &DocumentBuilder{pool: pool, settings: settings}
}

// poolRow is one msg_dispatch_pools row, as the document needs it.
type poolRow struct {
	code             string
	clientIdentifier *string
	concurrency      int32
	rateLimit        *int32
}

// Build renders the current document.
//
// Pools: every msg_dispatch_pools row, whatever its status. Status governs
// nothing downstream — neither the pool-code resolver a claimed job's poolCode
// goes through nor the scheduler's claim query filters on it — so a suspended
// pool's jobs are still stamped with its code. Excluding it here would only
// make the document disagree with what jobs already carry, and the router
// would fall back to the synthesised default pool, quietly changing that
// pool's concurrency and rate limit with only a log line to explain it.
//
// Queues: one per (tenant, priority actually in use). A tenant is a client's
// identifier, or "platform" for client-less dispatch. A tenant gets a DEFAULT
// queue when it owns a dispatch pool (any status) or an ACTIVE subscription;
// the platform tenant always qualifies. It additionally gets a HIGH_PRIORITY
// queue when at least one of its ACTIVE subscriptions reads that way. An extra
// queue that turns out unused costs nothing — queues are created lazily and
// the router already tolerates one that does not exist yet.
func (b *DocumentBuilder) Build(ctx context.Context) (common.RouterConfig, error) {
	pools, err := b.loadPools(ctx)
	if err != nil {
		return common.RouterConfig{}, err
	}
	tenants, highPriority, err := b.loadTenants(ctx, pools)
	if err != nil {
		return common.RouterConfig{}, err
	}
	return common.RouterConfig{
		ProcessingPools: b.buildPools(pools),
		Queues:          b.buildQueues(tenants, highPriority),
	}, nil
}

func (b *DocumentBuilder) loadPools(ctx context.Context) ([]poolRow, error) {
	rows, err := b.pool.Query(ctx,
		`SELECT code, client_identifier, concurrency, rate_limit FROM msg_dispatch_pools ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []poolRow
	for rows.Next() {
		var p poolRow
		if err := rows.Scan(&p.code, &p.clientIdentifier, &p.concurrency, &p.rateLimit); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// loadTenants returns the tenants with dispatch work, in a stable order, and
// the subset needing a HIGH_PRIORITY queue.
func (b *DocumentBuilder) loadTenants(ctx context.Context, pools []poolRow) ([]string, map[string]bool, error) {
	seen := map[string]bool{dispatchqueue.TenantPlatform: true}
	tenants := []string{dispatchqueue.TenantPlatform}
	add := func(identifier *string) {
		t := tenantOf(identifier)
		if !seen[t] {
			seen[t] = true
			tenants = append(tenants, t)
		}
	}
	for _, p := range pools {
		add(p.clientIdentifier)
	}

	rows, err := b.pool.Query(ctx,
		`SELECT client_identifier, queue FROM msg_subscriptions WHERE status = 'ACTIVE' ORDER BY id`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	highPriority := make(map[string]bool)
	for rows.Next() {
		var identifier, queue *string
		if err := rows.Scan(&identifier, &queue); err != nil {
			return nil, nil, err
		}
		add(identifier)
		// Lenient by design: NULL, blank and legacy text all read as DEFAULT,
		// so only an explicit HIGH_PRIORITY opens that lane.
		if dispatchqueue.ForPublishing(queue) == dispatchqueue.PriorityHighPriority {
			highPriority[tenantOf(identifier)] = true
		}
	}
	return tenants, highPriority, rows.Err()
}

// buildPools namespaces every pool exactly as the scheduler stamps it, so a
// stamped code always has a match in the document.
//
// The document deliberately carries no *-DEFAULT-POOL rows: the router matches
// a pool code exactly, then synthesises any code ending in -DEFAULT-POOL on
// demand, and only then warns and falls back to the bare DEFAULT-POOL it
// always injects itself. So both platform-DEFAULT-POOL and
// {client}-DEFAULT-POOL self-create, and a named pool missing from the
// document is degraded — warned and routed to the fallback — never lost.
func (b *DocumentBuilder) buildPools(pools []poolRow) []common.PoolConfig {
	out := make([]common.PoolConfig, 0, len(pools))
	for _, p := range pools {
		pc := common.PoolConfig{
			Code:        dispatchqueue.ComposePoolCode(p.code, p.clientIdentifier),
			Concurrency: uint32(max(p.concurrency, 0)),
		}
		// A nil or non-positive rate limit is "unlimited", which the wire
		// spells as an absent field.
		if p.rateLimit != nil && *p.rateLimit > 0 {
			rate := uint32(*p.rateLimit)
			pc.RateLimitPerMinute = &rate
		}
		out = append(out, pc)
	}
	return out
}

func (b *DocumentBuilder) buildQueues(tenants []string, highPriority map[string]bool) []common.QueueConfig {
	queues := make([]common.QueueConfig, 0, len(tenants))
	for _, tenant := range tenants {
		defaultQueue, err := b.queueFor(tenant, dispatchqueue.PriorityDefault)
		if err != nil {
			logOmittedTenant(tenant, err)
			continue
		}
		var highQueue *common.QueueConfig
		if highPriority[tenant] {
			q, err := b.queueFor(tenant, dispatchqueue.PriorityHighPriority)
			if err != nil {
				logOmittedTenant(tenant, err)
				continue
			}
			highQueue = &q
		}
		// Both composed before either is appended: a tenant whose identifier
		// is too long to compose ANY of its queue names is omitted entirely,
		// never half-published with only its DEFAULT queue.
		queues = append(queues, defaultQueue)
		if highQueue != nil {
			queues = append(queues, *highQueue)
		}
	}
	return queues
}

func (b *DocumentBuilder) queueFor(tenant string, priority dispatchqueue.Priority) (common.QueueConfig, error) {
	name, err := dispatchqueue.ComposeName(b.settings.Prefix, tenant, priority, b.settings.SQS)
	if err != nil {
		return common.QueueConfig{}, err
	}
	// Connections and visibility timeout are left at zero deliberately: the
	// router applies its own defaults (1 connection, 120s), and naming them
	// here would freeze this platform's opinion into every consumer.
	return common.QueueConfig{Name: name, URI: b.settings.QueueURIFor(name)}, nil
}

// logOmittedTenant reports a tenant dropped from the document. One bad
// identifier must not take every other tenant's routing down with it, so the
// build continues.
func logOmittedTenant(tenant string, err error) {
	if _, ok := errors.AsType[*dispatchqueue.NameTooLongError](err); ok {
		slog.Warn("dispatch queue name too long for SQS; omitting this tenant from the router-config "+
			"document rather than failing the whole document",
			"category", "CONFIGURATION", "tenant", tenant, "err", err)
		return
	}
	slog.Warn("could not compose a dispatch queue name; omitting this tenant from the router-config document",
		"category", "CONFIGURATION", "tenant", tenant, "err", err)
}

// tenantOf is the tenant segment for a client identifier: the identifier
// itself, or the reserved platform tenant when there is none.
func tenantOf(clientIdentifier *string) string {
	if clientIdentifier == nil || *clientIdentifier == "" {
		return dispatchqueue.TenantPlatform
	}
	return *clientIdentifier
}
