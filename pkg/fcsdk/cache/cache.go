// Package cache is a pluggable key-value cache with required TTL on
// every write. The interface is byte-oriented so it stays driver-agnostic;
// typed access goes through the generic Get / Set / GetOrSet helpers,
// which JSON-encode/decode values.
//
// TTL is non-optional on purpose — caches without expiry silently grow
// into a memory leak in long-running services. Every write must pick a
// deadline. Use a long Duration if you really want "rarely expires".
//
// # Backends
//
//   - MemoryCache — process-local, default for tests and single-pod dev.
//   - cache/postgrescache — sqlx-style table-backed cache (opt-in import).
//   - cache/rediscache — Redis-backed cache via go-redis (opt-in import).
//
// # Example
//
//	c := cache.NewMemory()
//	_ = cache.Set(ctx, c, "user:123", &user, 60*time.Second)
//	u, _, _ := cache.Get[User](ctx, c, "user:123")
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Cache is the pluggable contract. Implementations store opaque bytes;
// typed access is provided by the Get / Set / GetOrSet free helpers.
//
// TTL is required on every write — a cache entry without an expiry is
// almost always a bug in long-running services. If you genuinely want
// near-permanent storage, pass a very long Duration.
type Cache interface {
	// GetBytes reads the raw bytes for key. Returns (nil, false, nil) for a
	// miss OR for an entry whose TTL has elapsed (expired entries are
	// treated as missing regardless of whether the backend has cleaned
	// them up yet).
	GetBytes(ctx context.Context, key string) ([]byte, bool, error)

	// SetBytes writes value for key, expiring after ttl. Overwrites any
	// existing value. Implementations must reject zero / negative TTLs
	// by returning ErrInvalidTTL.
	SetBytes(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Delete removes key. Returns nil whether or not the key existed.
	Delete(ctx context.Context, key string) error
}

// Sentinel errors. Use errors.Is to branch.
var (
	// ErrInvalidTTL — TTL was zero or negative.
	ErrInvalidTTL = errors.New("cache: TTL must be greater than zero")
	// ErrBackend — backend-level I/O failure (network, query, etc.).
	// Wrap with fmt.Errorf("...: %w", ErrBackend) for context.
	ErrBackend = errors.New("cache: backend error")
)

// SerializeError signals a Go → JSON failure on a Set / GetOrSet call.
type SerializeError struct{ Cause error }

func (e *SerializeError) Error() string { return "cache: serialize: " + e.Cause.Error() }
func (e *SerializeError) Unwrap() error { return e.Cause }

// DeserializeError signals a JSON → Go failure on a Get / GetOrSet call.
type DeserializeError struct{ Cause error }

func (e *DeserializeError) Error() string { return "cache: deserialize: " + e.Cause.Error() }
func (e *DeserializeError) Unwrap() error { return e.Cause }

// EnsurePositiveTTL is the standard guard at the trait boundary so
// every backend returns the same error shape.
func EnsurePositiveTTL(ttl time.Duration) error {
	if ttl <= 0 {
		return ErrInvalidTTL
	}
	return nil
}

// Get reads key and JSON-decodes into T. Returns (zero, false, nil) on
// a miss / expired entry. Returns *DeserializeError if the stored
// bytes don't decode into T.
func Get[T any](ctx context.Context, c Cache, key string) (T, bool, error) {
	var zero T
	raw, ok, err := c.GetBytes(ctx, key)
	if err != nil {
		return zero, false, err
	}
	if !ok {
		return zero, false, nil
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return zero, false, &DeserializeError{Cause: err}
	}
	return v, true, nil
}

// Set JSON-encodes value and writes it with the given TTL.
func Set[T any](ctx context.Context, c Cache, key string, value T, ttl time.Duration) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return &SerializeError{Cause: err}
	}
	return c.SetBytes(ctx, key, raw, ttl)
}

// GetOrSet returns the cached value if present; otherwise calls supplier,
// caches the result with ttl, and returns it.
//
// Not atomic across replicas: two callers racing on the same key may
// both invoke supplier (the loser's write overwrites the winner's).
// If you need exactly-once supplier execution, layer a
// lock.LockProvider around the call.
func GetOrSet[T any](
	ctx context.Context,
	c Cache,
	key string,
	ttl time.Duration,
	supplier func(ctx context.Context) (T, error),
) (T, error) {
	v, ok, err := Get[T](ctx, c, key)
	if err != nil {
		return v, err
	}
	if ok {
		return v, nil
	}
	fresh, err := supplier(ctx)
	if err != nil {
		var zero T
		return zero, err
	}
	if err := Set(ctx, c, key, fresh, ttl); err != nil {
		return fresh, err
	}
	return fresh, nil
}
