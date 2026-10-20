# Function runtime performance: what QuickJS-in-wasm costs (2026-10-19)

*Measured 2026-10-19/20 on one dev machine. Benchmark code: `bench/function-runtimes/` (run `bench/function-runtimes/run.sh`; raw
outputs in `bench/function-runtimes/results/`). Every number below is tagged **[M]** (measured by me for this document) or **[Q]**
(quoted from an existing doc; the doc is named). Nothing here is estimated unless it says "projected", and projections say how.*

## 0. Answer in ten lines

1. For plain request handling, JSON shaping and host calls, QuickJS-in-wazero is **fine**: 10 µs per call for a trivial function, 0.34 ms
   for a 2 KB JSON map, 0.1 ms for 20 host calls **[M]**. Next to a 2-10 ms DB query or a 20-100 ms HTTP fetch that is noise.
2. For **CPU-heavy transforms it is 100x too slow**: the 15 KB XML parse + map costs **67 ms** on QuickJS-in-wazero versus **0.54 ms on V8**
   (124x) **[M]**. That is the same size as the I/O it is wrapped around, and it caps a 4-core runner at about 56 such calls/s.
3. The cost has **two layers**. (a) Interpreter vs JIT: QuickJS and goja (a pure-Go interpreter) are both ~15-18 ms on the XML case, V8 is 0.5 ms.
   (b) On top of that, the runner's **deadline mechanism** (`wazero.WithCloseOnContextDone(true)`, `internal/functions/engine/engine.go`) multiplies
   guest CPU time by **3.7-4.8x for QuickJS, 8-10x for compiled Rust and 4-8x for Go** guests **[M]**. Layer (b) is the cheapest thing to attack.
4. Compiling QuickJS with `-O3` instead of `-Os` changes nothing material (<= 20% on one workload) **[M]**; it is not a lever.
5. A pure-Go interpreter (goja) runs the same unmodified bundles 4-11x faster than QuickJS-in-wazero **[M]** but has no memory cap, which conflicts
   with the runner's "memory is a budget, never an OOM" principle.
6. V8 via cgo (`tommie/v8go`) is fast single-threaded (28x on JSON, 180x on XML) **[M]** but needs clang >= 21, adds ~76 MB to a binary and 112-230 MB of
   per-platform V8 libraries to the module graph, and misbehaved under concurrency in my harness. Not recommended.
7. A separate Rust V8 runner serving `runtime=js` is **technically clean**: the same TypeScript bundles (two real BEVCO bundles built with the real
   `@flowcatalyst/fn`) run unchanged on V8 with identical describe output, host-call sequence and response **[M]**. It is a new component, not a flag
   (section 6.4 lists what it must implement).
8. Compiled guests (Rust on wazero 5-77x, on wasmtime 28-707x faster than QuickJS) are the right home for hot transforms, but standard Go-to-wasm is
   **not** faster than QuickJS for JSON (0.7x) **[M]**.
9. **Recommendation:** stay on QuickJS-in-wazero now; spend the first effort on layer (b) (deadline mechanism for the JS runtime) and measure on Linux;
   write down a tripwire (section 7) that triggers the Rust V8 runner; steer XML/heavy-transform functions to Rust until then.
10. Biggest caveat: the machine was **not idle** (IDE indexing, load average 20-290 during the runs). Ratios are trustworthy to roughly +-1.5x,
    absolute microseconds to +-2x (section 2.3). No Linux or cloud-vCPU numbers.

## 1. What was measured

### 1.1 Machine and versions

