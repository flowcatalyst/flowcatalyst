//go:build wasmtime

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"

	"github.com/flowcatalyst/flowcatalyst-go/bench/function-runtimes/wasmtime-go-spike/wtengine"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/budget"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/engine"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/jsengine"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/runtimes"
)

// The spike's own three-method view of an engine, so every measurement runs
// the same code against wazero (the production engine, untouched) and against
// wasmtime (wtengine). It is also a first draft of what phase 2's Engine
// interface has to cover.
type inst interface {
	Handle(ctx context.Context, frame []byte, h engine.Host) ([]byte, error)
	Broken() bool
	Close()
}

type fn interface {
	New(ctx context.Context) (inst, error)
}

type rt interface {
	Name() string
	Load(ctx context.Context, kind string, artifact []byte) (fn, error)
	Close()
}

// ---- wazero: internal/functions/engine through runtimes.Loader, as the runner does.

type wzRT struct {
	e *engine.Engine
	l *runtimes.Loader
}

func newWZ(ctx context.Context) rt {
	b, _ := budget.New(16<<30, 0)
	e, err := engine.New(ctx, engine.Config{Budget: b, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		panic(err)
	}
	return &wzRT{e: e, l: runtimes.NewLoader(e)}
}

func (w *wzRT) Name() string { return "wazero" }
func (w *wzRT) Close()       { _ = w.e.Close(context.Background()) }

type wzFn struct{ p *runtimes.Prepared }
type wzInst struct{ *engine.Instance }

func (i wzInst) Close() { _ = i.Instance.Close(context.Background()) }

func (w *wzRT) Load(ctx context.Context, kind string, artifact []byte) (fn, error) {
	r := runtimes.Wasm
	if kind == "js" {
		r = runtimes.JS
	}
	p, err := w.l.Prepare(ctx, r, artifact)
	if err != nil {
		return nil, err
	}
	return wzFn{p}, nil
}

func (f wzFn) New(ctx context.Context) (inst, error) {
	i, err := f.p.Module.Instantiate(ctx, f.p.InstanceConfig(engine.InstanceConfig{MemoryCapBytes: *capBytes}))
	if err != nil {
		return nil, err
	}
	return wzInst{i}, nil
}

// ---- wasmtime: wtengine.

type wtRT struct {
	name string
	e    *wtengine.Engine
	js   *wtengine.Module
}

func newWT(cfg wtengine.Config, name string) rt {
	e, err := wtengine.New(cfg)
	if err != nil {
		panic(err)
	}
	return &wtRT{name: name, e: e}
}

func (w *wtRT) Name() string { return w.name }
func (w *wtRT) Close()       { w.e.Close() }

type wtFn struct {
	m       *wtengine.Module
	preload *engine.Preload
}

func (w *wtRT) Load(_ context.Context, kind string, artifact []byte) (fn, error) {
	if kind == "js" {
		if w.js == nil {
			m, err := w.e.Compile(jsengine.Wasm, true)
			if err != nil {
				return nil, err
			}
			w.js = m
		}
		return wtFn{m: w.js, preload: &engine.Preload{Export: jsengine.LoadExport, Data: artifact}}, nil
	}
	m, err := w.e.Compile(artifact, false)
	if err != nil {
		return nil, err
	}
	return wtFn{m: m}, nil
}

func (f wtFn) New(ctx context.Context) (inst, error) {
	return f.m.Instantiate(ctx, wtengine.InstanceConfig{MemoryCapBytes: *capBytes, Preload: f.preload})
}

// makeRT builds an engine by its label:
//
//	wazero    the production engine (JS on the cooperative runtime, Wasm on the preemptible one)
//	wasmtime  epochs on, epoch deadline only (fc_js_deadline not armed)
//	wt-armed  epochs on, and the JS engine's own interrupt armed as in production; epoch at +grace
//	wt-noep   epochs off (only to price the epoch checks; cannot stop a runaway)
func makeRT(ctx context.Context, label string) rt {
	switch label {
	case "wazero":
		return newWZ(ctx)
	case "wasmtime":
		return newWT(wtengine.Config{Epochs: true, Tick: *tick}, label)
	case "wt-armed":
		return newWT(wtengine.Config{Epochs: true, Tick: *tick, ArmJS: true, Grace: *grace}, label)
	case "wt-map":
		return newWT(wtengine.Config{Epochs: true, Tick: *tick, StateMap: true}, label)
	case "wt-noep":
		return newWT(wtengine.Config{Epochs: false, ArmJS: true}, label)
	}
	panic("unknown engine " + label)
}

func engines() []string { return strings.Split(*engList, ",") }

// ---- the host: the same object behind both engines.

// benchHost serves log and config.get the way the runner's capabilities do
// (internal/functions/runner/capabilities.go logCall and lookup: decode the
// meta, check the key is declared, answer abi.Found and the value). It is a
// copy of that logic because capabilities is unexported in package runner; the
// fc.call / fc.take plumbing in front of it is the engine's own on wazero and
// wtengine's port on wasmtime. Every call is counted.
type benchHost struct {
	log      *slog.Logger
	declared []string
	values   map[string]string
	calls    atomic.Int64
}

func newHost() *benchHost {
	return &benchHost{
		// A real JSON handler (the work of formatting a record), into io.Discard.
		log:      slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})),
		declared: []string{"BENCH"},
		values:   map[string]string{"BENCH": "hello-config-value"},
	}
}

func (h *benchHost) Call(ctx context.Context, op abi.Op, meta, body []byte) ([]byte, []byte, *abi.Error) {
	h.calls.Add(1)
	switch op {
	case abi.OpLog:
		var l abi.Log
		if err := jsonUnmarshal(meta, &l); err != nil {
			return nil, nil, abi.Errorf(abi.CodeBadRequest, "meta is not the expected JSON: "+err.Error())
		}
		level := slog.LevelInfo
		switch strings.ToLower(l.Level) {
		case "debug":
			level = slog.LevelDebug
		case "warn", "warning":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		}
		attrs := []any{"fn.invocation", "inv_1"}
		for k, v := range l.Attrs {
			attrs = append(attrs, "guest."+k, v)
		}
		h.log.Log(ctx, level, l.Msg, attrs...)
		return nil, nil, nil
	case abi.OpConfigGet, abi.OpSecretGet:
		var k abi.Key
		if err := jsonUnmarshal(meta, &k); err != nil {
			return nil, nil, abi.Errorf(abi.CodeBadRequest, "meta is not the expected JSON: "+err.Error())
		}
		declared := false
		for _, d := range h.declared {
			declared = declared || d == k.Key
		}
		if !declared {
			return nil, nil, abi.Errorf(abi.CodeNotDeclared, fmt.Sprintf("config %q is not declared by this version", k.Key))
		}
		v, ok := h.values[k.Key]
		b, _ := jsonMarshal(abi.Found{Found: ok})
		return b, []byte(v), nil
	}
	return nil, nil, abi.Errorf(abi.CodeUnknownOp, fmt.Sprintf("op %d is not part of ABI v1", op))
}
