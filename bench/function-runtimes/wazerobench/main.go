// Command wazerobench measures function artifacts on the platform's real
// function engine (internal/functions/engine, wazero) through the same
// runtimes.Loader the runner uses: a "wasm" artifact is compiled as is, a "js"
// artifact is a bundled script loaded into the shared QuickJS engine.
//
//	wazerobench -mode lat   -a label=kind=path[,...]   latency + throughput per workload
//	wazerobench -mode cold  -a ...                     compile / instantiate / first call
//	wazerobench -mode mem   -a ...                     resident memory per function / instance
//
// Output is one `RESULT ...` line per cell; docs/function-runtime-performance-2026-10-19.md
// has the tables.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/budget"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/engine"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/runtimes"
)

type artifact struct{ label, kind, path string }

type host struct{}

func (host) Call(_ context.Context, op abi.Op, _, _ []byte) ([]byte, []byte, *abi.Error) {
	if op == abi.OpConfigGet {
		return []byte(`{"found":true}`), []byte("hello-config-value"), nil
	}
	return nil, nil, nil
}

var (
	fixtures    = flag.String("fixtures", "bench/function-runtimes/fixtures", "fixtures dir")
	mode        = flag.String("mode", "lat", "lat|cold|mem")
	arts        = flag.String("a", "", "label=kind=path,...")
	runs        = flag.Int("runs", 5, "latency runs per cell")
	dur         = flag.Duration("dur", 1500*time.Millisecond, "measured time per run")
	warm        = flag.Duration("warm", 500*time.Millisecond, "warm-up per cell")
	concs       = flag.String("conc", "1,4,14", "throughput concurrencies")
	tdur        = flag.Duration("tdur", 3*time.Second, "throughput duration")
	only        = flag.String("w", "1,2,3,4,5", "workloads")
	smokeBody   = flag.String("body", "", "smoke: request body file")
	smokePath   = flag.String("path", "/healthz", "smoke: request path")
	smokeMethod = flag.String("method", "GET", "smoke: request method")
	smokeCaller = flag.String("caller", "anonymous", "smoke: caller kind")
	bgctx       = flag.Bool("bgctx", false, "call with a context that has no Done channel (no deadline checks in guest loops)")
)

type input struct{ frame, expect []byte }

func loadInputs() map[int]input {
	read := func(n string) []byte {
		b, err := os.ReadFile(filepath.Join(*fixtures, n))
		if err != nil {
			panic(err)
		}
		return b
	}
	bodies := map[int][]byte{1: read("input2k.json"), 2: read("input2k.json"), 3: read("trip15k.xml"), 4: nil, 5: nil}
	m := map[int]input{}
	for w, b := range bodies {
		meta := []byte(fmt.Sprintf(`{"w":%d}`, w))
		m[w] = input{frame: abi.EncodeFrame(meta, b), expect: read(fmt.Sprintf("expected-w%d.txt", w))}
	}
	return m
}

func newEngine(ctx context.Context, cache string) *engine.Engine {
	b, _ := budget.New(8<<30, 0)
	e, err := engine.New(ctx, engine.Config{Budget: b, CacheDir: cache})
	if err != nil {
		panic(err)
	}
	return e
}

func prepare(ctx context.Context, l *runtimes.Loader, a artifact) *runtimes.Prepared {
	data, err := os.ReadFile(a.path)
	if err != nil {
		panic(err)
	}
	rt := runtimes.Wasm
	if a.kind == "js" {
		rt = runtimes.JS
	}
	p, err := l.Prepare(ctx, rt, data)
	if err != nil {
		panic(fmt.Sprintf("%s: %v", a.label, err))
	}
	return p
}

func instantiate(ctx context.Context, p *runtimes.Prepared) *engine.Instance {
	i, err := p.Module.Instantiate(ctx, p.InstanceConfig(engine.InstanceConfig{MemoryCapBytes: 512 << 20, Stderr: os.Stderr}))
	if err != nil {
		panic(err)
	}
	return i
}

func callOnce(ctx context.Context, i *engine.Instance, in input, check bool) {
	out, err := i.Handle(ctx, in.frame, host{})
	if err != nil {
		panic(err)
	}
	if check {
		_, body, err := abi.DecodeFrame(out)
		if err != nil || !bytes.Equal(body, in.expect) {
			panic(fmt.Sprintf("output mismatch: err=%v got %.120q want %.120q", err, body, in.expect))
		}
	}
}

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

func main() {
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour) // a Done channel, as the runner's per-call deadline gives
	defer cancel()
	if *bgctx {
		ctx = context.Background()
	}
	var list []artifact
	for _, it := range strings.Split(*arts, ",") {
		p := strings.SplitN(it, "=", 3)
		list = append(list, artifact{p[0], p[1], p[2]})
	}
	inputs := loadInputs()
	var ws []int
	for _, s := range strings.Split(*only, ",") {
		n, _ := strconv.Atoi(s)
		ws = append(ws, n)
	}
	switch *mode {
	case "lat":
		for _, a := range list {
			lat(ctx, a, inputs, ws)
		}
	case "cold":
		for _, a := range list {
			cold(ctx, a, inputs)
		}
	case "smoke":
		smoke(ctx, list[0])
	case "mem":
		for _, a := range list {
			memory(ctx, a, inputs)
		}
	}
}

