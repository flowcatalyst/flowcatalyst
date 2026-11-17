//go:build wasmtime

// Command spike answers the wasmtime-go spike's questions with measurements
// (docs/function-runtime-wasmtime-go-spike-2026-11-17.md). Every mode runs the
// same code against the production wazero engine and against wtengine.
//
//	spike -mode equiv      byte-identical outputs, real host calls (question 1)
//	spike -mode runaway    runaways stopped at the deadline, nothing left running (2)
//	spike -mode gcpause    GC stop-the-world pauses while a runaway runs (3)
//	spike -mode mem        a guest past its memory cap (4)
//	spike -mode lat        W1-W5 latency, interleaved (5)
//	spike -mode hostcost   cost of one host call (5)
//	spike -mode conc       N goroutines on W3: throughput, p99, OS threads (7)
//	spike -mode migrate    one store used from many OS threads in turn (7)
//
// Run from bench/function-runtimes/wasmtime-go-spike (paths are relative to it).
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/metrics"
	"runtime/pprof"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
)

var (
	mode     = flag.String("mode", "equiv", "equiv|runaway|gcpause|mem|lat|hostcost|conc|migrate")
	engList  = flag.String("engines", "wazero,wasmtime", "engines: wazero, wasmtime, wt-armed, wt-noep")
	capBytes = flag.Uint64("cap", 512<<20, "instance memory cap, bytes")
	tick     = flag.Duration("tick", time.Millisecond, "epoch tick")
	grace    = flag.Duration("grace", 20*time.Millisecond, "wt-armed: epoch deadline after the JS deadline")
	deadline = flag.Duration("deadline", 250*time.Millisecond, "runaway/gcpause deadline")
	cases    = flag.String("cases", "", "runaway/gcpause/mem cases (default: all)")
	rounds   = flag.Int("rounds", 3, "runaway rounds per case")
	passes   = flag.Int("passes", 2, "lat: interleaved passes over the engines")
	runs     = flag.Int("runs", 5, "lat: runs per cell")
	dur      = flag.Duration("dur", 1500*time.Millisecond, "lat: measured time per run")
	warm     = flag.Duration("warm", 500*time.Millisecond, "lat: warm-up per cell")
	only     = flag.String("w", "1,2,3,4,5", "lat: workloads")
	guests   = flag.String("guests", "qjs,go", "lat/equiv: guests")
	conc     = flag.String("conc", "1,14,64", "conc: goroutine counts")
	cdur     = flag.Duration("cdur", 5*time.Second, "conc: duration per cell")
	cw       = flag.Int("cw", 3, "conc: workload")
	watch    = flag.Duration("watch", 0, "runaway: after each round, sample process CPU every second for this long")
	cpuprof  = flag.String("cpuprofile", "", "write a CPU profile")
	mtxprof  = flag.String("mutexprofile", "", "write a mutex contention profile")
	root     = flag.String("bench", "..", "bench/function-runtimes directory")
)

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
func jsonMarshal(v any) ([]byte, error)   { return json.Marshal(v) }

func main() {
	flag.Parse()
	fmt.Printf("# %s/%s %s GOMAXPROCS=%d ncpu=%d\n", runtime.GOOS, runtime.GOARCH, runtime.Version(), runtime.GOMAXPROCS(0), runtime.NumCPU())
	ctx := context.Background()
	if *cpuprof != "" {
		f, err := os.Create(*cpuprof)
		if err != nil {
			panic(err)
		}
		_ = pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}
	if *mtxprof != "" {
		runtime.SetMutexProfileFraction(1)
		defer func() {
			f, _ := os.Create(*mtxprof)
			_ = pprof.Lookup("mutex").WriteTo(f, 0)
			f.Close()
		}()
	}
	switch *mode {
	case "equiv":
		equiv(ctx)
	case "runaway":
		runaway(ctx)
	case "gcpause":
		gcpause(ctx)
	case "mem":
		memory(ctx)
	case "lat":
		latency(ctx)
	case "hostcost":
		hostcost(ctx)
	case "conc":
		concurrency(ctx)
	case "migrate":
		migrate(ctx)
	case "cold":
		cold(ctx)
	case "idle":
		for _, e := range engines() {
			r := makeRT(ctx, e)
			f := load(ctx, r, guestByLabel("qjs"))
			i := newInst(ctx, f)
			call(i, inputs()[1].frame, newHost())
			time.Sleep(500 * time.Millisecond)
			var t0 uint64
			if w, ok := r.(*wtRT); ok {
				t0 = w.e.Ticks()
			}
			cpu := idleCPU(3 * time.Second)
			rate := 0.0
			if w, ok := r.(*wtRT); ok {
				rate = float64(w.e.Ticks()-t0) / 3
			}
			// A plain time.Ticker of the same period, for comparison.
			tk := time.NewTicker(*tick)
			n, ts := 0, time.Now()
			for time.Since(ts) < time.Second {
				<-tk.C
				n++
			}
			tk.Stop()
			fmt.Printf("RESULT metric=idle engine=%s tick=%v cpu_ms_per_second=%.1f threads=%d epoch_ticks_per_second=%.0f plain_ticker_per_second=%d\n", e, *tick, float64(cpu)/3e6, osThreads(), rate, n)
			i.Close()
			r.Close()
		}
	default:
		fmt.Fprintln(os.Stderr, "unknown mode")
		os.Exit(2)
	}
}

