// Package engine runs function modules on wazero (docs/function-runner-plan.md
// §6.1): one runtime per runner with a disk compilation cache, the `fc` host
// module every guest calls capabilities through, the load-time module check,
// and instances whose linear memory is charged to the runner's budget.
//
// Trust model (owner ruling 2026-10-21): every function is first-party code, written and
// published by the platform's operators; customers never run their own code. The sandbox
// isolates faults and limits reach, and is not hardened against a hostile author. Do not
// extend the engine to run third-party code without a new owner decision.
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
	"sync/atomic"
	"time"

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

// Engine owns the wazero runtimes shared by every module the runner loads.
//
// There are two, because wazero fixes how a call is stopped when the runtime is
// built (docs/function-runtime-deadlines.md):
//
//   - rt is preemptible: WithCloseOnContextDone(true) makes wazero check the
//     context at every loop header and function entry, so ANY guest stops at
//     its deadline. That check costs a compiled guest 4-10x on compute. Every
//     module a function author supplies (Wasm) runs here.
//   - coop is cooperative: no such check. Only the platform's own JS engine
//     runs here, because it enforces the deadline itself, from inside, through
//     QuickJS's interrupt handler (armed per call with fc_js_deadline), and the
//     host keeps a watchdog for the case where that does not stop it.
type Engine struct {
	rt     wazero.Runtime
	coop   wazero.Runtime
	cache  wazero.CompilationCache // shared by both runtimes
	budget *budget.Budget
	log    *slog.Logger
	stuck  atomic.Int64
}

// ExportJSDeadline is the export a cooperative module must have: (ms i64) arms
// the deadline for the next call, 0 clears it. It is the JS engine's, not part
// of ABI v1.
const ExportJSDeadline = "fc_js_deadline"

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
	// One cache serves both runtimes: wazero folds the termination setting
	// into the module ID the cache is keyed by, so the same bytes compiled for
	// each runtime are separate entries (TestSharedCacheNeverCrossesRuntimes
	// pins that, so a wazero upgrade that changed it fails here).
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
	e := &Engine{cache: cache, budget: cfg.Budget, log: log}
	e.rt = wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().
		WithCompilationCache(cache).
		WithCloseOnContextDone(true).
		WithMemoryLimitPages(maxPages))
	e.coop = wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().
		WithCompilationCache(cache).
		WithCloseOnContextDone(false).
		WithMemoryLimitPages(maxPages))
	for _, rt := range []wazero.Runtime{e.rt, e.coop} {
		if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
			_ = e.Close(ctx)
			return nil, fmt.Errorf("engine: wasi: %w", err)
		}
		if err := e.instantiateHostModule(ctx, rt); err != nil {
			_ = e.Close(ctx)
			return nil, err
		}
	}
	return e, nil
}

// Close releases the runtimes and every module compiled on them.
func (e *Engine) Close(ctx context.Context) error {
	return errors.Join(e.rt.Close(ctx), e.coop.Close(ctx), e.cache.Close(ctx))
}

// Stuck is how many cooperative calls are running past their deadline plus the
// watchdog grace: the runner gave up on them and answered the caller, but
// their goroutines are still burning CPU because wasm cannot be killed from
// outside on this runtime. A non-zero value for long means the process should
// be restarted; see StuckLimit.
func (e *Engine) Stuck() int64 { return e.stuck.Load() }

// StuckLimit is the number of stuck calls at which the engine reports itself
// unhealthy (Healthy), so the orchestrator replaces the process.
const StuckLimit = 4

// Healthy is false when StuckLimit or more calls have outlived their deadline.
func (e *Engine) Healthy() bool { return e.stuck.Load() < StuckLimit }

// Budget is the budget the engine charges guest memory to.
func (e *Engine) Budget() *budget.Budget { return e.budget }

// Module is a compiled, checked function module. Instances are created from it.
type Module struct {
	e        *Engine
	rt       wazero.Runtime
	coop     bool
	cm       wazero.CompiledModule
	minBytes uint64
	maxBytes uint64 // declared maximum, or 4 GiB when undeclared
	hasInit  bool
	size     int
}

// Compile compiles wasm (served from the disk cache when it has seen these
// bytes before) and checks it against ABI v1. A failed check is a *LoadError.
func (e *Engine) Compile(ctx context.Context, wasm []byte) (*Module, error) {
	return e.compile(ctx, e.rt, false, wasm)
}

// CompileCooperative compiles the platform's own JS engine onto the
// cooperative runtime (see Engine). The module must export fc_js_deadline,
// which is how every call is bounded. Never use it for a module a function
// author supplied: nothing outside the module can stop it.
func (e *Engine) CompileCooperative(ctx context.Context, wasm []byte) (*Module, error) {
	return e.compile(ctx, e.coop, true, wasm)
}

func (e *Engine) compile(ctx context.Context, rt wazero.Runtime, coop bool, wasm []byte) (*Module, error) {
	cm, err := rt.CompileModule(ctx, wasm)
	if err != nil {
		return nil, &LoadError{Code: LoadInvalidModule, Detail: err.Error()}
	}
	m := &Module{e: e, rt: rt, coop: coop, cm: cm, size: len(wasm)}
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
	// Preload, when set, is handed to the instance once after _initialize
	// (a JS function's script, loaded into the shared JS engine).
	Preload *Preload
}

