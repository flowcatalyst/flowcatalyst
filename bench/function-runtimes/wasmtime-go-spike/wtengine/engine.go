//go:build wasmtime

// Package wtengine runs ABI v1 function modules on wasmtime through
// wasmtime-go (cgo). It is a spike (docs/function-runtime-wasmtime-go-spike-2026-11-17.md):
// a port of internal/functions/engine's call path, not a replacement for it.
//
// What is the same as the production engine: the ABI v1 module contract
// (exports, imports, memory), the fc.call / fc.take host functions with the
// same framing, copies and error frames (abi.EncodeFrame, abi.Pack), the same
// Host interface (engine.Host) behind them, _initialize, the JS engine's
// preload (fc_js_load) and fc_js_deadline arming, and the outcome errors
// (engine.ErrDeadline, ErrTrap, ErrOutOfMemory, ErrExit, ErrMalformed), which
// are the production engine's own values.
//
// What differs, because wasmtime differs: a call is stopped by epoch
// interruption (a ticker goroutine increments the engine's epoch; each call
// sets its store's deadline in ticks), on every module, JS or compiled; the
// memory cap is the store limiter; there is no watchdog, abandon path or
// stuck count, because a call cannot outlive its epoch deadline.
//
// Build with -tags wasmtime (and CGO_ENABLED=1). Without the tag the package
// is a stub (stub.go) and pulls no cgo in.
package wtengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	wasmtime "github.com/bytecodealliance/wasmtime-go/v50"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/engine"
)

// Available reports whether the binary was built with wasmtime.
const Available = true

// Config configures an Engine.
type Config struct {
	// Epochs compiles epoch checks into every module and runs the ticker.
	// Off is only for measuring what the checks cost.
	Epochs bool
	// Tick is the epoch period (default 1 ms): the resolution of a deadline.
	Tick time.Duration
	// ArmJS also arms the JS engine's own interrupt (fc_js_deadline) on
	// cooperative modules, as production does on wazero. The epoch deadline
	// stays the backstop either way.
	ArmJS bool
	// Grace is added to the context deadline for the epoch deadline, so that
	// when ArmJS is on the guest's own clean abort normally comes first.
	Grace time.Duration
	// Stderr receives the guests' WASI stderr (inherit) when true.
	InheritStderr bool
	// StateMap makes the fc host functions find their call state through a
	// sync.Map keyed by the store's context pointer instead of Caller.Data,
	// which takes wasmtime-go's process-wide store lock (see the spike document,
	// question 7). wasmtime-go still takes that lock once per host call in its
	// own trampoline.
	StateMap bool
	Logger   *slog.Logger
}

// Engine owns one wasmtime engine and the linker every instance is made with.
type Engine struct {
	cfg    Config
	eng    *wasmtime.Engine
	linker *wasmtime.Linker
	log    *slog.Logger
	stop   chan struct{}
	ticker sync.WaitGroup
	ticks  atomic.Uint64 // epochs issued; the engine's epoch counter
	start  time.Time     // epoch n is due at start + n*Tick
	states sync.Map      // store context pointer -> *state, when cfg.StateMap
}

const page = 1 << 16