// ---- artifacts and inputs

type guest struct{ label, kind, path string }

func guestByLabel(l string) guest {
	switch l {
	case "qjs":
		return guest{"qjs", "js", filepath.Join(*root, "js/dist/function.js")}
	case "go":
		return guest{"go", "wasm", filepath.Join(*root, "go-guest/fcbench-go.wasm")}
	case "gospike":
		return guest{"gospike", "wasm", "build/gospike.wasm"}
	}
	panic("unknown guest " + l)
}

func mustRead(p string) []byte {
	b, err := os.ReadFile(p)
	if err != nil {
		panic(err)
	}
	return b
}

type input struct{ frame, expect []byte }

func inputs() map[int]input {
	fx := filepath.Join(*root, "fixtures")
	bodies := map[int][]byte{1: mustRead(fx + "/input2k.json"), 2: mustRead(fx + "/input2k.json"), 3: mustRead(fx + "/trip15k.xml"), 4: nil, 5: nil}
	m := map[int]input{}
	for w, b := range bodies {
		m[w] = input{frame: abi.EncodeFrame([]byte(fmt.Sprintf(`{"w":%d}`, w)), b), expect: mustRead(fmt.Sprintf("%s/expected-w%d.txt", fx, w))}
	}
	return m
}

func metaFrame(meta string) []byte { return abi.EncodeFrame([]byte(meta), nil) }

// jsScript is a JS function whose handler body is body (as cooperative_test.go's jsHandler).
func jsScript(body string) []byte {
	return []byte(`globalThis.__fc = {
  describe() { return "{}"; },
  handle(meta, body) { ` + body + ` },
};`)
}

const jsOK = `return { meta: '{"status":200}', body: "ok" };`

func sha(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:8]) }

func bodyOf(frame []byte) []byte {
	_, b, err := abi.DecodeFrame(frame)
	if err != nil {
		return nil
	}
	return b
}

func load(ctx context.Context, r rt, g guest) fn {
	f, err := r.Load(ctx, g.kind, mustRead(g.path))
	if err != nil {
		panic(fmt.Sprintf("%s %s: %v", r.Name(), g.label, err))
	}
	return f
}

func newInst(ctx context.Context, f fn) inst {
	i, err := f.New(ctx)
	if err != nil {
		panic(err)
	}
	return i
}

func withTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}

// ---- process state: CPU time and OS threads

func cpuTime() time.Duration {
	var ru syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}

// osThreads is the process's OS thread count.
func osThreads() int {
	if runtime.GOOS == "linux" {
		b, err := os.ReadFile("/proc/self/status")
		if err == nil {
			for l := range strings.SplitSeq(string(b), "\n") {
				if v, ok := strings.CutPrefix(l, "Threads:"); ok {
					n, _ := strconv.Atoi(strings.TrimSpace(v))
					return n
				}
			}
		}
		return -1
	}
	out, err := exec.Command("ps", "-M", "-p", strconv.Itoa(os.Getpid())).Output()
	if err != nil {
		return -1
	}
	return len(strings.Split(strings.TrimSpace(string(out)), "\n")) - 1
}

// idleCPU is the CPU the process burns over d while this goroutine sleeps:
// about zero when nothing is left running, about d per leaked spinning thread.
// (No runtime.GC here: on wazero a collection waits for a guest that is still
// running, which is the very pause question 3 measures.)
func idleCPU(d time.Duration) time.Duration {
	c0 := cpuTime()
	time.Sleep(d)
	return cpuTime() - c0
}

// ---- GC pause histogram (as cooperative_test.go)

const gcPauses = "/sched/pauses/total/gc:seconds"

func readGCPauses() *metrics.Float64Histogram {
	s := []metrics.Sample{{Name: gcPauses}}
	metrics.Read(s)
	return s[0].Value.Float64Histogram()
}

func longestPauseSince(before, after *metrics.Float64Histogram) time.Duration {
	for i, n := range slices.Backward(after.Counts) {
		if n > before.Counts[i] {
			edge := after.Buckets[i+1]
			if math.IsInf(edge, 1) {
				edge = after.Buckets[i]
			}
			return time.Duration(edge * float64(time.Second))
		}
	}
	return 0
}

// ---- stats

func pct(d []time.Duration, p float64) time.Duration {
	s := slices.Clone(d)
	slices.Sort(s)
	return s[int(float64(len(s)-1)*p)]
}

func us(d time.Duration) float64 { return float64(d) / 1e3 }

func median(v []float64) float64 {
	s := slices.Clone(v)
	slices.Sort(s)
	return s[len(s)/2]
}

func ms(d time.Duration) string { return fmt.Sprintf("%.1fms", float64(d)/1e6) }

func errText(err error) string {
	if err == nil {
		return "nil"
	}
	s := err.Error()
	s = strings.ReplaceAll(s, "\n", " | ")
	if len(s) > 160 {
		s = s[:160] + "..."
	}
	return s
}

var _ = bytes.Equal
var _ sync.Mutex
