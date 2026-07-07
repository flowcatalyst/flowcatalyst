// Package versioncache answers "has this principal (or a role it holds)
// changed since a given moment?" cheaply enough to check on every request.
//
// Two tiers, mirroring internal/platform/shared/ratelimit: a distributed
// Store (Redis, actively updated by Bump on every principal/role write) in
// front of the source of truth in Postgres, and — layered on top by Reader
// — a bounded in-process cache so most checks never leave the goroutine.
// Build selects Redis when FC_REDIS_URL is reachable, else a Noop store that
// always misses (every check falls through to Postgres — correct, just
// slower). The rest of the platform depends only on the Store interface.
package versioncache

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	keyPrefix           = "fc:pv:"
	redisConnectTimeout = 2 * time.Second
	// leakGuardTTL bounds Redis growth from principals that get deleted
	// without ever being explicitly evicted from cache. It is not a
	// freshness mechanism — Bump keeps live entries current.
	leakGuardTTL = 30 * 24 * time.Hour
)

// Store is the distributed version-cache backend contract. Bump is
// best-effort and never surfaces an error to its caller — a degraded cache
// must never block a principal/role write. Get reports a miss (ok=false)
// rather than an error whenever the key is simply absent.
type Store interface {
	Bump(ctx context.Context, principalID string, at time.Time)
	Get(ctx context.Context, principalID string) (at time.Time, ok bool, err error)
}

// NoopStore always misses and never records — used when Redis isn't
// configured or unreachable. Every lookup falls through to the DB fallback.
type NoopStore struct{}

// Bump does nothing.
func (NoopStore) Bump(context.Context, string, time.Time) {}

// Get always misses.
func (NoopStore) Get(context.Context, string) (time.Time, bool, error) {
	return time.Time{}, false, nil
}

// Build selects the backend: Redis when FC_REDIS_URL is set and reachable
// (PING within a short timeout), else NoopStore. The choice is logged at
// startup.
func Build(ctx context.Context) Store {
	url := os.Getenv("FC_REDIS_URL")
	if url == "" {
		slog.Info("FC_REDIS_URL not set; principal version cache is DB-only (no distributed tier)")
		return NoopStore{}
	}
	s, err := NewRedisStore(ctx, url)
	if err != nil {
		slog.Warn("FC_REDIS_URL set but Redis unreachable; principal version cache is DB-only", "err", err)
		return NoopStore{}
	}
	slog.Info("principal version cache: Redis")
	return s
}

// RedisStore stores each principal's version as a single string value at
// "fc:pv:{principalID}", written by Bump on every principal/role mutation
// and read by Reader on a local-cache miss.
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore connects, PINGs to confirm liveness, and returns the store.
// Returns an error (never panics) so Build can fall back to NoopStore.
func NewRedisStore(ctx context.Context, url string) (*RedisStore, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("invalid redis url: %w", err)
	}
	client := redis.NewClient(opts)

	pingCtx, cancel := context.WithTimeout(ctx, redisConnectTimeout)
	defer cancel()
	if pong, err := client.Ping(pingCtx).Result(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	} else if pong != "PONG" {
		_ = client.Close()
		return nil, fmt.Errorf("unexpected redis PING response: %s", pong)
	}
	return &RedisStore{client: client}, nil
}

// Bump publishes a principal's new version. Best-effort: a failure is
// logged, never returned — the caller is mid-transaction and a cache write
// must never fail a domain write.
func (s *RedisStore) Bump(ctx context.Context, principalID string, at time.Time) {
	if err := s.client.Set(ctx, keyPrefix+principalID, at.UTC().Format(time.RFC3339Nano), leakGuardTTL).Err(); err != nil {
		slog.Warn("versioncache: redis bump failed", "principal", principalID, "err", err)
	}
}

// Get reads a principal's cached version. A missing key is a clean miss
// (ok=false, err=nil), not an error.
func (s *RedisStore) Get(ctx context.Context, principalID string) (time.Time, bool, error) {
	v, err := s.client.Get(ctx, keyPrefix+principalID).Result()
	if err == redis.Nil {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("redis get: %w", err)
	}
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("versioncache: parse cached value: %w", err)
	}
	return t, true, nil
}
