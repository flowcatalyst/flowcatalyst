package runner

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/engine"
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
type version struct {
	fn       *function
	fnID     string
	number   int
	digest   string
	describe *abi.Describe
	router   *abi.Router
	roles    []string // guarded by Runner.mu

	mu       sync.Mutex // guards module, pool, state transitions
	state    versionState
	reason   string
	module   *engine.Module
	pool     *pool
	lastUsed atomic.Int64 // unix nanos
	inflight sync.WaitGroup
}

func (v *version) touch() { v.lastUsed.Store(time.Now().UnixNano()) }

func (v *version) currentState() (versionState, string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.state, v.reason
}

// acquirePool returns the version's pool, recompiling an evicted module first.
func (v *version) acquirePool(ctx context.Context, r *Runner) (*pool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	switch v.state {
	case stateReady:
		return v.pool, nil
	case stateEvicted:
		if err := r.compileLocked(ctx, v); err != nil {
			return nil, err
		}
		return v.pool, nil
	case statePreparing:
		return nil, errPreparing
	case stateFailed:
		return nil, errFailed
	}
	return nil, errUnloaded
}

// evictLocked drops the compiled module and instances; the version stays
// routable and recompiles (from the disk cache) on its next call.
func (v *version) evictLocked(ctx context.Context) {
	if v.pool != nil {
		v.pool.close()
		v.pool = nil
	}
	if v.module != nil {
		_ = v.module.Close(ctx)
		v.module = nil
	}
	v.state = stateEvicted
}

// close unloads the version after its in-flight calls finish (or grace ends).
func (v *version) close(grace time.Duration) {
	done := make(chan struct{})
	go func() { v.inflight.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(grace):
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.pool != nil {
		v.pool.close()
		v.pool = nil
	}
	if v.module != nil {
		_ = v.module.Close(context.Background())
		v.module = nil
	}
	v.state = stateClosed
}
