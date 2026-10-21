package runner

import (
	"context"
	"io"
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
	mod  *engine.Module
	cfg  engine.InstanceConfig
	warm bool // keep at least one idle instance ready
	// stdio builds an instance's stdout/stderr writers around its sink.
	stdio  func(*invSink) (stdout, stderr io.Writer)
	mu     sync.Mutex
	idle   []idleInstance
	closed bool
}

// pooledInstance is an instance with the sink its writers consult to learn
// which invocation is running on it.
type pooledInstance struct {
	*engine.Instance
	sink *invSink
}

type idleInstance struct {
	inst  *pooledInstance
	since time.Time
}

func newPool(mod *engine.Module, cfg engine.InstanceConfig, warm bool, stdio func(*invSink) (io.Writer, io.Writer)) *pool {
	return &pool{mod: mod, cfg: cfg, warm: warm, stdio: stdio}
}

func (p *pool) create(ctx context.Context) (*pooledInstance, error) {
	sink := &invSink{}
	cfg := p.cfg
	if p.stdio != nil {
		cfg.Stdout, cfg.Stderr = p.stdio(sink)
	}
	inst, err := p.mod.Instantiate(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &pooledInstance{Instance: inst, sink: sink}, nil
}

// get returns an idle instance, or creates one. engine.ErrNoMemory means the
// budget cannot cover a new instance right now.
func (p *pool) get(ctx context.Context) (*pooledInstance, error) {
	p.mu.Lock()
	if n := len(p.idle); n > 0 {
		inst := p.idle[n-1].inst
		p.idle = p.idle[:n-1]
		p.mu.Unlock()
		return inst, nil
	}
	p.mu.Unlock()
	return p.create(ctx)
}

// put returns an instance after a call. A broken, worn-out or over-grown
// instance is closed instead of kept.
func (p *pool) put(inst *pooledInstance) {
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
	inst, err := p.create(ctx)
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
	var drop []*pooledInstance
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