// New builds an engine: config, the WASI preview1 imports, the fc host module
// on a linker shared by every store, and the epoch ticker.
func New(cfg Config) (*Engine, error) {
	if cfg.Tick <= 0 {
		cfg.Tick = time.Millisecond
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	c := wasmtime.NewConfig()
	c.SetEpochInterruption(cfg.Epochs)
	eng := wasmtime.NewEngineWithConfig(c)
	l := wasmtime.NewLinker(eng)
	if err := l.DefineWasi(); err != nil {
		return nil, fmt.Errorf("wtengine: wasi: %w", err)
	}
	e := &Engine{cfg: cfg, eng: eng, linker: l, log: cfg.Logger, stop: make(chan struct{}), start: time.Now()}
	i32, i64 := wasmtime.NewValType(wasmtime.KindI32), wasmtime.NewValType(wasmtime.KindI64)
	if err := l.FuncNew(abi.ImportModule, abi.ImportCall,
		wasmtime.NewFuncType([]*wasmtime.ValType{i32, i32, i32}, []*wasmtime.ValType{i64}), e.hostCall); err != nil {
		return nil, err
	}
	if err := l.FuncNew(abi.ImportModule, abi.ImportTake,
		wasmtime.NewFuncType([]*wasmtime.ValType{i32}, nil), e.hostTake); err != nil {
		return nil, err
	}
	if cfg.Epochs {
		// The epoch follows the monotonic clock, not the number of wake-ups: a
		// Go ticker can fire late or drop ticks (measured: a 1 ms ticker fired
		// ~340-380 times a second in a Linux VM), so each wake-up increments the
		// epoch as many times as whole ticks have passed since the start. A
		// deadline is then late by at most one wake-up's lateness, never by a
		// factor.
		e.ticker.Go(func() {
			t := time.NewTicker(cfg.Tick)
			defer t.Stop()
			start := e.start
			for {
				select {
				case <-e.stop:
					return
				case <-t.C:
					for target := uint64(time.Since(start) / cfg.Tick); e.ticks.Load() < target; {
						eng.IncrementEpoch()
						e.ticks.Add(1)
					}
				}
			}
		})
	}
	return e, nil
}

// Ticks is how many times the epoch has been incremented.
func (e *Engine) Ticks() uint64 { return e.ticks.Load() }

// Close stops the ticker and frees the engine.
func (e *Engine) Close() {
	close(e.stop)
	e.ticker.Wait()
	e.linker.Close()
	e.eng.Close()
}

// Module is a compiled, checked module.
type Module struct {
	e        *Engine
	m        *wasmtime.Module
	coop     bool
	hasInit  bool
	minBytes uint64
}

// Compile compiles and checks wasm. coop marks the platform's JS engine
// (it must export fc_js_deadline); on wasmtime it runs under the same epoch
// deadline as everything else.
func (e *Engine) Compile(wasm []byte, coop bool) (*Module, error) {
	m, err := wasmtime.NewModule(e.eng, wasm)
	if err != nil {
		return nil, &engine.LoadError{Code: engine.LoadInvalidModule, Detail: err.Error()}
	}
	mod := &Module{e: e, m: m, coop: coop}
	if err := mod.check(); err != nil {
		m.Close()
		return nil, err
	}
	return mod, nil
}

// Deserialize loads code Serialize produced (a disk cache); the engine's
// configuration and wasmtime version must be the ones that produced it.
func (e *Engine) Deserialize(b []byte, coop bool) (*Module, error) {
	m, err := wasmtime.NewModuleDeserialize(e.eng, b)
	if err != nil {
		return nil, &engine.LoadError{Code: engine.LoadInvalidModule, Detail: err.Error()}
	}
	mod := &Module{e: e, m: m, coop: coop}
	if err := mod.check(); err != nil {
		m.Close()
		return nil, err
	}
	return mod, nil
}

// Serialize is the compiled code, for a disk cache (NewModuleDeserialize).
func (m *Module) Serialize() ([]byte, error) { return m.m.Serialize() }

// Close frees the compiled module.
func (m *Module) Close() { m.m.Close() }

type sig struct{ params, results []wasmtime.ValKind }

var (
	kI32, kI64      = wasmtime.KindI32, wasmtime.KindI64
	requiredExports = map[string]sig{
		abi.ExportMarker:   {nil, nil},
		abi.ExportAlloc:    {[]wasmtime.ValKind{kI32}, []wasmtime.ValKind{kI32}},
		abi.ExportHandle:   {[]wasmtime.ValKind{kI32, kI32}, []wasmtime.ValKind{kI64}},
		abi.ExportDescribe: {nil, []wasmtime.ValKind{kI64}},
	}
	fcImports = map[string]sig{
		abi.ImportCall: {[]wasmtime.ValKind{kI32, kI32, kI32}, []wasmtime.ValKind{kI64}},
		abi.ImportTake: {[]wasmtime.ValKind{kI32}, nil},
	}
)

func kinds(ts []*wasmtime.ValType) []wasmtime.ValKind {
	out := make([]wasmtime.ValKind, len(ts))
	for i, t := range ts {
		out[i] = t.Kind()
	}
	return out
}

func sameSig(ft *wasmtime.FuncType, want sig) bool {
	p, r := kinds(ft.Params()), kinds(ft.Results())
	eq := func(a, b []wasmtime.ValKind) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	return eq(p, want.params) && eq(r, want.results)
}

// check is the production engine's ABI v1 module check (engine/check.go),
// on wasmtime's types.
func (m *Module) check() error {
	funcs := map[string]*wasmtime.FuncType{}
	var mem *wasmtime.MemoryType
	for _, ex := range m.m.Exports() {
		t := ex.Type()
		if ft := t.FuncType(); ft != nil {
			funcs[ex.Name()] = ft
		}
		if mt := t.MemoryType(); mt != nil && ex.Name() == abi.ExportMemory {
			mem = mt
		}
	}
	if _, ok := funcs[abi.ExportMarker]; !ok {
		return &engine.LoadError{Code: engine.LoadABIUnknown, Detail: "the module does not export " + abi.ExportMarker}
	}
	for name, want := range requiredExports {
		ft, ok := funcs[name]
		if !ok {
			return &engine.LoadError{Code: engine.LoadExportMissing, Detail: "the module does not export " + name}
		}
		if !sameSig(ft, want) {
			return &engine.LoadError{Code: engine.LoadExportSignature, Detail: name + " has the wrong signature"}
		}
	}
	if m.coop {
		ft, ok := funcs[engine.ExportJSDeadline]
		if !ok || !sameSig(ft, sig{[]wasmtime.ValKind{kI64}, nil}) {
			return &engine.LoadError{Code: engine.LoadExportMissing, Detail: "a cooperative module must export " + engine.ExportJSDeadline}
		}
	}
	if ft, ok := funcs[abi.ExportInitialize]; ok {
		if !sameSig(ft, sig{}) {
			return &engine.LoadError{Code: engine.LoadExportSignature, Detail: abi.ExportInitialize + " must take and return nothing"}
		}
		m.hasInit = true
	}
	for _, im := range m.m.Imports() {
		name := ""
		if n := im.Name(); n != nil {
			name = *n
		}
		switch im.Module() {
		case abi.WASIModule:
		case abi.ImportModule:
			want, ok := fcImports[name]
			ft := im.Type().FuncType()
			if !ok || ft == nil || !sameSig(ft, want) {
				return &engine.LoadError{Code: engine.LoadImportNotAllowed, Detail: fmt.Sprintf("%s.%s is not an fc import of ABI v1", im.Module(), name)}
			}
		default:
			return &engine.LoadError{Code: engine.LoadImportNotAllowed, Detail: fmt.Sprintf("imports %s.%s", im.Module(), name)}
		}
		if im.Type().MemoryType() != nil {
			return &engine.LoadError{Code: engine.LoadImportNotAllowed, Detail: "the module imports a memory"}
		}
	}
	if mem == nil {
		return &engine.LoadError{Code: engine.LoadExportMissing, Detail: "the module does not export its memory as " + abi.ExportMemory}
	}
	m.minBytes = mem.Minimum() * page
	return nil
}

// InstanceConfig configures one instance.
type InstanceConfig struct {
	MemoryCapBytes uint64
	Preload        *engine.Preload
}

// state is the store's data: what the fc host functions reach through the
// Caller (wasmtime-go's Caller.Data), the counterpart of the production
// engine's context value.
type state struct {
	ctx     context.Context
	host    engine.Host
	pending []byte
	mem     *wasmtime.Memory
}

// Instance is one store with one instance in it. Not safe for concurrent use;
// it may be used from different goroutines one call at a time.
type Instance struct {
	e                                 *Engine
	store                             *wasmtime.Store
	st                                *state
	mem                               *wasmtime.Memory
	alloc, handle, describe, deadline *wasmtime.Func
	capBytes                          uint64
	broken                            bool
	closed                            bool
	calls                             int
	key                               uintptr
}

// noDeadline is the epoch deadline outside a call (instantiate, close): far
// enough that it never fires.
const noDeadline = 1 << 62

// Instantiate creates a store capped at cfg.MemoryCapBytes and instantiates
// the module in it, then runs _initialize and the preload.
func (m *Module) Instantiate(ctx context.Context, cfg InstanceConfig) (*Instance, error) {
	if m.minBytes > cfg.MemoryCapBytes {
		return nil, &engine.LoadError{Code: engine.LoadMemoryOverCap, Detail: "minimum memory over the cap"}
	}
	st := &state{}
	store := wasmtime.NewStoreWithData(m.e.eng, st)
	store.Limiter(int64(cfg.MemoryCapBytes), -1, 1, -1, 1)
	wc := wasmtime.NewWasiConfig()
	if m.e.cfg.InheritStderr {
		wc.InheritStderr()
	}
	store.SetWasi(wc)
	if m.e.cfg.Epochs {
		store.SetEpochDeadline(noDeadline)
	}
	key := uintptr(unsafe.Pointer(store.Context()))
	if m.e.cfg.StateMap {
		m.e.states.Store(key, st)
	}
	inst, err := m.e.linker.Instantiate(store, m.m)
	if err != nil {
		m.e.states.Delete(key)
		store.Close()
		return nil, &engine.LoadError{Code: engine.LoadInstantiate, Detail: err.Error()}
	}
	i := &Instance{e: m.e, store: store, st: st, capBytes: cfg.MemoryCapBytes, key: key}
	i.mem = inst.GetExport(store, abi.ExportMemory).Memory()
	st.mem = i.mem
	i.alloc = inst.GetFunc(store, abi.ExportAlloc)
	i.handle = inst.GetFunc(store, abi.ExportHandle)
	i.describe = inst.GetFunc(store, abi.ExportDescribe)
	if m.coop {
		i.deadline = inst.GetFunc(store, engine.ExportJSDeadline)
	}
	if m.hasInit {
		if _, err := i.run(ctx, inst.GetFunc(store, abi.ExportInitialize)); err != nil {
			err = i.classify(ctx, err)
			i.Close()
			return nil, fmt.Errorf("wtengine: _initialize: %w", err)
		}
	}
	if p := cfg.Preload; p != nil {
		if err := i.preload(ctx, p, inst); err != nil {
			i.Close()
			return nil, err
		}
	}
	return i, nil
}

func (i *Instance) preload(ctx context.Context, p *engine.Preload, inst *wasmtime.Instance) error {
	fn := inst.GetFunc(i.store, p.Export)
	if fn == nil {
		return &engine.LoadError{Code: engine.LoadExportMissing, Detail: "the module does not export " + p.Export}
	}
	i.st.ctx, i.st.host = ctx, nil
	r, err := i.alloc.Call(i.store, int32(len(p.Data)))
	if err != nil {
		return &engine.LoadError{Code: engine.LoadInstantiate, Detail: "preload: " + i.classify(ctx, err).Error()}
	}
	ptr := uint32(r.(int32))
	if !i.write(ptr, p.Data) {
		return &engine.LoadError{Code: engine.LoadInstantiate, Detail: "preload: fc_alloc returned a buffer outside memory"}
	}
	rc, err := i.run(ctx, fn, int32(ptr), int32(len(p.Data)))
	if err != nil {
		return &engine.LoadError{Code: engine.LoadInstantiate, Detail: "preload: " + i.classify(ctx, err).Error()}
	}
	if rc.(int32) != 0 {
		return &engine.LoadError{Code: engine.LoadInstantiate, Detail: p.Export + " refused the data"}
	}
	i.broken = false
	return nil
}

func (i *Instance) write(ptr uint32, b []byte) bool {
	d := i.mem.UnsafeData(i.store)
	if uint64(ptr)+uint64(len(b)) > uint64(len(d)) {
		return false
	}
	copy(d[ptr:], b)
	return true
}

// Handle runs one request frame through fc_handle.
func (i *Instance) Handle(ctx context.Context, request []byte, host engine.Host) ([]byte, error) {
	return i.invoke(ctx, i.handle, host, request, true)
}

// Describe runs fc_describe with no capabilities.
func (i *Instance) Describe(ctx context.Context) ([]byte, error) {
	return i.invoke(ctx, i.describe, nil, nil, false)
}

func (i *Instance) invoke(ctx context.Context, fn *wasmtime.Func, host engine.Host, input []byte, withInput bool) ([]byte, error) {
	if i.broken {
		return nil, errors.New("wtengine: instance is broken")
	}
	i.st.ctx, i.st.host, i.st.pending = ctx, host, nil
	defer func() { i.st.ctx, i.st.host = nil, nil }()
	var args []any
	if withInput {
		r, err := i.alloc.Call(i.store, int32(len(input)))
		if err != nil {
			return nil, i.classify(ctx, err)
		}
		ptr := uint32(r.(int32))
		if !i.write(ptr, input) {
			i.broken = true
			return nil, fmt.Errorf("%w: fc_alloc returned %d, outside memory", engine.ErrMalformed, ptr)
		}
		args = []any{int32(ptr), int32(len(input))}
	}
	r, err := i.run(ctx, fn, args...)
	if err != nil {
		return nil, i.classify(ctx, err)
	}
	ptr, n := abi.Unpack(uint64(r.(int64)))
	d := i.mem.UnsafeData(i.store)
	if uint64(ptr)+uint64(n) > uint64(len(d)) {
		i.broken = true
		return nil, fmt.Errorf("%w: [%d, +%d) is outside memory", engine.ErrMalformed, ptr, n)
	}
	i.calls++
	return append([]byte(nil), d[ptr:ptr+n]...), nil
}

// errDeadlineStop is the guest stopping itself at its deadline (fc_js_deadline).
var errDeadlineStop = fmt.Errorf("%w: the guest stopped at its deadline", engine.ErrDeadline)

// run calls fn under the call's deadline: the epoch deadline always (when
// epochs are on), and the JS engine's own interrupt too when ArmJS is set.
// The call runs on the calling goroutine: it is a cgo call, so it holds this
// goroutine's OS thread until it returns, and the epoch guarantees it returns.
func (i *Instance) run(ctx context.Context, fn *wasmtime.Func, args ...any) (any, error) {
	limit := 30 * time.Second // as the production engine's cooperativeFallback
	dl, hasDeadline := ctx.Deadline()
	if hasDeadline {
		limit = time.Until(dl)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("%w: the deadline had already passed", engine.ErrDeadline)
	}
	if i.deadline != nil && i.e.cfg.ArmJS {
		ms := int64((limit + time.Millisecond - 1) / time.Millisecond)
		if _, err := i.deadline.Call(i.store, ms); err != nil {
			return nil, err
		}
	}
	if i.e.cfg.Epochs {
		// The deadline is the epoch due just after it on the wall clock (epoch n
		// is due at start + n*Tick), given relative to the epochs issued so far.
		// The ticker never issues an epoch before it is due, so the call never
		// stops before its deadline; when the ticker lags (Linux VM: wake-ups
		// 2-3 ms apart) the epochs it owes are issued on its next wake-up, so the
		// call stops at most one wake-up late. A deadline relative to the current
		// count alone fired up to 2 ms early after a lagging ticker caught up.
		due := uint64((time.Until(dl)+time.Since(i.e.start)+i.e.cfg.Grace)/i.e.cfg.Tick) + 1
		if !hasDeadline {
			due = uint64((limit+time.Since(i.e.start))/i.e.cfg.Tick) + 1
		}
		issued := i.e.ticks.Load()
		t := uint64(1)
		if due > issued {
			t = due - issued
		}
		i.store.SetEpochDeadline(t)
		defer i.store.SetEpochDeadline(noDeadline)
	}
	r, err := fn.Call(i.store, args...)
	if err == nil && hasDeadline && !time.Now().Before(dl) {
		return nil, errDeadlineStop
	}
	return r, err
}

// Trap codes and exit status, for classify.
func trapCode(err error) (wasmtime.TrapCode, bool) {
	var t *wasmtime.Trap
	if errors.As(err, &t) {
		if c := t.Code(); c != nil {
			return *c, true
		}
	}
	return 0, false
}

func exitStatus(err error) (int32, bool) {
	var we *wasmtime.Error
	if errors.As(err, &we) {
		return we.ExitStatus()
	}
	return 0, false
}

// classify maps a failed call to the production engine's outcome errors and
// marks the instance broken.
//
// Out of memory: wasmtime's store limiter refuses a grow past the cap
// (memory.grow answers -1) but does not say it did, and wasmtime-go exposes no
// limiter callback, so a failure is called out-of-memory when the memory is
// within one grow of the cap. That is a heuristic; see the spike document.
func (i *Instance) classify(ctx context.Context, err error) error {
	i.broken = true
	if errors.Is(err, engine.ErrDeadline) {
		return err
	}
	if c, ok := trapCode(err); ok && c == wasmtime.Interrupt {
		return fmt.Errorf("%w: epoch deadline (%w)", engine.ErrDeadline, err)
	}
	if ctx.Err() != nil {
		return fmt.Errorf("%w: %w", engine.ErrDeadline, context.Cause(ctx))
	}
	nearCap := uint64(i.mem.DataSize(i.store))+16*page > i.capBytes
	if code, ok := exitStatus(err); ok {
		if nearCap {
			return fmt.Errorf("%w: exit code %d at %d bytes", engine.ErrOutOfMemory, code, i.mem.DataSize(i.store))
		}
		return fmt.Errorf("%w: exit code %d", engine.ErrExit, code)
	}
	if nearCap {
		return fmt.Errorf("%w: %w", engine.ErrOutOfMemory, err)
	}
	if c, ok := trapCode(err); ok {
		return fmt.Errorf("%w: trap code %d: %w", engine.ErrTrap, c, err)
	}
	return fmt.Errorf("%w: %w", engine.ErrTrap, err)
}

// Broken reports whether the instance failed and must not be reused.
func (i *Instance) Broken() bool { return i.broken }

// MemoryBytes is the instance's linear memory size now.
func (i *Instance) MemoryBytes() int64 { return int64(i.mem.DataSize(i.store)) }

// Close frees the store (and with it the instance and its memory).
func (i *Instance) Close() {
	if i.closed {
		return
	}
	i.closed, i.broken = true, true
	i.e.states.Delete(i.key)
	i.store.Close()
}

// Result codes in the high half of fc.call's return (engine/host.go).
const (
	callOK    = 0
	callError = 1
)

// stateOf finds the call state of the store the host function was called from.
func (e *Engine) stateOf(c *wasmtime.Caller) *state {
	if e.cfg.StateMap {
		v, _ := e.states.Load(uintptr(unsafe.Pointer(c.Context())))
		st, _ := v.(*state)
		return st
	}
	st, _ := c.Data().(*state)
	return st
}

func trapf(format string, a ...any) *wasmtime.Trap {
	return wasmtime.NewTrap(fmt.Sprintf(format, a...))
}

// hostCall implements fc.call(op, ptr, len) → code<<32 | resultLen, as
// engine/host.go does on wazero.
func (e *Engine) hostCall(c *wasmtime.Caller, args []wasmtime.Val) ([]wasmtime.Val, *wasmtime.Trap) {
	st := e.stateOf(c)
	if st == nil || st.ctx == nil {
		return nil, trapf("fc.call outside an invocation")
	}
	op, ptr, n := abi.Op(uint32(args[0].I32())), uint32(args[1].I32()), uint32(args[2].I32())
	st.pending = nil
	d := st.mem.UnsafeData(c)
	if uint64(ptr)+uint64(n) > uint64(len(d)) {
		return ret(e.fail(st, abi.Errorf(abi.CodeBadRequest, fmt.Sprintf("frame [%d, +%d) is outside memory", ptr, n)))), nil
	}
	meta, body, err := abi.DecodeFrame(d[ptr : ptr+n])
	if err != nil {
		return ret(e.fail(st, abi.Errorf(abi.CodeBadRequest, err.Error()))), nil
	}
	if st.host == nil {
		return ret(e.fail(st, abi.Errorf(abi.CodeCapabilityUnavailable, op.String()+" is not available here"))), nil
	}
	meta, body = append([]byte(nil), meta...), append([]byte(nil), body...)
	rMeta, rBody, cerr := e.serve(st.ctx, st.host, op, meta, body)
	if cerr != nil {
		return ret(e.fail(st, cerr)), nil
	}
	if len(rMeta) == 0 {
		rMeta = []byte("{}")
	}
	st.pending = abi.EncodeFrame(rMeta, rBody)
	return ret(abi.Pack(callOK, uint32(len(st.pending)))), nil
}

func ret(v uint64) []wasmtime.Val { return []wasmtime.Val{wasmtime.ValI64(int64(v))} }

func (e *Engine) serve(ctx context.Context, h engine.Host, op abi.Op, meta, body []byte) (rMeta, rBody []byte, err *abi.Error) {
	defer func() {
		if p := recover(); p != nil {
			e.log.Error("function host call panicked", "op", op.String(), "panic", p, "stack", string(debug.Stack()))
			rMeta, rBody, err = nil, nil, abi.Errorf(abi.CodeUnavailable, "the host failed serving "+op.String())
		}
	}()
	return h.Call(ctx, op, meta, body)
}

func (e *Engine) fail(st *state, cerr *abi.Error) uint64 {
	meta, _ := json.Marshal(cerr)
	st.pending = abi.EncodeFrame(meta, nil)
	return abi.Pack(callError, uint32(len(st.pending)))
}

// hostTake implements fc.take(dst).
func (e *Engine) hostTake(c *wasmtime.Caller, args []wasmtime.Val) ([]wasmtime.Val, *wasmtime.Trap) {
	st := e.stateOf(c)
	if st == nil || st.pending == nil {
		return nil, trapf("fc.take with no pending result")
	}
	dst := uint32(args[0].I32())
	d := st.mem.UnsafeData(c)
	if uint64(dst)+uint64(len(st.pending)) > uint64(len(d)) {
		return nil, trapf("fc.take destination [%d, +%d) is outside memory", dst, len(st.pending))
	}
	copy(d[dst:], st.pending)
	st.pending = nil
	return nil, nil
}
