package router

import (
	"sync"
	"sync/atomic"
	"time"
)

// Flush TTL bounds. A target that answers 2xx with {"flushGroup": true}
// suppresses further deliveries for that message group; delaySeconds on the
// same response sets how long, clamped to these bounds.
const (
	DefaultFlushTTL = 60 * time.Second
	MaxFlushTTL     = 5 * time.Minute
)

// GroupFlushRegistry suppresses delivery for message groups a target has
// asked us to stop sending — a per-message-group circuit breaker, the
// group-scoped sibling of the per-endpoint BreakerRegistry.
//
// Semantics: a flushed group's messages are ACKed without any HTTP call,
// which spends no rate-limit token and holds no concurrency slot. That is
// only safe because the TARGET asks for it: it is asserting that it already
// owns these records (the message-pointer pattern) and will re-drive them
// itself. A target whose messages carry the only copy of the payload must
// never set flushGroup.
//
// Suppression is TTL-bounded rather than explicitly cleared, so it
// self-heals with no resume protocol: once the window lapses the next
// message of the group goes through as a probe. If the group is still
// blocked the target simply flushes again; if it has recovered, delivery
// resumes on its own.
type GroupFlushRegistry struct {
	mu    sync.Mutex
	until map[string]time.Time

	flushes    atomic.Uint64 // groups flushed (each flushGroup response)
	suppressed atomic.Uint64 // messages ACKed without delivery
}

// NewGroupFlushRegistry builds an empty registry.
func NewGroupFlushRegistry() *GroupFlushRegistry {
	return &GroupFlushRegistry{until: make(map[string]time.Time)}
}

// Flush suppresses the group for ttl (clamped to [0, MaxFlushTTL];
// non-positive means DefaultFlushTTL). An empty group is a no-op: an
// ungrouped message has no siblings to suppress. Re-flushing extends the
// window rather than shortening it, so a probe that lands mid-window can
// never pull the expiry in.
func (r *GroupFlushRegistry) Flush(group string, ttl time.Duration) bool {
	if group == "" {
		return false
	}
	if ttl <= 0 {
		ttl = DefaultFlushTTL
	}
	if ttl > MaxFlushTTL {
		ttl = MaxFlushTTL
	}
	expiry := time.Now().Add(ttl)

	r.mu.Lock()
	defer r.mu.Unlock()
	if current, ok := r.until[group]; ok && current.After(expiry) {
		return false
	}
	r.until[group] = expiry
	r.flushes.Add(1)
	return true
}

// Suppressed reports whether the group is currently flushed, evicting the
// entry as it expires so the next message probes the target. Counts each
// suppressed message.
func (r *GroupFlushRegistry) Suppressed(group string) bool {
	if group == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	expiry, ok := r.until[group]
	if !ok {
		return false
	}
	if !time.Now().Before(expiry) {
		delete(r.until, group)
		return false
	}
	r.suppressed.Add(1)
	return true
}

// SuppressedUntil reports when a group's suppression lapses. Unlike
// Suppressed it counts nothing and evicts nothing — it is the read-only
// view for operators asking "why is this group quiet?".
func (r *GroupFlushRegistry) SuppressedUntil(group string) (time.Time, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	expiry, ok := r.until[group]
	if !ok || !time.Now().Before(expiry) {
		return time.Time{}, false
	}
	return expiry, true
}

// Clear lifts suppression for a group immediately (operator override).
func (r *GroupFlushRegistry) Clear(group string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.until, group)
}

// Stats reports the live suppression set plus lifetime counters.
func (r *GroupFlushRegistry) Stats() (active int, flushes, suppressed uint64) {
	r.mu.Lock()
	now := time.Now()
	for _, expiry := range r.until {
		if now.Before(expiry) {
			active++
		}
	}
	r.mu.Unlock()
	return active, r.flushes.Load(), r.suppressed.Load()
}
