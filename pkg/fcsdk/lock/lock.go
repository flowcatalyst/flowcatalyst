// Package lock is a pluggable distributed-lock contract. Used by
// consumer apps to serialise work across replicas — typically
// concurrent=false scheduled jobs that may fire on more than one pod.
//
// Every backend takes a required TTL on Acquire — the lock self-
// expires after the deadline so a crashed holder doesn't permanently
// poison the key. Pick a TTL longer than your expected critical
// section plus headroom; the lock can be released early via
// Handle.Release.
//
// # Backends
//
//   - NoOp — every Acquire succeeds.
//   - Memory — process-local mutex.
//   - lock/postgreslock — table-based, opt-in import (uses pgxpool).
//   - lock/redislock — SET NX PX + Lua release, opt-in import
//     (uses github.com/redis/go-redis/v9).
//
// # Example
//
//	lp := lock.NewMemory()
//	h, err := lp.Acquire(ctx, "orders:dispatch", 30*time.Second)
//	if err != nil { return err }
//	if h == nil { return nil } // contended; skip this firing
//	defer h.Release(ctx)
//	// critical section …
package lock

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Sentinel errors. Use errors.Is to branch.
var (
	// ErrInvalidTTL — TTL was zero or negative.
	ErrInvalidTTL = errors.New("lock: TTL must be greater than zero")
	// ErrBackend — backend-level I/O failure.
	ErrBackend = errors.New("lock: backend error")
)

// Provider is the pluggable distributed-lock contract.
//
// Acquire is non-blocking: it returns (nil, nil) immediately when the
// key is held by another holder. The caller decides whether to retry,
// skip, or fail. ttl bounds how long a crashed holder can keep the
// lock for.
type Provider interface {
	Acquire(ctx context.Context, key string, ttl time.Duration) (Handle, error)
}

// Handle is returned by a successful Acquire. Always call Release
// when the critical section is done; Go has no Drop, so leaving
// Release uncalled relies entirely on TTL expiry to free the key.
type Handle interface {
	// Release frees the lock. Idempotent — safe to call multiple
	// times, though only the first call does work.
	Release(ctx context.Context) error
}

// EnsurePositiveTTL is the standard guard at the trait boundary so
// every backend returns the same error shape.
func EnsurePositiveTTL(ttl time.Duration) error {
	if ttl <= 0 {
		return ErrInvalidTTL
	}
	return nil
}

// BackendErr wraps a driver error as ErrBackend so callers can use
// errors.Is(err, lock.ErrBackend).
func BackendErr(format string, a ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrBackend}, a...)...)
}
