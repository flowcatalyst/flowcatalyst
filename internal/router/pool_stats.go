package router

import (
	"time"
)

// OldestMediatingAge is how long this pool's longest-running delivery has been
// inside the mediator, or 0 when no worker is active.
//
// It is the number that makes a wedged pool legible. ActiveWorkers alone
// cannot: a pool at 1/1 is indistinguishable from a healthy busy pool and a
// pool whose single slot has been held by one delivery for forty minutes. The
// mediator's production timeout is 15 minutes over 3 attempts, so a worker can
// legitimately hold its slot — and, at concurrency 1, the whole pool and every
// message group in it — for the better part of an hour.
func (p *Pool) OldestMediatingAge() time.Duration {
	p.mediatingMu.Lock()
	defer p.mediatingMu.Unlock()
	var oldest time.Time
	for _, e := range p.mediating {
		if oldest.IsZero() || e.MediatedAt.Before(oldest) {
			oldest = e.MediatedAt
		}
	}
	if oldest.IsZero() {
		return 0
	}
	return time.Since(oldest)
}

// MediatingSnapshot returns the messages currently inside this pool's workers.
// Never reaped, so a long-running delivery stays listed for its full duration —
// the reliable answer to "which records are mediating right now" (its length
// equals ActiveWorkers).
func (p *Pool) MediatingSnapshot() []MediatingEntry {
	p.mediatingMu.Lock()
	defer p.mediatingMu.Unlock()
	out := make([]MediatingEntry, 0, len(p.mediating))
	for _, e := range p.mediating {
		out = append(out, e)
	}
	return out
}

// ActiveWorkers returns the count of goroutines currently inside processOne —
// the size of the mediating set, not a counter kept alongside it.
func (p *Pool) ActiveWorkers() uint32 {
	p.mediatingMu.Lock()
	defer p.mediatingMu.Unlock()
	return uint32(len(p.mediating))
}

// QueueSize returns the count of messages accepted but not yet being
// delivered: buffered in a group queue, waiting on a concurrency slot, or
// sitting out a retry backoff.
func (p *Pool) QueueSize() uint32 { return p.queueSize.Load() }

// Concurrency returns the current concurrency cap.
func (p *Pool) Concurrency() uint32 { return p.sem.capacity() }

// RateLimitPerMinute returns the current rate-limit (or nil if disabled)
// — the shape PoolStats reports on the wire.
func (p *Pool) RateLimitPerMinute() *uint32 {
	rate := p.limiter.Rate()
	if rate == 0 {
		return nil
	}
	return &rate
}

// IsRateLimited reports whether the limiter currently has no spare tokens.
func (p *Pool) IsRateLimited() bool { return p.limiter.IsLimited() }

// MessageGroupCount returns the number of message groups currently live in
// this pool: holding buffered messages, or with a drainer delivering one. A
// group that has drained, been released or been acked away under
// BLOCK_ON_ERROR is not counted.
func (p *Pool) MessageGroupCount() uint32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return uint32(len(p.groupQs))
}

// GroupInfo is one live message group's snapshot row for the operator
// "blocked groups" view (R-04): which groups this pool is currently holding,
// how full each is, and why nothing may be moving. A "live" entry is one
// with a groupQueue in groupQs — buffered awaiting a drainer, actively being
// drained, or parked; a fully-drained group is deleted from groupQs (see
// drainGroup's empty-buffer exit), and so is one its drainer released or
// acked away (takeGroup) or the parked-group sweep released, so none of those
// shows up here.
type GroupInfo struct {
	// Group is the message group id (Message.GroupID()).
	Group string
	// PoolCode is this pool's code — carried per-row so a caller merging
	// snapshots across pools (the dashboard's blocked-groups panel spans
	// every pool) doesn't have to track which Pool a GroupInfo came from.
	PoolCode string
	// Buffered is how many messages are sitting in this group's FIFO right
	// now (not counting one a drainer currently holds mid-delivery — that
	// message has already been popped off the buffer).
	Buffered int
	// Working is true while a drainer goroutine owns this group (draining
	// it one message at a time). False means the group is buffered with no
	// drainer running — either freshly enqueued (about to be picked up) or
	// parked (see ParkedAt).
	Working bool
	// ParkedAt is when the group was last left with no drainer (clearWorking)
	// — the zero time while Working is true, or if it has never been parked.
	// A parkedAt that keeps ageing with Working still false is the signature
	// ReleaseParkedGroups sweeps: nothing has come back to resume the group.
	ParkedAt time.Time
	// Suppressed reports whether GroupFlushRegistry currently suppresses
	// this group (a target asked to stop receiving it — see
	// GroupFlushRegistry's doc comment). SuppressedUntil is the zero time
	// when Suppressed is false.
	Suppressed      bool
	SuppressedUntil time.Time
}

// GroupSnapshot returns a point-in-time view of every live message group
// this pool is holding, for the operator "blocked groups" view (R-04).
//
// Thread-safe and allocation-light: one slice sized to len(groupQs) up
// front, no further allocation beyond the returned GroupInfo values
// themselves. p.mu is held only long enough to copy each groupQueue's
// group/working/parkedAt/len(msgs) — the GroupFlushRegistry lookup (which
// takes its OWN lock) happens after p.mu is released, so the two locks are
// never nested.
func (p *Pool) GroupSnapshot() []GroupInfo {
	p.mu.Lock()
	out := make([]GroupInfo, 0, len(p.groupQs))
	for group, gq := range p.groupQs {
		out = append(out, GroupInfo{
			Group:    group,
			PoolCode: p.cfg.Code,
			Buffered: len(gq.msgs),
			Working:  gq.working,
			ParkedAt: gq.parkedAt,
		})
	}
	p.mu.Unlock()

	for i := range out {
		if until, ok := p.flushes.SuppressedUntil(out[i].Group); ok {
			out[i].Suppressed = true
			out[i].SuppressedUntil = until
		}
	}
	return out
}

// FlushStats returns this pool's group-flush suppression stats (R-52/R-53):
// how many groups are currently suppressed, and the lifetime flush/suppressed
// counters — see GroupFlushRegistry.Stats.
func (p *Pool) FlushStats() (active int, flushes, suppressed uint64) {
	return p.flushes.Stats()
}

// ActiveFlushes returns every group currently suppressed on this pool
// (R-52's monitoring listing).
func (p *Pool) ActiveFlushes() []GroupSuppression {
	return p.flushes.ActiveSuppressions()
}

// ClearFlush lifts an active suppression on this pool early (R-52's operator
// override). Reports whether a currently-active suppression existed to
// lift.
func (p *Pool) ClearFlush(group string) bool {
	return p.flushes.Clear(group)
}

// Stats returns the dashboard-shaped snapshot of this pool.
func (p *Pool) Stats() PoolStats {
	m := p.metrics.Snapshot()
	return PoolStats{
		PoolCode:           p.cfg.Code,
		Concurrency:        p.Concurrency(),
		ActiveWorkers:      p.ActiveWorkers(),
		OldestMediatingMs:  uint64(p.OldestMediatingAge().Milliseconds()),
		QueueSize:          p.queueSize.Load(),
		QueueCapacity:      p.queueCapacity(),
		MessageGroupCount:  p.MessageGroupCount(),
		RateLimitPerMinute: p.RateLimitPerMinute(),
		IsRateLimited:      p.IsRateLimited(),
		TotalDeferred:      p.Deferred(),
		Metrics:            &m,
		Histogram:          p.metrics.HistogramSnapshot(),
	}
}
