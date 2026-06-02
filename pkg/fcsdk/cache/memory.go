package cache

import (
	"context"
	"sync"
	"time"
)

type memEntry struct {
	value     []byte
	expiresAt time.Time
}

// MemoryCache is a process-local cache for tests and single-pod
// deployments. Expired entries are reaped lazily on read — no
// background sweeper, so memory cost is bounded by the number of
// distinct keys ever written (until each is read again or
// ReapExpired is called).
type MemoryCache struct {
	mu  sync.RWMutex
	m   map[string]memEntry
	now func() time.Time
}

// NewMemory builds an empty MemoryCache.
func NewMemory() *MemoryCache {
	return &MemoryCache{m: map[string]memEntry{}, now: time.Now}
}

// GetBytes returns the value if present and not expired.
func (c *MemoryCache) GetBytes(_ context.Context, key string) ([]byte, bool, error) {
	now := c.now()
	// Fast read path.
	c.mu.RLock()
	e, ok := c.m[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false, nil
	}
	if e.expiresAt.After(now) {
		// Copy: callers may mutate, and the cached bytes must stay stable.
		out := make([]byte, len(e.value))
		copy(out, e.value)
		return out, true, nil
	}
	// Expired — escalate to write lock to evict.
	c.mu.Lock()
	if e2, ok := c.m[key]; ok && !e2.expiresAt.After(now) {
		delete(c.m, key)
	}
	c.mu.Unlock()
	return nil, false, nil
}

// SetBytes inserts or replaces key with the given TTL.
func (c *MemoryCache) SetBytes(_ context.Context, key string, value []byte, ttl time.Duration) error {
	if err := EnsurePositiveTTL(ttl); err != nil {
		return err
	}
	stored := make([]byte, len(value))
	copy(stored, value)
	c.mu.Lock()
	c.m[key] = memEntry{value: stored, expiresAt: c.now().Add(ttl)}
	c.mu.Unlock()
	return nil
}

// Delete removes key. Idempotent — returns nil whether or not it existed.
func (c *MemoryCache) Delete(_ context.Context, key string) error {
	c.mu.Lock()
	delete(c.m, key)
	c.mu.Unlock()
	return nil
}

// ReapExpired walks the map and drops entries whose TTL has elapsed.
// The lazy reap on read covers the common case; call this from a
// periodic task if you write keys that are rarely read back.
func (c *MemoryCache) ReapExpired(_ context.Context) {
	now := c.now()
	c.mu.Lock()
	for k, e := range c.m {
		if !e.expiresAt.After(now) {
			delete(c.m, k)
		}
	}
	c.mu.Unlock()
}