func lat(ctx context.Context, a artifact, inputs map[int]input, ws []int) {
	e := newEngine(ctx, "")
	defer e.Close(ctx)
	l := runtimes.NewLoader(e)
	p := prepare(ctx, l, a)
	for _, w := range ws {
		in := inputs[w]
		i := instantiate(ctx, p)
		callOnce(ctx, i, in, true) // correctness: byte-identical to the reference
		t0 := time.Now()
		for time.Since(t0) < *warm {
			callOnce(ctx, i, in, false)
		}
		var p50s, p99s, means []float64
		var total int
		for r := 0; r < *runs; r++ {
			var ds []time.Duration
			t0 := time.Now()
			for time.Since(t0) < *dur {
				s := time.Now()
				callOnce(ctx, i, in, false)
				ds = append(ds, time.Since(s))
			}
			var sum time.Duration
			for _, d := range ds {
				sum += d
			}
			p50s = append(p50s, us(pct(ds, .5)))
			p99s = append(p99s, us(pct(ds, .99)))
			means = append(means, us(sum)/float64(len(ds)))
			total += len(ds)
		}
		_ = i.Close(ctx)
		fmt.Printf("RESULT rt=%s w=%d metric=lat p50_us=%.2f p50_min=%.2f p50_max=%.2f p99_us=%.2f p99_min=%.2f p99_max=%.2f mean_us=%.2f calls=%d runs=%d\n",
			a.label, w, median(p50s), slices.Min(p50s), slices.Max(p50s), median(p99s), slices.Min(p99s), slices.Max(p99s), median(means), total, *runs)
		for _, cs := range strings.Split(*concs, ",") {
			c, _ := strconv.Atoi(cs)
			var rates []float64
			for r := 0; r < 3; r++ {
				rates = append(rates, throughput(ctx, p, in, c))
			}
			fmt.Printf("RESULT rt=%s w=%d metric=tput c=%d rps=%.0f rps_min=%.0f rps_max=%.0f\n", a.label, w, c, median(rates), slices.Min(rates), slices.Max(rates))
		}
	}
}

func throughput(ctx context.Context, p *runtimes.Prepared, in input, c int) float64 {
	insts := make([]*engine.Instance, c)
	for k := range insts {
		insts[k] = instantiate(ctx, p)
		for t0 := time.Now(); time.Since(t0) < 300*time.Millisecond; {
			callOnce(ctx, insts[k], in, false)
		}
	}
	var n atomic.Int64
	var stop atomic.Bool
	var wg sync.WaitGroup
	start := time.Now()
	for k := 0; k < c; k++ {
		wg.Add(1)
		go func(i *engine.Instance) {
			defer wg.Done()
			var local int64
			for !stop.Load() {
				callOnce(ctx, i, in, false)
				local++
			}
			n.Add(local)
		}(insts[k])
	}
	time.Sleep(*tdur)
	stop.Store(true)
	wg.Wait()
	el := time.Since(start).Seconds()
	for _, i := range insts {
		_ = i.Close(ctx)
	}
	return float64(n.Load()) / el
}

