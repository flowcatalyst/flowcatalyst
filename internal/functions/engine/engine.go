// Package engine runs function modules on wazero (docs/function-runner-plan.md
// §6.1): one runtime per runner with a disk compilation cache, the `fc` host
// module every guest calls capabilities through, the load-time module check,
// and instances whose linear memory is charged to the runner's budget.
//
// The engine knows nothing about HTTP, routing or auth: an Instance takes a
// request frame and returns a response frame, and host calls are delegated to
// the Host the caller passes in.
package engine

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/budget"
)

// Config configures an Engine.
type Config struct {
	// CacheDir holds compiled code across restarts; empty keeps it in memory.
	CacheDir string
	// Budget is charged for every guest's linear memory. Required.
	Budget *budget.Budget
	// Logger receives host-side diagnostics (not guest output).
	Logger *slog.Logger
}

// Engine owns the wazero runtime shared by every module the runner loads.
type Engine struct {
	rt     wazero.Runtime
	cache  wazero.CompilationCache
	budget *budget.Budget
	log    *slog.Logger
}

// maxPages is the runtime-wide ceiling (4 GiB); instances are capped lower by
// their allocator.
const maxPages = 65536

// New starts an engine.
func New(ctx context.Context, cfg Config) (*Engine, error) {
	if cfg.Budget == nil {
		return nil, errors.New("engine: a budget is required")
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	var cache wazero.CompilationCache
	if cfg.CacheDir != "" {
		c, err := wazero.NewCompilationCacheWithDir(cfg.CacheDir)
		if err != nil {
			return nil, fmt.Errorf("engine: compilation cache %s: %w", cfg.CacheDir, err)
		}
		cache = c
	} else {
		cache = wazero.NewCompilationCache()
	}
	rt := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().
		WithCompilationCache(cache).
		WithCloseOnContextDone(true).
		WithMemoryLimitPages(maxPages))
	e := &Engine{rt: rt, cache: cache, budget: cfg.Budget, log: log}
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("engine: wasi: %w", err)
	}
	if err := e.instantiateHostModule(ctx); err != nil {
		_ = rt.Close(ctx)
		return nil, err
	}
	return e, nil
}

// Close releases the runtime and every module compiled on it.
func (e *Engine) Close(ctx context.Context) error {
	err := e.rt.Close(ctx)
	return errors.Join(err, e.cache.Close(ctx))
}

// Budget is the budget the engine charges guest memory to.
func (e *Engine) Budget() *budget.Budget { return e.budget }

// Module is a compiled, checked function module. Instances are created from it.
type Module struct {
	e        *Engine
	cm       wazero.CompiledModule
	minBytes uint64
	maxBytes uint64 // declared maximum, or 4 GiB when undeclared
	hasInit  bool
	size     int
}

// Compile compiles wasm (served from the disk cache when it has seen these
// bytes before) and checks it against ABI v1. A failed check is a *LoadError.
func (e *Engine) Compile(ctx context.Context, wasm []byte) (*Module, error) {
	cm, err := e.rt.CompileModule(ctx, wasm)
	if err != nil {
		return nil, &LoadError{Code: LoadInvalidModule, Detail: err.Error()}
	}
	m := &Module{e: e, cm: cm, size: len(wasm)}
	if err := m.check(); err != nil {
		_ = cm.Close(ctx)
		return nil, err
	}
	return m, nil
}

// MinMemoryBytes is the module's declared minimum linear memory.
func (m *Module) MinMemoryBytes() uint64 { return m.minBytes }

// Size is the module's wasm size in bytes.
func (m *Module) Size() int { return m.size }

// Close frees the compiled module. Instances must be closed first.
func (m *Module) Close(ctx context.Context) error { return m.cm.Close(ctx) }

// InstanceConfig configures one instance.
type InstanceConfig struct {
	// MemoryCapBytes caps the instance's linear memory (the function's memoryMb).
	MemoryCapBytes uint64
	// Stdout and Stderr receive the guest's WASI output; nil discards it.
	Stdout, Stderr io.Writer
}

// ErrNoMemory means the budget cannot cover a new instance's minimum memory.
var ErrNoMemory = errors.New("engine: memory budget exhausted")

