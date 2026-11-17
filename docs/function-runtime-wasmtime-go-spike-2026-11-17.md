# wasmtime-go for deployed runners: spike results (2026-11-17)

Phase 1 of `function-runtime-decision-2026-10-25.md`: can the Go runner use wasmtime through
`wasmtime-go` (cgo) in the Linux server images while everything else stays on wazero? Code and raw
output: `bench/function-runtimes/wasmtime-go-spike` (README there says how to run each measurement;
`results/` holds every number quoted here). Nothing under `internal/`, `cmd/` or the root
`go.mod`/`go.sum` was changed.

> **Outcome (owner, 2026-11-17): the route this spike tested was dropped, and wasmtime is parked.**
> The measurements below stand. The recommendation at the end (proceed with `wasmtime-go` on a fork
> of the binding) was not taken: when or if deployed runners move to wasmtime, it is with a Rust-only
> runner, a separate binary, and nothing of wasmtime goes into the Go runner. See
> `function-runtime-decision-2026-10-25.md`, the amendment and section 3. Read "the plan", "phase 2"
> and "phase 4" below as the plan of 2026-10-25, before that amendment.

## Short answer

The engine does what the decision expected: byte-identical outputs on the shipped QuickJS engine and
a Go guest through real ABI v1 host calls; every runaway stopped within a few milliseconds of its
deadline by epoch interruption, including the two shapes wazero cannot stop (a BigInt chain and the
~20 s `indexOf`); garbage collections no longer wait for a running guest (≤0.5 ms on macOS against
20 s on wazero); 1.6x faster on JS W3 and about 10x on a compiled Go guest. On Linux arm64 natively
and amd64 under emulation.

The binding is the weak part, in three ways the plan did not foresee, each fixable:

1. **Go 1.27.4 refuses to link wasmtime-go v50's prebuilt libraries** (a new cgo security check
   rejects objects with embedded LLVM bitcode). The image build has to strip the sections from a
   copy of the module (shown working) or build `libwasmtime.a` itself.
2. **A process-wide mutex in wasmtime-go** is taken on every host call and every call into wasm.
   With host calls under concurrency it costs 2-6x throughput against wazero (W5 at 14 goroutines:
   51-71k against 113-293k calls/s, p99 ~20x worse at 64). A 45-line patch to the binding (in
   `results/wasmtime-go-store-lock-patch.diff`) recovers most of it (138k).
3. **The binding exposes less than the C API**: no memory creator (so no per-grow budget charge), no
   limiter callback (so out-of-memory is inferred), no epoch-deadline callback, no pooling
   allocator, and WASI stdout/stderr only to a file path or inherited, never an `io.Writer`.

Recommendation: **proceed with wasmtime-go, on a small fork of the binding that this repository owns**
(or patches upstreamed first), not the Rust route. Details under "Recommendation".

## Machine and versions

