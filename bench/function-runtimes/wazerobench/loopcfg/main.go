// Command loopcfg isolates what wazero's CloseOnContextDone (the runner's
// deadline mechanism, engine.New) costs each guest: every workload, single
// thread, with the setting on (what the runner does) and off.
//
//	loopcfg label=path[.wasm|.js] ...      (a .js bundle runs on the shared QuickJS engine)
//
// The calls use a context WITH a Done channel in both cases, as the runner's
// per-call deadline does; only the runtime-level setting differs.
package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/jsengine"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

func main() {
	fx := "bench/function-runtimes/fixtures/"
	rd := func(n string) []byte { b, _ := os.ReadFile(fx + n); return b }
	bodies := map[int][]byte{1: rd("input2k.json"), 2: rd("input2k.json"), 3: rd("trip15k.xml")}
	for _, arg := range os.Args[1:] {
		label, path, _ := strings.Cut(arg, "=")
		wasm := rd2(path)
		script := ""
		if strings.HasSuffix(path, ".js") {
			script, wasm = string(wasm), jsengine.Wasm
			if p := os.Getenv("JSENGINE"); p != "" { // measure a differently built engine.wasm
				wasm = rd2(p)
			}
		}
		for _, cod := range []bool{true, false} {
			ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
			rt := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().WithCloseOnContextDone(cod))
			wasi_snapshot_preview1.Instantiate(ctx, rt)
			rt.NewHostModuleBuilder("fc").
				NewFunctionBuilder().WithGoModuleFunction(api.GoModuleFunc(func(_ context.Context, _ api.Module, s []uint64) { s[0] = 0 }), []api.ValueType{api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32}, []api.ValueType{api.ValueTypeI64}).Export("call").
				NewFunctionBuilder().WithGoModuleFunction(api.GoModuleFunc(func(_ context.Context, _ api.Module, s []uint64) {}), []api.ValueType{api.ValueTypeI32}, nil).Export("take").
				Instantiate(ctx)
			m, err := rt.Instantiate(ctx, wasm)
			if err != nil {
				panic(err)
			}
			if init := m.ExportedFunction("_initialize"); init != nil { // Go/TinyGo reactor modules
				if _, err := init.Call(ctx); err != nil {
					panic(err)
				}
			}
			if script != "" {
				p, _ := m.ExportedFunction("fc_alloc").Call(ctx, uint64(len(script)))
				m.Memory().Write(uint32(p[0]), []byte(script))
				if r, err := m.ExportedFunction("fc_js_load").Call(ctx, p[0], uint64(len(script))); err != nil || r[0] != 0 {
					panic(fmt.Sprint("load ", err))
				}
			}
			for w := 1; w <= 4; w++ { // W5 needs a real host; the deadline hook's cost is the loop/entry checks
				meta := []byte(fmt.Sprintf(`{"w":%d}`, w))
				in := make([]byte, 4+len(meta)+len(bodies[w]))
				binary.LittleEndian.PutUint32(in, uint32(len(meta)))
				copy(in[4:], meta)
				copy(in[4+len(meta):], bodies[w])
				var ds []time.Duration
				for t0 := time.Now(); time.Since(t0) < 1500*time.Millisecond || len(ds) < 3; {
					p, _ := m.ExportedFunction("fc_alloc").Call(ctx, uint64(len(in)))
					m.Memory().Write(uint32(p[0]), in)
					s := time.Now()
					if _, err := m.ExportedFunction("fc_handle").Call(ctx, p[0], uint64(len(in))); err != nil {
						panic(err)
					}
					ds = append(ds, time.Since(s))
				}
				slices.Sort(ds)
				fmt.Printf("RESULT rt=%s w=%d closeOnContextDone=%v p50_us=%.2f min_us=%.2f max_us=%.2f calls=%d\n", label, w, cod, float64(ds[len(ds)/2])/1e3, float64(ds[0])/1e3, float64(ds[len(ds)-1])/1e3, len(ds))
			}
			rt.Close(ctx)
			cancel()
		}
	}
}

func rd2(p string) []byte {
	b, err := os.ReadFile(p)
	if err != nil {
		panic(err)
	}
	return b
}
