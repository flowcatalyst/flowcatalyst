package scheduler

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultPoolCode is the router's global fallback pool. A job with neither a
// dispatch pool nor a resolvable client publishes exactly this, which is the
// behaviour that predates per-client namespacing.
const DefaultPoolCode = "DEFAULT-POOL"

// DefaultPoolSuffix is the one structural read permitted on a composed pool
// code. The composed form is `{clientIdentifier}-{poolCode}` and BOTH halves may
// themselves contain hyphens, so `-` is not a safe delimiter to split on and
// nothing may reconstruct the parts. A suffix test is unambiguous because the
// literal is fixed.
const DefaultPoolSuffix = "-" + DefaultPoolCode

// poolRef is a dispatch pool's routing identity: its code and the client that
// owns it. ClientIdentifier is empty for a platform-level pool.
type poolRef struct {
	Code             string
	ClientIdentifier string
}

// PoolCodeResolver composes the pool code a dispatch job publishes.
//
// It exists because the code cannot be read off the job: msg_dispatch_jobs
// carries dispatch_pool_id and client_id, while the code lives on
// msg_dispatch_pools and the client identifier on tnt_clients. Resolving by
// join is deliberately avoided — the claim query runs FOR UPDATE SKIP LOCKED,
// and joining would extend row locking to the joined table unless written as
// FOR UPDATE OF msg_dispatch_jobs, putting a join in the hot claim path for
// data that changes almost never.
//
// Both maps refresh on the same cadence as the paused-connection cache. Pools
// and clients are few and change rarely, so a stale read costs at most one TTL
// of routing to the previous code.
type PoolCodeResolver struct {
	pool *pgxpool.Pool
	ttl  time.Duration

	mu          sync.RWMutex
	pools       map[string]poolRef // dispatch pool id → code + owning client
	clients     map[string]string  // client id → identifier
	lastRefresh time.Time
}

// NewPoolCodeResolver wires the resolver.
func NewPoolCodeResolver(pool *pgxpool.Pool, ttl time.Duration) *PoolCodeResolver {
	return &PoolCodeResolver{
		pool:        pool,
		ttl:         ttl,
		pools:       make(map[string]poolRef),
		clients:     make(map[string]string),
		lastRefresh: time.Now().Add(-2 * ttl), // force an initial refresh
	}
}

// Resolve returns the pool code to publish for a job, per the ruled chain:
//
//	pool set, pool has a client identifier   → {clientIdentifier}-{poolCode}
//	pool set, pool is platform-level         → {poolCode}, unprefixed
//	no pool, job's client resolves           → {clientIdentifier}-DEFAULT-POOL
//	neither                                  → DEFAULT-POOL
//
// Namespacing is required because msg_dispatch_pools is unique on
// (code, client_id) — two clients may each own a pool coded FAST with different
// concurrency — while the router keys pools by code alone and treats one code
// with differing settings as a conflict to reject rather than two pools. Flat
// codes would merge two clients' traffic into whichever config won.
//
// The composition happens HERE, at publish time, never at routing time: the
// router then routes by the code it is handed and needs to know nothing about
// clients, which is what confines this to the scheduler.
//
// A resolution failure is never fatal. An unknown pool id (deleted pool) falls
// through to the client's default pool, and an unknown client to the global one,
// so a job always publishes a routable code rather than being dropped.
func (r *PoolCodeResolver) Resolve(ctx context.Context, poolID, clientID string) string {
	if err := r.ensureFresh(ctx); err != nil {
		// Serve whatever is cached: routing a job to the previous code is far
		// better than stalling the claim on a lookup failure.
		slog.Warn("pool code cache refresh failed; resolving from stale cache", "err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if poolID != "" {
		if p, ok := r.pools[poolID]; ok && p.Code != "" {
			if p.ClientIdentifier != "" {
				return p.ClientIdentifier + "-" + p.Code
			}
			return p.Code
		}
	}
	if clientID != "" {
		if identifier, ok := r.clients[clientID]; ok && identifier != "" {
			return identifier + DefaultPoolSuffix
		}
	}
	return DefaultPoolCode
}

func (r *PoolCodeResolver) ensureFresh(ctx context.Context) error {
	r.mu.RLock()
	fresh := time.Since(r.lastRefresh) < r.ttl
	r.mu.RUnlock()
	if fresh {
		return nil
	}
	return r.refresh(ctx)
}

func (r *PoolCodeResolver) refresh(ctx context.Context) error {
	pools := make(map[string]poolRef)
	rows, err := r.pool.Query(ctx,
		`SELECT id, code, client_identifier FROM msg_dispatch_pools`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, code string
		var identifier *string
		if err := rows.Scan(&id, &code, &identifier); err != nil {
			rows.Close()
			return err
		}
		ref := poolRef{Code: code}
		if identifier != nil {
			ref.ClientIdentifier = *identifier
		}
		pools[id] = ref
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	clients := make(map[string]string)
	crows, err := r.pool.Query(ctx, `SELECT id, identifier FROM tnt_clients`)
	if err != nil {
		return err
	}
	for crows.Next() {
		var id, identifier string
		if err := crows.Scan(&id, &identifier); err != nil {
			crows.Close()
			return err
		}
		clients[id] = identifier
	}
	crows.Close()
	if err := crows.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	r.pools = pools
	r.clients = clients
	r.lastRefresh = time.Now()
	r.mu.Unlock()
	slog.Debug("pool code cache refreshed", "pools", len(pools), "clients", len(clients))
	return nil
}

// IsDefaultPoolCode reports whether code names a fallback pool — the global
// DEFAULT-POOL or any per-client {identifier}-DEFAULT-POOL. This is the only
// permitted structural read of a composed code.
func IsDefaultPoolCode(code string) bool {
	return code == DefaultPoolCode || strings.HasSuffix(code, DefaultPoolSuffix)
}
