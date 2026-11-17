# Function deadlines: how a call is stopped, and what that costs

Status: implemented 2026-10-20 (Go runner); interrupt countdown and the process pause measured
and changed 2026-11-17. Applies to JavaScript functions only; Wasm functions
(Rust, Go, TinyGo, anything compiled) are unchanged. Background and measurements:
`docs/function-runtime-performance-2026-10-19.md` section 2.4.

## The problem

A function has a deadline (`timeoutMs`). Something has to stop a guest that runs past it. On
wazero there is exactly one mechanism for that, and it is chosen when the runtime is built:
`WithCloseOnContextDone(true)`. It makes the compiler put a check at every loop header and
function entry. It works on any guest, however hostile, and it costs a lot: 3.7-4.8x on QuickJS
compute, 4-8x on Go and 8-10x on Rust (measured, performance doc 2.4). It is a runtime-wide
setting, so the old design paid it for every JS function too.

## What changed

The engine now owns two wazero runtimes (`internal/functions/engine/engine.go`):

| Runtime | Stops a call by | Who runs on it |
|---|---|---|
| `rt`, preemptible (`WithCloseOnContextDone(true)`) | wazero, from outside, at any instruction boundary it instrumented | every module an author supplies (the `wasm` runtime) |
| `coop`, cooperative (no check) | the guest, from inside, plus a host watchdog | the platform's own JS engine only (`jsengine/engine.wasm`) |

The split is by trust. Code the platform wrote and controls may rely on itself to stop; code an
author compiled may not. `CompileCooperative` is the only way onto `coop`, the loader calls it
for the shared JS engine and nothing else, and a module that does not export `fc_js_deadline`
is refused there (so an arbitrary module cannot be put on `coop` by mistake).

### Inside the JS engine

`clients/fn-js-engine/src/lib.rs` installs a QuickJS interrupt handler. The host calls the
export `fc_js_deadline(ms)` before every `fc_handle`, `fc_describe` and script load; the handler
compares a monotonic clock with that deadline and tells QuickJS to abort. The abort is not
catchable by `try/catch/finally`.

