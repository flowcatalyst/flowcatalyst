package versioncache

import (
	"context"
	"log/slog"
	"time"

	lru "github.com/hashicorp/golang-lru/v2/expirable"
)

// FallbackFunc computes a principal's current version straight from the
// source of truth (Postgres) on a cache miss. Implementations should fold
// in anything that should invalidate the principal's cached tokens — not
// just the principal row itself but, e.g., the roles it holds.
type FallbackFunc func(ctx context.Context, principalID string) (time.Time, error)

// Reader is the read path callers use: a bounded, per-entry-TTL local cache
// in front of a distributed Store, in front of FallbackFunc. Keyed by
// principal ID (not by individual token) — every concurrent token/session
// for the same principal shares one entry.
type Reader struct {
	store    Store
	local    *lru.LRU[string, time.Time]
	fallback FallbackFunc
}

// NewReader builds a Reader. size bounds the local cache's entry count;
// ttl bounds how long a local entry is trusted before it's re-checked
// against store/fallback — this is what bounds the propagation delay for
// changes Bump can't cheaply target (see role-driven changes in the
// principal repository).
func NewReader(store Store, size int, ttl time.Duration, fallback FallbackFunc) *Reader {
	if store == nil {
		store = NoopStore{}
	}
	return &Reader{
		store:    store,
		local:    lru.NewLRU[string, time.Time](size, nil, ttl),
		fallback: fallback,
	}
}

// Version returns the principal's current version (the latest relevant
// updated_at): local cache hit, else distributed Store hit (populating the
// local cache), else FallbackFunc (populating both).
func (r *Reader) Version(ctx context.Context, principalID string) (time.Time, error) {
	if v, ok := r.local.Get(principalID); ok {
		return v, nil
	}

	v, ok, err := r.store.Get(ctx, principalID)
	if err != nil {
		slog.Warn("versioncache: store get failed; falling back to source of truth", "principal", principalID, "err", err)
	} else if ok {
		r.local.Add(principalID, v)
		return v, nil
	}

	v, err = r.fallback(ctx, principalID)
	if err != nil {
		return time.Time{}, err
	}
	r.store.Bump(ctx, principalID, v)
	r.local.Add(principalID, v)
	return v, nil
}
