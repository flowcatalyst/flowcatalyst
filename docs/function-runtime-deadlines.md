# Function deadlines: how a call is stopped, and what that costs

Status: implemented 2026-10-20 (Go runner). Applies to JavaScript functions only; Wasm functions
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
compares a monotonic clock with that deadline and tells QuickJS to abort. QuickJS polls its
handler from the bytecode loop (every ~10k opcodes), from the regexp matcher and from long
builtins, and the abort is not catchable by `try/catch/finally`.

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

## What was tested

`internal/functions/engine/cooperative_test.go`, against the real engine, deadline 250 ms. Every
case below stops as a deadline outcome and the instance is discarded:

- empty loop, busy arithmetic, string building, JSON churn, random-comparator sort
- a loop that swallows the abort in `try/catch` and in `try/finally`
- catastrophic regexp backtracking (`/(a+)+b/` on 48 characters), and the same inside a loop
- an `async` function that `await`s forever (microtask loop), and an unending promise chain
- a BigInt squaring chain, and a 20-million-element array fill

Timing: the loops stop on the deadline to within a few ms. The slowest native stretch found was
the BigInt chain at about 0.8 s past the deadline (one long native `toString`/multiply that does
not poll). Native builtins are bounded by the function's memory cap, so this is the worst case,
not an unbounded one; `stuckGrace` is 2 s to cover it.

Also tested: an expired context never starts the guest; unbounded recursion fails fast (see
below); the watchdog path (a hand-written wasm guest that ignores its deadline,
`testdata/stuck.wat`) gives `ErrStuck`, counts, does not close the instance under the call, and
settles when the guest returns; Wasm modules stay on the preemptible runtime; the engine is
unhealthy at the limit; the runner's existing 150 ms `/spin` end-to-end test passes unchanged.

## Known limits (accepted, written down)

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
  its restart threshold, repeatedly. None of the runaway shapes above reached the abandon path
  (only the hand-written wasm guest did), so this is unbuilt rather than urgent. The fix, if it is
  ever seen, is to mark the version failed (permanent, with a reason) on the first `ErrStuck`.
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

**Not measured: Linux amd64.** OrbStack here runs arm64; amd64 would be emulated and prove
nothing. The mechanism is architecture-independent (interrupt handler in the guest, the same
wazero compiler path without the checks), but the absolute numbers on cloud amd64 vCPUs will
differ. Run `bench/function-runtimes/wazerobench` on the production instance type before
quoting capacity.