QuickJS does not call the handler at every opportunity. It counts *poll points* (function and
constructor calls, backward jumps, a few object helpers, the regexp matcher's steps) and calls
the handler when the count reaches `JS_INTERRUPT_COUNTER_INIT`. Stock QuickJS sets that to
10 000; the engine is built with **100** (a one-line change in a vendored copy of
`rquickjs-sys`, see "Rebuilding the engine"). A builtin that runs as one native call (a sort
without a comparator, a string search, BigInt arithmetic, `JSON.parse`) has no poll points
inside it and runs to its end.

`fc_js_deadline` is the JS engine's own export. It is not part of ABI v1 and no author-facing
contract changes (`docs/function-stability-contract.md`).

### The host side (`Instance.run`)

For a cooperative instance each call:

1. takes the remaining time from the context deadline (30 s if the context has none, so nothing
   is ever unbounded), rounds it **up** to whole milliseconds, and arms the guest;
2. runs the wasm call on its own goroutine;
3. waits for it, or for the watchdog at `deadline + stuckGrace` (2 s);
4. if the guest stopped itself at or past the deadline, reports `ErrDeadline` (the interrupted
   guest answers a generic 500 frame; that frame is thrown away and the caller sees 504
   `FUNCTION_TIMEOUT`, as before);
5. if the watchdog fires, the call is **abandoned**: the caller gets `ErrStuck` (which is an
   `ErrDeadline`, so 504), the instance is marked broken and *not* closed under the running
   call, `engine.Stuck()` is incremented, and the goroutine closes the instance and decrements
   the count if the call ever returns.

Every deadline outcome discards the instance, as before.

### When stuck calls pile up

A wasm call cannot be killed from outside on a runtime without the checks, so a guest that
ignores its interrupt keeps its thread. The runner makes that visible and bounded:

- gauge `fc_fn_stuck_calls`;
- an ERROR log line per abandoned call;
- at `engine.StuckLimit` (4) stuck calls `/healthz` and `/readyz` return 503, so the
  orchestrator replaces the process. Four simultaneous stuck calls cannot happen through normal
  use; they mean something the interrupt handler cannot reach, so a restart is the answer.

## The process pause

Go cannot preempt a goroutine while it runs wazero's compiled code. It can stop it only when the
guest returns to Go: a host call (in a runaway, the handler's clock read), or one of wazero's
exits to Go (memory grow, stack grow; wazero 1.12). A garbage collection that starts in between
stops every other goroutine first and then waits for the guest, so **the whole process is
paused** (every request, the
watchdog's timer, health checks) until the guest next reaches the host. The same holds for
anything else that stops the world. So the gap between two host calls of a running guest is a
process-wide pause, not only that call's overrun.

Measured (macOS arm64, M4 Pro, 2026-11-17) with an instrumented build that recorded the gap
between handler calls, while another goroutine stopped the world in a loop: in every case the
longest stop-the-world wait matched the longest gap between handler calls (for example 14.1 ms
gap / 13.7 ms wait, 28.8 / 27.7 ms, 1 139 / 1 134 ms). The host call is the point where the
collector gets the goroutine.

With the stock countdown of 10 000 the gap depends on how expensive a poll point is. The BigInt
squaring chain does a multiply of up to ~200 kbit and a `toString(2)` per iteration, about two
poll points: the handler ran **once**, 1.14 s into a 250 ms call. A collection started during the
call took 1.09-1.13 s and the call returned at 1.14-1.18 s even with the watchdog's grace cut to
100 ms. On a CI runner (`-race`, Linux) the same test stopped after 6.2 s.

Longest gap between handler calls (= longest stop-the-world wait), deadline 250 ms, by countdown:

| Runaway | 10 000 | 1 000 | 300 | 100 (shipped) |
|---|---|---|---|---|
| BigInt squaring chain | 1 139 ms | 159 ms | 46 ms | 17 ms |
| string building | 29 ms | 7 ms | 1.3 ms | 0.5 ms |
| JSON churn | 14 ms | 4 ms | 5 ms | 1.3 ms |
| array fill 2e6 + reduce | 27 ms | 31 ms | 29 ms | 31 ms |
| promise chain | 15 ms | 17 ms | 16 ms | 15 ms |
| every other shape in the test | < 3.5 ms | < 3.5 ms | < 3.5 ms | < 4.5 ms |

The array fill and promise chain do not move with the countdown: their gap is native work with
no poll point in it (for the fill, the 2e6-element `fill` call; for the promise chain the native
stretch was not identified).

`TestCooperativeRunawayDoesNotHoldTheProcess` runs every runaway shape with collections started
every 5 ms and reads the runtime's GC pause histogram (`/sched/pauses/total/gc:seconds`): no
pause may exceed 500 ms. That bound is a regression guard, not the measurement: the CI runner
ran the previous engine's stretch about seven times slower than the M4 Pro did. With the shipped
engine the longest was 29 ms (array fill); under `-race`, `GOMAXPROCS=2` and every core busy, at
most 34 ms. With the previous engine it fails on the BigInt shape: 1.34 s unloaded, 1.61 s loaded
(histogram bucket edges).

## What was tested

`internal/functions/engine/cooperative_test.go`, against the real engine, deadline 250 ms. Every
case below stops as a deadline outcome and the instance is discarded:

- empty loop, busy arithmetic, string building, JSON churn, random-comparator sort
- a loop that swallows the abort in `try/catch` and in `try/finally`
- catastrophic regexp backtracking (`/(a+)+b/` on 48 characters), and the same inside a loop
- an `async` function that `await`s forever (microtask loop), and an unending promise chain
- a BigInt squaring chain, and a repeated 2-million-element array fill

Timing (`TestCooperativeRunawaysStopAtTheDeadline`, shipped engine): every shape stops within
40 ms of the deadline (promise chain 27-38 ms, array fill ~7 ms, the rest 0-1 ms). Under `-race`,
`GOMAXPROCS=2` and every core busy: at most 38 ms past the deadline (previous engine: 1.0 s, the
BigInt shape).

### Single builtins do not poll

One call to a builtin runs without calling the host, so it is both an overrun and a process
pause. Measured, one call each, with the 256 MiB instance cap (the countdown does not change
these):

| Operation | Without a host call |
|---|---|
| `String.prototype.indexOf`, a 100 000-character needle in 200 000 characters | 21.9 s |
| `Float64Array.prototype.sort`, 2e7 elements | 3.4 s |
| BigInt to decimal string, 1 Mbit (QuickJS's BigInt size limit) | 2.2 s |
| `String.prototype.replaceAll` on 50 MB | 1.3 s (3.1 s in all; memory growth breaks it up) |
| `Array.prototype.sort` without a comparator, 2e6 numbers | 1.0-1.1 s |
| `JSON.parse`, 40 MB | 0.75 s |
| `String.prototype.indexOf` over 1e8 characters, short needle | 0.6-0.8 s |
| BigInt divide 1 Mbit by 512 kbit / multiply 2 x 512 kbit | 0.4-0.6 s / 0.27-0.29 s |
| `"x".repeat(1e8)` | 0.4-0.5 s |
| `Uint8Array` sort, 1e8 elements | 0.5 s |
| `Array.prototype.join`, 1e7 elements | 0.2 s |

Most of these are bounded by the memory cap or by QuickJS's 1 Mbit BigInt limit. String search
is not: QuickJS's `indexOf` (and so `includes`, `split` and `replace` with a string pattern) is
the naive algorithm, time proportional to haystack x needle, so a call on inputs of a few hundred
kilobytes can run for minutes. A sort with a comparator, and a regexp, do poll.

Also tested: an expired context never starts the guest; unbounded recursion fails fast (see
below); the watchdog path (a hand-written wasm guest that ignores its deadline,
`testdata/stuck.wat`) gives `ErrStuck`, counts, does not close the instance under the call, and
settles when the guest returns; Wasm modules stay on the preemptible runtime; the engine is
unhealthy at the limit; the runner's existing 150 ms `/spin` end-to-end test passes unchanged.

## Known limits (accepted, written down)

- **A single builtin call pauses the process for as long as it runs.** See "Single builtins do
  not poll": measured up to 3.4 s for memory-bounded ones, and string search has no bound but
  its inputs (21.9 s on 300 KB). During such a call a garbage collection holds every goroutine,
  the watchdog's timer included, so `stuckGrace` does not bound it either: the call is abandoned,
  at the earliest, when the builtin returns. Accepted by the owner (2026-11-17): functions are
  first-party code, and a deployed runner is its own process, so what pauses is one pool's
  runner. A host that can stop compiled code removes the limit (on wasmtime the same `indexOf`
  stopped 0.3-3.2 ms past its deadline and held nothing:
  `function-runtime-wasmtime-go-spike-2026-11-17.md`), but that move is parked
  (`function-runtime-decision-2026-10-25.md`, amended 2026-11-17).

- **Client hang-up does not stop a JS call.** Cancellation of the request context is not
  observed by the guest on the cooperative runtime (only the deadline is). A JS function whose
  caller went away runs to completion or its deadline. It was previously stopped immediately.
  Bounded by `timeoutMs`; if it matters, a host import the interrupt handler polls would fix it.
- **Unbounded recursion** traps ("out of bounds memory access") instead of raising a catchable
  `RangeError`: QuickJS's own stack limit does not take effect in this wasm build. Identical
  before and after this change. The call fails with a 500 and the instance is discarded, in
  milliseconds.
- **The abandon path leaks a thread** until the guest returns, if it does. That is the price of
  not having wazero's checks; the stuck limit and health check bound it.
- **No per-version quarantine yet.** Stability contract C-50 asks a runner that cannot preempt to
  quarantine a version that gets stuck. Here a stuck call only counts toward the process-wide
  limit, so one function that reliably defeats the interrupt handler could take the process to
  its restart threshold, repeatedly. None of the runaway shapes in the test reaches the abandon
  path (only the hand-written wasm guest does); a single long builtin call (a quadratic string
  search, a large sort) does. The fix, if it is ever seen, is to mark the version failed
  (permanent, with a reason) on the first `ErrStuck`.
- **Not measured on amd64.** See the measurements section.

## If a wazero upgrade or a new JS engine changes the assumptions

- The cache: both runtimes share one compilation cache. That is safe only because wazero hashes
  the termination setting into the module ID the cache is keyed by.
  `TestSharedCacheNeverCrossesRuntimes` fails (it does not hang) if that ever stops being true;
  the fix is a cache directory per runtime.
- A different JS engine (V8 isolates, goja) would not use `coop` at all; it has its own
  interrupt mechanism. See `docs/function-runtime-performance-2026-10-19.md` section 6.

## Rebuilding the engine

`clients/fn-js-engine/build.sh` (needs a wasi-sdk and the `wasm32-wasip1` Rust target). The
embedded `internal/functions/jsengine/engine.wasm` must be rebuilt after any change to
`src/lib.rs`; `fc_js_deadline` has to stay exported or the engine will not load.

`rquickjs-sys` comes from `clients/fn-js-engine/vendor/rquickjs-sys` (`[patch.crates-io]` in
`Cargo.toml`): version 0.11.0 as published, trimmed to the files its build reads, with one line
changed, `#define JS_INTERRUPT_COUNTER_INIT 100` (was 10 000). That `#define` has no `#ifndef`
guard, and neither rquickjs nor its build script offers a setting, so the source has to change.
`diff -r` against the crates.io copy in `~/.cargo/registry/src/` shows that line and the files
left out. On an rquickjs upgrade, vendor the new `rquickjs-sys` and re-apply the line; then
`TestCooperativeRunawayDoesNotHoldTheProcess` checks it took effect (it fails with 10 000).

`rquickjs-sys`'s build script looks for the wasi-sdk in `WASI_SDK` and, when that is unset,
downloads wasi-sdk 24 from GitHub into the build directory (it then compiles with the `CC_`
and `CFLAGS_wasm32_wasip1` that `build.sh` sets, so the download goes unused). `build.sh` sets
`WASI_SDK` to `WASI_SDK_PATH` to stop the download; the output is unchanged by it.

The 2026-11-17 build: wasi-sdk 25.0 (`wasi-sdk-25.0-arm64-macos`, clang 19.1.5), rustc and cargo
1.98.1, wasm-opt version 133, on macOS arm64. With the same toolchain, `build.sh` on the source
before the change reproduced the previous `engine.wasm` byte for byte, once the build
directory's path was the same: QuickJS's `assert` messages embedded the absolute path of the C
sources under `target/`, and Rust's panic locations the paths of the cargo registry and the
toolchain, so a checkout at another path gave a binary that differed in those strings.

`build.sh` now remaps those paths (`-ffile-prefix-map` for the C sources, `--remap-path-prefix`
for Rust: the crate directory to `.`, the cargo home to `/cargo`, the rustup home to `/rustup`),
so the engine is the same bytes wherever it is built and holds no home directory. Built at two
different paths with the toolchain above it is 717 948 bytes, sha256
`e71372441f00b4847c9c4870d309c3f076883fcb606c3fe373b612a061c9a592`. The name of the host
toolchain (`stable-aarch64-apple-darwin`) is still in it, so a build on another platform differs
there.

## Measurements

Same JS bundle and workloads as the performance doc (`bench/function-runtimes`), through the real
loader and `Instance.Handle`, single thread, p50, old engine on the preemptible runtime vs new
engine on the cooperative one. Built from the previous commit in a scratch worktree and run
interleaved (old, new, old, new) on one machine; the two runs of each agree within 3%.

| Workload | macOS arm64 (M4 Pro) old / new | Linux arm64 (OrbStack, 14 vCPU) old / new |
|---|---|---|
| W1 trivial call | 9.3-9.6 / 4.9-5.1 us (1.9x) | 7.3 / 3.9-4.0 us (1.9x) |
| W2 small JSON | 340-343 / 89-90 us (3.8x) | 331-337 / 92-93 us (3.6x) |
| W3 XML parse, 15 KB (the BEVCO shape) | 64-66 / 17.9-18.5 ms (3.6x) | 64-65 / 18.3-18.8 ms (3.5x) |
| W4 pure compute loop | 5.0-5.3 / 1.00-1.01 s (5.1x) | 5.1-5.3 / 0.95-1.00 s (5.3x) |
| W5 echo through a host call | 100-102 / 35 us (2.9x) | 88-91 / 36-37 us (2.5x) |

So about 3.5x on realistic work, 5x on the worst case, and about 2x even on a trivial call. The
projection in the performance doc (3.7-4.8x) held. The runaway tests also pass on Linux arm64.

**The countdown (2026-11-17, macOS arm64, M4 Pro).** `wazerobench -mode lat`, the same bundle,
single thread, p50, engines with the countdown at 10 000 (previous), 1 000, 300 and 100 run
interleaved, two passes each:

| Workload | 10 000 | 1 000 | 300 | 100 (shipped) |
|---|---|---|---|---|
| W1 trivial call | 5.04-5.12 us | 5.12-5.17 us | 5.12 us | 5.25-5.29 us |
| W2 small JSON | 89.0-90.0 us | 88.5-89.8 us | 90.2-90.8 us | 90.5 us |
| W3 XML parse, 15 KB | 19.1-19.2 ms | 19.1-19.8 ms | 18.6-19.3 ms | 18.8-18.9 ms |
| W4 pure compute loop | 1.01-1.02 s | 1.03-1.04 s | 1.01-1.03 s | 1.01-1.04 s |
| W5 echo through a host call | 34.8-35.2 us | 34.5-36.7 us | 35.6 us | 35.4-35.6 us |

W1 is 0.1-0.2 us (about 3%) higher at 100 in both passes, inside the spread of the individual
runs (4.96-5.33 us at 10 000, 5.12-5.38 us at 100); W2-W5 show no difference beyond the
run-to-run spread. The most handler calls per unit of work come from a tight
loop of JS function calls (`a = f(a)`, 3e7 times, about 90 ns a call): 2 618 ms at 10 000,
2 667 ms at 300, 2 665 ms at 100, mean of six interleaved runs each, so about +2% at worst.

**Not measured: Linux amd64.** OrbStack here runs arm64; amd64 would be emulated and prove
nothing. The mechanism is architecture-independent (interrupt handler in the guest, the same
wazero compiler path without the checks), but the absolute numbers on cloud amd64 vCPUs will
differ. Run `bench/function-runtimes/wazerobench` on the production instance type before
quoting capacity.

