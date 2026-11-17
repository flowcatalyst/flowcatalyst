//go:build wasmtime

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

func rssMB() float64 {
	runtime.GC()
	debug.FreeOSMemory()
	time.Sleep(100 * time.Millisecond)
	if b, err := os.ReadFile("/proc/self/status"); err == nil { // Linux (no ps in the slim image)
		for l := range strings.SplitSeq(string(b), "\n") {
			if v, ok := strings.CutPrefix(l, "VmRSS:"); ok {
				k, _ := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "kB")))
				return float64(k) / 1024
			}
		}
	}
	out, _ := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(os.Getpid())).Output()
	k, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return float64(k) / 1024
}

// memory runs guests that allocate past their cap (-cap): the call must fail
// alone, recognisably, and the next call and the process must be unaffected.
func memory(ctx context.Context) {
	all := []rcase{
		{name: "js-typed-arrays", kind: "js", script: jsScript(`const a = []; for (;;) { a.push(new Uint8Array(1 << 20)); }`), frame: metaFrame(`{"w":0}`), okFn: jsScript(jsOK), okReq: metaFrame(`{"w":0}`)},
		{name: "js-strings", kind: "js", script: jsScript(`const s = "x".repeat(1 << 20); const a = []; for (;;) { a.push(s + a.length); }`), frame: metaFrame(`{"w":0}`), okFn: jsScript(jsOK), okReq: metaFrame(`{"w":0}`)},
		{name: "go-alloc", kind: "wasm", path: "build/gospike.wasm", frame: metaFrame(`{"w":11}`), okReq: metaFrame(`{"w":13}`)},
	}
	fmt.Printf("# instance cap %d MiB\n", *capBytes>>20)
	for _, e := range engines() {
		r := makeRT(ctx, e)
		base := rssMB()
		for _, c := range all {
			if *cases != "" && !strings.Contains(","+*cases+",", ","+c.name+",") {
				continue
			}
			bad, ok := loadCase(ctx, r, c)
			for round := 1; round <= *rounds; round++ {
				i := newInst(ctx, bad)
				t0 := time.Now()
				cctx, cancel := withTimeout(30 * time.Second)
				out, err := i.Handle(cctx, c.frame, newHost())
				el := time.Since(t0)
				cancel()
				peak := rssMB()
				i.Close()
				after := rssMB()
				j := newInst(ctx, ok)
				cctx, cancel = withTimeout(5 * time.Second)
				o2, nerr := j.Handle(cctx, c.okReq, newHost())
				cancel()
				j.Close()
				next := "ok"
				if nerr != nil || string(bodyOf(o2)) != "ok" {
					next = fmt.Sprintf("FAILED %v", nerr)
				}
				resp := ""
				if err == nil {
					resp = fmt.Sprintf("%.200q", out)
				}
				fmt.Printf("RESULT metric=mem engine=%s case=%s round=%d elapsed=%s outcome=%q rss_base=%.0fMB rss_before_close=%.0fMB rss_after_close=%.0fMB next_call=%s err=%q resp=%s\n",
					e, c.name, round, ms(el), outcome(err), base, peak, after, next, errText(err), resp)
			}
		}
		closeRT(r)
	}
}