| | |
|---|---|
| Host | Apple M4 Pro, 14 cores, 48 GB, macOS 27.2.1 (load average 2-5 during the runs) |
| Linux | OrbStack 29.8.2 VM on the same host, kernel 7.2.12-orbstack, 14 vCPU, Debian bookworm (glibc 2.36) |
| Linux amd64 | the same VM, x86_64 under emulation: **timings emulated, not to be trusted** |
| Go | 1.27.4 on Linux; **1.27.2 for the macOS wasmtime builds** (1.27.4 refuses the macOS library, below) |
| wasmtime-go | v50.0.0 (wasmtime 50; released 2026-10-21; the newest on the module proxy today) |
| wazero | v1.12.0, through the production `internal/functions/engine` and `runtimes.Loader`, untouched |
| JS engine | `internal/functions/jsengine/engine.wasm`, sha256 `e71372441f00...a592` (the shipped one) |
| Guests | `bench/function-runtimes/js/dist/function.js` (sha256 `513e5427...`, copied from the main checkout's build), `go-guest` (GOOS=wasip1 c-shared), and `guests/gospike` (loop, allocation, host-call loop) |

## How it was measured

`wtengine` is a port of the production engine's call path to wasmtime-go: the ABI v1 module check,
`fc.call`/`fc.take` with the same framing, copies and error frames (the `abi` package), WASI
preview1 (wasmtime's own), `_initialize`, the JS engine preload (`fc_js_load`), the production
outcome errors (`engine.ErrDeadline` and the rest, imported), a store per instance with the memory
cap as its limiter, and epoch interruption with a ticker goroutine. The spike is its own module
under the main module's import path, so it imports `internal/functions/...` directly.

Every measurement runs the same code against both engines with **the same host object**. That host
serves `log` (decoded as `abi.Log`, formatted by a real `slog` JSON handler) and `config.get`
(declared-key check, `abi.Found`, value), with the same logic as `runner/capabilities.go`; it is a
copy of that logic because `capabilities` is unexported. On wazero the `fc` plumbing in front of it is
the production `engine/host.go`; on wasmtime it is `wtengine`'s port.

Engines in the tables: **wazero** (production: JS on the cooperative runtime with the in-guest
interrupt and watchdog, Wasm on the preemptible runtime); **wasmtime** (epochs on, 1 ms tick, epoch
deadline only); **wt-armed** (epoch deadline at +20 ms, and `fc_js_deadline` armed as production
does); **wt-noep** (epochs off: prices the checks); **wt-map** (as wasmtime, host state through a
`sync.Map` instead of `Caller.Data`).

## 1. Equivalence

`-mode equiv`: each case three times on one instance (the answer must not drift), whole response
frame compared byte for byte across engines, body against the reference file where there is one,
host calls counted.

| Case | Reference | Host calls / invocation | Result (all 5 engines) |
|---|---|---|---|
| QuickJS bundle W1-W5 | `fixtures/expected-w*.txt` | 0, 0, 0, 0, **20** | identical frames, reference OK |
| Go guest W1-W5 | same | 0, 0, 0, 0, **20** | identical frames, reference OK |
| JS: declared key, undeclared key (`NOT_DECLARED`), op 99 (`UNKNOWN_OP`), log with attributes, meta not JSON (`BAD_REQUEST`) | none: engines compared | 5 | identical frames |
| the same with no host (`CAPABILITY_UNAVAILABLE` x5) | none | 0 | identical frames |
| gospike, 7 `config.get` | `252` | 7 | identical, OK |

`all_identical=true` on macOS arm64, Linux arm64 and Linux amd64 (emulated), 13 cases each
(`results/equiv-*.txt`). The QuickJS and Go W1-W5 frames are also identical to each other (same
reference outputs).

## 2. Deadline

`-mode runaway`, deadline 250 ms, a fresh instance per round, then a call on another fresh instance,
then the process's CPU over 500 ms (`getrusage`). "Past deadline" is the range over the rounds
(5 on Linux, 3 on macOS; wazero 1-2). Columns for wasmtime: macOS / Linux arm64 / amd64*.

| Runaway | wazero (production) | wasmtime, epoch only | wt-armed (JS interrupt + epoch at +20 ms) |
|---|---|---|---|
| JS `for (;;) {}` | +0.1-4.3 ms, JS interrupt | macOS +0.3-0.9, Linux arm64 +0.6-3.0, amd64* +1.5-5.7 ms | +0.1-7.2 ms (JS interrupt) |
| Go guest unconditional loop (compiled) | +0.6-8.4 ms (preemptible runtime: its 8-10x hook) | +0.7-1.2 / +1.3-3.2 / +0.4-6.7 ms | +21-23.5 ms (epoch; no JS interrupt) |
| BigInt squaring chain | +2.8-10.7 ms | +0.4-0.7 / +0.7-2.3 / +2.0-3.6 ms | +0.2-11.3 ms |
| `indexOf`, 1e5-character needle in 2e5 characters | **abandoned at +2.0 s** (`ErrStuck`); the guest kept a core busy ~17 s more | +0.7-1.2 / +0.7-3.1 / +0.7-2.9 ms | +20.2-24.2 ms (epoch) |
| unbounded recursion | trap in 3-40 ms (out of bounds memory access) | trap in 0.2-1.9 ms (stack overflow, trap code 0) | the same |

\* amd64 under emulation.

The `indexOf` script is reconstructed: `"a".repeat(2e5).indexOf("a".repeat(1e5 - 1) + "b")`. On
wazero it ran about 19 s here (the deadlines doc measured 21.9 s): the watchdog answered at 2.25 s
and the process then burned a full core for another 16.5 s (macOS) / 16.1 s (Linux arm64)
(`runaway-wazero-indexof-*.txt`, CPU per second after the call: `1003 999 995 ... 243 0`).

**What the caller sees on wasmtime:** `engine.ErrDeadline` (wrapping a wasmtime `Trap` with code
`Interrupt`), the instance broken and closed, so the runner's existing 504 `FUNCTION_TIMEOUT` path
applies unchanged. With wt-armed the JS shapes that poll stop themselves first and give the same
"stopped at its deadline" outcome as wazero.

**Next call and leftovers:** the call on a fresh instance answered "ok" after every runaway on every
platform. No thread is left running: process CPU after the call is the epoch ticker's idle cost
(next table), the same with or without a runaway before it; wazero after the abandoned `indexOf`:
504 ms per 500 ms.

**One bug found and fixed in the spike:** a Go 1 ms ticker fires only 335-407 times a second in this
Linux VM (a plain `time.Ticker`, no wasmtime involved; 994-1000 on macOS). Counting ticks as
milliseconds made Linux deadlines 2.5-3x late (640-815 ms for 250 ms). The ticker now issues as
many epochs as whole ticks have passed on the monotonic clock, and a call's deadline is the epoch
due after it on that clock; a deadline taken relative to the current count fired up to 2 ms early
after a lagging ticker caught up. Phase 4 must do the same.

**The ticker's cost** (`-mode idle`, a loaded engine doing nothing):

| | wazero | wasmtime, 1 ms tick | wasmtime, 10 ms tick | wasmtime, epochs off |
|---|---|---|---|---|
| CPU, ms per second | 0.1 | macOS 53-84, Linux arm64 15.5-19.5 | macOS 7.7-10.7, Linux 9.0-10.9 | 0.1 |

A 10 ms tick makes a deadline up to ~10 ms late (measured: +2.9-9.0 ms on macOS). Whether the ticker
should run only while a call is in flight is a phase-4 choice; not measured.

## 3. The process pause

`-mode gcpause`, as `TestCooperativeRunawayDoesNotHoldTheProcess`: `runtime.GC()` every 5 ms during
the runaway call, the longest stop-the-world pause from `/sched/pauses/total/gc:seconds` (bucket upper
edge), and how late a 1 ms sleep loop woke at worst.

| Runaway | wazero: collections / longest pause / timer late | wasmtime (and wt-armed): longest pause / timer late |
|---|---|---|
| JS loop | 46 / 0.07-0.12 ms / 0.2 ms | macOS ≤0.52 ms / ≤1.2 ms |
| BigInt chain | **4-5 / 12.6-14.7 ms / 11.5-13.8 ms** | macOS ≤0.10 ms / ≤0.6 ms |
| `indexOf` | **1 / ≤21.5 s / 20.2-20.9 s** (the whole call) | macOS ≤0.11 ms / ≤0.4 ms |
| Go loop | 43 / ≤0.16 ms / 0.2 ms (preemptible runtime) | macOS ≤0.08 ms / ≤0.4 ms |
| any shape, Linux arm64 | (JS loop 0.16-4.2 ms, the VM's floor) | ≤3.7 ms / ≤8.2 ms, 26-32 collections |
| any shape, amd64* | | ≤6.3 ms / ≤6.2 ms |

Your expectation holds: the guest runs inside a cgo call, which the collector treats as stopped, so
a collection does not wait for it. The pauses on wasmtime are the same size as wazero's when its
guest is not a runaway. On Linux both engines show a floor of a few milliseconds (the VM's timer
resolution, above), not tied to the guest shape. A side effect: on wazero a `runtime.GC()` called
while the abandoned `indexOf` guest still ran blocked for the remaining ~20 s (first version of the
spike's CPU probe).

## 4. Memory limit

`-mode mem`, cap 64 MiB, three rounds each (`results/mem-*.txt`):

| Guest | wazero | wasmtime |
|---|---|---|
| JS, `Uint8Array(1 MiB)` in a loop | returns a 500 `FUNCTION_FAILED` frame (QuickJS's out-of-memory exception, handled by the engine) in 4-17 ms | the same frame, 4-12 ms |
| JS, 1 MiB strings in a loop | the same, 18-29 ms | the same, 10-27 ms |
| Go guest, 1 MiB slices in a loop | `ErrOutOfMemory` (unreachable trap after a refused grow), 12-32 ms | `ErrOutOfMemory`, 8-21 ms |
| next call, fresh instance | ok | ok |
| RSS after close, rounds 1..8 (macOS) | stable (49 / 96 MB) | stable (103-104 / 206 MB) |

How the limit is set: `Store.Limiter(memorySize, -1, 1, -1, 1)` per store, i.e. the function's
`memoryMb` as the store's maximum linear memory; a grow past it makes `memory.grow` answer -1 (no
trap), exactly as wazero's allocator does when it refuses.

What the budget (`internal/functions/budget`) would map to: today `InstanceMemory` is a wazero
`experimental.MemoryAllocator`, so every grow is charged to the runner-wide budget and a grow the
budget cannot cover fails inside the guest, and `Refused` tells an out-of-memory trap from others.
wasmtime-go offers none of that: no allocator hook and no limiter callback. Options, none built:
(a) charge the whole cap at instantiate (exact, pessimistic: a 256 MiB function holds 256 MiB of
budget per instance); (b) charge the minimum at instantiate and reconcile `Memory.DataSize` after each
call (over-commit bounded by the sum of caps in flight); (c) the C API's
`wasmtime_config_host_memory_creator_set` (grow callbacks), which wasmtime-go does not expose: a
fork could, and would give parity. The spike classifies out-of-memory by a heuristic (memory within
1 MiB of the cap when the call failed): right in every run here, but a heuristic.

RSS: wasmtime keeps ~60-90 MB more than wazero after the same work (stable across rounds, so not a
per-call leak); not investigated.

## 5. Speed

`-mode lat`, single thread, one warm instance per cell, p50 in µs (ms where marked), two
interleaved passes, range of the passes' medians. Epochs on unless noted.

**Linux arm64** (native):

| Workload | QuickJS: wazero | QuickJS: wasmtime | QuickJS: epochs off | Go: wazero (preemptible) | Go: wasmtime | Go: epochs off |
|---|---|---|---|---|---|---|
| W1 trivial | 4.17-4.38 | 4.08-4.21 | 4.33-4.42 | 5.92-6.12 | 3.58 | 3.29-3.33 |
| W2 JSON 2 KB | 92.6-92.8 | 64.5-64.7 | 50.7-51.3 | 418-450 | 42.6-42.8 | 35.2-35.5 |
| W3 XML 15 KB | 18.8-19.4 ms | **11.65-11.72 ms** | 9.59-9.65 ms | 13.4-13.7 ms | **1.27-1.32 ms** | 1.12 ms |
| W4 compute | 1.01-1.02 s | 0.61-0.62 s | 0.64 s | 77.5-79.0 ms | 8.1-8.3 ms | 7.9 ms |
| W5 20 host calls | 50.9-51.5 | 47.6-47.7 | 43.8-43.9 | **23.9-24.5** | **27.7-27.8** | 27.6-27.7 |

**macOS arm64:** QuickJS W3 18.7-19.0 ms against 11.5-12.2 ms; W4 1.03-1.04 s against 0.62-0.66 s;
W5 48.5-54.8 against 46.8-48.2 µs; Go W3 12.9-14.3 ms against 1.25-1.27 ms; Go W5 28.8-31.1 against
26.9-27.8 µs (`results/lat-darwin-arm64.txt`).

So: QuickJS 1.6x faster on W3 (the Rust spike's 1.7-2x with epochs on: 10.6 against 18 ms), 1.6x on
W4; a compiled Go guest 8-10x faster, because wazero has to run it with its per-loop hook. Epoch
checks cost 13-27% (QuickJS W3 +21%, W2 +27%; Go W3 +13%); on W4 none measurable. W5 gains least:
the host calls eat the gain, and for the Go guest on Linux wasmtime is 15% slower than wazero.

**Per host call** (`-mode hostcost`: (p50 with 1000 `config.get` calls - p50 with 0) / 1000; one
`fc.call` plus one `fc.take`, so two guest-to-host crossings):

| | wazero | wasmtime | wt-map |
|---|---|---|---|
| Go guest, Linux arm64 | 0.79-0.80 µs | 1.05-1.06 µs | 1.02-1.11 µs |
| QuickJS, Linux arm64 | 1.64 µs | 1.92-1.93 µs | 1.89-1.90 µs |
| Go guest, macOS | 0.76 µs | 1.00 µs | |
| QuickJS, macOS | 1.56-1.61 µs | 1.84-1.90 µs | |

wasmtime adds about 0.25-0.3 µs per round trip single-threaded (about 0.13 µs per crossing). Inside
it, per host call, wasmtime-go makes several more cgo calls (one per argument read, the caller
context, the memory's base and size, the result) and takes a process-wide lock twice (question 7).

Cold path (`-mode cold`, Linux arm64): compiling the JS engine 159 ms on wazero (5.7 ms from its
disk cache) against 49 ms on wasmtime; the Go guest 830 ms (28 ms cached) against 176 ms; a
wasmtime module serializes to 2.1 MB in 0.2 ms and deserializes in 0.4 ms; an instance with the
bundle preloaded 9.8 ms against 7.4 ms.

## 6. cgo and build isolation

Measured on a copy of HEAD in a scratch directory with a `//go:build wasmtime` file in
`internal/functions/engine` importing wasmtime-go (the phase-4 shape), `go get` and `go mod tidy`
(`build-isolation.sh`, `results/build-isolation-darwin-arm64.txt`):

| Check | Result |
|---|---|
| `go.mod` / `go.sum` | +1 require line / +2 hash lines |
| `CGO_ENABLED=0 go build ./cmd/fcdev ./cmd/fc-server`, no tag | builds; `go list -deps` has no wasmtime-go package |
| the same with `-tags wasmtime` | fails to compile (wasmtime-go's files are all cgo): the tag file should be `wasmtime && cgo`, with a stub for `wasmtime && !cgo` that says so |
| `-tags wasmtime`, cgo, Go 1.27.2 | builds |
| `-tags wasmtime`, Go 1.27.4 | **refused**: `disallowed LTO section __LLVM,__bitcode` (macOS) / `".llvmbc"` (Linux) |
| fresh module cache: `go build ./cmd/fcdev`, `go vet ./internal/functions/...`, `go test ./internal/functions/engine` | fetch nothing of wasmtime-go |
| fresh module cache: `go mod download`, `go mod tidy` | fetch the **72 MB** zip (269 MB unpacked). Note the current `Dockerfile` runs `go mod download`, so the pure-Go image would download it too unless that step names its modules. |

**The Go 1.27.4 refusal.** Go 1.27.4 (2026-11-02) scans every library in a `#cgo LDFLAGS -L`
directory inside the module cache or the working tree and rejects objects with LTO or bitcode
sections (`cmd/go/internal/work/security.go`, `checkLinkerObjectRegion`). Go 1.27.2 and 1.26.3 have
no such check (their `security.go` has no "disallowed LTO"). Every prebuilt `libwasmtime.a` in
wasmtime-go v50.0.0 has them: `.llvmbc`/`.llvmcmd` in 267 (linux-aarch64), 274 (linux-x86_64) and
454 (windows) members, `__LLVM,__bitcode` in 229 / 237 (macOS). There is no opt-out flag. What works
(the spike's `Dockerfile`, Go 1.27.4): copy the module into the build tree, `objcopy
--remove-section=.llvmbc --remove-section=.llvmcmd` on the target's library, `replace` the module to
the copy. The check still scans the copy and passes. A path outside the working tree, the module
cache and `$TMPDIR` is not scanned at all; that is a loophole, not a fix. Building `libwasmtime.a`
from source without bitcode (`-C embed-bitcode=no`) would also do; not tried. Not checked: whether
upstream wasmtime-go or Go already track this.

**What the module ships:** static libraries for linux-x86_64 (68.8 MB), linux-aarch64 (60.1 MB),
macos-x86_64 (41.9 MB), macos-aarch64 (39.3 MB), windows-x86_64 (70.8 MB); linux-gnu only, no musl,
no other architectures. Licence: Apache-2.0 (the binding's `LICENSE`); the licences of the Rust
crates linked into `libwasmtime.a` were not audited. Releases: one major a month, each with a new
import path (`/v46` 2026-06-29, `/v47` 07-20, `/v48` 08-20, `/v49` 09-21, `/v50` 10-21), occasional
patch releases (v43.0.2, v46.0.1); v50.0.0 is the newest on the proxy today. So an upgrade is an
import-path change every month, and with the fork recommended below, a rebase.

**The Linux image:** cgo needs a C toolchain (present in `golang:*-bookworm`) and glibc (the
libraries are linux-gnu; the current runtime image is Alpine/musl, so it would move to a glibc base;
musl was not tested because it needed Alpine packages from outside the allowed sources). The current
`Dockerfile` cross-compiles from the build platform with `CGO_ENABLED=0`; with cgo each architecture
is built natively (or under emulation: the amd64 image built in 78 s here) or with a cross C
toolchain. The binary links `libc.so.6`, `libm.so.6`, `libgcc_s.so.1` dynamically. Size: the
smallest program that loads the JS engine (`cmd/sizeprobe`, `-s -w`) is 7.8 MB on wazero and 24.5 MB
on wasmtime on arm64 (+16.7 MB), 8.3 and 29.3 MB on amd64 (+21.0 MB).

## 7. Threads and concurrency

| Question | Answer (measured unless marked) |
|---|---|
| Calls and OS threads | A call into wasm is a blocking cgo call and holds its OS thread for its whole length. 64 concurrent calls took the process to 90-95 threads (wazero: 17-18, bounded by GOMAXPROCS); Go does not give the threads back (they stayed after the run). An idle wasmtime engine has 20-31 threads against wazero's 11-16 (wasmtime's compile workers, inferred). |
| Where the epoch ticker lives | One goroutine per engine calling `Engine.IncrementEpoch` (a cgo call). |
| Can a Store move between goroutines | Yes: one instance used by 200 goroutines in turn, each locked to a new OS thread (200 distinct thread ids), 200/200 answers correct, on all three platforms (`-mode migrate`). Never concurrently. |
| What it means for a pool at concurrency N | N in-flight calls = N OS threads running wasm, scheduled by the kernel, not by Go. Host callbacks have to get a Go P back each time (GOMAXPROCS), which is where they queue. |

`-mode conc`, each goroutine its own warm instance, 5 s (W3) / 4 s (W5):

| | wazero c=1 / 14 / 64 | wasmtime c=1 / 14 / 64 |
|---|---|---|
| W3 QuickJS, Linux arm64, calls/s | 51-53 / 422 / 508-515 | 84-85 / 857-891 / 840-878 |
| W3 p99 at c=64 | 259-266 ms | 117-122 ms |
| W5 QuickJS, Linux arm64, calls/s | 18.3k / 113k / 159k | 19.9k / **58.6k / 57.4k** |
| W5 Go, Linux arm64, calls/s | 33.7k / 240k / 257k | 33.1k / **71.0k / 66.9k** |
| W5 p99 at c=64, Linux arm64 | 0.58-0.65 ms | **19-21 ms** |
| W5 Go, macOS, calls/s | 30.7k / 293k / 299k | 28.7k / 52.6k / 45.2k |

W3 (no host calls) scales on both and wasmtime keeps its 1.6-2x. W5 does not scale on wasmtime. CPU
and mutex profiles (`-cpuprofile`, `-mutexprofile`) put it in wasmtime-go: `getDataInStore` takes a
process-wide `sync.Mutex` (`gStoreLock`) to look a store's Go data up in a global map, on every
`Func.Call` and twice per host call (in the trampoline and in `Caller.Data`), and engine-level host
functions take a second global lock (`gEngineFuncLock`); 67% of mutex delay, and 60-67% of CPU in
`pthread_cond_wait/signal` behind it.

| W5, macOS, calls/s at c=14 / 64 | QuickJS | Go |
|---|---|---|
| wazero | 151k / 158k | 293k / 299k |
| wasmtime-go v50.0.0 | 51k / 43k | 53k / 45k |
| + host state through a `sync.Map` (wt-map; spike code only) | 72k / 58k | 72k / 62k |
| + both global maps as `sync.Map` in a patched copy of the binding (`results/wasmtime-go-store-lock-patch.diff`, 45 lines) | **111k / 116k** | **138k / 137k** (p99 at 64: 2.1 ms) |

The patched binding was measured on macOS only. What remains after it (about half of wazero's rate
for the Go guest) is cgo callback cost: re-entering Go, getting a P, thread hand-offs.

## 8. What the phase-2 engine interface has to cover

From building `wtengine` beside the production engine; the spike's own three-method interface
(`cmd/spike/rt.go`: load, instantiate, handle) was enough to run every measurement, and is a first
draft.

| Concern | wazero today | wasmtime | So the interface must |
|---|---|---|---|
| Deadline | two runtimes fixed at build (preemptible for Wasm, cooperative for the JS engine), `CompileCooperative` refuses a module without `fc_js_deadline`, in-guest interrupt, watchdog, `ErrStuck`, `Stuck()`, `Healthy()`, `StuckLimit`, `ErrCloseStuck` | one mechanism for every module: epoch deadline per call; nothing can outlive it, so no watchdog, stuck count or abandon path; arming `fc_js_deadline` stays optional (cleaner stop for JS that polls, same outcome) | take the deadline from the context and own the mechanism; keep `Stuck/Healthy` as engine methods (always 0 / true on wasmtime) or move them out of the runner's contract; "cooperative" becomes a wazero detail, not a caller's choice |
| Epoch clock | n/a | a ticker per engine that follows the monotonic clock (above), started and stopped with the engine; idle cost 1-8% of a core | be closable; tick period a setting |
| Compile cache | `wazero.CompilationCache` on a directory, keyed by module ID including the termination setting (`TestSharedCacheNeverCrossesRuntimes`) | `Module.Serialize` / `NewModuleDeserialize`, valid only for the same wasmtime version and config (epochs on or off) | key a cache by engine, engine version and config, not by wasm bytes alone |
| Instances | one module instance per wazero `Module`; `Calls`, `MemoryBytes`, `Broken` drive the pool's recycling (`pool.go`) | one `Store` per instance; `Store.Close` frees it; a store may move between goroutines but must not be closed under a running call (it cannot outlive the epoch, so that is simple) | keep `Calls`, `MemoryBytes`, `Broken`, `Close`; discard after any outcome error, as now |
| Host functions | a host module per runtime; per-call state through a context value; a Go panic becomes an error | defined once on a linker; per-call state through the store (`Caller.Data`: global lock) or the embedder's own map; must return a `Trap`, not panic; the context has to be carried in store state | take the `Host` per call (as `Handle` does) and leave the plumbing inside |
| WASI preview1 | wazero's, with `io.Writer` stdout/stderr (the runner's per-invocation trace sink), injectable clocks, sleep and random | wasmtime's own: stdout/stderr only to a file path or inherited; its own clocks and random | keep `Stdout/Stderr io.Writer` in `InstanceConfig`; wasmtime needs a pipe per instance or a Go `fd_write` shadowing WASI's (`Linker.AllowShadowing`); not built |
| Memory | `budget.InstanceMemory` is a wazero `experimental.MemoryAllocator`: per-grow charge, `Refused` | store limiter only; per-grow charging needs the C API's memory creator (not in the binding) | move the allocator out of `budget` into the wazero engine; express "memory cap" and "charge to budget" separately; report out-of-memory as an outcome the engine decides |
| Traps | `sys.ExitError` codes (incl. deadline codes), wazero error strings; recursion is "out of bounds memory access" after ~10 ms | `*wasmtime.Trap` with a code (`Interrupt`, `StackOverflow` for recursion in <1 ms, ...), `*wasmtime.Error` with `ExitStatus()` | map to the shared outcome errors (`ErrDeadline`, `ErrTrap`, `ErrOutOfMemory`, `ErrExit`, `ErrMalformed`); conformance compares outcomes, never messages |
| Module check | `check.go` on wazero's `CompiledModule` | the same rules on `Module.Exports/Imports` (ported in `wtengine.check`) | describe a compiled module's exports, imports and memory so `check` is written once |
| Threads | goroutines, bounded by GOMAXPROCS | one OS thread per in-flight call, kept by Go afterwards | the runner's concurrency limits are the thread bound; say so |

What in the current code assumes wazero (to change in phase 2): `budget/allocator.go` (imports
`wazero/experimental`); `engine/engine.go`, `host.go`, `check.go`; the concrete `*engine.Engine`,
`*engine.Module`, `*engine.Instance` in `runtimes/runtimes.go` (Loader, `Prepared`), `runner/pool.go`
and `runner/runner.go`; `engine.New` in `runner.New`, `cmd/fcdev/fn.go` and
`server/wire_services.go`; `Healthy()` gating `/healthz`/`/readyz` and the `fc_fn_stuck_calls` gauge
(`invoke.go`, `metrics.go`); `engine.LoadError` in `platform/function/operations/publish.go` (a shared
type, fine to keep). The runner's error handling (`errors.Is(..., engine.ErrDeadline)` and the rest)
already works unchanged with wasmtime because `wtengine` returns the same sentinel errors.

## Not tested

- The real runner end to end (`runner.Runner` is wired to `*engine.Engine`); only `log` and
  `config.get` behind a copy of the runner's logic; no http fetch, emit, dispatch, database, trace or
  platform-token ops.
- musl / Alpine; Go 1.27.4 on macOS with stripped libraries (Mach-O stripping not attempted); a
  `libwasmtime.a` built from source; the pooling allocator, the memory creator, the epoch-deadline
  callback (none exposed by the binding).
- Native amd64 timings (the amd64 numbers are emulated); a cloud instance type.
- The patched binding on Linux; `-race`; a long soak; many engines or functions at once; the cause
  of wasmtime's higher RSS after work.
- Guest stdout/stderr capture into the trace sink.

## Recommendation against the gate

Gate (phase 1): the shipped QuickJS engine and a Go guest through the real ABI v1 host calls,
epoch deadline, memory limit, Linux amd64 and arm64 in Docker; byte-identical outputs, runaway
killed, measured speed.

| Gate item | Met? |
|---|---|
| Byte-identical outputs, real host calls | yes, 13 cases on 3 platforms, 20 host calls per W5 invocation |
| Runaway killed | yes, including BigInt and `indexOf` (wazero: 20 s held, a core leaked), nothing left running, next call fine |
| Memory limit | yes, per call, same outcomes as wazero; budget parity needs more than the binding offers |
| Linux amd64 and arm64 in Docker | yes (amd64 emulated) |
| Speed measured | yes: JS 1.6x, compiled Go 8-10x single-threaded; host-call-heavy work under concurrency 2-6x worse with the stock binding |

The evidence points to **proceeding with wasmtime-go, not switching to Rust**: everything that went
wrong is in the thin binding layer (a lock, unexposed C API, prebuilt-library packaging), each
fixable in a few dozen lines of Go and C or a build step, while the ABI host stays the one Go
implementation. The Rust route would re-implement that host to avoid problems this spike did not
find in wasmtime itself.

What the plan did not foresee, and phase 4 needs:

1. **Own a fork of wasmtime-go** (or land the changes upstream first): the global-lock fix (measured
   2.5-3x on W5 at concurrency), and exposing the memory creator (budget parity) and limiter or
   epoch callbacks. A monthly rebase on the binding's monthly major.
2. **Package `libwasmtime.a` for Go 1.27.4+**: strip the bitcode sections in the image build (shown)
   or build the library without them; and keep the build on glibc, natively per architecture.
3. **The epoch ticker follows the wall clock**, and its idle cost and tick period are a decision.
4. **Bound in-flight calls per process**: each holds an OS thread.

If the fork is unacceptable to the owner, the stock binding still meets the gate, but runners with
host-call-heavy functions at high concurrency would be slower than on wazero today.
