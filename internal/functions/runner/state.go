package runner

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/runtimes"
)

// function is one function the runner serves. Its settings (limits, config,
// secrets, DB bindings) are swapped atomically when desired state changes;
// its versions are guarded by the runner's lock.
type function struct {
	id       string
	address  string
	settings atomic.Pointer[control.Function]
	permits  atomic.Pointer[chan struct{}] // capacity = maxConcurrency; replaced to resize
	versions map[int]*version
	live     int            // version number the platform wants live
	serving  int            // version number actually routed as live (new-before-old)
	aliases  map[string]int // named alias → version number
}

func (f *function) snapshot() *control.Function { return f.settings.Load() }

// tryAcquire takes a permit without waiting. A resize swaps the channel;
// calls holding a permit release it to the channel they took it from.
func (f *function) tryAcquire() (release func(), ok bool) {
	ch := *f.permits.Load()
	select {
	case ch <- struct{}{}:
		return func() { <-ch }, true
	default:
		return nil, false
	}
}

// setConcurrency resizes the permits when the limit changed.
func (f *function) setConcurrency(n int) {
	if cur := f.permits.Load(); cur != nil && cap(*cur) == n {
		return
	}
	ch := make(chan struct{}, n)
	f.permits.Store(&ch)
}

// Version lifecycle on the runner.
type versionState int32

const (
	statePreparing versionState = iota // downloading / compiling
	stateReady                         // compiled; answers calls
	stateFailed                        // could not be prepared; reason says why
	stateEvicted                       // compiled code dropped while idle; recompiles on the next call
	stateClosed                        // unloaded
)

// version is one loaded version of a function.
//
// Lock order: Runner.mu before version.mu, never the reverse — nothing holding
// a version's lock takes the runner's (a version reaches its function
// through fn, whose settings are atomic).
//
// version.mu is only ever held for short field updates, never across a
// download, compile or instantiate: the lifecycle state is an atomic so the
// hot paths (resolve, heartbeat) read it without any lock, and a slow load of
// one version cannot stall anything else.
type version struct {
	fn       *function
	fnID     string
	number   int
	digest   string
	runtime  string
	describe *abi.Describe
	router   *abi.Router
	roles    []string // guarded by Runner.mu

	state  atomic.Int32 // versionState; written under mu, read anywhere
	reason atomic.Pointer[string]

	mu         sync.Mutex // guards prepared, pool, closing, loading, cancelPrep and state transitions
	prepared   *runtimes.Prepared
	pool       *pool
	closing    bool               // close has begun: nothing may be installed or retried
	loading    chan struct{}      // non-nil while an evicted version is recompiling (single flight)
	cancelPrep context.CancelFunc // aborts the running prepare

	// Retry bookkeeping for a failed version (see Runner.retryFailed).
	attempts atomic.Int32
	retryAt  atomic.Int64 // unix nanos; 0 = never retry (the failure is permanent)

	lastUsed atomic.Int64 // unix nanos
	inflight sync.WaitGroup
	work     sync.WaitGroup // a running prepare
}

func (v *version) touch() { v.lastUsed.Store(time.Now().UnixNano()) }

func (v *version) currentState() (versionState, string) {
	st := versionState(v.state.Load())
	if rp := v.reason.Load(); rp != nil && st == stateFailed {
		return st, *rp
	}
	return st, ""
}

// setStateLocked moves the version to st. Caller holds v.mu.
func (v *version) setStateLocked(st versionState, reason string) {
	v.reason.Store(&reason)
	v.state.Store(int32(st))
}

// acquirePool returns the version's pool, recompiling an evicted module first.
// The recompile runs without v.mu; concurrent callers wait for it and give up
// with ctx.
func (v *version) acquirePool(ctx context.Context, r *Runner) (*pool, error) {
	for {
		v.mu.Lock()
		//exhaustive:ignore closed falls through to the shared error below
		switch versionState(v.state.Load()) {
		case stateReady:
			p := v.pool
			v.mu.Unlock()
			return p, nil
		case stateEvicted:
			if ch := v.loading; ch != nil {
				v.mu.Unlock()
				select {
				case <-ch:
					continue
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			ch := make(chan struct{})
			v.loading = ch
			v.mu.Unlock()
			p, pl, err := r.compile(ctx, v, false)
			v.mu.Lock()
			v.loading = nil
			close(ch)
			if err != nil {
				v.mu.Unlock()
				return nil, err
			}
			if !v.installLocked(p, pl, stateEvicted) {
				v.mu.Unlock()
				p.Close(context.Background())
				return nil, errUnloaded
			}
			v.setStateLocked(stateReady, "")
			v.touch()
			v.mu.Unlock()
			return pl, nil
		case statePreparing:
			v.mu.Unlock()
			return nil, errPreparing
		case stateFailed:
			v.mu.Unlock()
			return nil, errFailed
		}
		v.mu.Unlock()
		return nil, errUnloaded
	}
}

// installLocked attaches a freshly compiled module and pool when the version is
// still in state from and not closing. It reports false (installing nothing)
// otherwise, and the caller must release what it compiled. Caller holds v.mu.
func (v *version) installLocked(p *runtimes.Prepared, pl *pool, from versionState) bool {
	if v.closing || versionState(v.state.Load()) != from {
		return false
	}
	v.prepared, v.pool = p, pl
	return true
}

// evictLocked drops the compiled module and instances; the version stays
// routable and recompiles (from the disk cache) on its next call.
func (v *version) evictLocked(ctx context.Context) {
	if v.pool != nil {
		v.pool.close()
		v.pool = nil
	}
	v.prepared.Close(ctx)
	v.prepared = nil
	v.setStateLocked(stateEvicted, "")
}

// close unloads the version: it stops any prepare, waits for in-flight calls
// (up to grace), then releases the module and instances. New calls are refused
// from the moment the grace ends or the calls finish. Nothing an in-flight call
// uses is released under it: if grace runs out first, the release is deferred
// until the last call returns.
func (v *version) close(grace time.Duration) {
	v.mu.Lock()
	v.closing = true
	cancel := v.cancelPrep
	v.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	v.work.Wait() // a prepare notices closing and stops; nothing is left half-installed

	done := make(chan struct{})
	go func() { v.inflight.Wait(); close(done) }()
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}

	v.mu.Lock()
	pl, prep := v.pool, v.prepared
	v.pool, v.prepared = nil, nil
	v.setStateLocked(stateClosed, "")
	v.mu.Unlock()

	release := func() {
		if pl != nil {
			pl.close()
		}
		prep.Close(context.Background())
	}
	select {
	case <-done:
		release()
	default:
		go func() { <-done; release() }()
	}
}
