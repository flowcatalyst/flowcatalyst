//go:build wasmtime

package main

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/engine"
)

// A runaway shape: a guest, and the request that makes it run away.
type rcase struct {
	name   string
	kind   string // js | wasm
	script []byte // js
	path   string // wasm
	frame  []byte
	okFn   []byte // the follow-up function (js), or nil for the same wasm module
	okReq  []byte
}

func runawayCases() []rcase {
	js := func(name, body string) rcase {
		return rcase{name: name, kind: "js", script: jsScript(body), frame: metaFrame(`{"w":0}`), okFn: jsScript(jsOK), okReq: metaFrame(`{"w":0}`)}
	}
	all := []rcase{
		js("js-loop", `for (;;) {}`),
		// cooperative_test.go's "bigint": few, expensive poll points.
		js("js-bigint", `let n = 3n; for (;;) { n = n * n; if (n.toString(2).length > 1e5) n = 3n; }`),
		// The deadlines doc's 21.9 s case, reconstructed: a 100 000-character
		// needle searched for in 200 000 characters. QuickJS's indexOf is the
		// naive search, and a needle that matches all but its last character at
		// every position makes it compare ~1e10 characters, with no poll point.
		js("js-indexof", `const h = "a".repeat(2e5); const n = "a".repeat(1e5 - 1) + "b"; return { meta: '{"status":200}', body: String(h.indexOf(n)) };`),
		// Unbounded recursion: traps (QuickJS's C stack runs off linear memory),
		// on wasmtime through its signal handler. Checks cgo + signals.
		js("js-recursion", `const f = (n) => f(n + 1) + 1; for (;;) { try { f(0); } catch (e) {} }`),
		{name: "go-loop", kind: "wasm", path: "build/gospike.wasm", frame: metaFrame(`{"w":10}`), okReq: metaFrame(`{"w":13}`)},
	}
	if *cases == "" {
		return all
	}
	var out []rcase
	for _, c := range all {
		if strings.Contains(","+*cases+",", ","+c.name+",") {
			out = append(out, c)
		}
	}
	return out
}

func loadCase(ctx context.Context, r rt, c rcase) (bad, ok fn) {
	var err error
	if c.kind == "js" {
		if bad, err = r.Load(ctx, "js", c.script); err != nil {
			panic(err)
		}
		if ok, err = r.Load(ctx, "js", c.okFn); err != nil {
			panic(err)
		}
		return bad, ok
	}
	bad, err = r.Load(ctx, "wasm", mustRead(c.path))
	if err != nil {
		panic(err)
	}
	return bad, bad
}

func outcome(err error) string {
	switch {
	case err == nil:
		return "returned"
	case errors.Is(err, engine.ErrStuck):
		return "ErrStuck (abandoned)"
	case errors.Is(err, engine.ErrDeadline):
		return "ErrDeadline"
	case errors.Is(err, engine.ErrOutOfMemory):
		return "ErrOutOfMemory"
	case errors.Is(err, engine.ErrTrap):
		return "ErrTrap"
	case errors.Is(err, engine.ErrExit):
		return "ErrExit"
	}
	return "other"
}

func runaway(ctx context.Context) {
	fmt.Printf("# deadline %v, epoch tick %v, rounds %d\n", *deadline, *tick, *rounds)
	for _, e := range engines() {
		r := makeRT(ctx, e)
		for _, c := range runawayCases() {
			bad, ok := loadCase(ctx, r, c)
			for round := 1; round <= *rounds; round++ {
				i := newInst(ctx, bad)
				t0 := time.Now()
				cctx, cancel := withTimeout(*deadline)
				_, err := i.Handle(cctx, c.frame, newHost())
				el := time.Since(t0)
				cancel()
				broken := i.Broken()
				i.Close()
				// The next call, on a fresh instance, must work.
				j := newInst(ctx, ok)
				cctx, cancel = withTimeout(5 * time.Second)
				out, nerr := j.Handle(cctx, c.okReq, newHost())
				cancel()
				j.Close()
				next := "ok"
				if nerr != nil || string(bodyOf(out)) != "ok" {
					next = fmt.Sprintf("FAILED %v %q", nerr, bodyOf(out))
				}
				idle := idleCPU(500 * time.Millisecond)
				fmt.Printf("RESULT metric=runaway engine=%s case=%s round=%d elapsed=%s past_deadline=%s outcome=%q broken=%v next_call=%s idle_cpu_per_500ms=%s threads=%d err=%q\n",
					e, c.name, round, ms(el), ms(el-*deadline), outcome(err), broken, next, ms(idle), osThreads(), errText(err))
				if *watch > 0 {
					var series []string
					for t0 := time.Now(); time.Since(t0) < *watch; {
						series = append(series, fmt.Sprintf("%.0f", float64(idleCPU(time.Second))/1e6))
					}
					fmt.Printf("RESULT metric=runaway_watch engine=%s case=%s cpu_ms_per_second_after=[%s]\n", e, c.name, strings.Join(series, " "))
				}
			}
		}
		closeRT(r)
	}
}

// closeRT closes an engine without waiting forever for an abandoned wazero call.
func closeRT(r rt) {
	done := make(chan struct{})
	go func() { r.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		fmt.Printf("# %s: Close did not return in 15 s (abandoned calls still running)\n", r.Name())
	}
}

// gcpause measures what TestCooperativeRunawayDoesNotHoldTheProcess measures,
// on each engine: collections started every 5 ms during a runaway call, the
// longest stop-the-world pause from the runtime's histogram, and also how late
// a 1 ms timer fired at worst (how long the rest of the process was held).
func gcpause(ctx context.Context) {
	fmt.Printf("# deadline %v, a runtime.GC() every 5 ms during the call\n", *deadline)
	for _, e := range engines() {
		r := makeRT(ctx, e)
		for _, c := range runawayCases() {
			if c.name == "js-recursion" {
				continue
			}
			bad, _ := loadCase(ctx, r, c)
			for round := 1; round <= *rounds; round++ {
				i := newInst(ctx, bad)
				stop := make(chan struct{})
				var wg sync.WaitGroup
				var gcs int
				var late time.Duration
				before := readGCPauses()
				wg.Go(func() {
					for {
						select {
						case <-stop:
							return
						case <-time.After(5 * time.Millisecond):
						}
						runtime.GC()
						gcs++
					}
				})
				wg.Go(func() {
					for {
						select {
						case <-stop:
							return
						default:
						}
						t := time.Now()
						time.Sleep(time.Millisecond)
						late = max(late, time.Since(t)-time.Millisecond)
					}
				})
				cctx, cancel := withTimeout(*deadline)
				t0 := time.Now()
				_, err := i.Handle(cctx, c.frame, newHost())
				el := time.Since(t0)
				cancel()
				close(stop)
				wg.Wait()
				pause := longestPauseSince(before, readGCPauses())
				i.Close()
				fmt.Printf("RESULT metric=gcpause engine=%s case=%s round=%d elapsed=%s outcome=%q gcs=%d longest_stw_pause<=%s worst_1ms_timer_lateness=%s\n",
					e, c.name, round, ms(el), outcome(err), gcs, fmtDur(pause), ms(late))
			}
		}
		closeRT(r)
	}
}

func fmtDur(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%.1fus", float64(d)/1e3)
	}
	return ms(d)
}