Apple M4 Pro, 14 cores (10 performance + 4 efficiency), 48 GB, macOS 15.6.1 (Darwin 24.6.0), arm64. Go 1.27.2, rustc 1.98.1, Node 24.22.0,
TinyGo 0.43.0 (LLVM 22.1.4), wazero v1.12.0 (via the platform's own `internal/functions/engine`), wasmtime 49.0.1, deno_core 0.412.0 (V8 150.4),
goja `v0.0.0-20261019185110-cf7b1419c9c0`, `tommie/v8go` v0.39.0 (V8 15.5.35.25), fast-xml-parser 5.11.2, QuickJS through rquickjs 0.11 as in
`clients/fn-js-engine`. A cloud vCPU is slower than an M4 P-core; expect absolute numbers on the runner task to be worse (I did not measure Linux).

### 1.2 Runtimes under test

| Label | What it is | Where it runs |
|---|---|---|
| **qjs** | The bundle on QuickJS, shared engine `internal/functions/jsengine/engine.wasm`, via the real `runtimes.Loader` + `engine.Instance.Handle` | wazero, the runner's exact engine config (deadline hook on, mmap allocator, budget) |
| **goja** | Same bundle on goja, pure-Go ES interpreter | in-process Go |
| **v8go** | Same bundle on V8 embedded in Go through cgo (`tommie/v8go`) | in-process Go + cgo. Marshalling simplified (section 1.4) |
| **v8** | Same bundle on V8 via `deno_core` 0.412 (the Rust spike's pinned version), one warm isolate per instance, async handler, microtasks drained by the host | Rust, `bench/function-runtimes/rusthost` |
| **rust** | Hand-written Rust, ABI v1, `wasm32-unknown-unknown`, `opt-level=s` (the SDK examples' setting), serde_json + roxmltree | wazero, same engine as qjs |
| **rust-o3** | Same with `opt-level=3` | wazero |
| **wasmtime** | The `rust` wasm module on wasmtime 49 (Cranelift), epoch interruption on, 1 ms tick | Rust host |
| **go** | Standard Go 1.27 `GOOS=wasip1 -buildmode=c-shared` (what `fcdev fn build` does), encoding/json + encoding/xml | wazero |
| **tinygo** | TinyGo 0.43 `-target=wasip1 -buildmode=c-shared -opt=2 -scheduler=none` (see 1.5) | wazero |

All guests speak the platform's ABI v1 (`fc_alloc` / `fc_handle` / `fc.call`/`fc.take`) **raw, without the guest SDKs**. That isolates the engine
from the SDK router. The real `@flowcatalyst/fn` adds request-meta parsing, routing and response encoding on top: **[Q]** `docs/function-runner-plan.md`
§2.1/§2.4 put a JS handler that parses a request and builds a response at 72 / 245 µs p50 / p99 (13.9 µs for trivial echo logic), so a real JS function has a
per-call floor of roughly 70-100 µs on QuickJS, not the 9.7 µs of my W1.

### 1.3 Workloads (identical bytes out of every runtime)

Request meta is `{"w":N}`; each runtime's first call per workload is compared **byte for byte** to a reference output produced under Node
(`js/check-node.mjs`); a mismatch aborts the run. So the functions are provably equivalent.

| | Workload | Input | Output |
|---|---|---|---|
| **W1** | echo: pure call overhead | 2 KB JSON body | same bytes |
| **W2** | JSON transform: parse, map 22 order lines (qty x unit price), sum, serialize | 2.0 KB JSON | 1.0 KB JSON |
| **W3** | XML parse + map to JSON (`YardTrip` with namespace, attributes, 90 `Line` elements), the `yard-trip-processor/src/mapping.ts` mapping shape | **15.4 KB** XML | 10 KB JSON |
| **W4** | CPU loop: 5,000,000 xorshift32 iterations with a wrapping sum | none | decimal string |
| **W5** | host-call heavy: 10 x `log` (op 1) + 10 x `config.get` (op 2) per invocation, results consumed | none | decimal string |

W3 input caveat **[honest]**: the real 15 KB BluJay LeanXML sample is **not in either repo**. The only real fixture
(`bevco/functions/yard-trip-ingest/tests/fixtures/yard-trip.xml`) is 1.0 KB. I generated a 15.4 KB message with the same schema by repeating its `Line`
element (`fixtures/gen.mjs`). Parser cost scales with element count, so the shape is representative but a real message with a different mix will differ.
JS parses with fast-xml-parser (config as in `mapping.ts`), Rust with roxmltree, Go with `encoding/xml`.

W4 is run as written but QuickJS needs 5.3 s per call, so its cells have 3-5 samples. W1 is not "pure engine overhead": it includes `fc_alloc`, copying
2 KB in and out, and the Go-side frame handling.

### 1.4 Method

- Single-thread latency: 0.5 s warm-up, then **5 runs of 1.5 s**, every call individually timed. Reported: **median over runs** of the per-run p50 and p99,
  with the min-max of the per-run p50 as the spread. W3/W4 cells for the slow runtimes have few samples per run (stated by `calls=` in the raw files).
- Throughput: `c` goroutines/threads each with its own instance, 0.3 s warm-up, 3 s measured, **3 repeats, median**, c = 1, 4, 14 (14 = all cores).
- Cold start: 7-15 repetitions, median. "Cached" = wazero disk compilation cache / wasmtime `Module::serialize` + `deserialize` / V8 startup snapshot holding the
  evaluated bundle.
- Memory: process RSS after GC + `FreeOSMemory` (Go) or `proc_pid_rusage` (Rust): deltas for compile, first warm instance, and the mean of 39-40 further
  warm instances that each ran W1-W3. Compile-time transients inflate "compiled function" deltas (section 3.3).
- **v8go simplification:** v8go has no typed-array constructor, so the body crosses as a JS string and `__fc_call` returns a synthesised result. W2-W4 compute
  is identical; W1 and W5 are slightly optimistic. It also must `Release()` every `Value` or the V8 heap fills and aborts (observed).
- Equivalence proofs that are not performance: section 6.4.

### 1.5 Things that went wrong on the way (so you can judge the numbers)

- QuickJS `__fc_call` requires exactly 3 arguments; my first W5 omitted the third (`undefined`) and got `FUNCTION_FAILED`. The SDK always passes it.
- TinyGo's default scheduler (asyncify) made every export call cost 2.7 ms (W1 2.7 ms vs 5.6 µs after `-scheduler=none`). TinyGo is documented as unsupported
  for FlowCatalyst functions (`docs/function-examples-go-rust.md`); I include it as a data point, tuned.
- V8 under deno_core needs a tokio runtime context on the creating thread or V8 aborts with a "delayed task" message.
- v8go does not build with Apple clang 17 (libc++ `#pragma clang attribute` errors); it built with Homebrew clang 23.1.3 and `CGO_CXXFLAGS=-nostdinc++`
  (its README asks for clang >= 21).

## 2. Results

### 2.1 Single-thread latency, p50 / p99 **[M]**

Spread = min-max of the per-run p50 over 5 runs. `n/m` = not measured.

| Runtime | W1 echo | W2 JSON 2 KB | W3 XML 15 KB | W4 5M-iter loop | W5 20 host calls |
|---|---|---|---|---|---|
| **qjs** (current) | 9.7 / 16.5 µs (9.4-9.8) | 338 / 399 µs (336-339) | **66.7 / 67.6 ms** (66.6-66.9) | **5.33 s** (5.27-5.46) | 98 / 121 µs (98-99) |
| goja | 1.2 / 4.3 µs (1.0-1.4) | 29.9 / 146 µs (28-35) | 15.6 / 17.6 ms (15.2-18.3) | 1.35 s (1.35-2.13) | 14.3 / 92 µs (14.0-17.8) |
| v8go (cgo) | 5.3 / 9.8 µs (5.2-5.5) | 12.2 / 57 µs (11-22) | 372 / 683 µs (369-376) | 23.9 ms (23.1-24.1) | 29.5 / 38.7 µs (29-30) |
| **v8** (deno_core) | 0.38 / 1.3 µs | 4.4 / 7.1 µs (4.3-6.2) | 539 / 754 µs (395-752) | 6.9 ms (6.8-38.7) | 7.8 / 11.7 µs |
| rust on wazero (`s`) | 0.9 / 3.8 µs (0.8-1.0) | 63.7 / 82 µs (62.6-64.5) | 1.51 / 1.69 ms (1.50-1.52) | 69.2 ms (69.0-70.2) | 8.0 / 16.1 µs |
| rust on wazero (`o3`) | 0.9 / 3.8 µs | 66.4 / 146 µs | 1.21 / 11.2 ms | 35.9 ms (35.5-51.5) | 6.8 / 11.8 µs |
| **rust on wasmtime** | 0.33 / 0.4 µs | 6.5 / 7.9 µs | 316 / 6100 µs | 7.5 ms (7.4-16.9) | 3.5 / 4.5 µs |
| go (std) on wazero | 6.7 / 28.5 µs | 484 / 684 µs | 14.2 / 21.0 ms | 70.0 ms | 18.5 / 42 µs |
| tinygo on wazero | 5.6 / 103 µs | 283 / 1013 µs | 4.14 / 5.9 ms | 40.1 ms | 10.8 / 119 µs |

### 2.2 How much faster than QuickJS-in-wazero, and the microseconds saved per call **[M]**

Factor = qjs p50 / candidate p50. Microseconds in brackets = qjs p50 minus candidate p50 (what one call gains).

| Runtime | W1 echo | W2 JSON | W3 XML 15 KB | W4 loop | W5 host calls |
|---|---|---|---|---|---|
| goja | 8.0x (8.5 µs) | 11.3x (308 µs) | 4.3x (51 ms) | 3.9x (3.98 s) | 6.8x (84 µs) |
| v8go | 1.8x (4.4 µs) | 27.6x (326 µs) | 180x (66 ms) | 223x (5.31 s) | 3.3x (68 µs) |
| **v8 (deno_core)** | 25x (9.3 µs) | 77x (333 µs) | **124x (66.2 ms)** | 771x (5.32 s) | 12.7x (90 µs) |
| rust on wazero (`s`) | 11x (8.8 µs) | 5.3x (274 µs) | 44x (65.2 ms) | 77x (5.26 s) | 12.3x (90 µs) |
| rust on wazero (`o3`) | 11x | 5.1x | 55x | 149x | 14.4x |
| rust on wasmtime | 29x (9.3 µs) | 52x (331 µs) | 211x (66.4 ms) | 707x (5.32 s) | 27.7x (94.5 µs) |
| go (std) on wazero | 1.4x (3.0 µs) | **0.7x (slower by 146 µs)** | 4.7x (52 ms) | 76x | 5.3x (80 µs) |
| tinygo on wazero | 1.7x (4.1 µs) | 1.2x (55 µs) | 16x (62.6 ms) | 133x | 9.1x (87 µs) |

Reading it:

- **W1/W5 (call overhead, ABI boundary):** QuickJS costs ~10 µs per empty call and ~4.5 µs per host call (98 µs / 20, including building the `{code, meta, body}` result
  object each time). Rust on wazero pays ~0.4 µs per host call, wasmtime ~0.17 µs. A JS function that makes 20 host calls spends 0.1 ms in the boundary; that is real but
  small.
- **W2:** V8's JIT does the 2 KB JSON round trip in 4.4 µs; goja in 30 µs; QuickJS-in-wazero in 338 µs. Rust on wazero is **only 5x** faster than QuickJS here because the
  deadline hook (section 2.4) taxes it too; with the hook off Rust on wazero does W2 in 7.4 µs (pass 2, section 2.4), level with wasmtime.
- **W3:** QuickJS 67 ms, goja 15.6 ms, V8 0.54 ms. The two interpreters are in the same class once QuickJS's wazero tax is removed (section 2.4: 18 ms); V8's JIT is
  another 30x beyond that. Compiled Rust is 0.3-1.5 ms.
- **W4:** a pure arithmetic loop: V8 and wasmtime (JIT/AOT native code) 7 ms, QuickJS 5.3 s. This is the interpretive overhead plus the hook in its purest form; real functions
  are not this loopy, W3 is the realistic version.
- **Standard Go to wasm is not a performance answer for JSON/XML:** reflection-driven `encoding/json` + `encoding/xml` compiled to wasm are slower than QuickJS on W2 (0.7x)
  and only 4.7x faster on W3. TinyGo is better (16x on W3) but is not supported by the SDK.

### 2.3 Throughput, calls/s at concurrency 1 / 4 / 14 **[M]**, median of 3 x 3 s

| Runtime | W1 echo | W2 JSON | W3 XML | W4 loop | W5 host calls |
|---|---|---|---|---|---|
| **qjs** | 105k / 360k / 688k | 2.9k / 11.1k / 29.8k | **15 / 56 / 140** | 0.19 / 0.8 / 2 | 10.0k / 32.9k / 100k |
| goja | 603k / 1.03M / 970k | 27.8k / 47.9k / 55.1k | 62 / 96 / 127 | 0.7 / 2 / 1 | 50.5k / 92k / 57.9k |
| v8go | 120k / **43k / 13k** (c>1 degraded) | 70k / **46k** / crashed (V8 OOM) | 2.6k / n/m | 47 / n/m | 32k / n/m |
| **v8** | 2.35M / 4.2M / 4.7M | 164k / 560k / 887k | 2.6k / 5.0k / 7.6k | 143 / 513 / 1.5k | 124k / 225k / 183k |
| rust on wazero (`s`) | 759k / 1.07M / 1.12M | 15.6k / 52k / 139k | 661 / 2.6k / 6.5k | 14 / 57 / 103 | 118k / 398k / 720k |
| rust on wasmtime | 2.4M / 9.2M / 17.0M | 154k / 548k / 309k | 1.9k / 7.1k / 15.0k | 132 / 463 / 1.1k | 284k / 795k / 578k |
| go (std) on wazero | 87k / 55k / 430k | 2.0k / 8.0k / 17.6k | 66 / 259 / 717 | 14 / 56 / 87 | 47k / 184k / 231k |
| tinygo on wazero | 57k / 149k / 165k | 3.4k / 11.5k / 10.9k | 177 / 321 / 1.1k | 27 / 75 / 126 | 71k / 181k / 354k |

**Trust c = 1 most, c = 4 next, c = 14 least.** The 14 busy-core cells ran next to IDE indexers using 3-4 cores, so scaling above c = 4 is unreliable (e.g. wasmtime W2 and
Rust-`o3` fall between c = 4 and c = 14). QuickJS scales well to 4 (3.7x for W3, W5) which is the case that matters for a 2-4 vCPU task.

**Run-to-run noise.** A second pass (`results/pass2-under-heavy-load.txt`) ran at load average 30 rising to 160 and was 1.0-2.3x slower on short-call cells (qjs W2 338 to 445 µs, Rust
on wazero W2 64 to 135 µs, wasmtime W2 6.5 to 14.8 µs) while long-call cells barely moved (qjs W3 66.7 vs 69.2 ms, wasmtime W3 316 vs 316 µs). The conclusions are ratios of
10-700x, which this noise cannot change; any ratio below ~2x (e.g. goja's 4.3x on W3 is solid, Rust-`s` vs `o3` is not) should be read as "same class".

### 2.4 The deadline mechanism is a large share of the cost **[M]**

`internal/functions/engine/engine.go` builds the one wazero runtime with `WithCloseOnContextDone(true)` so a guest in an infinite loop stops at its deadline
(**[Q]** plan §2.1: 200.2 ms for a 200 ms deadline). wazero implements that with a check at every loop header and function entry in compiled code. I measured the same
guests, single thread, with that setting on and off (`wazerobench/loopcfg`, pass 2; the on-values agree with the main run, so the load noise is small here):

| Guest on wazero | W1 on / off | W2 on / off | W3 on / off | W4 on / off |
|---|---|---|---|---|
| qjs (engine `-Os`, as shipped) | 9.5 / 3.1 µs (3.1x) | 332 / 86 µs (3.9x) | 66.8 / 18.1 ms (3.7x) | 5.76 / 1.20 s (4.8x) |
| qjs (engine `-O3` + `wasm-opt -O3`) | 9.1 / 2.9 µs | 272 / 75 µs | 66.7 / 17.9 ms | 5.31 / 1.06 s |
| rust `s` | 0.7 / 0.4 µs | 75 / 7.4 µs (10x) | 1.50 / 0.16 ms (9.2x) | 67.6 / 7.2 ms (9.4x) |
| rust `o3` | 0.6 / 0.4 µs | 62 / 6.8 µs (9x) | 1.21 / 0.15 ms (8x) | 35.0 / 7.7 ms (4.5x) |
| go (std) | 4.9 / 2.8 µs | 614 / 134 µs (4.6x) | 20.8 / 4.9 ms (4.2x) | 70.6 / 8.9 ms (7.9x) |
| tinygo | 3.5 / 1.0 µs | 480 / 90 µs (5.3x) | 5.8 / 3.2 ms (1.8x) | 44.5 / 13.3 ms (3.3x) |

Consequences:

- The platform's guests pay **3.7-4.8x (QuickJS), 4-8x (Go) and 8-10x (Rust)** on compute for being preemptible. wasmtime's epoch interruption (used in the Rust repo) costs far less:
  Rust W4 7.5 ms on wasmtime with epochs vs 69 ms on wazero with the hook.
- With the hook off, QuickJS-in-wazero (W3 18 ms, W2 86 µs) lands level with goja (15.6 ms, 30 µs): the wasm-hosted QuickJS interpreter is not intrinsically worse than a good
  interpreter; the hook is most of the gap.
- **Rebuilding the engine at a higher optimisation level changes nothing material** (rows 1-2): I rebuilt `clients/fn-js-engine` from source with wasi-sdk 25 (outside the
  repo; `internal/functions/jsengine/engine.wasm` untouched). The `-Os` rebuild reproduced the shipped engine's timings.
- **Not measured, and why it matters:** whether the hook costs the same on amd64 Linux (production), and whether a JS runtime without the hook is safe. Removing it
  means QuickJS's own interrupt handler (cooperative, called from the bytecode loop) must enforce the deadline; a runaway in native QuickJS code (regexp, a giant `JSON.parse`) is not
  covered, and a stuck wasm call cannot be killed from Go. The upper bound in the table is what is on offer; the cost is that safety net.

### 2.5 Cold start **[M]**, medians

| Runtime | Artifact | Compile / load, cold | Compile / load, cached | Instance (+ script load) | First call | Whole function, cold -> cached |
|---|---|---|---|---|---|---|
| **qjs** | 154 KB bundle + shared 708 KB engine | engine **163 ms once per runner** | engine 6.0 ms (wazero disk cache) | **42 ms per instance** (parse + run the 154 KB bundle) | 0.07 ms | 204 ms -> 48 ms (first function); 42 ms per additional instance |
| goja | 154 KB bundle | n/a | n/a | 8.1 ms (new runtime + eval) | ~0.01 ms | 8.1 ms |
| v8go | 154 KB bundle | n/a | n/a | 1.6 ms (isolate + eval + first call) | included | 1.6 ms |
| **v8** (deno_core) | 154 KB bundle | n/a | startup snapshot with the bundle evaluated: 15 ms to build, 1.1 MB | 4.7 ms create + eval; **0.77 ms from the snapshot** | 0.02 ms | 4.7 ms -> 0.8 ms |
| rust on wazero (`s`) | 137 KB | 26.8 ms | 1.1 ms | 0.02 ms | 0.06 ms | 26.9 -> 1.2 ms |
| rust on wasmtime | 137 KB | 12.8 ms | 0.07 ms (`deserialize`, 407 KB `.cwasm`) | 0.03 ms | 0.03 ms | 12.8 -> 0.1 ms |
| go (std) | 4.64 MB | 826 ms | 27.8 ms | 3.2 ms | 0.03 ms | 829 -> 31 ms |
| tinygo | 759 KB | 154 ms | 5.3 ms | 0.11 ms | 0.01 ms | 154 -> 5.4 ms |

The JS instance cost is dominated by the bundle: 42 ms for 154 KB (fast-xml-parser inside). A tiny bundle costs ~2.5 ms **[Q]** plan §2.1. That per-instance cost is what a cold
pool pays when it scales out, and it is CPU, not I/O.

### 2.6 Memory **[M]**

Deltas of process RSS. "Compiled function" includes wazero/Cranelift compile-time transients that the allocator keeps; steady compiled size is smaller (**[Q]** plan §2.1:
Rust 2.75 MB resident per loaded function including one instance, JS shared engine 0.95 MB per function, 708 KB engine).

| Runtime | Shared / compiled, once | First warm instance | **Each further warm instance** | Notes |
|---|---|---|---|---|
| qjs | 33 MB (engine, shared by all JS functions) | +1.2 MB | **+1.14 MB** | declared min memory 2.1 MB, ~1.1 MB touched; plus the 154 KB source |
| goja | n/m | n/m | n/m | not measured |
| v8 | 15.3 MB for the first isolate incl. V8 platform | | **+6.5 MB** per isolate (150 KB bundle, W1-W3 run) | **[Q]** Rust spike: 1.65 MB per isolate for a trivial handler with a snapshot (`docs/function-runner-density.md` §2.4) |
| rust on wazero | 9.3 MB | +0.3 MB | +0.21 MB | min memory 1.25 MB |
| rust on wasmtime | 38.7 MB (Cranelift transient; 407 KB serialised) | +0.3 MB | +0.20 MB | |
| go (std) | 75 MB | +3.8 MB | +3.85 MB | 5 MB min memory |
| tinygo | 30 MB | +0.5 MB | +0.41 MB | |

At 1000 JS functions with one warm instance each: QuickJS ~1.1 GB (+ ~150 MB of sources), V8 with this bundle ~6.5 GB, V8 with a trivial handler **[Q]** ~1.6 GB. V8 is not the
density winner for fat bundles.

## 3. Does it matter? Capacity on a 2-4 vCPU runner task

### 3.1 Cost per call and where the time goes

CPU time per call on QuickJS-in-wazero **[M]**: W1 10 µs, W2 0.34 ms, W5 0.10 ms, **W3 67 ms**. Typical I/O around a function (the task's figures, not measured by me):
DB query 2-10 ms, HTTP fetch 20-100 ms (a platform dispatch call: no figure given, I did not measure it).

| Function does | JS CPU (qjs) | Plus I/O | JS share of latency | JS share on V8 |
|---|---|---|---|---|
| route + small JSON map (W2) + DB query 5 ms | 0.34 ms | 5 ms | 6% | 0.1% |
| route + small JSON map (W2) + HTTP 50 ms | 0.34 ms | 50 ms | 0.7% | 0.01% |
| 20 host calls (W5) | 0.10 ms | (the host calls are the I/O) | n/a | |
| **15 KB XML map (W3) + HTTP 50 ms** | **67 ms** | 50 ms | **57%** | 1% |
| **15 KB XML map (W3) + DB 5 ms** | **67 ms** | 5 ms | **93%** | 10% |

So for latency the JS engine only matters for XML-class transforms. The throughput question is separate: CPU-bound work consumes cores that I/O waits do not.

### 3.2 Cores needed = request rate x CPU per call

Fleet scenarios (total rate R = functions x requests/s per function). All CPU figures are single-thread p50 from section 2.1, M4 Pro; **a cloud vCPU may be 1.5-2.5x slower, so multiply
the qjs rows accordingly** (not measured). 4 vCPU = the budget.

| Scenario | R /s | W2 JSON on **qjs** | W2 on v8 | W3 XML on **qjs** | W3 on v8 | W5 host-call on qjs |
|---|---|---|---|---|---|---|
| 100 functions x 10/s | 1,000 | 0.34 cores | 0.004 | **67 cores** | 0.54 | 0.10 |
| 1000 functions x 10/s | 10,000 | **3.4 cores (at the limit)** | 0.04 | **670 cores** | 5.4 | 1.0 |
| 100 functions x 1000/s | 100,000 | **34 cores** | 0.44 | n/a | 54 | 9.8 |
| 1000 functions x 1000/s | 1,000,000 | n/a | 4.4 | n/a | n/a | 98 |

Measured ceilings on 4 cores **[M]** (c = 4 throughput): qjs W2 11k/s, W3 **56/s**, W5 33k/s; V8 W2 560k/s, W3 5.0k/s.

What that says:

- **100 functions at 10/s each (1k/s total) with JSON-shaped work:** QuickJS uses a third of a core. It does not matter. Same for host-call-heavy functions (0.1 core).
- **1000 functions at 10/s (10k/s total):** a JSON-transform fleet needs 3.4 cores of QuickJS. On one 4 vCPU task that is the entire budget before any I/O handling, TLS or the
  HTTP server itself, and on a slower cloud vCPU it exceeds it. This is where QuickJS becomes the bottleneck for JSON-class work. V8 would need 0.04 cores.
- **Any XML-class function:** QuickJS tops out at ~15 calls/s per core, ~56/s on four. A BEVCO-style message flow above ~50 messages/s (or a latency budget under ~70 ms for the mapping)
  is out of reach without changing something. Even 5 messages/s costs a third of a core and adds 67 ms to each.
- **High per-function rates (1000/s):** nobody should run these on QuickJS; at 100 functions x 1000/s even trivial-JSON work needs 34 cores.
- **I/O does not save QuickJS.** While a call waits on HTTP the instance is held (host calls are synchronous inside wazero), so concurrency in flight = rate x latency
  (10k/s x 50 ms = 500 instances, ~0.6 GB of QuickJS instances; each cold one costs 42 ms CPU to load). The CPU for the transform is spent regardless.

### 3.3 Cold pool fill

Restart or scale-out with 1000 JS functions, one instance each: QuickJS pays 1000 x 42 ms = 42 CPU-seconds (about 10 s on 4 cores) for this bundle size; V8 with a bundle snapshot 1000 x 0.8 ms
= 0.8 s (but the snapshot has caveats on macOS: **[Q]** `docs/function-runner-density.md` §10.1 records abort-under-churn with snapshots on macOS arm64, none on Linux). The runner can preload, so
this is a deploy-time cost, not a request-time one, unless the pool is evicting.

## 4. What the TypeScript shim needs from the engine, and where V8 differs

`@flowcatalyst/fn` uses only: core ECMAScript plus three host natives, `__fc_call(op, metaJSON, body)`, `__fc_utf8_encode`, `__fc_utf8_decode`
(`clients/fn-ts/src/native.ts`; contract in `clients/fn-js-engine/src/lib.rs`). It installs its own `console`. It has TextEncoder fallbacks for Node tests only.

### 4.1 Proof that bundles move unchanged

`wazerobench -mode smoke` (QuickJS, the runner's engine) and `v8bench smoke` (deno_core) run a **real production bundle**, describe plus one request, and print what the guest
asked the host for and what it answered. For the BEVCO `yard-trip-processor` (`POST /deliver`, webhook caller, the 15 KB message) and `yard-trip-ingest` (`GET /healthz`), the output
is **identical** on both engines: same describe JSON, same `log` host call and metadata, same `{"status":200,...}` meta, same `{"ack":true}` body (`results/smoke-yard-trip-processor-*.txt`).
Bundles come from the BEVCO `dist/function.js`, built with the real `fc-fn-build` settings (`esbuild --platform=neutral --format=iife`, with `mainFields: ["module","main"]` for fast-xml-parser).

### 4.2 Global and language-feature probe (`js/probe.js`, results in `results/probe-globals-*.txt`) **[M]**

Both engines lack, so no existing bundle can use them today: `fetch`, `Request`/`Response`/`Headers`, `URL`/`URLSearchParams`, `TextEncoder`/`TextDecoder`, `atob`/`btoa`, `crypto`,
`Buffer`, `process`, `structuredClone`, `AbortController`, `setTimeout`/`setInterval` (bare deno_core has no timers). `queueMicrotask` exists in both. Identical results: regexp (Unicode
properties, lookbehind, named groups), `Array.at`/`toSorted`/`Object.groupBy`/`Promise.withResolvers`, `replaceAll`, stable `sort`, `toFixed`/number printing, `Date` JSON, `normalize`.

Differences that **could change behaviour of an existing bundle** when moved from QuickJS to V8:

| Difference | QuickJS (current) | V8 (deno_core) | Effect |
|---|---|---|---|
| Time zone | always UTC (`getTimezoneOffset() = 0`) | process TZ (saw `+0100`) | local-time getters, `Date.toString()`, date strings without an offset parse differently. Pin `TZ=UTC` in the runner |
| `Intl`, `toLocale*` | no `Intl` (`ReferenceError`); `toLocaleString()` = `1234567.891` | full ICU: `1,234,567.891`, `Intl.NumberFormat("en-ZA",...)` = `R 1 234,50` | output text changes, no failure; code that feature-detects `Intl` changes branch |
| `localeCompare` | code-point-ish: `a,b,ä` | ICU: `a,ä,b` | sort order of non-ASCII strings |
| Error messages and `e.stack` | `cannot read property 'x' of null`; stack has no header line | `Cannot read properties of null (reading 'x')`; `TypeError: ...` header | matching on `e.message` breaks; log text differs |
| Stack overflow | runaway recursion **traps the wasm instance** (observed `out of bounds memory access`; call fails, instance replaced) | catchable `RangeError` | V8 is friendlier; error class changes |
| `performance` | present | **absent in bare deno_core** | a bundle calling `performance.now()` breaks unless the host provides it |
| `console` | none until the SDK installs one | built-in exists, SDK replaces it | none (the SDK overwrites `globalThis.console` on both) |
| `WebAssembly`, `Atomics`, `Temporal`, `Deno`, `SharedArrayBuffer` | absent (SharedArrayBuffer present) | present; `Deno.core` is visible to guest code | superset, but `Deno`/`WebAssembly` would have to be removed from the guest realm in a V8 runner (the Rust repo's JS runtime deliberately strips `Deno` and `WebAssembly`) |
| Memory limit | wasm memory cap, failed grow fails the call | V8 heap limit; near-limit the isolate is terminated | different failure shapes; both bounded |
| Async | host calls synchronous; promises settle via `execute_pending_job` | host drains microtasks after the call | none for current bundles (both handled `async` handlers identically) |

If a V8 runner also offers web APIs (the Rust repo's JS runtime does: `fetch`, `URL`, `TextEncoder`, `crypto.getRandomValues`, timers) bundles written for it would **not** run on QuickJS. Portability is
one-way, so keep the documented contract as the QuickJS subset.

## 5. Why the numbers look like this (and what I could not separate)

- Three tiers on the realistic workload W3: V8 JIT ~0.5 ms; interpreters (goja 15.6 ms, QuickJS-without-hook 18 ms) ~15-18 ms; QuickJS with the deadline hook 67 ms. The interpreter/JIT gap is
  inherent. The hook share is directly measured (section 2.4).
- I did not measure native (non-wasm) QuickJS, so I cannot say how much of QuickJS's 18 ms is wasm overhead versus QuickJS itself.
- Compiled languages: roxmltree on wasmtime does W3 in 0.3 ms, about 1.7x faster than V8-with-fast-xml-parser; the JIT is competitive with native for this shape of work.
- **[Q] cross-check:** the plan's own measurement of wazero vs wasmtime for JS echo (`docs/function-runner-plan.md` §2.4: wasmtime 7.8 µs, wazero 13.9 µs, "keep host crossings
  coarse") is consistent with W1/W5 here; it did not look at the deadline hook's effect on compute.

## 6. Options for the Go platform

### 6.1 (i) Stay on QuickJS-in-wazero

> **Update 2026-10-20: item 1 below is done** (`docs/function-runtime-deadlines.md`): JS runs on its own cooperative wazero runtime with a QuickJS interrupt handler. Measured 3.5x on realistic work and 5x on compute, macOS and Linux arm64; amd64 still unmeasured.

What it costs: everything in section 2. What is cheap to improve first:

1. **Deadline mechanism for the JS runtime (projected 3.7-4.8x, upper bound measured in 2.4).** Put JS on its own `wazero.Runtime` with `WithCloseOnContextDone(false)` and enforce
   the deadline inside QuickJS with `JS_SetInterruptHandler` (checks a clock or a flag through a host import; the rquickjs crate exposes `set_interrupt_handler`). Needs: confirmation that
   QuickJS regexp and big builtins poll the handler, a test that a runaway loop stops near its deadline, and a decision on the lost backstop (a stuck wasm call cannot be killed). Effort S-M
   plus verification on Linux/amd64.
2. **Bytecode instead of source at instance load (projected: removes most of the 42 ms per-instance load; not measured).** Compile the bundle to QuickJS bytecode at publish; `fc_js_load`
   evaluates bytecode. Matters for cold pool fill (section 3.3).
3. Not a lever: compiler optimisation level of the engine (measured, section 2.4).

Ceiling after 1 (using the hook-off numbers): W2 86 µs, W3 18 ms, W4 1.2 s; about 4x better and still 30x behind V8 on XML.

### 6.2 (ii) goja (pure Go interpreter) in the runner

**Measured** by me, same unmodified bundle, outputs byte-identical: 8x on echo, 11x on JSON, 4.3x on XML, 3.9x on the loop, 6.8x on host calls versus QuickJS-in-wazero (section 2.2). Cold 8 ms
per runtime. It ran an `async` bundle with fast-xml-parser first time. Cost: one dependency (`github.com/dop251/goja` + regexp2, x/text, sourcemap), no cgo, the whole bench binary is 15 MB. **Not measured:** memory per instance, GC pressure at hundreds of instances, behaviour of `vm.Interrupt` as a deadline. **Blocking issue:** goja has no
heap cap, so a guest that allocates without bound OOMs the whole runner, which breaks plan principle 4 ("memory is a budget, never an OOM") and the sandbox story (host and guest share a Go heap).
Reasonable only if memory abuse is handled another way (a separate process per pool with an OS limit, or accepting trusted authors). Not a drop-in.

### 6.3 (iii) V8 via cgo (`tommie/v8go`) inside the Go runner

**Measured** single-threaded with the simplified marshalling of 1.4: W2 12 µs (28x), W3 372 µs (180x), W4 24 ms (223x; deno_core V8 is 3.5x better on the same loop, probably flag
differences), W5 29.5 µs. **Concurrency did not work in my harness:** c = 4 and c = 14 got slower than c = 1 (W1 120k, 43k, 13k) and a W2 c = 14 run aborted with a V8 out-of-memory
despite `Release()` on every value. That may be my usage, but it is the first thing anyone would hit.

Build and supply chain **[M]**: needs **clang >= 21** with `-nostdinc++`; Apple clang 17 failed to compile it, Homebrew clang 23.1.3 worked. The `v8gobench` binary with a trivial main is **76.6 MB**
(`fcdev` today: 111 MB). V8 ships as prebuilt static libraries in per-platform Go modules, **112-230 MB each** (darwin_arm64 112 MB, linux_amd64 152 MB, linux_arm64 151 MB, windows 230 MB);
`go.sum` lists all seven platform modules. Community fork: the original `rogchap/v8go` was last tagged v0.9.0, `tommie/v8go` is current (v0.39.0, auto-bumps V8; 15.5.35.25 on 2026-10-16). cgo ends
`CGO_ENABLED=0` static binaries and cross-compilation without a C++ toolchain, and the release pipeline (the fcdev and server binaries) needs a C++ toolchain per target. V8 CVEs would need prompt
dependency bumps in a binary that runs tenant code. The sandbox is V8's (in-process). **Not recommended.**

### 6.4 (iv) A separate Rust V8 runner serving `runtime=js` through the generic runner plumbing

Feasibility is good; the *work* is mostly not V8. What the plumbing gives (commit `67c721dc`): desired state takes `runtimes=a,b`, heartbeats carry `capabilities`, promote refuses
`POOL_LACKS_RUNTIME`, `FC_FUNCTIONS_RUNNER_URL_<RT>` routes deliveries per runtime, `fng_versions.runtime` is a tag (migration 083), and `cmd/fcrunner-echo` shows a complete Go-v1 client in 62 lines
of `main`. Two ways to pick the tag: reuse `js` and give the JS pool a Rust runner (the Go runner would advertise only `wasm` for that pool), or add a new tag (`jsv8`) so both can coexist per pool.

What the Rust host must implement:

1. **Go control protocol v1** client: desired-state long poll with ETag/`revision`, heartbeat (`runnerId`, `pool`, `startedAt`, `revision`, `budget`, `versions[]` with LOADED/COMPILED/FAILED/RETRYING,
   `capabilities {protocol, runtimes[], runnerKind, version}`), digest-keyed artifact download, single-event emit, `dispatch-jobs`. Today `fc-fnhost-core` speaks the **Java** wire
   (`/desired-state`, no revision), so this is a second `ControlPlane` impl. `docs/runner-plumbing-costing-2026-10-19.md` §1 already sizes it (M) and recommends it be done runner-side.
2. **The invoke listener**: `/fn/{address}[@alias|@vN]/{path}`, two-label addresses, platform-bearer and webhook-signature auth, the Go JSON error bodies and outcome headers (2xx ack, 429
   + `Retry-After`, 422 + `FlowCatalyst-Outcome: reject`). The Go runner's equivalent is ~4,400 non-test lines in `internal/functions/{runner,control,abi}`.
3. **The JS function ABI**: evaluate the IIFE bundle, call `__fc.describe()` and `__fc.handle(metaJSON, Uint8Array)`, take `{meta, body: Uint8Array|string}` back, supply `__fc_call` /
   `__fc_utf8_*`. `v8bench` already does exactly this in ~100 lines and was verified byte-identical on W1-W5 and on two real bundles.
4. **All eleven host capabilities with the Go wire shapes** (log, config, secret, http with egress policy, emit, db query/exec/begin/commit/rollback, dispatch): the Go side is `runner/capabilities.go`
   + `db.go`, ~780 lines, to port to Rust (sqlx).
5. **The execution model.** Today's `fc-fnhost-js` (`docs/function-runner-density.md` §10) is **a fresh isolate per request** (~1 ms on Linux, 2.5 ms on macOS) loading an ES module with a default export and
   `flowcatalyst:function/*` imports. Go bundles are IIFE scripts that set `globalThis.__fc`; to get the latencies in section 2 the new loader must keep **warm isolates per instance**, as `v8bench`
   does. Deadline = isolate termination from a watchdog and heap limit = `wasmMemoryMb`: both already exist in `fc-fnhost-js` (`isolate.rs`, `allocator.rs`).
6. **Platform-side:** the Go platform still evaluates JS at publish to read `describe` (`internal/functions/runtimes`), so QuickJS stays in the platform process even if runtime execution moves.

Size, using the costing doc's scale: control + listener + ABI loader M-L, capability port M, tests/ops/release M. I did not build any of it. Costs of the Rust side **[Q]**: V8 150.4 static library
147 MB (macOS arm64) to 187 MB (Linux x86_64), ~60 MB binary (my `v8bench`: 60.6 MB), `deno_core` pinned `=0.412.0`, a prebuilt `librusty_v8.a` downloaded at build time. The Go platform remains pure Go;
the supply-chain exposure is a separate deployable. The runner can also be run per pool, so only pools that need it carry V8.

A cheaper variant to consider **before** that: keep the Go runner (auth, permits, capabilities, protocol) and put V8 in a **child process** that only evaluates bundles (stdin/unix-socket frames). Every
host call then crosses a process boundary; at ~20 µs each a 20-call function becomes ~0.4 ms, **worse** than QuickJS's 0.1 ms for chatty functions, so it only helps compute-heavy ones. Not measured.

### 6.5 (v) Compiled languages

Measured: Rust on wazero 5.3x (W2) to 44x (W3) faster than QuickJS, and ~10x more once the deadline hook is off; on wasmtime 52x to 211x. Rust W3 0.3-1.5 ms, W2 6-64 µs. The cost is authoring
language: BEVCO's functions are TypeScript. Std Go is not a performance answer for JSON (0.7x on W2) or XML (4.7x), TinyGo is better (16x on W3) but unsupported and had its own scheduler pitfall. Use Rust for
the few hot transforms (XML, large JSON), keep TypeScript for glue.

## 7. Recommendation

1. **Stay on QuickJS-in-wazero as the default JS runtime.** For the work it was chosen for (routing, small JSON, host calls, 1-10k calls/s total) it is cheap and the I/O around it dominates.
2. **Do the deadline-mechanism work next** (section 6.1 item 1) and re-measure on Linux/amd64 before the plan's phase-0 numbers are trusted. It is a 4x on every JS function, and the same
   conversation applies to Rust guests (9-10x on compute) because the hook is runtime-wide. This is the single largest measured inefficiency in the function runner, and it is in our code,
   not in QuickJS.
3. **Write the tripwire down** and build the Rust V8 runner only when it fires: any JS function whose p50 CPU exceeds ~20 ms per call at more than ~20 calls/s per runner (an XML/large-JSON transform),
   or JSON-class JS fleets exceeding ~2 vCPU of sustained engine time. Today's evidence: the BEVCO XML mapping would be the first function to hit it.
4. **Until then, steer hot transforms (XML, large JSON) to Rust guests** and document that std Go is not a faster alternative.
5. **Do not adopt v8go**; **do not adopt goja** unless the memory-cap gap is closed some other way (it is the fastest stopgap, 4-11x, and the only "no new deployable" option).
6. **Keep the generic runner plumbing as the escape hatch.** The measured fact that bundles run unchanged on V8 means choosing it later does not strand any function authors.

## 8. What I could not measure, and caveats

- **No Linux, no amd64, no cloud vCPU.** The hook cost, the QuickJS ratio and V8 ratios may differ; the plan's own phase 0 calls for a Linux re-measure. Expect absolute numbers on cloud vCPUs to be worse.
- **Shared, loaded machine.** IDE indexers and OS services used 3-4 cores for most of the runs; load average ranged 20-290. Latencies of short calls vary by up to ~2x between passes; ratios above 10x are safe, anything near 2x is not.
- **The 15 KB BluJay message does not exist in the repos**; W3 uses a synthesised message of the same schema and size, and a simplified mapping (`mapping.ts` shape, not its validation or warnings).
- **Raw ABI, no SDK.** Real JS functions add roughly 60-90 µs of SDK routing/encoding per call on QuickJS **[Q]** (section 1.2); that floor shrinks on V8 proportionally.
- **No HTTP, auth, signature checking, permit/queue or database costs** in any number; this is engine time, not request time.
- **goja:** no memory or GC data; deadline behaviour untested. **v8go:** simplified marshalling, concurrency anomaly unresolved, W3-W5 only at c = 1.
- **wasmtime:** epoch interruption on, fuel metering off (the Rust repo meters fuel; that costs extra). Memory deltas include Cranelift transients.
- **Deadline-hook-off safety** (section 2.4) is a hypothesis with a measured upper bound, not a tested design.
- **Native QuickJS** and **a Rust runner speaking Go v1** were not built or measured; section 6.4 is a reading of the code plus the costing doc.
- Per-instance memory for deno_core isolates depends strongly on the bundle (1.65 MB trivial **[Q]** vs 6.5 MB for the 150 KB bundle **[M]**).

## 9. Reproducing

```sh
bench/function-runtimes/run.sh > results.txt   # builds everything (needs go, node/npm, cargo + wasm32-unknown-unknown; tinygo optional), runs ~35 min serially
# extras (own modules): bench/function-runtimes/gojabench, bench/function-runtimes/v8gobench (clang >= 21, -nostdinc++)
```

Layout: `js/` (the one TypeScript workload source, bundled with the real `fc-fn-build` settings, plus `probe.js`), `fixtures/` (generator, inputs, byte-exact expected outputs), `rust-guest/`,
`go-guest/` (std Go and TinyGo), `wazerobench/` (latency, throughput, cold, memory, smoke on the platform's real engine; `loopcfg/` isolates the deadline hook), `rusthost/` (`v8bench`, `wtbench`),
`results/` (raw `RESULT` lines for every table above, including the engine-rebuild and smoke outputs).
