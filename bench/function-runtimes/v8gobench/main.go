// Command v8gobench runs the benchmark bundle on V8 embedded in Go via cgo
// (github.com/tommie/v8go) to price option "V8 in the Go runner". Its own
// module: the platform's go.mod and build are untouched.
//
// Marshalling is simplified compared with the real ABI (v8go has no typed-array
// constructor): the body crosses as a JS string and __fc_call returns a
// synthesised result with the body length from a cheap Go callback. Compute
// (W2, W3, W4) is byte-for-byte the same bundle; W1 and W5 are therefore
// optimistic by one string copy / one object allocation per call.
//
//	v8gobench [-script f.js] [-fixtures dir] [-runs 5] [-w 1,2,3,4,5] [-conc 1,4,14]
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	v8 "github.com/tommie/v8go"
)

const preamble = `globalThis.__fc_call = (op, meta, body) => ({ code: 0, meta: "", body: { length: __fc_call_raw(op) } });`

type fn struct {
	iso    *v8.Isolate
	ctx    *v8.Context
	fc     *v8.Object
	handle *v8.Function
}

func load(script string) *fn {
	iso := v8.NewIsolate()
	g := v8.NewObjectTemplate(iso)
	g.Set("__fc_call_raw", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		n := int32(0)
		if info.Args()[0].Int32() == 2 {
			n = int32(len("hello-config-value"))
		}
		v, _ := v8.NewValue(iso, n)
		return v
	}))
	g.Set("__fc_utf8_decode", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value { return info.Args()[0] }))
	g.Set("__fc_utf8_encode", v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value { return info.Args()[0] }))
	ctx := v8.NewContext(iso, g)
	if _, err := ctx.RunScript(script+"\n"+preamble, "fn.js"); err != nil {
		panic(err)
	}
	gl := ctx.Global()
	fcv, err := gl.Get("__fc")
	if err != nil {
		panic(err)
	}
	fc, _ := fcv.AsObject()
	hv, _ := fc.Get("handle")
	h, _ := hv.AsFunction()
	return &fn{iso, ctx, fc, h}
}

func (f *fn) call(meta string, body []byte) []byte {
	mv, _ := v8.NewValue(f.iso, meta)
	bv, _ := v8.NewValue(f.iso, string(body))
	defer mv.Release() // v8go keeps every Value alive until the context closes unless released
	defer bv.Release()
	r, err := f.handle.Call(f.fc, mv, bv)
	if err != nil {
		panic(err)
	}
	if r.IsPromise() {
		p, _ := r.AsPromise()
		if p.State() != v8.Fulfilled {
			panic("promise not fulfilled")
		}
		defer r.Release()
		r = p.Result()
	}
	defer r.Release()
	o, _ := r.AsObject()
	b, _ := o.Get("body")
	defer b.Release()
	return []byte(b.String())
}

func main() {
	script := flag.String("script", "bench/function-runtimes/js/dist/function.js", "")
	fx := flag.String("fixtures", "bench/function-runtimes/fixtures", "")
	runs := flag.Int("runs", 5, "")
	only := flag.String("w", "1,2,3,4,5", "")
	concs := flag.String("conc", "1,4,14", "")
	cold := flag.Bool("cold", false, "")
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
	if *cold {
		var ds []float64
		for i := 0; i < 15; i++ {
			s := time.Now()
			f := load(string(src))
			f.call(`{"w":1}`, bodies[1])
			ds = append(ds, float64(time.Since(s))/1e6)
		}
		slices.Sort(ds)
		fmt.Printf("RESULT rt=v8go metric=cold new_isolate_eval_first_call_ms=%.2f (median of 15)\n", ds[len(ds)/2])
		return
	}
	runtime.LockOSThread()
	f := load(string(src))
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
		fmt.Printf("RESULT rt=v8go w=%d metric=lat p50_us=%.2f p50_min=%.2f p50_max=%.2f p99_us=%.2f runs=%d\n", w, m(p50s), slices.Min(p50s), slices.Max(p50s), m(p99s), *runs)
		for _, cs := range strings.Split(*concs, ",") {
			var c int
			fmt.Sscan(cs, &c)
			var rates []float64
			for rep := 0; rep < 3; rep++ {
				var n atomic.Int64
				var stop atomic.Bool
				var wg, ready sync.WaitGroup
				ready.Add(c)
				start := make(chan struct{})
				for i := 0; i < c; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						runtime.LockOSThread()
						f := load(string(src))
						ready.Done()
						<-start
						for !stop.Load() {
							f.call(meta, bodies[w])
							n.Add(1)
						}
					}()
				}
				ready.Wait()
				t0 := time.Now()
				close(start)
				time.Sleep(3 * time.Second)
				stop.Store(true)
				wg.Wait()
				rates = append(rates, float64(n.Load())/time.Since(t0).Seconds())
			}
			slices.Sort(rates)
			fmt.Printf("RESULT rt=v8go w=%d metric=tput c=%d rps=%.0f rps_min=%.0f rps_max=%.0f\n", w, c, rates[1], rates[0], rates[2])
		}
	}
}