// Preload names an export (ptr, len) → i32 that takes a buffer allocated with
// fc_alloc; a non-zero result fails the instance.
type Preload struct {
	Export string
	Data   []byte
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
	mod, err := m.rt.InstantiateModule(experimental.WithMemoryAllocator(ctx, mem), m.cm, mc)
	if err != nil {
		mem.Cancel()
		return nil, &LoadError{Code: LoadInstantiate, Detail: err.Error()}
	}
	inst := &Instance{
		e:        m.e,
		mod:      mod,
		mem:      mem,
		alloc:    mod.ExportedFunction(abi.ExportAlloc),
		handle:   mod.ExportedFunction(abi.ExportHandle),
		describe: mod.ExportedFunction(abi.ExportDescribe),
	}
	if m.coop {
		inst.deadline = mod.ExportedFunction(ExportJSDeadline)
	}
	if m.hasInit {
		if _, err := mod.ExportedFunction(abi.ExportInitialize).Call(withState(ctx, nil)); err != nil {
			err = inst.classify(ctx, err)
			_ = inst.Close(context.Background())
			return nil, fmt.Errorf("engine: _initialize: %w", err)
		}
	}
	if p := cfg.Preload; p != nil {
		if err := inst.preload(ctx, p); err != nil {
			_ = inst.Close(context.Background())
			return nil, err
		}
	}
	return inst, nil
}

func (i *Instance) preload(ctx context.Context, p *Preload) error {
	fn := i.mod.ExportedFunction(p.Export)
	if fn == nil {
		return &LoadError{Code: LoadExportMissing, Detail: "the module does not export " + p.Export}
	}
	ctx = withState(ctx, nil)
	r, err := i.alloc.Call(ctx, uint64(len(p.Data)))
	if err != nil {
		return &LoadError{Code: LoadInstantiate, Detail: "preload: " + i.classify(ctx, err).Error()}
	}
	if !i.mod.Memory().Write(uint32(r[0]), p.Data) {
		return &LoadError{Code: LoadInstantiate, Detail: "preload: fc_alloc returned a buffer outside memory"}
	}
	rc, err := i.run(ctx, fn, r[0], uint64(len(p.Data)))
	if err != nil {
		return &LoadError{Code: LoadInstantiate, Detail: "preload: " + i.classify(ctx, err).Error()}
	}
	if len(rc) != 1 || rc[0] != 0 {
		return &LoadError{Code: LoadInstantiate, Detail: p.Export + " refused the data (see the function's log)"}
	}
	i.broken = false
	return nil
}

// Instance is one live instance of a module. It is not safe for concurrent
// use: a call borrows it exclusively.
type Instance struct {
	e                       *Engine
	deadline                api.Function // fc_js_deadline; set on cooperative instances only
	stuck                   atomic.Bool  // abandoned mid-call; its goroutine closes it
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
	// ErrStuck is a cooperative call that did not stop when its deadline
	// passed and was abandoned after the watchdog grace. It is also an
	// ErrDeadline. The instance is leaked until the call returns, if it ever does.
	ErrStuck = fmt.Errorf("%w: it did not stop and was abandoned", ErrDeadline)
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
	r, err := i.run(ctx, fn, params...)
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
	case errors.Is(err, ErrDeadline):
		return err
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
	if i.stuck.Load() {
		return nil // still running; the goroutine that abandoned it closes it
	}
	return i.mod.Close(ctx)
}

// cooperativeFallback bounds a cooperative call whose context has no deadline,
// so a runaway can never run unbounded.
const cooperativeFallback = 30 * time.Second

// stuckGrace is how long past its deadline a cooperative call may run before
// the watchdog abandons it. The slowest native stretch measured in QuickJS (a
// BigInt squaring chain) overran by about 0.8 s; 2 s covers it with room. A
// variable only so the watchdog test need not wait.
var stuckGrace = 2 * time.Second

// run calls fn. On a preemptible instance that is wazero's own context
// handling. On a cooperative one the guest is armed with the call's remaining
// time (QuickJS interrupts itself when it passes), the call runs on its own
// goroutine, and a watchdog abandons it stuckGrace after the deadline if the
// guest still has not returned.
func (i *Instance) run(ctx context.Context, fn api.Function, params ...uint64) ([]uint64, error) {
	if i.deadline == nil {
		return fn.Call(ctx, params...)
	}
	limit := cooperativeFallback
	dl, hasDeadline := ctx.Deadline()
	if hasDeadline {
		limit = time.Until(dl)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("%w: the deadline had already passed", ErrDeadline)
	}
	// Round up: the guest must never stop before the context would have.
	ms := uint64((limit + time.Millisecond - 1) / time.Millisecond)
	if _, err := i.deadline.Call(ctx, ms); err != nil {
		return nil, err
	}
	type result struct {
		r   []uint64
		err error
	}
	done := make(chan result, 1)
	go func() {
		r, err := fn.Call(ctx, params...)
		done <- result{r, err}
	}()
	watchdog := time.NewTimer(limit + stuckGrace)
	defer watchdog.Stop()
	select {
	case res := <-done:
		if res.err == nil && hasDeadline && !time.Now().Before(dl) {
			// The guest stopped itself at the deadline (or finished just past
			// it): either way the call is out of time and its result is not
			// the function's answer.
			return nil, fmt.Errorf("%w: the guest stopped at its deadline", ErrDeadline)
		}
		return res.r, res.err
	case <-watchdog.C:
		i.broken = true
		i.stuck.Store(true)
		n := i.e.stuck.Add(1)
		i.e.log.Error("function call did not stop at its deadline and was abandoned; its thread keeps running until it returns",
			"stuck_calls", n, "limit", StuckLimit)
		go func() {
			<-done
			i.e.stuck.Add(-1)
			_ = i.mod.Close(context.Background())
		}()
		return nil, ErrStuck
	}
}
