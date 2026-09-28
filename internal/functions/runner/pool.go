package runner

import (
	"context"
	"sync"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/engine"
)

// Instance-reuse policy (plan §6.4).
const (
	// recycleAfterCalls retires an instance after this many calls, bounding
	// slow leaks in guest allocators.
	recycleAfterCalls = 10_000
	// idleInstanceTTL closes idle instances not used for this long.
	idleInstanceTTL = 60 * time.Second
)

// pool holds the idle instances of one loaded version. An instance is never
// shared by concurrent calls: get hands one out exclusively and put takes it
// back (or closes it).
type pool struct {
	mod    *engine.Module
	cfg    engine.InstanceConfig
	warm   bool // keep at least one idle instance ready
	mu     sync.Mutex
	idle   []idleInstance
	closed bool
}

type idleInstance struct {
	inst  *engine.Instance
	since time.Time
}

func newPool(mod *engine.Module, cfg engine.InstanceConfig, warm bool) *pool {
	return &pool{mod: mod, cfg: cfg, warm: warm}
}

// get returns an idle instance, or creates one. engine.ErrNoMemory means the
// budget cannot cover a new instance right now.
func (p *pool) get(ctx context.Context) (*engine.Instance, error) {
	p.mu.Lock()
	if n := len(p.idle); n > 0 {
		inst := p.idle[n-1].inst
		p.idle = p.idle[:n-1]
		p.mu.Unlock()
		return inst, nil
	}
	p.mu.Unlock()
	return p.mod.Instantiate(ctx, p.cfg)
}

// put returns an instance after a call. A broken, worn-out or over-grown
// instance is closed instead of kept.
func (p *pool) put(inst *engine.Instance) {
	if inst.Broken() || inst.Calls() >= recycleAfterCalls || uint64(inst.MemoryBytes()) > p.cfg.MemoryCapBytes/2 {
		_ = inst.Close(context.Background())
		return
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		_ = inst.Close(context.Background())
		return
	}
	p.idle = append(p.idle, idleInstance{inst: inst, since: time.Now()})
	p.mu.Unlock()
}

// prewarm makes sure one idle instance exists (warm functions).
func (p *pool) prewarm(ctx context.Context) error {
	p.mu.Lock()
	have := len(p.idle) > 0 || p.closed
	p.mu.Unlock()
	if have {
		return nil
	}
	inst, err := p.mod.Instantiate(ctx, p.cfg)
	if err != nil {
		return err
	}
	p.put(inst)
	return nil
}

// trim closes instances idle for longer than ttl, keeping one for a warm pool.
func (p *pool) trim(now time.Time) {
	p.mu.Lock()
	keep := p.idle[:0]
	var drop []*engine.Instance
	for i, it := range p.idle {
		retain := now.Sub(it.since) < idleInstanceTTL || (p.warm && i == len(p.idle)-1 && len(keep) == 0)
		if retain {
			keep = append(keep, it)
		} else {
			drop = append(drop, it.inst)
		}
	}
	p.idle = keep
	p.mu.Unlock()
	for _, inst := range drop {
		_ = inst.Close(context.Background())
	}
}

// close closes every idle instance and refuses future ones.
func (p *pool) close() {
	p.mu.Lock()
	idle := p.idle
	p.idle, p.closed = nil, true
	p.mu.Unlock()
	for _, it := range idle {
		_ = it.inst.Close(context.Background())
	}
}