// Instantiate creates an instance, charging its minimum memory to the budget
// first. It returns ErrNoMemory when the budget cannot cover it.
func (m *Module) Instantiate(ctx context.Context, cfg InstanceConfig) (*Instance, error) {
	if m.minBytes > cfg.MemoryCapBytes {
		return nil, &LoadError{
			Code:   LoadMemoryOverCap,
			Detail: fmt.Sprintf("the module declares %d bytes of minimum memory, over the %d-byte cap", m.minBytes, cfg.MemoryCapBytes),
		}
	}
	mem := m.e.budget.Admit(m.minBytes, cfg.MemoryCapBytes)
	if mem == nil {
		return nil, ErrNoMemory
	}
	stdout, stderr := cfg.Stdout, cfg.Stderr
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	mc := wazero.NewModuleConfig().
		WithName(""). // anonymous: many instances of one module may coexist
		WithStartFunctions().
		WithStdout(stdout).
		WithStderr(stderr).
		WithSysWalltime().
		WithSysNanotime().
		WithSysNanosleep().
		WithRandSource(rand.Reader)
	mod, err := m.e.rt.InstantiateModule(experimental.WithMemoryAllocator(ctx, mem), m.cm, mc)
	if err != nil {
		mem.Cancel()
		return nil, &LoadError{Code: LoadInstantiate, Detail: err.Error()}
	}
	inst := &Instance{
		mod:      mod,
		mem:      mem,
		alloc:    mod.ExportedFunction(abi.ExportAlloc),
		handle:   mod.ExportedFunction(abi.ExportHandle),
		describe: mod.ExportedFunction(abi.ExportDescribe),
	}
	if m.hasInit {
		if _, err := mod.ExportedFunction(abi.ExportInitialize).Call(withState(ctx, nil)); err != nil {
			err = inst.classify(ctx, err)
			_ = inst.Close(context.Background())
			return nil, fmt.Errorf("engine: _initialize: %w", err)
		}
	}
	return inst, nil
}

// Instance is one live instance of a module. It is not safe for concurrent
// use: a call borrows it exclusively.
type Instance struct {
	mod                     api.Module
	mem                     *budget.InstanceMemory
	alloc, handle, describe api.Function
	calls                   int
	broken                  bool
}

// Outcome errors of a call. After any of them the instance is Broken and must
// be closed, never reused.
var (
	// ErrTrap is a guest trap (panic, unreachable, bad memory access).
	ErrTrap = errors.New("engine: the function trapped")
	// ErrDeadline is the call's context ending while the guest ran.
	ErrDeadline = errors.New("engine: the function ran past its deadline")
	// ErrOutOfMemory is a trap that followed a refused memory grow.
	ErrOutOfMemory = errors.New("engine: the function ran out of memory")
	// ErrExit is the guest calling proc_exit.
	ErrExit = errors.New("engine: the function exited")
	// ErrMalformed is output that is not a frame inside guest memory.
	ErrMalformed = errors.New("engine: the function returned a malformed frame")
)

// Handle runs one request frame through fc_handle and returns the response
// frame (a copy; it does not alias guest memory). host serves the guest's
// fc.call requests for the duration of the call.
func (i *Instance) Handle(ctx context.Context, request []byte, host Host) ([]byte, error) {
	return i.invoke(ctx, i.handle, host, request, true)
}

// Describe runs fc_describe with no capabilities and returns its document.
func (i *Instance) Describe(ctx context.Context) ([]byte, error) {
	return i.invoke(ctx, i.describe, nil, nil, false)
}

func (i *Instance) invoke(ctx context.Context, fn api.Function, host Host, input []byte, withInput bool) ([]byte, error) {
	if i.broken {
		return nil, errors.New("engine: instance is broken")
	}
	ctx = withState(ctx, host)
	var params []uint64
	if withInput {
		r, err := i.alloc.Call(ctx, uint64(len(input)))
		if err != nil {
			return nil, i.classify(ctx, err)
		}
		ptr := uint32(r[0])
		if !i.mod.Memory().Write(ptr, input) {
			i.broken = true
			return nil, fmt.Errorf("%w: fc_alloc returned %d, outside memory", ErrMalformed, ptr)
		}
		params = []uint64{uint64(ptr), uint64(len(input))}
	}
	r, err := fn.Call(ctx, params...)
	if err != nil {
		return nil, i.classify(ctx, err)
	}
	ptr, n := abi.Unpack(r[0])
	out, ok := i.mod.Memory().Read(ptr, n)
	if !ok {
		i.broken = true
		return nil, fmt.Errorf("%w: [%d, +%d) is outside memory", ErrMalformed, ptr, n)
	}
	i.calls++
	return append([]byte(nil), out...), nil
}

// classify maps a failed call to an outcome error and marks the instance broken.
func (i *Instance) classify(ctx context.Context, err error) error {
	i.broken = true
	var exit *sys.ExitError
	switch {
	case ctx.Err() != nil:
		return fmt.Errorf("%w: %w", ErrDeadline, context.Cause(ctx))
	case errors.As(err, &exit) && (exit.ExitCode() == sys.ExitCodeDeadlineExceeded || exit.ExitCode() == sys.ExitCodeContextCanceled):
		return fmt.Errorf("%w: %w", ErrDeadline, err)
	case i.mem.Refused.Load():
		return fmt.Errorf("%w: %w", ErrOutOfMemory, err)
	case errors.As(err, &exit):
		return fmt.Errorf("%w: exit code %d", ErrExit, exit.ExitCode())
	default:
		return fmt.Errorf("%w: %w", ErrTrap, err)
	}
}

// Broken reports whether the instance failed and must not be reused.
func (i *Instance) Broken() bool { return i.broken }

// Calls is how many calls completed on this instance.
func (i *Instance) Calls() int { return i.calls }

// MemoryBytes is the linear memory this instance has charged to the budget.
func (i *Instance) MemoryBytes() int64 { return i.mem.Charged() }

// Close frees the instance and returns its memory to the budget.
func (i *Instance) Close(ctx context.Context) error {
	i.broken = true
	return i.mod.Close(ctx)
}
