package router

import (
	"container/heap"
	"sync"
	"time"
)

// defaultDeferralBudget bounds how many messages one queue's consumer may
// have deferred-and-not-yet-returned at once (ServerConfig.DeferralBudget
// overrides it). A deferred message is still in flight from the broker's
// point of view — invisible on SQS, unacknowledged on NATS — and SQS FIFO
// stops delivering ANYTHING from a queue once 20,000 of its messages are in
// flight, which would turn the deferral into a broker-side head-of-line
// block of its own.
//
// It must be as LARGE as that ceiling allows, not merely safe: SQS FIFO
// hands out visible messages oldest-first, so reaching the other pools'
// messages behind a slow pool's backlog means deferring EVERY visible
// message of that backlog first. A budget smaller than the backlog defers
// what it can, pauses, and leaves the rest of the backlog in front of
// everyone else — the head-of-line block, moved along by budget-many
// messages (owner, 2026-09-22: 10k backlog, 5k budget, "5000+ in flight
// and nothing moves"). 15,000 leaves ~5,000 of the ceiling for what the
// pools hold in buffers and mediation. A slow backlog beyond that on a
// shared FIFO queue cannot be worked around from the consumer side at all;
// that job needs its own queue. On the other backends the budget is only
// what bounds the churn of re-deferring a wedged backlog.
const defaultDeferralBudget = 15000

// deferralLedger is one consumer's record of when its deferred messages are
// due back, so the poll loop can tell how many are still out
// (outstanding) and when the next one lands (earliest). A min-heap of
// return times: additions come in roughly reservation order but clamped and
// jittered ones do not, and pruning only ever needs the minimum.
type deferralLedger struct {
	mu    sync.Mutex
	times returnTimes
}

// returnTimes is the heap.Interface over unix-nanos.
type returnTimes []int64

func (h returnTimes) Len() int           { return len(h) }
func (h returnTimes) Less(i, j int) bool { return h[i] < h[j] }
func (h returnTimes) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *returnTimes) Push(x any)        { *h = append(*h, x.(int64)) }
func (h *returnTimes) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// add records a message due back at t.
func (l *deferralLedger) add(t time.Time) {
	l.mu.Lock()
	heap.Push(&l.times, t.UnixNano())
	l.mu.Unlock()
}

// outstanding drops every entry due at or before now and returns how many
// remain — the number of deferred messages the broker is still holding for
// this queue.
func (l *deferralLedger) outstanding(now time.Time) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := now.UnixNano()
	for len(l.times) > 0 && l.times[0] <= cutoff {
		heap.Pop(&l.times)
	}
	return len(l.times)
}

// earliest is when the next deferred message is due, if any are out.
func (l *deferralLedger) earliest() (time.Time, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.times) == 0 {
		return time.Time{}, false
	}
	return time.Unix(0, l.times[0]), true
}
