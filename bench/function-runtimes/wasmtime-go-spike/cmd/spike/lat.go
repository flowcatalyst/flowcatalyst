//go:build wasmtime

package main

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func call(i inst, frame []byte, h *benchHost) []byte {
	ctx, cancel := withTimeout(time.Hour) // a deadline, as the runner's per-call context has
	defer cancel()
	out, err := i.Handle(ctx, frame, h)
	if err != nil {
		panic(err)
	}
	return out
}

type latRes struct{ p50, p50min, p50max, p99 float64 }

func measure(i inst, frame []byte, h *benchHost) latRes {
	for t0 := time.Now(); time.Since(t0) < *warm; {
		call(i, frame, h)
	}
	var p50s, p99s []float64
	for range *runs {
		var ds []time.Duration
		for t0 := time.Now(); time.Since(t0) < *dur; {
			s := time.Now()
			call(i, frame, h)
			ds = append(ds, time.Since(s))
		}
		p50s = append(p50s, us(pct(ds, .5)))
		p99s = append(p99s, us(pct(ds, .99)))
	}
	return latRes{median(p50s), slices.Min(p50s), slices.Max(p50s), median(p99s)}
}

// latency is W1-W5, single thread, on one warm instance per cell, with the
// engines interleaved: pass 1 runs every engine, then pass 2 again.
func latency(ctx context.Context) {
	in := inputs()
	var ws []int
	for s := range strings.SplitSeq(*only, ",") {
		n, _ := strconv.Atoi(s)
		ws = append(ws, n)
	}
	for pass := 1; pass <= *passes; pass++ {
		for _, e := range engines() {
			r := makeRT(ctx, e)
			for _, gl := range strings.Split(*guests, ",") {
				f := load(ctx, r, guestByLabel(gl))
				for _, w := range ws {
					i := newInst(ctx, f)
					h := newHost()
					if out := call(i, in[w].frame, h); !bytes.Equal(bodyOf(out), in[w].expect) {
						panic(fmt.Sprintf("%s %s W%d: output differs from the reference", e, gl, w))
					}
					res := measure(i, in[w].frame, h)
					i.Close()
					fmt.Printf("RESULT metric=lat pass=%d engine=%s guest=%s w=%d p50_us=%.2f p50_min=%.2f p50_max=%.2f p99_us=%.2f\n",
						pass, e, gl, w, res.p50, res.p50min, res.p50max, res.p99)
				}
			}
			r.Close()
		}
	}
}

// hostcost prices one fc.call + fc.take round trip: the same guest making 0
// and 1000 config.get calls per invocation, (p50(1000) - p50(0)) / 1000.
func hostcost(ctx context.Context) {
	const n = 1000
	jsLoop := func(k int) []byte {
		return jsScript(fmt.Sprintf(`let s = 0; for (let i = 0; i < %d; i++) { s += __fc_call(2, '{"key":"BENCH"}', undefined).body.length; } return { meta: '{"status":200}', body: String(s) };`, k))
	}
	for pass := 1; pass <= *passes; pass++ {
		for _, e := range engines() {
			r := makeRT(ctx, e)
			// JS
			var p [2]latRes
			for k, cnt := range []int{0, n} {
				f, err := r.Load(ctx, "js", jsLoop(cnt))
				if err != nil {
					panic(err)
				}
				i := newInst(ctx, f)
				h := newHost()
				if got := string(bodyOf(call(i, metaFrame(`{"w":0}`), h))); got != strconv.Itoa(cnt*18) {
					panic("js host loop answered " + got)
				}
				p[k] = measure(i, metaFrame(`{"w":0}`), h)
				i.Close()
			}
			fmt.Printf("RESULT metric=hostcost pass=%d engine=%s guest=qjs p50_0calls_us=%.2f p50_1000calls_us=%.2f per_host_call_us=%.3f\n",
				pass, e, p[0].p50, p[1].p50, (p[1].p50-p[0].p50)/n)
			// Go
			f := load(ctx, r, guestByLabel("gospike"))
			for k, cnt := range []int{0, n} {
				i := newInst(ctx, f)
				h := newHost()
				fr := metaFrame(fmt.Sprintf(`{"w":12,"n":%d}`, cnt))
				if got := string(bodyOf(call(i, fr, h))); got != strconv.Itoa(cnt*36) {
					panic("go host loop answered " + got)
				}
				p[k] = measure(i, fr, h)
				i.Close()
			}
			fmt.Printf("RESULT metric=hostcost pass=%d engine=%s guest=go p50_0calls_us=%.2f p50_1000calls_us=%.2f per_host_call_us=%.3f\n",
				pass, e, p[0].p50, p[1].p50, (p[1].p50-p[0].p50)/n)
			r.Close()
		}
	}
}

