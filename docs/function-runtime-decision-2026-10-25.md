# Function runtimes: decision and plan (2026-10-25)

Status: **decided by the owner on 2026-10-25**, except the implementation route in section 3, which is
recommended and awaits confirmation. Evidence: `function-runtime-performance-2026-10-19.md`,
`function-runtime-workerd-spike-2026-10-21.md`, `function-runtime-wasmtime-spike-2026-10-21.md`,
`function-runtime-scriptc-spike-2026-10-21.md`, `function-runtime-deadlines.md`. Earlier rulings this
builds on: `runner-plumbing-costing-2026-10-19.md` section 5 (runtime fixed per function, runner-side
protocol adapters). Trust model: functions are first-party code only (2026-10-21); the sandbox is for
fault isolation, not hostile authors.

## 1. Decision

1. **One contract, several engines.** The control-plane protocol (desired state, heartbeat with
   capabilities, artifacts, emit, dispatch), the guest ABI v1 and the invoke listener stay as they are.
   An engine is an implementation detail of a runner.
2. **fcdev runs wazero.** Pure Go, static, cross-compiled, works on Windows. Nothing changes for
   developers.
3. **Deployed runners run wasmtime**, same contract. Reasons: about 2x faster on the QuickJS engine
   we ship (W3 XML 15 KB: 18 ms on wazero, 8.9-10.6 ms on wasmtime), and a reliable kill of a runaway
   compiled guest at its deadline through epoch interruption (15-20% cost), which wazero cannot give
   without a per-loop hook costing 3.7-10x.
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

## 3. Implementation route (recommended, to confirm)

**Recommended: an engine interface inside the existing Go runner, with wasmtime through
`wasmtime-go` behind a build tag, used only in the Linux server images.** The Go runner already
implements the whole ABI v1 host (about 4,400 lines in `internal/functions/{runner,control,abi}`), so
the ABI exists once and both engines run it. cgo is confined to the Linux amd64/arm64 server image
build, which has a C toolchain; `fcdev` and the default `go build` stay pure Go and use wazero.

Alternative considered: a Rust runner speaking the Go protocol v1 with wasmtime natively. It keeps cgo
out entirely but re-implements the ABI v1 host (preview1, fc.call/fc.take, the 11 ops, QuickJS
hosting), which the 2026-10-19 costing sized L and parked (decision 2 there scopes the Rust runner to
components). Choose it only if the wasmtime-go spike (phase 1) fails.

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
large payloads) runs against every engine and runner kind in CI: wazero always, wasmtime on Linux,
workerd when its runner exists. A test that differs between engines is a bug in one of them, or a
documented difference in `function-stability-contract.md`.

## 6. Plan

| Phase | Content | Gate |
|---|---|---|
| 1 | Spike `wasmtime-go`: the shipped QuickJS engine and a Go guest through the real ABI v1 host calls, epoch deadline, memory limit, Linux amd64 and arm64 in Docker | byte-identical outputs, runaway killed, measured speed; if it fails, switch to the Rust route |
| 2 | Engine interface in the Go runner (`Engine` with compile, instantiate, call, limits), wazero behind it, no behaviour change | existing runner tests green |
| 3 | Conformance suite as a reusable harness (section 5), run on wazero | suite in CI |
| 4 | wasmtime engine behind the build tag; server image built with it; suite on wasmtime | suite green on both; benchmark W1-W5 recorded |
| 5 | Runtime `jsv8` in the manifest, SDK and docs; promote notice | a `jsv8` function on a pool without it gets `POOL_LACKS_RUNTIME` |
| 6 | workerd runner kind (process pool per busy function, OS CPU limit, watchdog restart), built when the first function needs it | suite on workerd; one real function migrated |

Not decided here: pool sizing and autoscaling for workerd nodes, and whether busy functions move to
`jsv8` by hand or on a measured threshold (owner: by hand first).
