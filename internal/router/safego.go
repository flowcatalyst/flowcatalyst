package router

import (
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
)

// panicsRecovered counts every panic recovered at a goroutine boundary in
// this package, process-wide. Exported through PanicsRecovered for the
// Prometheus surface (fc_router_panics_recovered_total): a recovered panic
// keeps the process up, so without a counter it would be visible only to
// someone reading the logs at the right moment.
var panicsRecovered atomic.Uint64

// PanicsRecovered is how many panics this process has recovered at a
// router goroutine boundary since it started.
func PanicsRecovered() uint64 { return panicsRecovered.Load() }

// logRecovered records a panic recovered at a goroutine boundary: counted,
// and logged at ERROR with the goroutine's stack, so the cause can be found
// from the log line alone. A panic value on its own ("runtime error: index
// out of range") says nothing about where it happened.
//
// Call it only from inside a deferred recover: debug.Stack there still shows
// the frames that panicked.
func logRecovered(where string, r any, attrs ...any) {
	panicsRecovered.Add(1)
	args := make([]any, 0, len(attrs)+6)
	args = append(args, "where", where, "panic", r)
	args = append(args, attrs...)
	args = append(args, "stack", string(debug.Stack()))
	slog.Error("recovered panic", args...)
}

// safely runs fn and reports whether it panicked. A panic is recovered and
// logged (logRecovered) instead of taking the process down. For best-effort
// side calls into broker or target code — an ack, a nack, a settled report —
// whose failure must not stop the caller from finishing its own clean-up.
func safely(where string, fn func(), attrs ...any) (panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			logRecovered(where, r, attrs...)
			panicked = true
		}
	}()
	fn()
	return false
}

// idleGroup counts running goroutines like a sync.WaitGroup, but reports
// "none left" as a channel, so a caller with a deadline can select on it
// directly. The WaitGroup bridge it replaces (`go func() { wg.Wait();
// close(done) }()`) leaked its helper goroutine every time the deadline won:
// the helper stayed parked in Wait until the stragglers exited, which may
// be never, and Shutdown runs on every leadership loss.
//
// The zero value is ready to use.
type idleGroup struct {
	mu   sync.Mutex
	n    int
	idle chan struct{} // closed when n reaches zero; nil means "idle, not yet made"
}

// Add changes the count by delta. A negative count panics, as it does for
// sync.WaitGroup.
func (g *idleGroup) Add(delta int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.n == 0 && delta > 0 {
		// Leaving idle: a fresh channel for the next "all done".
		g.idle = make(chan struct{})
	}
	g.n += delta
	switch {
	case g.n < 0:
		panic("router: idleGroup counter went negative")
	case g.n == 0 && g.idle != nil:
		close(g.idle)
	}
}

// Done decrements the count by one.
func (g *idleGroup) Done() { g.Add(-1) }

// Idle returns a channel that is closed once no goroutine is counted. It is
// already closed when none is.
func (g *idleGroup) Idle() <-chan struct{} {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.n == 0 {
		if g.idle == nil {
			g.idle = make(chan struct{})
			close(g.idle)
		}
		return g.idle
	}
	return g.idle
}
