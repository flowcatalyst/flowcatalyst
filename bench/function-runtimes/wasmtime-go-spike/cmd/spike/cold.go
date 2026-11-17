//go:build wasmtime

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/bench/function-runtimes/wasmtime-go-spike/wtengine"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/budget"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/engine"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/jsengine"
)

// cold prices what a runner pays before a call: compiling the shared JS engine
// and the Go guest (wazero: no cache and with its disk cache; wasmtime: compile,
// and the serialized form a disk cache would hold), and one instance with the
// JS bundle preloaded, then its first call.
func cold(ctx context.Context) {
	const reps = 5
	bundle := mustRead(guestByLabel("qjs").path)
	gowasm := mustRead(guestByLabel("go").path)
	w1 := inputs()[1].frame
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	mediaMS := func(ds []time.Duration) string {
		f := make([]float64, len(ds))
		for i, d := range ds {
			f[i] = float64(d) / 1e6
		}
		return fmt.Sprintf("%.2fms", median(f))
	}

	// wazero
	for _, cached := range []bool{false, true} {
		dir := ""
		if cached {
			dir, _ = os.MkdirTemp("", "wzcache")
			defer os.RemoveAll(dir)
			b, _ := budget.New(4<<30, 0)
			e, _ := engine.New(ctx, engine.Config{Budget: b, CacheDir: dir, Logger: quiet})
			_, _ = e.CompileCooperative(ctx, jsengine.Wasm)
			_, _ = e.Compile(ctx, gowasm)
			_ = e.Close(ctx)
		}
		var cJS, cGo, inst, first []time.Duration
		for range reps {
			b, _ := budget.New(4<<30, 0)
			e, _ := engine.New(ctx, engine.Config{Budget: b, CacheDir: dir, Logger: quiet})
			t := time.Now()
			m, err := e.CompileCooperative(ctx, jsengine.Wasm)
			cJS = append(cJS, time.Since(t))
			if err != nil {
				panic(err)
			}
			t = time.Now()
			if _, err := e.Compile(ctx, gowasm); err != nil {
				panic(err)
			}
			cGo = append(cGo, time.Since(t))
			t = time.Now()
			i, err := m.Instantiate(ctx, engine.InstanceConfig{MemoryCapBytes: *capBytes, Preload: &engine.Preload{Export: jsengine.LoadExport, Data: bundle}})
			inst = append(inst, time.Since(t))
			if err != nil {
				panic(err)
			}
			t = time.Now()
			if _, err := i.Handle(ctx, w1, newHost()); err != nil {
				panic(err)
			}
			first = append(first, time.Since(t))
			_ = i.Close(ctx)
			_ = e.Close(ctx)
		}
		fmt.Printf("RESULT metric=cold engine=wazero disk_cache=%v compile_js_engine=%s compile_go_guest=%s instantiate_js_with_bundle=%s first_call_w1=%s\n",
			cached, mediaMS(cJS), mediaMS(cGo), mediaMS(inst), mediaMS(first))
	}

	// wasmtime
	var cJS, cGo, ser, deser, inst, first []time.Duration
	var serBytes int
	for range reps {
		e, err := wtengine.New(wtengine.Config{Epochs: true})
		if err != nil {
			panic(err)
		}
		t := time.Now()
		m, err := e.Compile(jsengine.Wasm, true)
		cJS = append(cJS, time.Since(t))
		if err != nil {
			panic(err)
		}
		t = time.Now()
		if _, err := e.Compile(gowasm, false); err != nil {
			panic(err)
		}
		cGo = append(cGo, time.Since(t))
		t = time.Now()
		b, err := m.Serialize()
		ser = append(ser, time.Since(t))
		if err != nil {
			panic(err)
		}
		serBytes = len(b)
		t = time.Now()
		if _, err := e.Deserialize(b, true); err != nil {
			panic(err)
		}
		deser = append(deser, time.Since(t))
		t = time.Now()
		i, err := m.Instantiate(ctx, wtengine.InstanceConfig{MemoryCapBytes: *capBytes, Preload: &engine.Preload{Export: jsengine.LoadExport, Data: bundle}})
		inst = append(inst, time.Since(t))
		if err != nil {
			panic(err)
		}
		t = time.Now()
		if _, err := i.Handle(ctx, w1, newHost()); err != nil {
			panic(err)
		}
		first = append(first, time.Since(t))
		i.Close()
		e.Close()
	}
	fmt.Printf("RESULT metric=cold engine=wasmtime compile_js_engine=%s compile_go_guest=%s serialize_js_engine=%s serialized_bytes=%d deserialize_js_engine=%s instantiate_js_with_bundle=%s first_call_w1=%s\n",
		mediaMS(cJS), mediaMS(cGo), mediaMS(ser), serBytes, mediaMS(deser), mediaMS(inst), mediaMS(first))
}
