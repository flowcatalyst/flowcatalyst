//go:build !wasmtime

package main

import (
	"context"
	"fmt"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/budget"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/engine"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/jsengine"
)

func probe(ctx context.Context) error {
	b, _ := budget.New(1<<30, 0)
	e, err := engine.New(ctx, engine.Config{Budget: b})
	if err != nil {
		return err
	}
	m, err := e.CompileCooperative(ctx, jsengine.Wasm)
	if err != nil {
		return err
	}
	i, err := m.Instantiate(ctx, engine.InstanceConfig{MemoryCapBytes: 64 << 20})
	if err != nil {
		return err
	}
	fmt.Println("wazero ok", i.MemoryBytes())
	return nil
}
