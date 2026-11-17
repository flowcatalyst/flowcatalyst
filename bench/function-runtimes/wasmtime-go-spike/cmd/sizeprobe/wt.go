//go:build wasmtime

package main

import (
	"context"
	"fmt"

	"github.com/flowcatalyst/flowcatalyst-go/bench/function-runtimes/wasmtime-go-spike/wtengine"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/jsengine"
)

func probe(ctx context.Context) error {
	e, err := wtengine.New(wtengine.Config{Epochs: true})
	if err != nil {
		return err
	}
	m, err := e.Compile(jsengine.Wasm, true)
	if err != nil {
		return err
	}
	i, err := m.Instantiate(ctx, wtengine.InstanceConfig{MemoryCapBytes: 64 << 20})
	if err != nil {
		return err
	}
	fmt.Println("wasmtime ok", i.MemoryBytes())
	return nil
}
