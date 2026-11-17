# Function runtimes: decision and plan (2026-10-25)

> **Implementation status (2026-11-10): this is a decision record, and most of it is not built.** What
> exists today is the wazero runner (`wasm`) and the JVM runtime, and the platform accepts only the
> runtimes `wasm`, `js` and `jvm` (`ValidRuntime`). The wasmtime engine, the Go engine interface that
> would select between engines, and the `jsv8` (workerd) runtime described below have **not** been
> built; read those parts as the plan, not as the current behaviour.

> **Amended by the owner on 2026-11-17: wasmtime is parked, and if it is taken up it is with a
> Rust-only runner.** Deployed runners stay on wazero for now. When or if they move to wasmtime, the
> runner is a separate binary written in Rust that speaks the control protocol; wasmtime is not
> put inside the Go runner, and the Go binaries get no cgo. This replaces decision 3 and the route in
> section 3 (the Go engine interface with `wasmtime-go` behind a build tag), and with it phases 1,
> 2 and 4 of the plan. The reasons are in section 3. Everything else stands: fcdev on wazero, one
> contract for every engine, workerd only as the opt-in `jsv8`.

Status: **decided by the owner on 2026-10-25**, including the implementation route then in section 3
(the Go engine interface, confirmed the same day), which the owner replaced on 2026-11-17 (see the
note above). Evidence: `function-runtime-performance-2026-10-19.md`,
`function-runtime-workerd-spike-2026-10-21.md`, `function-runtime-wasmtime-spike-2026-10-21.md`,
`function-runtime-scriptc-spike-2026-10-21.md`, `function-runtime-deadlines.md`, and for the
amendment `function-runtime-wasmtime-go-spike-2026-11-17.md`. Earlier rulings this
builds on: `runner-plumbing-costing-2026-10-19.md` section 5 (runtime fixed per function, runner-side
protocol adapters). Trust model: functions are first-party code only (2026-10-21); the sandbox is for
fault isolation, not hostile authors.

## 1. Decision

1. **One contract, several engines.** The control-plane protocol (desired state, heartbeat with
   capabilities, artifacts, emit, dispatch), the guest ABI v1 and the invoke listener stay as they are.
   An engine is an implementation detail of a runner.
2. **fcdev runs wazero.** Pure Go, static, cross-compiled, works on Windows. Nothing changes for
   developers.
3. **Deployed runners stay on wazero for now; wasmtime is parked (amended 2026-11-17).** When or if
   deployed runners move to wasmtime, it is with a Rust-only runner (section 3), same contract. The
   reasons to move are unchanged: about 2x faster on the QuickJS engine we ship (W3 XML 15 KB: 18 ms
   on wazero, 8.9-10.6 ms on wasmtime), and a reliable kill of a runaway compiled guest at its
   deadline through epoch interruption (15-20% cost), which wazero cannot give without a per-loop
   hook costing 3.7-10x. Nothing forces the move today: the limit wazero leaves (a single QuickJS
   builtin call cannot be stopped and pauses its runner while it runs) is written down in
   `function-runtime-deadlines.md` and accepted. Take it up when a real function is too slow or a
   real runaway is seen.
4. **workerd is added only when a JavaScript function needs it.** A function declares the runtime it
   needs (below). No workerd nodes are deployed until a function that needs them is deployed.
5. **The owner does not wait for wide adoption before choosing the better engine**; the risk is managed
   by the conformance suite (section 4), not by deferring.
6. scriptc (TypeScript compiled to Wasm) is not pursued now: 14x faster than QuickJS but common
   libraries (fast-xml-parser) do not run yet. Revisit when they do.

## 2. Engines compared

| | QuickJS in Wasm (today) | workerd (V8) |
|---|---|---|
| Speed | Interpreter. W3 XML: 8.9-18 ms | JIT. W3 XML: about 0.35 ms (about 50x) |
| Per-call limits | Hard CPU and memory limits per call; a runaway call is stopped | No per-call CPU limit in open-source workerd; a heap overrun aborts the process and every call in it (about 29 ms recovery) |
| Isolation | One Wasm instance per call | V8 isolates in one process |
| Compatibility | Modern JS, few web APIs, weaker npm compatibility | Web-standard APIs, bundled npm code runs as on Cloudflare |
| Operations | Inside our binary | Separate large C++ binary per OS; one core per process (a pool); Windows untested |

| | wazero | wasmtime |
|---|---|---|
| Speed | Baseline | About 2x on QuickJS |
| Build | Pure Go, no cgo | Rust (Cranelift); from Go needs cgo (`wasmtime-go`) |
| Limits | Cooperative deadlines; a compiled guest cannot be killed without a costly hook | Epoch interruption, fuel, memory limiter, pooling allocator |
| Maturity | Good | Bytecode Alliance, security process |

## 3. Implementation route (amended 2026-11-17)

**Decided by the owner on 2026-11-17: if wasmtime is taken up, it is a Rust-only runner.** A separate
binary, written in Rust, using wasmtime natively and speaking the control protocol v1 to the
platform, as the echo runner and the planned workerd runner do. Nothing of wasmtime goes into the Go
runner: no `wasmtime-go`, no cgo, no build tag. `fcdev`, `fc-server` and the default `go build` stay
pure Go on wazero.