// concurrency runs c goroutines, each with its own warm instance, on one
// workload for -cdur, and samples the OS thread count while they run.
func concurrency(ctx context.Context) {
	in := inputs()[*cw]
	for pass := 1; pass <= *passes; pass++ {
		for _, e := range engines() {
			r := makeRT(ctx, e)
			for _, gl := range strings.Split(*guests, ",") {
				f := load(ctx, r, guestByLabel(gl))
				for cs := range strings.SplitSeq(*conc, ",") {
					c, _ := strconv.Atoi(cs)
					insts := make([]inst, c)
					for k := range insts {
						insts[k] = newInst(ctx, f)
						call(insts[k], in.frame, newHost())
					}
					threads0 := osThreads()
					var stop atomic.Bool
					var wg sync.WaitGroup
					lats := make([][]time.Duration, c)
					start := time.Now()
					for k, i := range insts {
						wg.Go(func() {
							h := newHost()
							for !stop.Load() {
								s := time.Now()
								out := call(i, in.frame, h)
								lats[k] = append(lats[k], time.Since(s))
								if len(lats[k]) == 1 && !bytes.Equal(bodyOf(out), in.expect) {
									panic("concurrent output differs")
								}
							}
						})
					}
					maxThreads := 0
					for time.Since(start) < *cdur {
						time.Sleep(100 * time.Millisecond)
						maxThreads = max(maxThreads, osThreads())
					}
					stop.Store(true)
					wg.Wait()
					el := time.Since(start).Seconds()
					var all []time.Duration
					for _, l := range lats {
						all = append(all, l...)
					}
					for _, i := range insts {
						i.Close()
					}
					fmt.Printf("RESULT metric=conc pass=%d engine=%s guest=%s w=%d c=%d rps=%.0f p50_us=%.0f p99_us=%.0f threads_before=%d threads_max=%d threads_after=%d\n",
						pass, e, gl, *cw, c, float64(len(all))/el, us(pct(all, .5)), us(pct(all, .99)), threads0, maxThreads, osThreads())
				}
			}
			r.Close()
		}
	}
}

// migrate uses ONE instance from 200 goroutines in turn, each locked to its own
// new OS thread (a goroutine that exits while locked takes its thread with
// it), so consecutive calls on the store run on different OS threads; then a
// runaway on one thread followed by a fresh instance on another.
func migrate(ctx context.Context) {
	in := inputs()[3]
	for _, e := range engines() {
		r := makeRT(ctx, e)
		f := load(ctx, r, guestByLabel("qjs"))
		i := newInst(ctx, f)
		h := newHost()
		okN, tids := 0, map[int]bool{}
		for range 200 {
			done := make(chan bool)
			go func() {
				runtime.LockOSThread() // never unlocked: the thread exits with the goroutine
				tids[threadID()] = true
				done <- bytes.Equal(bodyOf(call(i, in.frame, h)), in.expect)
			}()
			if <-done {
				okN++
			}
		}
		i.Close()
		fmt.Printf("RESULT metric=migrate engine=%s calls=200 correct=%d distinct_os_threads=%d\n", e, okN, len(tids))
		r.Close()
	}
}
