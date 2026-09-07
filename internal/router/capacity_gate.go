package router

import "sync"

// capacityGate wakes every poll loop parked in Manager.awaitCapacity the
// moment some pool's capacity might have changed, replacing the fixed
// time.Sleep(2s) that loop used to fall back on with an event a pool (via
// Pool.capacityFreed) or a reconfigure raises on the crossing back under
// capacity (docs/spec/router.md §3.2, G12).
//
// Defect this replaces: measured on NATS JetStream (fetch ≈ 1ms), eight
// pollers sharing one 5,120-slot pool buffer filled it in well under a
// second, the workers drained it in ~0.7s, and every poller then sat out
// the rest of a fixed 2s pause with the router at 22% CPU instead of
// pulling more work the workers were ready for. One queue alone hit
// 7,916 deliveries/s (the workers' own limit, correctly); eight queues
// together dropped to 1,312/s purely from the pause.
//
// A channel that is closed and replaced on every signal, guarded by a
// mutex — Go's equivalent of a broadcasting condition variable, usable
// inside a select alongside ctx.Done(). The snapshot-before-check protocol
// in awaitCapacity is what closes the classic lost-wakeup race: a signal
// raised between a waiter's last capacity check and the moment it starts
// waiting would otherwise be missed forever, since nothing will wake it
// again. Snapshotting the channel BEFORE the check, then waiting on that
// snapshot, closes the window — a signal landing in between has already
// closed the snapshotted channel, so the wait returns at once instead of
// blocking on a signal that already happened.
type capacityGate struct {
	mu sync.Mutex
	ch chan struct{}
}

func newCapacityGate() *capacityGate {
	return &capacityGate{ch: make(chan struct{})}
}

// snapshot returns the channel to wait on. It closes the instant the next
// signal fires. Callers MUST snapshot before checking their condition, not
// after — see the type doc.
func (g *capacityGate) snapshot() <-chan struct{} {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.ch
}

// signal wakes every waiter parked on the current snapshot. Cheap even
// under load: one lock, one channel swap — and a close with nothing
// selecting on it costs no more than the lock itself.
func (g *capacityGate) signal() {
	g.mu.Lock()
	old := g.ch
	g.ch = make(chan struct{})
	g.mu.Unlock()
	close(old)
}
