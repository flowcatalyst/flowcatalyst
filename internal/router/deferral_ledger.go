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
// block of its own. Half of that, less what the pools themselves may hold,
// keeps well clear. On the other backends the budget is only what bounds
// the churn of re-deferring a wedged backlog.
const defaultDeferralBudget = 5000

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
