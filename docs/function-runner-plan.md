# Function runner — plan

*Status: in progress — see §0. Greenfield: nothing uses a function service today, so every
contract below (schema, guest ABI, manifest, API) is ours to choose. Written against `main` at
`0cfb595`; §0 tracks what has landed since.*

## 0. Progress (2026-09-28)

| Piece | State | Where |
|---|---|---|
| ABI v1, describe, routing | built | `internal/functions/abi` |
| Engine (wazero, disk cache, fc host module, preload) | built | `internal/functions/engine` |
| Memory budget + mmap allocator + limit detection | built | `internal/functions/budget` |
| Control-plane contract + runner client | built | `internal/functions/control` |
| Runner (invoke, auth, permits, capabilities incl. db/http/emit, reconcile, swap, eviction, heartbeat, public entry) | built | `internal/functions/runner` |
| Runtimes (`wasm`, `js` on one shared QuickJS engine) | built | `internal/functions/runtimes`, `jsengine`, `clients/fn-js-engine` |
| Go, Rust and TypeScript guest SDKs | built, proven through the runner | `clients/fn-go`, `clients/fn-rust`, `clients/fn-ts` |
| Platform registry, publish, versions, aliases, settings, promote wiring | built | `internal/platform/function` |
| Control-plane server | built | `internal/platform/function/control` |
| fc-server subsystem, fc-dev wiring | built | `internal/server/functions.go`, `cmd/fcdev` |
| `fcdev fn` init/build/describe/run/publish/promote/deploy/set/invoke/status | built | `cmd/fcdev/fn*.go` |
| End-to-end on fc-dev (deploy → event → signed delivery → function → emitted event) | verified 2026-09-28 | — |
| Terminal reject outcome; scheduled jobs honour 429 (§13.2, §13.3) | built | dispatch-job processing, scheduled-job dispatcher, migration 060 |
| Admin UI pages (functions list, detail: versions, aliases, declarations, settings) | built, live-verified in Chrome | `frontend/src/pages/functions` |
| Public routes: domain claims and route materialisation (platform side) | built | `internal/platform/functiondomain`, `internal/platform/function` (route.go, repository_routes.go, operations/routes.go), `internal/platform/function/control/document.go` |

Owner decisions §13.1–13.3 were taken as proposed (two-label addresses, the reject outcome,
scheduled-job 429). §13.4 was ruled 2026-09-28: no native runtime for now — Wasm only, with the
native Go option documented in §15 for if it is ever needed.

## 1. What it is

A function is a small WebAssembly module that the platform stores, versions and runs. Developers
write it in Go, Rust or (later) JavaScript against a FlowCatalyst guest SDK, publish it, and
promote a version to `live`. The platform then wires it up: subscriptions deliver events to it,
scheduled jobs fire it, and authenticated callers invoke it over HTTP. There is no server for the
developer to run.

