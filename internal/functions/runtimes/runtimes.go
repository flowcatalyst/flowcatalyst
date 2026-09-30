// Package runtimes turns a function artifact into something the engine can
// instantiate, for each runtime a version may declare:
//
//   - "wasm": the artifact is an ABI v1 module, compiled as is.
//   - "js":   the artifact is a bundled script; instances come from the shared
//     JS engine (compiled once per Loader) with the script preloaded.
//
// The runner prepares versions with it and the platform reads a published
// artifact's describe document with it, so both see an artifact identically.
package runtimes

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/engine"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/jsengine"
)

// Runtimes a version may declare.
const (
	Wasm = "wasm"
	JS   = "js"
)

// Valid reports whether r names a runtime ("" means Wasm).
func Valid(r string) bool { return r == "" || r == Wasm || r == JS }

// Prepared is an artifact ready to instantiate.
type Prepared struct {
	Module *engine.Module
	// Preload goes into every instance (the JS script); nil for Wasm.
	Preload *engine.Preload
	// Shared marks a module owned by the Loader (the JS engine): callers
	// must not close it.
	Shared bool
}

// Close releases the module unless the Loader owns it.
func (p *Prepared) Close(ctx context.Context) {
	if p != nil && p.Module != nil && !p.Shared {
		_ = p.Module.Close(ctx)
	}
}

// InstanceConfig returns cfg with the preload this artifact needs.
func (p *Prepared) InstanceConfig(cfg engine.InstanceConfig) engine.InstanceConfig {
	cfg.Preload = p.Preload
	return cfg
}

// Loader prepares artifacts on one engine.
type Loader struct {
	eng *engine.Engine
	// jsLock serialises the shared JS engine's compile (a one-slot channel so a
	// waiter can give up with its context). Only a success is cached: a failed
	// or cancelled compile is retried by the next caller.
	jsLock    chan struct{}
	jsMod     *engine.Module // guarded by jsLock
	compileJS func(ctx context.Context) (*engine.Module, error)
}

// NewLoader builds a loader for eng.
func NewLoader(eng *engine.Engine) *Loader {
	return &Loader{
		eng:    eng,
		jsLock: make(chan struct{}, 1),
		compileJS: func(ctx context.Context) (*engine.Module, error) {
			return eng.Compile(ctx, jsengine.Wasm)
		},
	}
}

// maxScriptBytes caps a JS artifact.
const maxScriptBytes = 16 << 20

// Prepare compiles (or, for JS, validates) an artifact of the given runtime.
func (l *Loader) Prepare(ctx context.Context, runtime string, artifact []byte) (*Prepared, error) {
	switch runtime {
	case "", Wasm:
		m, err := l.eng.Compile(ctx, artifact)
		if err != nil {
			return nil, err
		}
		return &Prepared{Module: m}, nil
	case JS:
		if len(artifact) > maxScriptBytes {
			return nil, &engine.LoadError{Code: engine.LoadInvalidModule, Detail: "the script is over 16 MiB"}
		}
		if !utf8.Valid(artifact) {
			return nil, &engine.LoadError{Code: engine.LoadInvalidModule, Detail: "the script is not UTF-8"}
		}
		m, err := l.jsEngine(ctx)
		if err != nil {
			return nil, err
		}
		return &Prepared{Module: m, Shared: true, Preload: &engine.Preload{Export: jsengine.LoadExport, Data: artifact}}, nil
	}
	return nil, &engine.LoadError{Code: engine.LoadInvalidModule, Detail: fmt.Sprintf("unknown runtime %q", runtime)}
}

func (l *Loader) jsEngine(ctx context.Context) (*engine.Module, error) {
	select {
	case l.jsLock <- struct{}{}:
		defer func() { <-l.jsLock }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if l.jsMod != nil {
		return l.jsMod, nil
	}
	m, err := l.compileJS(ctx)
	if err != nil {
		return nil, fmt.Errorf("the shared JS engine failed to compile: %w", err)
	}
	l.jsMod = m
	return m, nil
}

// Describe reads an artifact's describe document the way publish must: one
// instance with no capabilities, capped at capBytes of memory.
func (l *Loader) Describe(ctx context.Context, runtime string, artifact []byte, capBytes uint64) ([]byte, error) {
	p, err := l.Prepare(ctx, runtime, artifact)
	if err != nil {
		return nil, err
	}
	defer p.Close(context.Background())
	inst, err := p.Module.Instantiate(ctx, p.InstanceConfig(engine.InstanceConfig{MemoryCapBytes: capBytes}))
	if err != nil {
		return nil, err
	}
	defer func() { _ = inst.Close(context.Background()) }()
	doc, err := inst.Describe(ctx)
	if err != nil {
		return nil, err
	}
	if len(doc) == 0 {
		return nil, errors.New("the artifact returned an empty describe document")
	}
	return doc, nil
}