func cold(ctx context.Context, a artifact, inputs map[int]input) {
	const reps = 7
	var coldC, cachedC, inst, first, loadTotal []float64
	wasmBytes, _ := os.ReadFile(a.path)
	// shared JS engine compile is reported separately for js artifacts
	for r := 0; r < reps; r++ {
		e := newEngine(ctx, "")
		l := runtimes.NewLoader(e)
		s := time.Now()
		p := prepare(ctx, l, a)
		coldC = append(coldC, us(time.Since(s))/1e3)
		s = time.Now()
		i := instantiate(ctx, p)
		inst = append(inst, us(time.Since(s))/1e3)
		s = time.Now()
		callOnce(ctx, i, inputs[1], false)
		first = append(first, us(time.Since(s))/1e3)
		loadTotal = append(loadTotal, coldC[r]+inst[r]+first[r])
		_ = i.Close(ctx)
		_ = e.Close(ctx)
	}
	dir, _ := os.MkdirTemp("", "wzcache")
	defer os.RemoveAll(dir)
	e := newEngine(ctx, dir)
	prepare(ctx, runtimes.NewLoader(e), a)
	_ = e.Close(ctx)
	var cachedInst, cachedTotal []float64
	for r := 0; r < reps; r++ {
		e := newEngine(ctx, dir)
		l := runtimes.NewLoader(e)
		s := time.Now()
		p := prepare(ctx, l, a)
		c := us(time.Since(s)) / 1e3
		cachedC = append(cachedC, c)
		s = time.Now()
		i := instantiate(ctx, p)
		ci := us(time.Since(s)) / 1e3
		cachedInst = append(cachedInst, ci)
		s = time.Now()
		callOnce(ctx, i, inputs[1], false)
		cachedTotal = append(cachedTotal, c+ci+us(time.Since(s))/1e3)
		_ = i.Close(ctx)
		_ = e.Close(ctx)
	}
	// js: the per-function cost once the shared engine is compiled
	var perFn []float64
	if a.kind == "js" {
		e := newEngine(ctx, "")
		l := runtimes.NewLoader(e)
		p := prepare(ctx, l, a)
		for r := 0; r < 50; r++ {
			s := time.Now()
			i := instantiate(ctx, p)
			callOnce(ctx, i, inputs[1], false)
			perFn = append(perFn, us(time.Since(s))/1e3)
			_ = i.Close(ctx)
		}
		_ = e.Close(ctx)
	}
	pf := 0.0
	if len(perFn) > 0 {
		pf = median(perFn)
	}
	fmt.Printf("RESULT rt=%s metric=cold size_bytes=%d compile_cold_ms=%.1f compile_cached_ms=%.1f inst_ms=%.2f inst_cached_ms=%.2f first_call_ms=%.2f total_cold_ms=%.1f total_cached_ms=%.1f js_per_fn_after_engine_ms=%.2f (medians of %d)\n",
		a.label, len(wasmBytes), median(coldC), median(cachedC), median(inst), median(cachedInst), median(first), median(loadTotal), median(cachedTotal), pf, reps)
}

func rssMB() float64 {
	runtime.GC()
	debug.FreeOSMemory()
	time.Sleep(200 * time.Millisecond)
	out, _ := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(os.Getpid())).Output()
	k, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return float64(k) / 1024
}

// memory: RSS after the engine, after loading the function (compile + first
// instance warm), and per additional warm instance (n=40, each having run W2
// and W3 so allocator state is realistic).
func memory(ctx context.Context, a artifact, inputs map[int]input) {
	base := rssMB()
	e := newEngine(ctx, "")
	l := runtimes.NewLoader(e)
	afterEngine := rssMB()
	p := prepare(ctx, l, a)
	afterCompile := rssMB()
	i0 := instantiate(ctx, p)
	callOnce(ctx, i0, inputs[1], false)
	callOnce(ctx, i0, inputs[2], false)
	callOnce(ctx, i0, inputs[3], false)
	afterOne := rssMB()
	const n = 40
	insts := []*engine.Instance{i0}
	for k := 1; k < n; k++ {
		i := instantiate(ctx, p)
		callOnce(ctx, i, inputs[1], false)
		callOnce(ctx, i, inputs[2], false)
		callOnce(ctx, i, inputs[3], false)
		insts = append(insts, i)
	}
	afterN := rssMB()
	fmt.Printf("RESULT rt=%s metric=mem process_base_mb=%.1f engine_mb=%.1f compiled_function_mb=%.1f first_instance_warm_mb=%.1f per_extra_warm_instance_mb=%.2f charged_min_mb=%.2f\n",
		a.label, base, afterEngine-base, afterCompile-afterEngine, afterOne-afterCompile, (afterN-afterOne)/float64(n-1), float64(insts[0].MemoryBytes())/(1<<20))
}

type logHost struct{}

func (logHost) Call(_ context.Context, op abi.Op, meta, body []byte) ([]byte, []byte, *abi.Error) {
	fmt.Printf("host op=%d meta=%s body=%dB\n", op, meta, len(body))
	return nil, nil, nil
}

// smoke runs a REAL function bundle's describe and one request through the
// runner's engine, printing what the guest sent to the host and what it
// answered. -body file -path /deliver -method POST -caller webhook.
func smoke(ctx context.Context, a artifact) {
	e := newEngine(ctx, "")
	defer e.Close(ctx)
	p := prepare(ctx, runtimes.NewLoader(e), a)
	i := instantiate(ctx, p)
	d, err := i.Describe(ctx)
	fmt.Printf("describe err=%v\n%s\n", err, d)
	var body []byte
	if *smokeBody != "" {
		body, _ = os.ReadFile(*smokeBody)
	}
	req := fmt.Sprintf(`{"id":"inv_1","address":"x.y","version":1,"method":%q,"path":%q,"rawQuery":"","headers":{"content-type":["application/xml"]},"route":%q,"pathParams":{},"caller":{"kind":%q},"deadlineUnixMs":%d}`,
		*smokeMethod, *smokePath, *smokeMethod+" "+*smokePath, *smokeCaller, time.Now().Add(time.Hour).UnixMilli())
	out, err := i.Handle(ctx, abi.EncodeFrame([]byte(req), body), logHost{})
	m, b, _ := abi.DecodeFrame(out)
	fmt.Printf("handle err=%v\nmeta=%s\nbody=%s\n", err, m, b)
}