The **function runner** is a new fc-server subsystem (`FC_FUNCTIONS_ENABLED`) that executes those
modules in-process on [wazero](https://wazero.io), the pure-Go WebAssembly runtime. In production
it runs as its own deployment (one per pool) that uses the whole container. In fc-dev it runs
in the same process as everything else.

Five principles, each justified in the sections that follow:

1. **Every invocation is HTTP.** The router and scheduled-job dispatcher deliver to a function
   exactly as they deliver to any webhook target. The runner adds no new delivery path; it is a
   webhook target that hosts many functions.
2. **Code declares what it needs; the platform decides where and how much.** Endpoints, auth,
   subscriptions, schedules, config/secret keys, databases and outbound hosts are declared *in the
   function's code* and read from the artifact itself. Pool, limits and secret *values* are
   deployment settings held by the platform. An artifact cannot drift from its own manifest.
3. **The sandbox is the isolation boundary.** A guest reaches nothing except through host calls the
   runner implements. Credentials (DSNs, secrets, the runner's own token) stay in the runner. The
   runner enforces deadlines by preempting the guest, and enforces memory by refusing growth.
4. **Memory is a budget, never an OOM.** The runner knows its container limit, reserves what the
   host needs, and admits work against the remainder. A guest that wants more memory than the
   budget allows gets a failed `memory.grow`, and only its own call fails. The host is not killed.
5. **Our own ABI, our own SDKs.** A small calling convention designed for wazero, not Extism.
   The measurements (§2) show this is what makes Go fast here.

## 2. The measurements this design rests on

Spikes run 2026-09-28 on an M-series Mac (darwin/arm64), wazero v1.12.0, extism go-sdk v1.7.1,
JDK 25. No HTTP or auth layer. Treat these as relative numbers; phase 0 re-measures on Linux in a
container.

### 2.1 Per guest, on wazero

| Guest | `.wasm` size | Compile (cold) | Compile (disk cache) | Instantiate p50 | Warm call p50 / p99 | Resident per loaded function |
|---|---|---|---|---|---|---|
| Rust, own ABI (serde parse + response) | 85 KB | 22 ms | — | **20 µs** | **8.2 / 22.7 µs** | **2.75 MB** (compiled + 1 instance) |
| Rust via Extism PDK | 278 KB | 51 ms | — | 67 µs | 50 / 485 µs | 4.9 MB (Extism) / 1.7 MB compiled-only in a shared runtime |
| Standard Go (`GOOS=wasip1`, `-buildmode=c-shared`) | 4.4 MB | 1.0 s | **28 ms** | 1.9 ms (Go runtime init) | not measured | 7.9 MB compiled + 3.7 MB linear memory per instance |
| JavaScript (QuickJS via extism-js) | 2.5 MB | 328 ms | — | 574 µs | 153 / 272 µs | 7.6 MB compiled (the engine, per function) |
| **JavaScript on the shared engine (built)** | script only; engine 708 KB | 205 ms, once per runner | — | 2.5 ms (instance + load script) | **72 / 245 µs** | **0.95 MB** (instances only) |
| Go SDK example (`clients/fn-go`, net/http + database/sql) | 7.6 MB | 2.6 s | — | — | ~1 ms over HTTP | — |

Throughput with 8 goroutines on 8 instances: 283k calls/s (Rust, own ABI), 99k (Rust via
Extism), 35k (JS).

A guest in an infinite loop with a 200 ms deadline stopped at 200.2 ms
(`RuntimeConfig.WithCloseOnContextDone(true)`).

### 2.2 What the numbers decide

- **Own ABI over Extism.** The Extism Go SDK moves input and output through kernel host calls
  (one call per 8 bytes). The same Rust work is 6× faster per call (8.2 vs 50 µs), with 3× cheaper
  instantiation and 45% less memory, when the host writes directly into guest memory.
- **One shared `wazero.Runtime` per runner.** Extism creates a runtime and a copy of its kernel per
  plugin. In a shared runtime a Rust function's compiled code is 1.7 MB.
- **A disk compilation cache is mandatory.** A standard-Go guest compiles in 1 s cold and 28 ms
  from `wazero.NewCompilationCacheWithDir`. That makes it cheap to evict idle compiled modules and
  reload them on demand.
- **Instances are reused, not created per call.** Go and JS guests pay 0.6–1.9 ms and several MB
  to start their language runtime, so per-call instantiation is only viable for Rust-like guests.
  See §6.4 for the reuse policy.
- **JavaScript needs a shared engine.** Every JS function currently carries its own 2.5 MB copy of
  QuickJS (7.6 MB compiled). Compiling the engine once and shipping each function as bytecode
  (Javy's dynamic-link mode, or our own equivalent) makes a JS function cost only its instances.
  Phase 3.
- **wazero puts linear memory on the Go heap by default** (`make([]byte, min, cap)`, copy on
  grow). A custom `experimental.MemoryAllocator` gives mmap-backed memory outside the GC, freed
  deterministically on close, and a single choke point, `LinearMemory.Reallocate`, which may
  return nil to fail a grow. The budget (§7) is built on that choke point.

### 2.3 Reference: the JVM implementation on the same guests

For the record, the same `.wasm` files run through the Java function host's real Wasm path
(Endive runtime compiler + Extism, JDK 25, 20k calls of JIT warm-up):

| | JVM (Endive) | This design (wazero, own ABI) |
|---|---|---|
| Rust: compile / instantiate / warm call p50 | 506 ms / 214 µs / 18.7 µs | 22 ms / 20 µs / 8.2 µs |
| Rust: throughput (8 threads) / memory per function | 341k/s / 13.6 MB | 283k/s / 2.75 MB |
| JS: warm call p50 / throughput | 5.8 ms / 279 calls/s | 153 µs / 35k calls/s (Extism build, before the shared engine) |
| JS: functions loadable | ~60 before a 3 GB heap is exhausted | 100 in 812 MB |

The JVM's JS number is structural: Endive compiles each Wasm function to one JVM method, and
QuickJS's interpreter loop exceeds the JVM's 64 KB method limit. So the hot path of every JS call
runs in Endive's interpreter (confirmed with `InterpreterFallback.WARN`: one function, index 486),
and build-time compilation has the same limit. wazero compiles to native code and has no such
limit.

### 2.4 Reference: the Rust host (wasmtime) on the same guests

Measured 2026-09-28 on the same kind of machine (14-core M-series Mac), against the Rust repo's
density study (`docs/function-runner-density.md` there), with harnesses mirroring theirs.

| | Rust host (wasmtime) | Go host (wazero) |
|---|---|---|
| Rust echo guest, **Extism ABI** (native kernel on both): p50, calls/s at c=64 | 5.9 µs, 753k | 39.6 µs, 78k |
| JS echo guest (extism-js QuickJS, the same artifact): p50, calls/s at c=64 | 7.8 µs, 739k | 50.1 µs, 131k |
| **As each runner actually runs:** Wasm warm call | 5.9 µs | 8.2 µs (own ABI, different guest) |
| JS, the same echo logic: p50, calls/s at c=64 | 7.8 µs, 739k | 13.9 µs, 421k (shared engine, own ABI) |
| JS memory per function | 5.7 MB compiled; 0.63 MB fp from `.cwasm` | 0.95 MB (engine shared) |
| JS first call on a fresh instance | ≈0.8 ms (0.41 ms load + 0.42 ms call) | ≈2.5 ms (engine compiled once per runner) |
| Wasm memory per function | 0.64 MB; 0.05–0.15 MB from `.cwasm` | ≈1.7 MB compiled |
| Reload precompiled code | 0.22 ms (`.cwasm` mmap) | 0.74 ms (disk cache) |

Readings: wazero's host-call transition is far more expensive than wasmtime's, which a chatty ABI
multiplies (6–7×); our bulk-frame ABI keeps the real paths within 1.4–1.8×, invisible next to an
HTTP delivery. Density splits — the Go runner's shared JS engine beats per-function QuickJS, while
wasmtime's mmap'd precompiled code beats wazero for Wasm. Cold starts favour the Rust host. Keep
host crossings coarse in any future capability (§5.3).

## 3. Architecture

```
                 ┌──────────────────── platform (fc-server, FC_PLATFORM_ENABLED) ────────────────────┐
 fcdev fn …  ──▶ │ internal/platform/function: functions, versions, aliases, settings, artifacts      │
 (CI, humans)    │   publish = store + describe + validate      promote = materialise wiring          │
                 │ /api/functions/…  (huma, usecaseop)          /control/functions/… (runner role)    │
                 └───────────────┬──────────────────────────────────────────▲─────────────────────────┘
                                 │ creates subscriptions / scheduled jobs    │ long-poll desired state,
                                 │ whose target is the runner URL            │ heartbeat, artifacts, emit
                                 ▼                                           │
 router / dispatch ── signed POST ──▶ ┌──────── function runner (FC_FUNCTIONS_ENABLED) ───────────────┐
 scheduled jobs ──── signed POST ──▶ │ one wazero.Runtime · disk compile cache · mmap allocator       │
 API callers ─────── bearer ───────▶ │ budget (memory) · per-function permits · instance pools        │
                                     │ :8095 /fn/{address}[@{alias}|@v{n}]/{path}                     │
                                     │ host calls: log · config · secret · http · emit · db           │
                                     └────────────────────────────────────────────────────────────────┘
```

- **Deployment.** A subsystem toggle like the others in `internal/server/envcfg.go`, started from
  `internal/server/subsystems.go` (`StartFunctionRunner`). A runner-only deployment
  (`FC_PLATFORM_ENABLED=false FC_FUNCTIONS_ENABLED=true`) needs no database. Like a router-only
  deployment, it talks to the platform over HTTP, authenticating with client credentials
  (`pkg/fcsdk/auth.NewClientCredentialsProvider`) as a service account holding the role
  `platform:function-runner`. One deployment per pool.
- **fc-dev.** A `--functions` flag (default on) in `cmd/fcdev/start.go`. The runner runs
  in-process and uses the same control-plane client over loopback, so there is one code path.
  Artifacts are stored under the fc-dev state directory. No JDK, no child process.
- **The platform never runs guest code**, with one exception: at publish it instantiates the
  module with **no capabilities** to read its manifest (`fc_describe`, §5.1). That is safe because
  the module is sandboxed and every host call returns `CAPABILITY_UNAVAILABLE`.

## 4. The function model

| Concept | Definition |
|---|---|
| **Function** | Owned by an application (and optionally a client). Identity is the **address** `{applicationCode}.{name}`: two DNS labels, immutable, unique. `name` matches `^[a-z][a-z0-9-]{0,62}$`. |
| **Version** | An immutable artifact (sha256 digest) plus its describe document, numbered 1, 2, 3… per function. Status: `PUBLISHED` → `READY` (a runner loaded it) or `FAILED`; later `RETIRED`. Publishing the same digest twice returns the existing version. |
| **Alias** | A named pointer to a version. `live` drives wiring (subscriptions, schedules). Other names (`canary`, `qa`) are HTTP-only. |
| **Describe** | The function's declared needs, read from the artifact (§5.1). |
| **Settings** | Platform-held, per function: pool, warm, limits, config values, secret values, DB bindings (declared DB name → secret holding a DSN). Changing settings needs no new version. |

**Publish validates; promote materialises.** Publishing stores the artifact, reads the describe
document and validates it. It creates nothing else. Promoting a `READY` version to `live`
reconciles the platform objects so they match exactly what describe lists, in one transaction:
subscriptions and scheduled jobs owned by the function are created, updated or deleted, and the
function's dispatch pool is created if missing. Rolling back to an older version restores its
wiring. Promote refuses when a declared config/secret key or DB binding has no value
(`SETTINGS_MISSING`).

## 5. Guest ABI v1

### 5.1 Guest exports

| Export | Signature | Contract |
|---|---|---|
| `memory` | memory | The guest's linear memory. Exactly one. |
| `fc_abi_v1` | `() → ()` | Marker. Its presence declares ABI v1; the runner refuses modules without a known marker (`LOAD:ABI_UNKNOWN`). |
| `fc_alloc` | `(size i32) → ptr i32` | Allocate `size` bytes for the host to write into. The guest owns and frees them. |
| `fc_handle` | `(ptr i32, len i32) → i64` | Handle one request frame. Returns `ptr<<32 \| len` of a response frame in guest memory, valid until the next call into the guest. |
| `fc_describe` | `() → i64` | Returns `ptr<<32 \| len` of the describe JSON (§5.4). Must not depend on any host call. |
| `_initialize` | `() → ()`, optional | wasip1 reactor initialiser. Called once per instance, before anything else. |

### 5.2 Frames

Every payload in both directions uses one layout, so bodies are never base64-encoded:

```
u32 little-endian  metaLen
metaLen bytes      meta   (UTF-8 JSON)
remaining bytes    body   (raw)
```

Request meta:
`{"id","address","version","method","path","rawQuery","headers":{k:[v]},"route","pathParams":{},"caller":{…},"deadlineUnixMs"}`.
`route` is the matched endpoint pattern.
`caller` is one of these:

- `{"kind":"webhook"}` (signature verified)
- `{"kind":"anonymous"}`
- `{"kind":"principal","id","type","tier","clients","roles","applications","allApplications","permissions"}`
  (verified claims; `email`/`name` withheld)

Response meta: `{"status", "headers":{k:[v]}}`.

### 5.3 Host imports: module `fc`

| Import | Signature | Contract |
|---|---|---|
| `call` | `(op i32, ptr i32, len i32) → i64` | Run capability `op` with the request frame at `ptr/len`. Returns `code<<32 \| resultLen`. `code` 0 means success, otherwise an error code. The result frame (or an error frame `{"code","message"}`) is buffered host-side. |
| `take` | `(dst i32) → ()` | Copy the buffered result into guest memory at `dst`; the guest has allocated `resultLen` bytes. Clears the buffer. |

Two calls instead of the host calling back into `fc_alloc` avoids re-entrancy and keeps the host
independent of the guest's allocator.

| op | Capability | Request meta / body | Result meta / body |
|---|---|---|---|
| 1 | `log` | `{"level","msg","attrs"}` | — |
| 2 | `config.get` | `{"key"}` | `{"found"}` / value |
| 3 | `secret.get` | `{"key"}` | `{"found"}` / value |
| 4 | `http.fetch` | `{"method","url","headers","timeoutMs"}` / body | `{"status","headers"}` / body |
| 5 | `event.emit` | `{"type","source","subject","dedupId","correlationId","causationId","messageGroup","contentType"}` / data | `{"eventId"}` |
| 6 | `db.query` | `{"db","sql","params","tx"}` | `{"truncated"}` / rows JSON |
| 7 | `db.exec` | same | `{"rowsAffected"}` |
| 8–10 | `db.begin` / `db.commit` / `db.rollback` | `{"db"}` / `{"tx"}` | `{"tx"}` / — |

Error codes are shared across ops: `NOT_DECLARED`, `NOT_ALLOWED` (http allowlist, event type not
owned), `DEADLINE`, `TOO_LARGE`, `UNAVAILABLE` (retryable), `BAD_REQUEST`, `CAPABILITY_UNAVAILABLE`
(describe-time), plus `DB_*` classes from SQLSTATE (`DB_CONSTRAINT`, `DB_SYNTAX`, `DB_TIMEOUT`,
`DB_UNAVAILABLE`, `DB_ERROR`, `DB_TX_UNKNOWN`). A capability failure is never a trap.

**Allowed imports:** `fc` and `wasi_snapshot_preview1`. Anything else refuses the module at load
(`LOAD:IMPORT_NOT_ALLOWED`). WASI is served by wazero's module with no preopened directories,
empty args and environment, the real clock, crypto random, and stdout/stderr routed to the
function's log at INFO/WARN.

### 5.4 Describe

```json
{
  "abi": 1,
  "endpoints": [
    {"method": "POST", "path": "/events/order-created", "auth": "webhook"},
    {"method": "GET", "path": "/api/orders/{id}", "auth": "platform",
     "cors": {"origins": ["https://app.acme.com"]}},
    {"path": "/healthz", "auth": "none", "maxBodyBytes": 0}
  ],
  "subscriptions": [
    {"eventType": "orders:order:order:created", "path": "/events/order-created",
     "mode": "IMMEDIATE", "maxRetries": 3, "dataOnly": false}
  ],
  "schedules": [{"cron": "*/5 * * * *", "timezone": "UTC", "path": "/events/tick"}],
  "config": ["GREETING"], "secrets": ["STRIPE_KEY"], "db": ["main"],
  "httpAllow": ["api.stripe.com", "*.acme.com"], "emits": ["orders:order:order:shipped"]
}
```

Validation at publish (strict; unknown keys rejected):

- Every endpoint names `auth` (`webhook` | `platform` | `none`); there is no default. A `webhook`
  endpoint is `POST`-only.
- A subscription or schedule `path` must be a literal path that matches a `webhook` endpoint.
- An omitted subscription `mode` means the platform-wide default (`NEXT_ON_ERROR`,
  `common.DefaultDispatchMode`), the same as every other subscription. A default must not quietly
  weaken ordering; a function that wants `IMMEDIATE` says so.
- Every type in `emits` must be owned by the function's application.
- Keys match `^[A-Za-z][A-Za-z0-9_./-]{0,99}$`.
- Paths use Go 1.22 `ServeMux` pattern syntax (`{id}`, `{rest...}`).

## 6. The runner

Package layout (all new): `internal/functions/{abi,engine,budget,runner,reconcile,hostcap}`.

### 6.1 Engine (`engine`)

- One `wazero.Runtime` per runner, configured with `WithCloseOnContextDone(true)`, a
  `CompilationCacheWithDir` under `FC_FUNCTIONS_CACHE_DIR`, and a memory limit of 65,536 pages
  (the per-instance cap is applied by the allocator).
- **Module check at load:** required exports are present and correctly typed, imports are within
  the allowlist, and the declared minimum memory is ≤ the function's `memoryMb`.
- **Allocator:** an `experimental.MemoryAllocator` that reserves `max` bytes with `mmap(PROT_NONE)`
  and commits with `mprotect` on `Reallocate`. It charges each committed delta to the budget and
  returns nil when the budget refuses, so the guest sees `memory.grow` return -1. `Free` unmaps
  and credits the budget. The initial `Reallocate(min)` must succeed, because wazero indexes it
  unconditionally. Admission (§7) therefore reserves the module's minimum *before* instantiating.
  Linux and Darwin via `golang.org/x/sys/unix`; elsewhere fall back to the default allocator
  without grow-gating.

### 6.2 Invocation path (`runner`)

`GET|POST|… /fn/{address}/{path...}` → `live`; `/fn/{address}@{alias}/…` → a named alias;
`/fn/{address}@v{n}/…` → an explicit version. An explicit version requires a platform token with
`platform:function:version:invoke`, and the endpoint's own auth is not applied.

1. Resolve address and version. Unknown address → 404. A version that is known but still
   preparing or failed to load → 503 with `Retry-After`, never 404.
2. Match the endpoint against describe (method + pattern). No match → 404; wrong method → 405.
3. **Authenticate per endpoint:**
   - `webhook`: `pkg/fcsdk/webhook.Validator` with the delivery signing secret from desired state.
   - `platform`: `pkg/fcsdk/auth` token validation against the platform's JWKS.
   - `none`: nothing.
4. Enforce `maxBodyBytes` (default 1 MiB) while reading the body.
5. **Take a permit** (per-function semaphore, `maxConcurrency`). Non-blocking: when exhausted,
   answer `429` + `Retry-After: 1`. Dispatch-job delivery treats 429 as a deferral that spends no
   retry budget (`dispatchjob/processing.advance`).
6. **Admit memory** (§7): if the budget can't cover a new instance's minimum, answer
   `503 Retry-After: 1`.
7. Borrow an instance (§6.4), derive `ctx` with the endpoint's deadline, write the frame, call
   `fc_handle`, read the response frame.
8. Map the outcome:

| Outcome | Status and body |
|---|---|
| Guest response | the guest's status, headers and body |
| Trap, panic or malformed frame | `500` with fixed body `{"error":"FUNCTION_FAILED"}`; the guest's message goes to the log |
| Deadline | `504` |
| Allocator refusal mid-call | `500` (the guest ran out of memory) |

The instance is discarded after any failure.

Every call is observable: `fc_fn_invocations_total{address,version,outcome}`,
`fc_fn_duration_seconds` (histogram), and `slog` attributes
`fn.address`/`fn.version`/`fn.invocation`. Guest logs are emitted as `slog` records with the same
attributes.

### 6.3 Host capabilities (`hostcap`)

Every capability receives the invocation `ctx`, so the deadline flows to pgx, `net/http` and
emit without per-call arithmetic.

- **config / secret:** declared keys only, values from desired state.
- **http.fetch:** a runner-owned `http.Client`. The allowlist is checked against the URL host
  *and* again at dial time on every redirect hop, to stop redirect escapes. Response body capped
  at 16 MiB (`TOO_LARGE`).
- **event.emit:** the runner posts to a control endpoint (§8.3) as itself, naming the function.
  The platform checks the type is in describe `emits` and owned by the application, then ingests.
  The function never holds an application credential.
- **db:** one `pgxpool.Pool` per distinct DSN, shared across functions, bounded per runner
  (`FC_FUNCTIONS_MAX_DB_POOLS`), closed when idle. A transaction is bound to the invocation and
  rolled back by `defer` if still open when the call ends. Rows are capped at 10,000 or 8 MiB
  (`truncated`). Params are bound, never interpolated. SQL and params are never logged.

### 6.4 Instance pools

Each loaded version keeps an idle stack of instances (never shared by concurrent calls):

- **Reuse** a healthy instance after a successful call.
- **Discard** after any trap, deadline, failed grow, or malformed frame.
- **Recycle** after `N` calls (default 10,000) or when committed memory exceeds half the
  instance's cap. This bounds leaks in guest allocators.
- **Trim** idle instances after 60 s.
- **Warm** functions keep one pre-instantiated instance ready at all times.
- A Rust-like guest may opt into `fresh` per call (a setting, not describe): instances are created
  in the background ahead of demand and never reused.

### 6.5 Reconcile (`reconcile`)

- **Long-poll** `GET /control/functions/desired?pool=P` with `If-None-Match` and `wait=30s`. The
  platform answers as soon as the pool's revision changes (§8.3), so a promote takes effect in
  milliseconds. On error, back off and keep serving what is loaded; a platform outage never
  unloads anything.
- **Prepare** each version not yet held: download the artifact by digest (§8.4), verify sha256,
  compile (disk cache), run the module check.
- **Swap new before old:** route to the new live version as soon as it is ready, drain the old
  one's in-flight calls, then close it.
- Warm versions load eagerly. Lazy versions compile on first call and are evicted after
  `FC_FUNCTIONS_IDLE_EVICT` (default 30 min) of no calls.
- **Heartbeat** every 15 s and after every change: per version `LOADED | COMPILED | FAILED(reason)`,
  plus the budget report (§7). The platform marks a version `READY` the first time any runner
  reports it `LOADED`.
- Candidates (`PUBLISHED`, not yet on an alias) are in desired state so a runner proves them
  loadable before anyone promotes.

## 7. Memory and CPU sizing

**Owner guidance:** in production the runner gets all of its container's memory and CPU minus what
the host needs; fc-dev runs everything in about 500 MB.

```
limit   = FC_FUNCTIONS_MEMORY_LIMIT, else the cgroup v2 memory.max, else physical RAM
reserve = FC_FUNCTIONS_MEMORY_RESERVE, default max(128 MiB, 10% of limit)
budget  = limit − reserve
  ├─ linear memory   hard: every committed page is charged; grow refused at the budget
  └─ compiled code   soft: LRU-evicted (idle versions first, never warm ones) when
                     RSS (cgroup memory.current, else process RSS) crosses 90% of limit
```

- **CPU:** Go (1.25+) already derives `GOMAXPROCS` from the cgroup CPU quota. The runner adds no
  CPU knob. Fairness comes from per-function permits and preemptive deadlines.
- **GOMEMLIMIT** is set to `reserve` at runner start (only when the runner is the process's sole
  subsystem), because guest memory and compiled code live outside the Go heap.
- **fc-dev:** `FC_FUNCTIONS_MEMORY_LIMIT` defaults to 256 MiB. That keeps roughly 10–15 Go/JS or
  70 Rust functions compiled at once. Others reload from the disk cache in about 30 ms on first
  call.
- **Heartbeat budget report:** `{limitBytes, reserveBytes, linearBytes, compiledBytes, rssBytes,
  loaded, evictions}`, shown on the function status page.

### 7.1 Default limits

Defaults for new functions, overridable per function up to ceilings in platform config:

| Limit | Default | Notes |
|---|---|---|
| `memoryMb` (per instance) | 64 | A cap, charged by use. A generous cap costs nothing until touched. |
| `maxConcurrency` | 16 | Per function per runner. Host-wide concurrency is bounded by memory, not a count. |
| `timeoutMs` | 30,000 | Per endpoint override allowed (≤ the function's value). |
| `maxBodyBytes` | 1 MiB | Per endpoint. |

## 8. Platform side (`internal/platform/function`)

### 8.1 Schema: `internal/migrate/sql/059_functions.sql`

Tables are prefixed `fng_` (owner decision, 2026-09-28): the Java, Rust and Go platforms share
databases with incompatible function-runner schemas — Java keeps `fn_`, Rust uses `fnr_`, Go uses
`fng_` — until one implementation is chosen and renamed back to `fn_`. The `NOTIFY` channel is
`fng_desired` for the same reason.

| Table | Columns |
|---|---|
| `fng_functions` | `id` (TSID, prefix `fn_`), `application_id`, `client_id` null, `name`, `address` unique, `description`, `pool`, `warm`, `limits` jsonb, audit columns |
| `fng_versions` | `id`, `function_id`, `number`, `digest`, `size_bytes`, `abi`, `describe` jsonb, `status`, `failure` jsonb, `ready_at`, `published_by`, `created_at`; unique `(function_id, number)` and `(function_id, digest)` |
| `fng_aliases` | `function_id`, `name`, `version_id`, `updated_at`, `updated_by`; PK `(function_id, name)` |
| `fng_settings` | `function_id`, `kind` (`CONFIG`, `SECRET`, `DB`), `key`, `value` (config plain; secrets and DSNs encrypted at rest the way service-account/connection secrets are); PK `(function_id, kind, key)` |
| `fng_runners` | `id`, `pool`, `heartbeat_at`, `report` jsonb |
| `fng_pool_revisions` | `pool` PK, `revision` bigint |

`fng_pool_revisions` is bumped in the same transaction as any promote, alias or settings change,
which also issues `NOTIFY fng_desired, '<pool>'`.

Also:

- `msg_subscriptions`: widen `chk_msg_subscriptions_source` to admit `FUNCTION`, and add a
  nullable `function_id`. SDK sync ignores `FUNCTION` rows.
- The scheduled-job table: nullable `function_id`.

Queries go through sqlc (`make sqlc`), the repository through `repocommon`, and every write through
`usecaseop` operations with the locked authz model (coarse permission on the controller, resource
scope in the operation).

### 8.2 Public API (`/api/functions`, huma via `apiroute`)

| Route | Purpose |
|---|---|
| `POST /api/functions` · `GET` (list, filter by application/client/address prefix) · `GET /{id}` · `PATCH /{id}` (settings: pool, warm, limits) · `DELETE /{id}` | Function CRUD. Delete cascades versions, aliases, wiring and artifacts. |
| `PUT /api/functions/{id}/artifacts/{digest}` | Raw upload. Verifies sha256. Idempotent. |
| `POST /api/functions/{id}/versions` `{digest}` | Publish: describe via a no-capability instance, validate, store. |
| `GET …/versions` · `POST …/versions/{n}/retire` | Retiring a version an alias points at is refused. |
| `PUT …/aliases/{name}` `{version}` · `DELETE …/aliases/{name}` | `live` materialises wiring (§4). Refuses non-`READY` versions. |
| `PUT` / `DELETE …/config/{key}`, `…/secrets/{key}`, `…/db/{name}` | Settings values. Secrets are write-only and never returned. |
| `GET …/status` | Runners that hold it, per-version state, last failures. |

**Permissions** (four segments, `platform:function:<resource>:<action>`, so `platform:*:*:*` covers
them): `function:view`, `function:manage`, `version:publish`, `alias:promote`, `secret:manage`,
`version:invoke`, and the runner's own `runner:control`. **Roles:** `platform:function-publisher` (CI) and `platform:function-runner`.
**Reach:** client-owned functions follow `CheckScopeAccess`; platform-owned functions (null
`client_id`) are anchor-only.

New routes go into the lockfile with `make api-bump`; `make api-diff` must pass.

### 8.3 Control plane (`/control/functions`, role `platform:function-runner`)

| Route | Purpose |
|---|---|
| `GET /desired?pool=P&wait=30s` | The pool's document; `ETag` = revision. Waits on `LISTEN fng_desired` with a 5 s re-check fallback. |
| `POST /heartbeat` | Runner report (§6.5, §7). Marks versions `READY`/`FAILED`. |
| `GET /artifacts/{digest}` | `302` to a presigned S3 URL, or streamed from the file store. The runner holds no storage credentials. |
| `POST /events` `{functionId, event}` | Emit on a function's behalf (§6.3). |

Desired document, per function: `address`, `id`, `limits`, `warm`, `webhookSecret`, `config`,
`secrets`, `db` (name → DSN), and versions `[{number, digest, abi, describe, roles:
["live"|"candidate"|"alias:<name>"]}]`. The document holds secrets, so it is never logged. It is
served only to the runner role.

### 8.4 Artifact store

Content-addressed by sha256:

- `file://` (fc-dev default, under the state directory)
- `s3://bucket/prefix` (new dependency `aws-sdk-go-v2/service/s3`; the AWS SDK is already in
  `go.mod`)

Configured with `FC_FUNCTIONS_ARTIFACT_STORE`. The runner keeps a disk cache by digest under
`FC_FUNCTIONS_CACHE_DIR` and re-verifies the digest on every read from disk.

### 8.5 Wiring at promote

- One **dispatch pool** per function (`fn-{address}` with dots mapped to dashes), so a slow
  function throttles only itself.
- **Subscriptions** (source `FUNCTION`, `function_id` set) whose endpoint is
  `FC_FUNCTIONS_RUNNER_URL` with `{pool}` substituted, plus `/fn/{address}{path}`.
- **Scheduled jobs** with the same target URL.
- **Signing:** both delivery paths sign with the application's service account (scheduled jobs
  directly; subscriptions through a connection owned by the application whose service account is
  the application's). Desired state carries that secret as `webhookSecret`, so the runner verifies
  exactly what the platform signs. Work package 8 confirms this against
  `dispatchjob/processing` credential resolution before building on it.

### 8.6 Public routes (as built)

Two new aggregates, both `fng_`-prefixed (migration `062_function_routes.sql`):

- **`internal/platform/functiondomain`** — a claimed hostname **zone** (`fng_domains`): `id`
  (TSID prefix `fdm`), `zone` (unique, lowercase), `client_id` (nullable = platform/anchor-owned).
  A claim is verified by being made — no DNS step. It covers the zone itself and every hostname
  under it (`functiondomain.Covers`). `POST /api/function-domains` refuses `ZONE_OVERLAP` when an
  existing claim by a **different** owner covers the new zone or is covered by it (checked both
  directions); the same owner may claim overlapping zones. `DELETE /api/function-domains/{id}`
  refuses `DOMAIN_IN_USE` while some route's hostname is still covered by the zone. Gated on
  `platform:function:domain:manage`, granted alongside the dispatch-pool admin permissions
  (`platform:messaging-admin`) — claims are an operator action.
- **`internal/platform/function`** (route.go, repository_routes.go, operations/routes.go) — a
  function's own route rows (`fng_routes`, TSID prefix `frt`, FK `function_id` → `fng_functions`
  ON DELETE CASCADE): `hostname`, `path_prefix` (normalised, `"/"` for root), `alias` (nullable =
  live). `PUT /api/functions/{id}/routes` upserts by `(hostname, pathPrefix)` — a write that hits
  an existing row owned by the SAME function is an update; owned by a DIFFERENT function refuses
  `ROUTE_TAKEN` (unique index on `(hostname, path_prefix)` backs this at the DB level too). The
  hostname must be covered by a zone claimed by the function's own client (or, for a
  platform-owned function, a platform zone) — `ROUTE_HOST_NOT_CLAIMED` otherwise. An `alias` must
  either be empty (live) or name an alias that already exists on the function —
  `ALIAS_NOT_FOUND` otherwise. `DELETE /api/functions/{id}/routes/{routeId}` removes one route.
  Both writes bump the function's runner pool revision in the same transaction as settings.go's
  `PutSetting`/`DeleteSetting` do, so a route change reaches runners without waiting for a
  promote. Gated on `platform:function:route:manage`; `GET .../routes` uses the ordinary
  `function:view` permission, like the alias/settings sub-resource lists.
- **Control document:** `internal/platform/function/control/document.go`'s `buildDesired` fills
  `Desired.Routes` with every route owned by a function in the requested pool (`Address` from the
  owning function, `Alias` as stored, `""` = live) — the exact shape
  `internal/functions/runner/public.go`'s `buildRoutes`/`match` already consume. Because the
  ETag is a hash of the whole rendered document, a route change is visible to a held long-poll the
  same way a promote is.

## 9. Guest SDKs

**Go: `clients/fn-go`** (its own module, no dependencies; standard Go 1.24+ with `go:wasmexport`,
which works from a library package). Handlers are plain
`net/http`:

```go
func init() {
    fn.Webhook("POST /events/order-created", onOrderCreated)
    fn.Platform("GET /api/orders/{id}", getOrder)
    fn.Open("GET /healthz", health)
    fn.Subscribe("orders:order:order:created", "/events/order-created")
    fn.Schedule("*/5 * * * *", "/events/tick")
    fn.Config("GREETING"); fn.Secret("STRIPE_KEY"); fn.DB("main")
    fn.HTTPAllow("api.stripe.com"); fn.Emits("orders:order:order:shipped")
}

func main() {}

func getOrder(w http.ResponseWriter, r *http.Request) {
    p := fn.Caller(r.Context()).Principal()   // nil unless auth=platform
    db, _ := sql.Open("fc", "main")           // database/sql driver over fc.call
    res, err := fn.HTTPClient().Get("https://api.stripe.com/…") // RoundTripper over fc.call
    …
}
```

- `fn.Retry(w, d)` writes `429` + `Retry-After`.
- `fn.Emit(ctx, fn.Event{…})` returns `(eventID, error)`; `errors.Is(err, fn.ErrRetryable)`
  distinguishes platform trouble from a refusal.
- `init()` declarations feed both `fc_describe` and the in-guest `ServeMux`.
- TinyGo support is a phase-0 measurement, not a commitment.

**Rust: `clients/fn-rust`** (crate `flowcatalyst-fn`, `wasm32-unknown-unknown`). Same declarations
through a builder, with handlers returning `Result<Response, Error>`.

**JavaScript: `clients/fn-ts`** (in progress) on the shared engine. A JS artifact is one bundled
script defining `globalThis.__fc = {describe, handle}`; the engine protocol is documented in
`clients/fn-js-engine/src/lib.rs`. Versions carry `runtime: "js"`.

**Proof through the runner:** `TestSDKExamples` (`internal/functions/runner`) runs each SDK's
example artifact through the real engine and runner — describe as publish reads it, then calls in
every auth mode. It found three real mismatches the SDKs' own fakes could not. Run it for every SDK
change.

**Local testing (not built yet):** `fntest` would run a guest module in-process on the real engine, with fake
capabilities, from a normal `go test`.

## 10. Developer surface (`fcdev fn …`)

| Command | What it does |
|---|---|
| `fn init --lang go\|rust <dir>` | Scaffold a project. |
| `fn build [dir]` | Go: `GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared`. Rust: `cargo build --release --target wasm32-unknown-unknown`. Then runs `fc_describe` locally and validates. |
| `fn publish <address> <module.wasm>` | Upload + publish. Creates the function on first publish if `--create`. |
| `fn promote <address> <n> [--alias name]` | Point an alias at a version. |
| `fn deploy` | build → publish → wait for `READY` → promote. |
| `fn watch` | `deploy` on every save (fc-dev). |
| `fn invoke <address>[@alias\|@vN] [--path] [--data]` | Call the function; mints its own token from fc-dev's CLI credentials. |
| `fn status <address>` / `fn logs <address>` | Status; logs are tailed from fc-dev's in-process log ring. |

## 11. Security model

- **What may run:** artifacts published by a caller holding `platform:function:version:publish`, verified
  by digest end to end (upload, store, runner cache, runner load). Artifact signing (sigstore-go:
  full bundle, certificate-log and trust-root verification) is a phase-4 option, not a v1
  requirement.
- **What a running function may touch:** only what describe declares and the settings bind.
  Enforced by the import allowlist and the runner's capability implementations: no filesystem, no
  environment, no sockets.
- **What a caller must prove:** per endpoint (signature / platform token / nothing); versioned
  calls also need `version:invoke`.
- **Resource exhaustion:** deadline preemption, grow-gated memory, per-function permits, bounded
  DB pools, response caps. One function's failure never takes down the runner.

## 12. Phases

**Phase 0 — spikes (short, measured, Linux container).**
- mmap allocator with grow-gating: correctness and overhead.
- Standard Go vs TinyGo guest: size, instantiate, memory.
- Javy dynamic-link JS: per-function and per-instance cost.
- wazero disk-cache reload time for large modules.
- Re-run §2's table at 1 and 2 vCPU in Docker, including density at 256 MiB and 2 GiB.

**Phase 1 — core, event-triggered and HTTP.**
- Platform: schema, CRUD, artifacts, publish (describe), promote with wiring, settings, control
  plane (long-poll + heartbeat).
- Runner: engine, budget, pools, private listener, three auth modes, reconcile.
- Capabilities: log, config, secrets.
- SDKs and tooling: `pkg/fn`, `flowcatalyst-fn`, fcdev in-process runner, and `fn`
  init/build/publish/promote/deploy/invoke/watch.
- **Acceptance:** in fc-dev, `fn deploy` a Go function that subscribes to an event type. Emit that
  event from the SDK outbox; the function receives the signed delivery and acks. The dispatch job
  shows `COMPLETED`, and the function's status shows the version `LOADED`. A second version's
  promote swaps with zero failed calls under a constant-rate load. A spinning guest is 504'd at its
  deadline while a neighbour's p99 stays within 2× its solo p99. A guest allocating past the
  budget fails only its own call.

**Phase 2 — capabilities and operation.**
- http, emit, db capabilities, with the `database/sql` driver and `RoundTripper` in the Go SDK (built).
- Status page and function pages in the SPA; metrics dashboard.
- Named-alias HTTP; retire; explicit-version invoke.

**Phase 3 — JavaScript and the public edge.**
- Shared-engine JS guests.
- Public listener: claimed domains, `Host`-based routing, CORS, trusted proxies.

**Phase 4 — optional, not scheduled.**
- Native Go process runtime — deferred by the owner (2026-09-28); the option is written up in §15.
- Artifact signing.
- Autoscaling signals from dispatch-pool depth.

### 12.1 Phase 1 work packages

Sized for one agent each. Every package names its tests and runs a mutation check.

| WP | Scope | Depends on |
|---|---|---|
| 1 | `internal/functions/abi` + `engine`: frames, `fc` host module, module check, compile cache, Rust test fixture (own ABI) built into `testdata/` | — |
| 2 | `budget` + mmap allocator + RSS watchdog; unit tests for grow refusal and free/credit | 1 |
| 3 | Platform schema (migration 059), entities, sqlc queries, repository, CRUD ops + API, artifact store (`file://`, `s3://`) | — |
| 4 | Publish (no-capability describe via WP1's engine) + validation + versions/aliases/settings ops | 1, 3 |
| 5 | Control plane: desired document, revision + `NOTIFY`, long-poll, heartbeat, artifacts route | 3 |
| 6 | `runner` + `reconcile`: listener, routing, auth modes, permits, pools, swap/drain, heartbeat client; `StartFunctionRunner` + envcfg + `docs/environment-variables.md` | 1, 2, 5 |
| 7 | `clients/fn-go` Go guest SDK; `clients/fn-rust` (built; `fntest` not built) | 1 |
| 8 | Promote wiring: dispatch pool, `FUNCTION` subscriptions, scheduled jobs, signing-secret path (confirm §8.5 first) | 4 |
| 9 | fcdev: `--functions`, in-process runner, `fn` commands, acceptance script | 6, 7, 8 |

## 13. Owner decisions needed

1. **Address shape.** `{applicationCode}.{name}` (two labels, proposed), or add a service label.
2. **A terminal outcome for functions.** Today only 401/403 end a dispatch job early; any other
   non-2xx retries to `maxRetries`. Proposal: `fn.Reject(reason)` answers `422` with header
   `FlowCatalyst-Outcome: reject`, which `dispatchjob/processing.advance` treats as terminal on the
   first attempt. This changes delivery semantics for every subscriber that sends the header, and
   in practice only functions will.
3. **Scheduled jobs honouring 429.** The scheduled-job dispatcher counts every non-2xx as a failed
   attempt. Proposal: treat `429` + `Retry-After` as a deferral there too, matching dispatch jobs.
4. **Phase 4 scope.** *Ruled 2026-09-28:* Wasm only for now; a native Go runtime is added later only if
   needed, and §15 records how.

## 14. Non-goals

- Running untrusted third-party code as a multi-tenant public service. The sandbox is strong, but
  CPU fairness is by permits and deadlines, not per-function CPU quotas.
- The WebAssembly component model / WASI preview 2. wazero does not implement it; our ABI is
  small enough to version ourselves.
- A JVM runtime, or compatibility with any other function-service artifact format.
- Subscription filters (the platform has no filter column; out of scope here).

## 15. Option: a native Go process runtime (deferred)

*Not built. Owner, 2026-09-28: add it later only if a real need appears. This section is so that
whoever picks it up starts from a design, not a blank page.*

**When it would be worth it.** A function that needs what the Wasm sandbox cannot give: real
parallelism inside one call (goroutines across cores — a Wasm instance is single-threaded), cgo or
a native library, a dependency that does not build for `wasip1` (most database drivers, anything
using raw sockets), or sustained CPU work where native code beats wazero's compiler by a margin
that matters. Glue code — webhooks, transforms, API calls, queries through the `db` capability —
does not need it.

**Why it is viable in Go at all.** A static Go binary starts in milliseconds and idles at roughly
10 MB, so one process per function version is affordable; the same model on the JVM is not.

**Shape.**

- **Artifact:** a static `linux/amd64` or `linux/arm64` binary (`CGO_ENABLED=0` by default), one per
  architecture a pool runs; a version carries `runtime: "native"` and the digest per architecture.
- **Contract:** the binary serves plain HTTP on a Unix socket it inherits as file descriptor 3
  (socket activation). The runner creates and listens on the socket *before* starting the process,
  so the kernel queues connections until the function accepts them: lazy start needs no
  readiness probe. Request meta (caller, invocation, deadline) travels as headers; the SDK is the
  Go SDK's `net/http` API unchanged, served from the socket instead of `fc_handle`.
- **Describe:** `binary --fc-describe` prints the describe document and exits, run by publish in a
  throwaway sandbox (no network, a short deadline).
- **Capabilities:** config and secrets are read from the runner over a second socket at start (never
  environment variables, which other same-user processes can read); `emit` and `log` go to the
  runner as today; databases and HTTP are the function's own — it holds the DSN and opens its own
  sockets. This is the one real change in the security model: a native function is trusted code
  published by CI, and `httpAllow` becomes a convention unless the pool runs it in a network
  namespace.
- **Lifecycle:** start on first call, stop after the idle timeout, `warm` keeps one running; a crash
  restarts with backoff; a version swap starts the new process, routes to it, then sends the old one
  `SIGTERM` after its in-flight calls drain.
- **Isolation and limits:** one process per version, a separate uid where the host allows. Where
  the runner can create cgroups (EC2, Kubernetes with delegation — not Fargate), each process gets
  `memory.max` and `cpu.max`, which gives real noisy-neighbour isolation the Wasm runtime cannot.
  Elsewhere: `GOMEMLIMIT` set for the child, an RSS watchdog that kills it over its limit, and the
  runner's per-function permits and deadlines as today. A deadline cannot stop a goroutine inside
  the process; a function that repeatedly overruns is killed and restarted.
- **Budget:** a native function is charged its memory limit against the runner's budget while its
  process runs, since the runner cannot see inside it.

**Cost to build.** Roughly the runner's `runtimes` seam plus a process supervisor (start, socket,
restart, drain, watchdog), a `runtime: "native"` path through publish, and the SDK's socket entry
point — perhaps a third of the Wasm runtime's size, most of it the supervisor and its tests.