What it costs, and is accepted with the route: the Rust runner has its own host for the guests it
runs. For ABI v1 core modules (which is what the shipped QuickJS engine and the Go SDK's guests are)
that means the ABI v1 host again in Rust (preview1, fc.call/fc.take, the 11 ops, QuickJS hosting),
which the 2026-10-19 costing sized L and parked; its decision 2 scopes the Rust runner to components
on the Go platform. Whether the Rust runner runs ABI v1 core modules or components only is **not
decided here** and is the first question when wasmtime is taken up. Two hosts for one ABI make the
conformance suite of section 5 a precondition, not an option.

**Replaced: wasmtime inside the Go runner.** The 2026-10-25 route was an engine interface inside the
existing Go runner, with wasmtime through `wasmtime-go` behind a build tag, used only in the Linux
server images, so that the ABI v1 host (about 4,400 lines in `internal/functions/{runner,control,abi}`)
existed once. Its phase 1 spike ran on 2026-11-17 (`function-runtime-wasmtime-go-spike-2026-11-17.md`).
The engine did what was expected of it: byte-identical outputs, every runaway stopped within
milliseconds of its deadline including the two shapes wazero cannot stop, no process pause during a
garbage collection, 1.6x on JS and about 10x on a compiled Go guest. The costs were all in the Go
binding, and they are why the route was dropped:

- Go 1.27.4 refuses to link `wasmtime-go` v50's prebuilt libraries (embedded LLVM bitcode); the image
  build would strip them or build the library itself.
- A process-wide lock in the binding, taken on every call and host call: at 14 concurrent callers the
  JS host-call workload ran 58.6k calls/s against wazero's 113k, with p99 at 64 callers about 20 ms
  against 0.6 ms. Fixing it means owning a patched copy of the binding.
- No hook for the memory budget's per-grow charge.
- The server image moves to glibc and grows by 17-21 MB, `go mod download` fetches a 72 MB module,
  each call in flight holds an OS thread, and the binding has a new major version, on a new import
  path, every month.

workerd runs as its own runner kind (a separate process speaking protocol v1, like the echo runner),
because it cannot be embedded.

## 4. How a function picks its runtime

Built already (commit `67c721dc`, migration 083): `fng_versions.runtime` is a tag, desired state is
filtered by `runtimes=`, heartbeats carry `capabilities.runtimes`, promote refuses `POOL_LACKS_RUNTIME`,
and `FC_FUNCTIONS_RUNNER_URL_<RT>` routes deliveries per runtime. So:

- JavaScript functions default to runtime `js` (QuickJS, on wazero or wasmtime: the engine is the
  runner's business, not the function's).
- A JavaScript function that needs V8 declares runtime `jsv8` in its manifest. It is deployed only to
  pools whose runners advertise `jsv8`. With no such runner, promote answers `POOL_LACKS_RUNTIME`
  and the function shows a notice; it never falls back silently.
- The runtime stays fixed per function (2026-10-19 decision 1).

## 5. Conformance

Two or three engines drift unless the same tests run on all of them. One guest suite (ABI ops, limits
and deadlines, time and randomness, errors and outcome headers, trace records, dispatch declarations,
large payloads) runs against every engine and runner kind in CI: wazero always, the Rust wasmtime
runner and workerd when their runners exist. A test that differs between engines is a bug in one of them, or a
documented difference in `function-stability-contract.md`.

## 6. Plan

As amended on 2026-11-17. Phases keep their numbers.

| Phase | Content | Gate | Status |
|---|---|---|---|
| 1 | Spike `wasmtime-go`: the shipped QuickJS engine and a Go guest through the real ABI v1 host calls, epoch deadline, memory limit, Linux amd64 and arm64 in Docker | byte-identical outputs, runaway killed, measured speed | Done 2026-11-17. The gate was met (amd64 under emulation); the route was dropped for the binding's costs (section 3) |
| 2 | Engine interface in the Go runner (`Engine` with compile, instantiate, call, limits), wazero behind it, no behaviour change | existing runner tests green | Dropped: there is no second engine in the Go runner |
| 3 | Conformance suite as a reusable harness (section 5), run on wazero | suite in CI | Stands; a precondition for any second runner |
| 4 | ~~wasmtime engine behind the build tag~~ A Rust-only wasmtime runner (section 3): a separate binary speaking protocol v1; first question, which guests it runs | suite green on both; benchmark W1-W5 recorded | Parked; nothing built |
| 5 | Runtime `jsv8` in the manifest, SDK and docs; promote notice | a `jsv8` function on a pool without it gets `POOL_LACKS_RUNTIME` | Stands |
| 6 | workerd runner kind (process pool per busy function, OS CPU limit, watchdog restart), built when the first function needs it | suite on workerd; one real function migrated | Stands |

Not decided here: pool sizing and autoscaling for workerd nodes, and whether busy functions move to
`jsv8` by hand or on a measured threshold (owner: by hand first).
