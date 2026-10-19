// Command gojabench runs the same JS bundle (js/dist/function.js) on goja, the
// pure-Go ES5.1+ interpreter, for comparison with QuickJS-in-wazero. It is its
// own module so goja is not added to the platform's go.mod.
//
//	gojabench [-script f.js] [-fixtures dir] [-runs 5] [-w 1,2,3,4,5] [-conc 1,4,14]
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dop251/goja"
)

type fn struct {
	vm     *goja.Runtime
	handle goja.Callable
}

func load(script string) *fn {
	vm := goja.New()
	vm.Set("__fc_call", func(c goja.FunctionCall) goja.Value {
		op := c.Argument(0).ToInteger()
		o := vm.NewObject()
		o.Set("code", 0)
		if op == 2 {
			o.Set("meta", `{"found":true}`)
			o.Set("body", u8(vm, []byte("hello-config-value")))
		} else {
			o.Set("meta", "")
			o.Set("body", u8(vm, nil))
		}
		return o
	})
	vm.Set("__fc_utf8_encode", func(c goja.FunctionCall) goja.Value { return u8(vm, []byte(c.Argument(0).String())) })
	vm.Set("__fc_utf8_decode", func(c goja.FunctionCall) goja.Value {
		b, _ := c.Argument(0).Export().([]byte)
		return vm.ToValue(string(b))
	})
	if _, err := vm.RunString(script); err != nil {
		panic(err)
	}
	h, ok := goja.AssertFunction(vm.Get("__fc").ToObject(vm).Get("handle"))
	if !ok {
		panic("no handle")
	}
	return &fn{vm, h}
}

func u8(vm *goja.Runtime, b []byte) goja.Value {
	if b == nil {
		b = []byte{}
	}
	u, err := vm.New(vm.Get("Uint8Array"), vm.ToValue(vm.NewArrayBuffer(b)))
	if err != nil {
		panic(err)
	}
	return u
}

func (f *fn) call(meta string, body []byte) []byte {
	r, err := f.handle(goja.Undefined(), f.vm.ToValue(meta), u8(f.vm, body))
	if err != nil {
		panic(err)
	}
	if p, ok := r.Export().(*goja.Promise); ok {
		if p.State() != goja.PromiseStateFulfilled {
			panic("promise not fulfilled")
		}
		r = p.Result()
	}
	b := r.ToObject(f.vm).Get("body")
	if s, ok := b.Export().(string); ok {
		return []byte(s)
	}
	out, _ := b.Export().([]byte)
	return out
}

func main() {
	script := flag.String("script", "bench/function-runtimes/js/dist/function.js", "")
	fx := flag.String("fixtures", "bench/function-runtimes/fixtures", "")
	runs := flag.Int("runs", 5, "")
	only := flag.String("w", "1,2,3,4,5", "")
	concs := flag.String("conc", "1,4,14", "")
	flag.Parse()
	src, _ := os.ReadFile(*script)
	rd := func(n string) []byte {
		b, err := os.ReadFile(*fx + "/" + n)
		if err != nil {
			panic(err)
		}
		return b
	}
	bodies := map[int][]byte{1: rd("input2k.json"), 2: rd("input2k.json"), 3: rd("trip15k.xml")}

	s := time.Now()
	f := load(string(src))
	fmt.Printf("RESULT rt=goja metric=cold size_bytes=%d new_runtime_and_eval_ms=%.1f\n", len(src), float64(time.Since(s))/1e6)

	for _, ws := range strings.Split(*only, ",") {
		var w int
		fmt.Sscan(ws, &w)
		meta := fmt.Sprintf(`{"w":%d}`, w)
		want := rd(fmt.Sprintf("expected-w%d.txt", w))
		if got := f.call(meta, bodies[w]); !bytes.Equal(got, want) {
			panic(fmt.Sprintf("w%d mismatch: %.100q", w, got))
		}
		for t0 := time.Now(); time.Since(t0) < 500*time.Millisecond; {
			f.call(meta, bodies[w])
		}
		var p50s, p99s []float64
		for r := 0; r < *runs; r++ {
			var ds []time.Duration
			for t0 := time.Now(); time.Since(t0) < 1500*time.Millisecond || len(ds) < 2; {
				s := time.Now()
				f.call(meta, bodies[w])
				ds = append(ds, time.Since(s))
			}
			slices.Sort(ds)
			p50s = append(p50s, float64(ds[len(ds)/2])/1e3)
			p99s = append(p99s, float64(ds[int(float64(len(ds)-1)*.99)])/1e3)
		}
		m := func(v []float64) float64 { s := slices.Clone(v); slices.Sort(s); return s[len(s)/2] }
		fmt.Printf("RESULT rt=goja w=%d metric=lat p50_us=%.2f p50_min=%.2f p50_max=%.2f p99_us=%.2f runs=%d\n", w, m(p50s), slices.Min(p50s), slices.Max(p50s), m(p99s), *runs)
		for _, cs := range strings.Split(*concs, ",") {
			var c int
			fmt.Sscan(cs, &c)
			var rates []float64
			for rep := 0; rep < 3; rep++ {
				fs := make([]*fn, c)
				for i := range fs {
					fs[i] = load(string(src))
				}
				var n atomic.Int64
				var stop atomic.Bool
				var wg sync.WaitGroup
				t0 := time.Now()
				for i := range fs {
					wg.Add(1)
					go func() {
						defer wg.Done()
						for !stop.Load() {
							fs[i].call(meta, bodies[w])
							n.Add(1)
						}
					}()
				}
				time.Sleep(3 * time.Second)
				stop.Store(true)
				wg.Wait()
				rates = append(rates, float64(n.Load())/time.Since(t0).Seconds())
			}
			slices.Sort(rates)
			fmt.Printf("RESULT rt=goja w=%d metric=tput c=%d rps=%.0f rps_min=%.0f rps_max=%.0f\n", w, c, rates[1], rates[0], rates[2])
		}
	}
}
