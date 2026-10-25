# Function stability contract

*Status: proposed for owner ruling (2026-10-19). Written from the code at `ef335f50` and the plans
named in section 8. The contract has two kinds of statement: **IN FORCE** (true of the code today
and now promised) and **PROPOSED** (does not exist yet; needs an owner decision, section 6). Anything
the author could not confirm in code is marked **UNVERIFIED**.*

## 0. Why this exists

The owner is going to write hundreds of functions. Two things must never force a change to their
source:

1. A platform, SDK or runner release in the normal course of events.
2. A change of runner: wazero to wasmtime, QuickJS to V8, a Rust or Java host, a native process.

The rule that makes both true is simple: **function source depends only on the SDK's public API,
and the SDK depends on the ABI, the describe document and the JS environment, which are frozen
and versioned.** Everything below that line (wazero, QuickJS, fuel, memory accounting, the control
protocol) is *runner-defined* and function source must not depend on it.

```
 function source
   |  uses
 SDK public API (fn-go / fn-ts / fn-rust)          layer 1   semver; only an SDK major breaks source
   |  emits                          |  calls
 describe document (layer 2)         ABI v1 binary contract (layer 3)    frozen, additive only
                                     JS environment contract (layer 4)   frozen per jsEnv number
   |
 platform-facing semantics (layer 5): auth, signing, caller, dispatch, declarations, limits
   |
 runner-defined (layer 6): NOT promised -- wazero, QuickJS, fuel, memory, CPU, scheduling
```

## 0.1 Trust model

**Trust model (owner ruling 2026-10-21).** Functions are first-party code. Everything a
function runs is written, reviewed and published by the people who operate this platform, in
order to provide services to their customers. Customers never publish or run their own code on
FlowCatalyst, there are no plans for them to, and nothing here may be built, documented, exposed
or configured to allow it. The sandbox (WebAssembly, capability declarations, limits, deadlines)
exists for fault isolation, least privilege and fair sharing between our own functions. It is not
designed, hardened or tested to contain a hostile author. Treat any idea of letting a customer or
other third party run code here as a new product that needs a new owner decision first: it would
need real hardening (VM-level isolation, per-tenant pools, CPU and memory quotas), which this
system does not have.

Consequences that follow from it: a runner may rely on a function to be well-behaved in ways it
could not rely on a stranger (for example, a cooperative deadline with a watchdog instead of
preemption); publishing and promoting versions are privileged operations for the platform's own
staff; no feature should be designed around a customer authoring a function.

## 1. What is promised, and to whom

| Layer | Promised to | Promise | Breaks only on |
|---|---|---|---|
| 1. SDK public API | function authors | Source written against a released SDK minor builds and behaves the same on every later release of the **same major** | SDK major bump |
| 2. Describe document | platform, runners, SDKs | A describe document valid today is accepted by every later platform and runner for the support window (section 2.6) | describe version bump (PROPOSED field, see 2.3) |
| 3. ABI v1 | SDK authors, runner authors | A compiled module built for ABI v1 loads and runs on every runner that claims `wasm`, unchanged | ABI v2 (different marker export), after the support window |
| 4. JS environment | function authors (TS) | Exactly the globals in section 1.2 exist, with the stated behaviour, on every runner that claims `js` | `jsEnv` number bump (PROPOSED) |
| 5. Platform-facing semantics | function authors, operators | Auth, signing, caller, dispatch, declarations and portable limits behave as in section 1.3 | owner decision (section 6) |
| 6. Runner-defined | nobody | Nothing. Documented so nobody builds on it by accident | any release |

### 1.1 Layer 1: function source and SDK public API

**IN FORCE (as a rule; the SDKs are all pre-1.0, see below).** The public API of an SDK is
everything its README and `docs/published/60-functions.md` name: the declaration functions, the
handler signature, `Request`/`Caller`/`Principal`, the response helpers, the capability calls and the
error types. Concretely:

| Concept | Go (`clients/fn-go`) | TypeScript (`clients/fn-ts`) | Rust (`clients/fn-rust`) |
|---|---|---|---|
| Endpoints | `Webhook`, `Platform`, `Open` | `fn.webhook`, `fn.platform`, `fn.open` | `App::webhook`, `platform`, `open` |
| Wiring | `Subscribe`, `Schedule` | `fn.subscribe`, `fn.schedule` | `subscribe`, `schedule` |
| Needs | `Config`, `Secret`, `DB`, `HTTPAllow`, `Emits`, `Dispatches` | `fn.config`, `secret`, `db`, `httpAllow`, `emits`, `dispatches` | `config`, `secret`, `db`, `http_allow`, `emits`, `dispatches_with` |
| Capabilities | `Emit`, `Dispatch`, `HTTPClient`, `Log`, `database/sql` driver `"fc"` | `fn.emit`, `fn.dispatch`, `fn.fetch`, `fn.log`, `console` | `Context::{config_get, secret_get, http_fetch, emit, dispatch, db_*, log*}` |
| Trace logs (additive; SDKs unreleased) | `TraceContexts`, `Trace`, `Tracing`, `SetReference`, `WithLevel`, `WithReference` | `fn.traceContexts`, `req.trace`, `req.tracing`, `req.setReference` | `trace_context`, `Context::{trace, tracing, set_reference}` |
| Outcomes | `Retry`, `Reject` | `fn.retry`, `fn.reject` | `Response` helpers |
| Testing | `FakeHost`, `InvokeForTest`, `DescribeForTest` | `testing.ts` (`installFakeHost`; not exported from the index) | `testing.rs`, `FakeHost` |

What is **not** public API and may change in any release: package-internal files (`abi_wasip1.go`,
`native.ts`, `host.rs`, frame codecs), the `export!` macro's expansion, the shape of the bundle
`fc-fn-build` produces, `globalThis.__fc`, `__fc_call`, `__fc_utf8_*`, and the test seams. Function
source must never import or reach for them.

**Known state (verified):** `clients/fn-go/VERSION` = 0.1.0, `clients/fn-ts/VERSION` = 0.3.0
(`package.json` is also `private: true`), `clients/fn-rust/VERSION` = 0.0.0 (`Cargo.toml` 0.1.0, the
two disagree). Under semver, 0.x promises nothing. **PROPOSED:** each SDK is declared 1.0.0 on the
day this contract is adopted, so that "an SDK major bump is the only place source breaks" is true
from the first function the owner writes at scale. Until then the promise is made by this document,
not by the version number.

**Cross-language rule (PROPOSED).** The same declaration has the same name, same defaults and same
error behaviour in all three SDKs (the README of fn-ts already says it mirrors fn-go and fn-rust).
A new capability or option lands in all three, or in none, before it is documented as public.

### 1.2 Layer 4: the JavaScript runtime environment

What a TypeScript function actually runs in is **QuickJS (rquickjs 0.11, `Context::full`) inside
`clients/fn-js-engine`, compiled to wasm and run on wazero.** The engine registers exactly three
natives (`__fc_call`, `__fc_utf8_encode`, `__fc_utf8_decode`, `lib.rs`), loads the bundle with
`eval`, drains pending promise jobs, and then calls `globalThis.__fc.describe()` / `.handle()`.
The bundle is an esbuild IIFE, `--platform=neutral --target=es2020` (`bin/fc-fn-build.js`).

Everything not registered by the engine or installed by the SDK is whatever QuickJS itself ships.
The contract names three classes. It is derived from reading the engine, the SDK and its README
(the SDK comments say QuickJS has no `console`, `TextEncoder`, `Headers` or `URLSearchParams`).
**UNVERIFIED:** no test probes the live engine for the absence or presence of any global; the
conformance backlog (section 5.2, C-40..C-44) adds that probe.

| Facility | Class | Notes |
|---|---|---|
| ECMAScript language and built-in library **up to ES2020**: `Object`, `Array`, `String`, `Number`, `Math`, `JSON`, `Date`, `RegExp`, `Map`/`Set`/`WeakMap`/`WeakSet`, `Symbol`, `Promise`, `async`/`await`, generators, `Proxy`/`Reflect`, typed arrays/`ArrayBuffer`/`DataView`, `BigInt` | **Guaranteed** | The bundle target is ES2020, so syntax is down-levelled by esbuild; the *library* is what QuickJS provides (ES2020 library is the promised floor; QuickJS has more). |
| `console.log/info/warn/error/debug` | **Guaranteed** | Installed by the SDK when no `console` exists; routed to host op 1 (INFO/INFO/WARN/ERROR/DEBUG). Arguments are joined with a space; non-strings are `JSON.stringify`'d. No format specifiers, no `console.table/time/group/assert`. |
| `fn.log`, `fn.fetch`, `fn.emit`, `fn.dispatch`, `fn.config/secret/db` | **Guaranteed** | These are SDK API, not web APIs. |
| `fn.fetch(url, {method, headers, body, timeoutMs})` returning `FnResponse {status, headers, text(), json(), bytes()}` | **Guaranteed** | It is **not** the web `fetch`: no `Response`, no streams, fully buffered, only allow-listed hosts. |
| `Uint8Array` as the byte type; UTF-8 via the SDK's own helpers | **Guaranteed** | Function code should use `req.text()/json()/bytes()` and `fn.text/json`, not `TextEncoder`. |
| Async: a handler may return a Promise; `await` on SDK calls works | **Guaranteed** | The engine awaits the returned promise by driving the job queue (`p.finish()`). SDK calls are synchronous underneath and resolve immediately. |
| `Date.now()`, `new Date()`, `Math.random()` | **Best-effort** | Clock and randomness quality are runner-defined (the Go runner gives the real clock to the wasm module through WASI; QuickJS's `Math.random` seeding is **UNVERIFIED**). Do not use `Math.random` for anything security-relevant. |
| `Intl`, locale-sensitive `toLocaleString`, `String.prototype.normalize` and any ES2021+ library feature | **Best-effort** | May be absent or reduced in QuickJS, and may differ on V8. Do not rely on them. |
| `RegExp` features beyond ES2020 (lookbehind, named groups, `/v`, `/d`) | **Best-effort** | Engine-dependent. |
| Global `fetch`, `Request`, `Response`, `Headers`, `AbortController`, streams | **Not provided** | Use `fn.fetch`. |
| `TextEncoder`, `TextDecoder`, `atob`, `btoa`, `structuredClone` | **Not provided** | The SDK uses engine natives; the `TextEncoder` fallback in `native.ts` exists only for `node --test`. |
| `URL`, `URLSearchParams` | **Not provided** | The SDK hand-rolls query parsing (`FnQuery`); `req.query` is the API. |
| `crypto` / `crypto.subtle` / `getRandomValues` | **Not provided** | A function that needs HMAC or hashing needs a pure-JS library bundled in. A host-provided crypto op is a possible addition (a possible additive op; not proposed in detail here). |
| `setTimeout`, `setInterval`, `queueMicrotask`, any timer or event loop | **Not provided** | Promises settle only from already-resolved work; a promise that never settles ends in a 500. There is no way to sleep. |
| `Buffer`, `process`, `require`, ESM `import` of node builtins, `fs`, `net` | **Not provided** | The bundle is one script; npm packages are bundled in at build time and must be pure JS. |
| Threads, workers, `SharedArrayBuffer`, `Atomics` | **Not provided** | A wasm instance is single-threaded and serves one call at a time. |
| Dynamic `import()`, `eval`, `new Function` | **Best-effort, discouraged** | The engine itself uses `eval` to load; guest use is not promised. |

**The portability rule for JS** (this is the point of the table): function source must stay inside
the intersection of what every runner that may claim `js` provides. V8 (deno_core, the Rust
platform's JS host) has `fetch`, `URL`, `TextEncoder`, timers and more; code that uses them works
there and breaks on QuickJS. So the contract is the *smaller* set above, and any runner that
claims `js` must provide at least that set. Extra globals on a particular runner are
runner-defined and not promised.

**Resource limits visible to JS code (runner-defined, UNVERIFIED in detail):** the engine sets a
512 KiB native stack limit (`MAX_STACK`); it does **not** set a QuickJS heap limit, so heap growth
is bounded by the wasm instance's `memoryMb`, and exhausting it ends the call (the instance is
discarded) rather than throwing a catchable JS error. Deep recursion throws a catchable JS
`InternalError` on QuickJS and a `RangeError` on V8; do not catch either.

### 1.3 Layer 5: platform-facing semantics

These hold for every runner of every runtime. They are the behaviours operators and callers
observe, not implementation details.

**Addressing and invocation (IN FORCE).** Address is `{applicationCode}.{name}`, immutable. A runner
serves `/fn/{address}[@{alias}|@v{n}]/{path}`; no match is 404, wrong method 405 with `Allow`, a
loading version 503 + `Retry-After`, busy 429 + `Retry-After: 1`, deadline 504, body over cap 413
`BODY_TOO_LARGE`, a guest failure 500 `{"error":"FUNCTION_FAILED"}` with the message in the log
only. Explicit-version calls need a platform token with `platform:function:version:invoke`.
(Another runner family may use a different invocation URL on its own listener, e.g. the Rust and
Java hosts' `/functions/...`; the platform only ever builds `/fn/...` URLs, see 3.3.)

**Webhook verification and who signs (IN FORCE).**
- An `auth: webhook` endpoint is POST-only. The runner reads the body, then verifies
  `X-FlowCatalyst-Signature` (HMAC-SHA256 over the delivery, per `pkg/fcsdk/webhook.Validator`)
  and `X-FlowCatalyst-Timestamp` (ISO-8601 with milliseconds, or Unix seconds; 300 s freshness
  window) with the `webhookSecret` in desired state. Failure is 401 `INVALID_SIGNATURE`; the guest
  never runs.
- `webhookSecret` is the signing secret of **one service account**. Default: the function's
  application's service account (the oldest active account linked to the application). Override
  (**F35**, migration 084): `fng_functions.service_account_id`, set with `fcdev fn set --service-account`
  / `PATCH /api/functions/{id}` `serviceAccountId` / `clearServiceAccount`. A set-but-inactive or
  deleted account yields an empty secret and every delivery is refused; it never falls back to the
  application's account. Credentials are cached for 1 minute.
- **Consequence the author must know:** deliveries are signed by whoever the *delivery path*
  says. Subscription and schedule deliveries use the application's account unless the connection
  says otherwise; **dispatch-job deliveries (ops 11) are signed by the declared connection's
  service account** (F24), not by the application's. A function that is the target of a
  `fn.dispatch` from a connection `yard-conn` must have that connection's service account set as
  its verifying account, or every dispatch to it is a 401 and the job retries to exhaustion. This
  is the single most likely "hundreds of functions" trap; see section 7, G-12.

**Caller shape (IN FORCE, frozen in 2.2).** `webhook` (signature verified), `anonymous` (auth
`none`), or `principal` with `id`, `type`, `tier`, `clients` (client ids), `clientCodes` (client
identifiers, `["*"]` for anchors), `roles`, `applications`, `allApplications`, `permissions`.
`permissions` is the whitespace-split `scope` claim of the platform access token. Only tokens that are
API credentials are accepted (identity tokens are refused). `email` and `name` are never passed and
the `Authorization` header is removed before the guest sees the request. Permission matching has
`*` segment-wildcard semantics, mirrored in all three SDKs (`HasPermission` / `hasPermission` /
`has_permission`).

**Declarations (IN FORCE).**
- `config` / `secret` keys: only declared keys are readable (`NOT_DECLARED` otherwise); values are
  platform settings; promote is refused (`SETTINGS_MISSING`) while one has no value. Secrets are
  write-only.
- `db`: only declared names; DSN is a platform setting; `?` placeholders; rows capped at 10 000 or
  8 MiB (`truncated`); an open transaction is rolled back when the call ends.
- `httpAllow`: hostname or `*.suffix` (wildcard needs two labels after `*.`); checked on the URL and
  on every redirect; private/loopback/link-local/metadata destinations refused at dial; response
  capped at 16 MiB (`TOO_LARGE`).
- `emits`: only declared types owned by the function's application; every event needs a `dedupId`.
- `dispatches`: `{target, path, connection (required), pool?}`; target must be a function of the
  same application (and client); `fn.dispatch` supplies payload and job fields only, never a URL,
  pool or connection; not idempotent; payload max 256 KiB; metadata keys must not start with `fn.`.

**Delivery outcomes (IN FORCE).** 2xx delivered; 429 + `Retry-After` retried without spending an
attempt; 422 + `FlowCatalyst-Outcome: reject` terminal; anything else a failed attempt.

**Limits.** `limits` is a platform setting, never part of the artifact.

| Limit | Semantics | Portable? |
|---|---|---|
| `timeoutMs` (default 30 000; endpoint override may only lower it) | Wall-clock budget per call. Passed to the guest as `deadlineUnixMs`; the runner answers 504 when it passes. Propagates to db, http and emit calls. | **Yes.** Every runner must enforce it, by preemption where it can and by abandoning the call where it cannot. |
| `maxBodyBytes` (default 1 MiB; endpoint override; runner ceiling 32 MiB) | Request body cap, 413 beyond it. | **Yes.** |
| `maxConcurrency` (default 16) | Per function per runner; 429 `FUNCTION_BUSY` + `Retry-After: 1` beyond it. | **Yes.** |
| `memoryMb` (default 64) | wazero: a hard cap on one instance's linear memory; a refused grow fails only that call. JVM plan: an admission weight with no per-call enforcement. V8 (Rust host): isolate heap. | **No.** Portable only as "an upper bound the author asks for"; enforcement strength is runner-defined (section 7, G-6). |
| Fuel / instruction budget | Rust host only (`limits.maxFuel`, wasmtime fuel). Not in the Go `Limits` struct at all. | **No.** Runner-defined; no function may depend on it. |
| CPU time, CPU share | Nothing. Fairness is by permits and deadlines. | **No.** |

## 2. Frozen items

"Frozen" means: **may be added to, never changed or removed**, inside ABI v1 / describe version 1,
and only by the change process in section 6. A change that is not purely additive is a new
version.

### 2.1 ABI v1: guest exports and host imports (IN FORCE)

| Item | Signature | Notes |
|---|---|---|
| export `memory` | linear memory | Exactly one |
| export `fc_abi_v1` | `() -> ()` | Marker. A module with no known marker is refused `LOAD:ABI_UNKNOWN` |
| export `fc_alloc` | `(size i32) -> i32` | Guest owns the bytes |
| export `fc_handle` | `(ptr i32, len i32) -> i64` | Returns `ptr<<32 \| len` of a response frame, valid until the next call into the guest |
| export `fc_describe` | `() -> i64` | Same packing; must not call any host import |
| export `_initialize` | `() -> ()` | Optional wasip1 reactor init, called once before anything else |
| import `fc.call` | `(op i32, ptr i32, len i32) -> i64` | Returns `code<<32 \| resultLen`; 0 = success |
| import `fc.take` | `(dst i32) -> ()` | Copies the buffered result into guest memory at `dst`, then clears it |
| host ops 12 `trace`, 13 `tracing` and 14 `trace.reference` (through `fc.call`) | see section 2.4 | **Additive, IN FORCE**: the trace-log ops (`docs/function-trace-design.md`). A runner that predates them answers `UNKNOWN_OP`; the SDKs treat that as "not tracing" and never fail the call |
| allowed import modules | `fc`, `wasi_snapshot_preview1` | Anything else: `LOAD:IMPORT_NOT_ALLOWED` |
| WASI environment | no preopens, empty args and env, real clock, crypto random, stdout/stderr to the function log at INFO/WARN | |

Load failure codes (`LOAD:<code>`, `internal/functions/engine/check.go`): `INVALID_MODULE`,
`ABI_UNKNOWN`, `EXPORT_MISSING`, `EXPORT_SIGNATURE`, `IMPORT_NOT_ALLOWED`, `MEMORY_OVER_CAP`,
`INSTANTIATE`.

### 2.2 Frames, request, response, caller (IN FORCE)

Frame: `u32 little-endian metaLen | meta (UTF-8 JSON) | body (raw bytes, never base64)`. Used in
both directions and for host calls.

| Frame | Fields (JSON meta) |
|---|---|
| Request (host to `fc_handle`) | `id`, `address`, `version`, `method`, `path`, `rawQuery`, `headers{name:[values]}`, `route`, `pathParams{}` (always an object), `caller{}`, `deadlineUnixMs`; additive and optional: `host`, `remoteAddr`, `public` |
| Response (guest to host) | `status`, `headers{name:[values]}` (optional) |
| Caller | `kind` = `webhook` \| `anonymous` \| `principal`; for `principal`: `id`, `type`, `tier`, `clients`, `clientCodes`, `roles`, `applications`, `allApplications`, `permissions` |
| Error frame (host call failure) | `{"code","message"}`; `code` is also the high word of `fc.call`'s return value (nonzero on failure) |

Reading rules (both directions): **a reader ignores unknown meta fields.** Go and TS SDKs do this
today (`encoding/json`, `JSON.parse`). The Rust SDK uses serde derives; whether any struct sets
`deny_unknown_fields` is **UNVERIFIED** and is conformance item C-60. The host's response parser
is **UNVERIFIED** for unknown response fields.

### 2.3 Describe document (IN FORCE; version field PROPOSED)

The describe document is read at publish by the platform (`abi.ParseDescribe`, strict), again by the
platform on promote (`alias.go`), and by the runner when it loads desired state (`runner.go`).
**All three use `DisallowUnknownFields`.** A new key is therefore rejected by any component that
predates it.

| Key | Type | Frozen rules |
|---|---|---|
| `abi` | int | Must equal 1. Today this field is also the only describe version. |
| `endpoints[]` | `{method?, path, auth, maxBodyBytes?, timeoutMs?, cors?}` | `auth` is `webhook` \| `platform` \| `none`, required, no default. Webhook is POST only. Paths use Go 1.22 `ServeMux` syntax (`{id}`, `{rest...}`). `cors`: `origins`, `methods?`, `headers?`, `allowCredentials?` |
| `subscriptions[]` | `{eventType, path, mode?, maxRetries?, dataOnly?, timeoutSeconds?}` | `path` literal and landing on a webhook endpoint. Omitted `mode` = platform default `NEXT_ON_ERROR` |
| `schedules[]` | `{cron, timezone?, path, payload?}` | 6-field cron, seconds first |
| `config[]`, `secrets[]` | string keys | `^[A-Za-z][A-Za-z0-9_./-]{0,99}$` |
| `db[]` | names | `^[a-z][a-z0-9_-]{0,62}$` |
| `httpAllow[]` | hosts | hostname or `*.suffix`; no IPs, no localhost |
| `emits[]` | event types | four colon-separated segments |
| `dispatches[]` | `{target, path, connection, pool?}` | `connection` required; one entry per (target, path) |
| `traceContexts[]` | `{name, description?}` | **Additive, IN FORCE.** `name` matches `^[a-z][a-z0-9-]{0,39}$` and is not `http` (the built-in capture context); at most 20; unique; `description` at most 200 characters. Like every key, a platform or runner that predates it rejects the describe, so release platform and runner first, SDK second |

**Adding a key is additive in meaning but not in transport.** The ordering rule that follows
(IN FORCE as a rule, because it is forced by the strict parser): **platform and runner first, SDK
second.** Concretely: release a platform + runner that accepts the key; only then release the SDK
version that emits it. The `dispatches[].pool/connection` change already followed this ("a platform
that predates them rejects the describe at publish as an unknown key", plan 5.4), by design.

**PROPOSED additions** (not implemented; every one needs platform + runner support first):

```json
{ "abi": 1,
  "describeVersion": 1,
  "sdk":      {"name": "fn-ts", "version": "1.2.0"},
  "requires": {"ops": [11], "runtime": "js", "jsEnv": 1, "describeVersion": 1} }
```

- `describeVersion` separates "the describe schema" from `abi`. Absent means 1.
- `sdk` records which SDK built the artifact, so a bulk redeploy can target "every function built
  by fn-go below 1.4". Informational only.
- `requires` lets an artifact state what it cannot run without. A runner **refuses at load**
  (`LOAD:REQUIREMENT_UNMET`, a permanent failure) instead of discovering it per call as
  `UNKNOWN_OP`; the platform refuses **promote** when no live runner of the pool advertises the
  requirement, the same way it already does for runtimes (`POOL_LACKS_RUNTIME`). Runners advertise
  `abiOps`, `describeVersions`, `jsEnv` in `Capabilities.Extra` (a place that exists today).

### 2.4 Host ops (IN FORCE)

| op | Name | Request meta / body | Result meta / body |
|---|---|---|---|
| 1 | `log` | `{level, msg, attrs}` | none |
| 2 | `config.get` | `{key}` | `{found}` / value |
| 3 | `secret.get` | `{key}` | `{found}` / value |
| 4 | `http.fetch` | `{method, url, headers, timeoutMs}` / body | `{status, headers}` / body |
| 5 | `event.emit` | `{type, source, subject, dedupId, correlationId, causationId, messageGroup, contentType}` / data | `{eventId}` |
| 6 | `db.query` | `{db, sql, params, tx}` | `{truncated}` / JSON array of row objects (keys in select order) |
| 7 | `db.exec` | same | `{rowsAffected}` |
| 8 | `db.begin` | `{db}` | `{tx}` |
| 9 | `db.commit` | `{tx}` | none |
| 10 | `db.rollback` | `{tx}` | none |
| 11 | `dispatch.create` | `{target, path, code, subject, metadata[{key,value}], messageGroup, mode, correlationId, causationId, contentType, descriptor, maxRetries, timeoutSeconds, dataOnly}` / payload | `{jobId}` |
| 12 | `trace` | `{context, level?, msg, data?, reference?}`; `level` is `debug` \| `info` \| `warn` \| `error` (default `info`), `data` a JSON object | none. A silent no-op for a context that is not live. Errors: `NOT_DECLARED` (context neither in `traceContexts` nor `http`), `BAD_REQUEST` (malformed meta; `http` is not writable by functions), `TOO_LARGE` (`msg` over 4096 bytes, `data` over 32768 bytes; the SDKs cut `msg` to 4096 bytes and `reference` to 200 characters before sending) |
| 13 | `tracing` | `{context}` | `{live}`: true iff the context is switched on for this function now and this invocation was sampled in. `NOT_DECLARED` as above |
| 14 | `trace.reference` | `{reference}` | none. Sets the invocation's business key, stamped on its later trace, http and log records (empty clears it); a `reference` on a single `trace` call is that record's only. `TOO_LARGE` over 200 characters, `BAD_REQUEST` for malformed meta; the SDKs' `setReference` sends it and swallows every error |

Wire conventions that are frozen with the ops: `db` byte params are base64 strings and
`Date`/`time.Time` params are ISO-8601 (no per-parameter type tag exists); dispatch `metadata` is
always the `[{key,value}]` array on the wire (SDKs may accept a map). The runner's `dispatch`
handler is the one op that parses its meta strictly (an unknown key such as `pool` is
`BAD_REQUEST`); all others are lenient.

**Error codes (IN FORCE):** `NOT_DECLARED`, `NOT_ALLOWED`, `DEADLINE`, `TOO_LARGE`, `UNAVAILABLE`
(the only retryable class), `BAD_REQUEST`, `CAPABILITY_UNAVAILABLE` (describe time), `UNKNOWN_OP`,
`DB_CONSTRAINT`, `DB_SYNTAX`, `DB_TIMEOUT`, `DB_UNAVAILABLE`, `DB_ERROR`, `DB_TX_UNKNOWN`. A
capability failure is never a trap. Code strings are frozen; SDKs treat codes as opaque strings, so
adding a code is additive, but an SDK must map an unknown code to a non-retryable generic error.

### 2.5 How new ops are introduced and detected

**Today (IN FORCE):** a new op takes the next number (12, 13, ...). A runner that predates it answers
`UNKNOWN_OP` at call time. That is a *runtime* discovery: a function that calls op 12 on an old
runner fails on the first call, not at deploy. Ops 11, 12 and 13 were added this way.

**PROPOSED:** the `requires.ops` mechanism in 2.3. Rule: an SDK that exposes a new op must put it
in `requires.ops` whenever the function declares or uses it (for op 11 that is exactly when
`dispatches` is non-empty, already a describe fact). A runner that does not know the op refuses the
version at load. Until `requires` exists, the interim rule is: **release a new op to all runners
(and the Java/Rust hosts, if they are in service) before the SDK that calls it.**

An op's number, request meta and result meta are then frozen. Behaviour may be widened (a new
optional meta field, additive) but not narrowed.

### 2.6 Versioning, deprecation and the support window

| Thing | Rule |
|---|---|
| SDKs | Semver per SDK. Minor/patch: no source break. **Major: the only place function source may break**, and it ships with a codemod or a written migration, and the previous major keeps building (and keeps working in production) for the support window. |
| ABI | Integer in the marker export name (`fc_abi_v1`). A new ABI is `fc_abi_v2`; a runner may support several markers. A module exports exactly one marker. |
| Describe | `abi` today; `describeVersion` (PROPOSED). |
| JS environment | `jsEnv` integer (PROPOSED), carried in `requires`. Bumped when a global is removed or its behaviour changes. Adding a global is not a bump. |
| Support window (PROPOSED, owner to set N) | **N = 12 months** after the successor reaches GA, for ABI, describe and `jsEnv`. **6 months** for an SDK major after the next major's GA. Within the window the platform and every runner accept the old form; after it they may refuse at publish (new versions only; already-published versions keep loading until retired). |
| Announcing a deprecation | (1) a dated "Deprecations" section in `docs/published/60-functions.md`; (2) `fcdev fn build` prints a warning when the describe it reads uses a deprecated form; (3) the SDK emits a build-time warning for deprecated calls; (4) PROPOSED: the runner logs `LOAD:DEPRECATED` once per version load and the function status page lists it. The warning names the replacement and the removal date. No behaviour is removed before its date. |

## 3. The runtime and target matrix

### 3.1 Runtime tags

A **runtime tag** names the *artifact format and its calling contract*, not the engine. The control
plane (IN FORCE, commit `67c721dc`) already routes on it: `runtimes=` filter on desired state,
`Capabilities.Runtimes` in heartbeats, `POOL_LACKS_RUNTIME` on promote, per-runtime URL
`FC_FUNCTIONS_RUNNER_URL_<RT>`, tag pattern `^[a-z][a-z0-9]{1,15}$` (migration 083).

**Contract rules for tags (PROPOSED as policy; the mechanism is built):**
1. `wasm` means exactly "an ABI v1 core module". Nothing else may use that tag. (The Rust platform
   today uses `wasm` for WASI 0.2 *components*; that must not leak into this platform. Components
   use `component`, as `docs/runner-plumbing-costing-2026-10-19.md` D2 recommends.)
2. `js` means exactly "an IIFE bundle that defines `globalThis.__fc` and uses only the JS
   environment of section 1.2". Two engines (QuickJS, V8) may both claim `js` **only if** they
   accept that same bundle and provide that same environment. If an engine needs a different bundle
   (the Rust host expects an ES module with a default-export `(Request) => Response` and
   `flowcatalyst:function/*` imports), it is a different tag.
3. Choosing between two runners of the same tag is a **pool** decision, never a function decision.
   A function's runtime is fixed by its first version (costing D1, recommended).

### 3.2 Matrix

| Artifact | Runtime tag | Runner | State | Portable to |
|---|---|---|---|---|
| Go (`GOOS=wasip1 -buildmode=c-shared`) core module, fn-go | `wasm` | wazero (`internal/functions/runner`) | **Built, proven** (`TestSDKExamples`) | Any runner that implements ABI v1: wasmtime or wasmer core-module host (not built; the Rust host refuses core modules, `WASM_CORE_MODULE_UNSUPPORTED`) |
| Rust (`wasm32-unknown-unknown`) core module, fn-rust | `wasm` | wazero | **Built, proven** | same as Go |
| TypeScript bundle, fn-ts | `js` | wazero + shared QuickJS engine | **Built, proven** | A V8 runner that provides section 1.2 and loads the IIFE (not built) |
| TinyGo core module, fn-go | `wasm` | wazero | **Not a commitment** (plan 2.2/9; fn-go is only built and tested with standard Go wasip1; UNVERIFIED for TinyGo) | would be the same ABI |
| Rust SDK artifact as a WASI 0.2 component | `component` (proposed) | wasmtime (Rust host) | **Not built here.** Different SDK crate on the Rust side (`fc-function-pdk`, `wasm32-wasip2`, `wasi:http`, WIT `flowcatalyst:function@0.1.x`) | Not ABI v1; **not portable** to or from the `wasm` core-module path |
| Rust-platform JS (V8 isolates, `deno_core`, `runtime: js`) | would need its own tag (bundle format differs) | Rust host | **Not built here** | Not the `@flowcatalyst/fn` bundle |
| Java jar | `jvm` | Java host | **ON HOLD** (`docs/java-function-runner-plan.md`) | Own SDK (`fn-java`); describe is shared, binary is not |
| Native Go process | `native` | none | Deferred (plan section 15) | Own socket contract |
| Test fixture | `echo` | `internal/functions/echorunner` | Built; no guest | Conformance skeleton only |

### 3.3 What is portable between runners

| Between | Portable? |
|---|---|
| Any two runners of tag `wasm` (core module) | **Yes**, the artifact is byte-identical; this is the point of ABI v1. |
| Any two runners of tag `js` | **Yes**, if both provide section 1.2. |
| `wasm` and `component` | **No.** Different binary format, different host interface, different SDK. The *source-level* declaration model is the same idea but the Rust and Go repos have separate SDK implementations. |
| Describe documents across Go, Java and Rust platforms | **Partly.** Java's manifest and the Rust host's manifest differ today (Rust has no `emits`/`dispatches`; address has 3 labels in Java/Rust vs 2 here). The Go describe is the reference. |
| Control protocol | Every runner of this platform speaks the Go v1 control protocol. The Java/Rust wire (`/desired-state`, no revision) is adapted **runner-side** (costing D3). Not part of this function-source contract. |
| Invocation URL | The platform builds `/fn/{address}...`; a runner that serves another shape must also serve this one. |

### 3.4 Requirement: the same source builds for either target

**REQUIREMENT (PROPOSED, to be verified per SDK):** *for each SDK, the same function source file
set must be buildable for every target that SDK's language supports, by changing only the build
command or an SDK build flag, never the source.* Targets: `wasm` core module; `component` once it
exists. A function must not contain `#[cfg(target...)]` or `//go:build` for its own logic.

Verification of today's SDKs (read from code):

| SDK | Targets today | Same source for both? | Verdict |
|---|---|---|---|
| fn-go | `GOOS=wasip1` core module (`abi_wasip1.go`, `//go:build wasip1`). Off-wasm builds get `FakeHost` (`abi_other.go`), so the same source also builds and tests natively. The whole ABI surface is one file. | Only one wasm target exists. A component target is not implemented (UNVERIFIED whether Go's toolchain can produce one without a post-build adapter step). | **Satisfied for the one target; not demonstrable for a second.** The one-file ABI seam is the right shape. |
| fn-rust | `wasm32-unknown-unknown` core module, glue behind `cfg(target_arch = "wasm32")`, `export!` macro emits the four exports. Off-wasm builds use a `FakeHost`. | No component backend. The Rust platform's guest SDK (`fc-function-pdk`) is a **different crate** with a different handler model (`wasi:http`), so a handler written for `flowcatalyst-fn` does not build as a component. | **Not satisfied for `component`.** Requires a `component` backend behind a Cargo feature whose public API (`App`, `Request`, `Response`, `Context`) is unchanged. |
| fn-ts | One bundle shape (IIFE with `globalThis.__fc`). The Rust platform's V8 host expects an ES module default export. | Would need `fc-fn-build --target=<v8-esm>` that wraps the same handlers; not built. | **Not satisfied for a V8/ESM target.** A build-time adapter, not a source change, is the fix. |

Each of these is a section 5 backlog item (C-70..C-72) and a section 7 risk (G-9).

## 4. Rules for authors of functions

These make the promises above hold. (PROPOSED as the "authoring guide"; each rule follows from
a layer.)

1. Import only the SDK's public API. No `__fc*`, no internal packages, no raw `unsafe`/`wasmimport`
   in Go function code.
2. Declare everything (`config`, `secret`, `db`, `httpAllow`, `emits`, `dispatches`, endpoints,
   subscriptions) through the SDK. Never build a describe document by hand.
3. Do not depend on timing, memory limits, fuel, CPU share, instance reuse or the order in which
   instances are chosen. A module-level variable may be reset at any time (instances are recycled
   after 10 000 calls, trimmed when idle, discarded after any failure).
4. Be idempotent: deliveries are at-least-once, `emit` needs a `dedupId`, `dispatch` is not
   idempotent so derive the decision from the delivery's identity.
5. In TypeScript use only section 1.2's guaranteed set. Bundle pure-JS libraries; check them for
   `TextEncoder`, `URL`, `crypto` and timers before adopting them.
6. Treat `deadlineUnixMs` as the budget; pass the request context to every call (Go).
7. Never log secrets or SQL parameters. Logs are not a stable interface.

## 5. Conformance: the executable contract

A runner conforms when it passes the suite below. The suite is the only place the contract is
*checked*; this document is the place it is *explained*. **Not built now; this is the backlog.**

### 5.1 Where it lives (PROPOSED)

```
conformance/functions/
  README.md               how to run against a runner; how to add a vector
  vectors/NN-name.json    data, not code: {name, describe, settings{config,secrets,db},
                           caller{webhook-signed | platform-token{claims} | none},
                           request{method,path,headers,body}, expect{status, headers-subset,
                           body-exact|json-subset}, controlCalls[] the runner must make}
  fixtures/<runtime>/     one fixture function per runtime implementing the same behaviours:
                           wasm/{go,rust}, js/ts, later component/, jvm/
  corpus/describe/valid|invalid/*.json    the describe validation matrix
internal/functions/conformance/           Go harness: fake control plane (records every call),
                                           starts the runner under test (in-process wazero via
                                           runner.New, or a child process on an ephemeral port),
                                           pushes desired state, waits for LOADED, runs vectors
                                           over HTTP, diffs HTTP result + recorded control calls,
                                           prints a vector x runner table
```

This matches what the Java plan (section 6.1) already specifies; there it is two options for the
path and this document picks one. Seeds that exist today and should be migrated, not rewritten:
`TestSDKExamples` (`internal/functions/runner/examples_test.go`), `internal/functions/fnfixture`
(Rust guest + `fixture.wasm`), `invoke_callers_test.go`, `invoke_hardening_test.go`,
`retry_test.go`, `fetch_guard_test.go`, `js_test.go`, `auth_test.go`, and
`internal/functions/echorunner` (the control-protocol skeleton). Any new runner (wasmtime core
module, V8 `js`, Java, native) must pass the suite for the tags it claims before it is installed in
a pool. A change to this contract must change a vector in the same PR.

### 5.2 Golden functions and assertions (the backlog)

| ID | Golden function | Assertions |
|---|---|---|
| C-01 | `echo` | Request frame round-trips: method, path, rawQuery, headers, body bytes (binary-safe, includes 0x00 and non-UTF-8), `route`, `pathParams` always an object |
| C-02 | `headers-status` | Guest status and multi-value headers reach the caller; 204 with no body; HEAD answered from GET with no body; `Content-Type` not invented |
| C-03 | `routing` | 404 unmatched; 405 + `Allow`; literal beats param beats rest; trailing `{rest...}` |
| C-04 | `big-response` | Response body up to the runner ceiling; beyond it a defined failure |
| C-10 | `panic` | Trap, panic, malformed frame, JS throw, JS rejected promise: 500 `FUNCTION_FAILED` (host-made) or `FUNCTION_PANIC` (guest-made); neighbour call unaffected; instance discarded |
| C-11 | `retry` / `reject` | 429 + integer `Retry-After` rounded **up**; 422 + `FlowCatalyst-Outcome: reject`; reason as plain text body |
| C-20 | `webhook-sig` | Valid signature accepted; wrong secret, missing header, stale timestamp (> 300 s), tampered body, GET to a webhook endpoint all refused before the guest runs |
| C-21 | `platform-claims` | Claim mapping to `Caller` (every field in 2.2, including `clientCodes` for `*`); `scope` becomes `permissions`; `Authorization` removed; identity-token refused; wildcard permission matching |
| C-22 | `anon` | auth `none`: caller `anonymous`; no credentials required |
| C-23 | `version-invoke` | `@vN` requires `platform:function:version:invoke` and reach |
| C-24 | `signer-override` | `webhookSecret` from a per-function service account is the one that verifies; empty secret refuses every delivery |
| C-30 | `config-secret` | Declared key found; undeclared `NOT_DECLARED`; missing value `found:false`; secrets never in logs |
| C-31 | `http` | Allowed host passes; undeclared host `NOT_ALLOWED`; redirect to a non-allowed host refused; private/loopback/metadata address refused; > 16 MiB `TOO_LARGE`; `timeoutMs` and deadline honoured |
| C-32 | `event` | Declared + owned type emits and returns `eventId`; undeclared / unowned `NOT_ALLOWED`; missing `dedupId` `BAD_REQUEST`; `UNAVAILABLE` is the only retryable class |
| C-33 | `db` | query/exec, `?` binding, base64 bytes and ISO dates, row order, 10 000 / 8 MiB `truncated`, begin/commit/rollback, tx auto-rolled back at call end, undeclared db `NOT_DECLARED`, DB_* classes |
| C-34 | `dispatch` | Declared pair creates a job (control call recorded with target, path, payload, array metadata, `fn.*` provenance); undeclared `NOT_DECLARED` before any control call; payload > 256 KiB `TOO_LARGE`; `fn.`-prefixed metadata key refused; `pool`/`connection` in the call refused `BAD_REQUEST`; refusal reaches the handler as an error |
| C-35 | `log` | Levels map; attrs preserved; stdout/stderr routed; logging never fails a handler |
| C-36 | `unknown-op` | Op 99 and an op the runner does not implement answer `UNKNOWN_OP`, not a trap |
| C-40 | `js-globals` | Exact presence/absence of every row in section 1.2 (`typeof` probe returned as JSON) |
| C-41 | `js-es-library` | ES2020 library smoke: Map/Set/Promise/async/generators/typed arrays/BigInt/JSON edge cases |
| C-42 | `js-console` | `console.*` routing and argument formatting |
| C-43 | `js-async` | Handler returning a Promise; `await` on SDK calls; never-settling promise ends in a defined 500, not a hang |
| C-44 | `js-limits` | Deep recursion and a runaway allocation end the call, not the runner |
| C-50 | `deadline` | A spinning guest is stopped at the deadline (504) within a tolerance, a neighbour's p99 stays within 2x solo; a runner that cannot preempt must still return 504 at the deadline and quarantine the version |
| C-51 | `memory` | A guest allocating past `memoryMb` fails only its own call (500 / documented code); the runner and a concurrent neighbour survive |
| C-52 | `body-limits` | Body over function cap and over endpoint cap: 413 `BODY_TOO_LARGE`; endpoint cap may only lower; 0 means none-allowed |
| C-53 | `concurrency` | At `maxConcurrency`: 429 `FUNCTION_BUSY` + `Retry-After: 1`; slots released after traps |
| C-54 | `timeout-override` | Endpoint `timeoutMs` lowers, never raises, the function timeout; deadline propagates into http/db/emit |
| C-60 | `describe-corpus` | Every document in `corpus/describe` accepted or rejected identically by `abi.ParseDescribe` and by each runner's own parser; unknown key rejected; each validation rule in 2.3 |
| C-61 | `forward-compat` | Request frame with an extra meta field is ignored by every SDK fixture |
| C-62 | `abi-load` | Missing marker, missing/mistyped export, disallowed import, over-cap memory: the exact `LOAD:*` codes; `_initialize` called once |
| C-63 | `abi-frame` | Zero-length body, max-size meta, `ptr<<32\|len` packing with high addresses, result valid until next call |
| C-70 | `same-source-fn-go` | The fn-go example builds for every declared target from identical source |
| C-71 | `same-source-fn-rust` | same for fn-rust (blocked until a `component` backend exists) |
| C-72 | `same-source-fn-ts` | same for fn-ts (blocked until a V8/ESM target exists) |
| C-80 | control: `runtimes-filter` | Runner receives only its runtimes; foreign runtime is skipped, never reported FAILED |
| C-81 | control: `revision-fence` | 304 on ETag; long-poll wake; applied revision echoed; stale FAILED report ignored |
| C-82 | control: `load-retry` | Transient fetch failure gives RETRYING then LOADED; permanent failure gives FAILED with a reason; digest mismatch FAILED |
| C-83 | control: `swap-drain` | Promote swaps new-before-old with zero failed calls under constant load |
| C-84 | control: `capabilities` | Heartbeat carries protocol, runtimes, kind, version; promote of an unrunnable runtime is `POOL_LACKS_RUNTIME` |

## 6. Change process

### 6.1 What needs an owner decision

| Change | Needs owner ruling? |
|---|---|
| Any change to an item frozen in section 2 other than a purely additive one | **Yes**, and it is a new version, not an edit |
| A new host op | **Yes** (number, meta, error behaviour, all three SDKs) |
| A new describe key | **Yes** (and the platform + runner first ordering) |
| Adding or removing a JS global, or changing `console` behaviour | **Yes** |
| Raising or lowering a default limit (`memoryMb`, `timeoutMs`, `maxBodyBytes`, `maxConcurrency`) | **Yes** for lowering (it can break running functions); raising is free |
| A new runtime tag or a new runner kind | **Yes** (and it must pass conformance) |
| Declaring an SDK 1.0 or a major bump | **Yes** |
| Setting or changing the support window N | **Yes** |
| Bug fixes that make the code match this document | No (but record in the PR) |
| New runner-defined behaviour (a metric, a cache, a faster allocator) | No |

### 6.2 How a breaking change is staged

1. **Additive first.** Introduce the new form beside the old one (new op number, new describe key,
   new SDK function, new `jsEnv`), platform and runner first, SDK second.
2. **Dual support.** The platform and every runner accept both forms. The SDK offers both; the old
   one is marked deprecated, with a replacement and a removal date (2.6).
3. **Deprecation window** of N months (PROPOSED 12 for ABI/describe/jsEnv, 6 for an SDK major).
   `fcdev fn build` and the status page warn.
4. **Bulk redeploy.** Rebuild every function against the new SDK and redeploy so no published
   version depends on the old form. With the tools that exist in this repo today:
   `fcdev fn deploy --skip-unchanged --json` per function (a changed artifact digest publishes and
   promotes; an unchanged one does nothing), `--no-promote` to stage and verify `READY` first,
   `fcdev fn status` for runner state, `fcdev fn prune` for old versions, promote/rollback with
   `fcdev fn promote`. The owner's CI pipeline `function=all` (UNVERIFIED: not in this repository)
   is the loop over every function; this contract needs that loop to emit the `--json` result per
   function so a run produces a table of `{address, version, digest, skipped}`. PROPOSED: record the
   `sdk` name and version in describe (2.3) so the loop can select "functions built by fn-go < X".
5. **Removal** only after the window, and only for *new* publishes; already-published versions keep
   loading until retired.
6. **Rollback** is always an alias change; the old version is still held (tagged versions are never
   pruned).

### 6.3 PR checklist (any PR that touches the ABI, describe, SDK API, or JS environment)

- [ ] Is the change purely additive? If not, it is a new version and has an owner ruling linked.
- [ ] Which layer (section 1) does it touch? Is it listed in the frozen tables? If a frozen item
      changed, the tables in this document changed in the same PR.
- [ ] Ordering: platform and runner release before the SDK that emits/uses it.
- [ ] Old runner + new SDK: what happens (refused at load, or `UNKNOWN_OP` at call)? Documented?
- [ ] New runner + old artifact: still loads and behaves the same?
- [ ] All three SDKs (Go, TS, Rust) changed together, or the feature is not yet public.
- [ ] If it adds an op: number reserved here, `abi/meta.go`, `fn-ts/src/ops.ts`, Rust `host.rs` and
      Go `host.go` constants, `UNKNOWN_OP` behaviour, and `requires.ops` (once it exists).
- [ ] If it adds a describe key: strict parser updated in `abi/describe.go`, corpus entries added,
      publish and promote and runner all accept it.
- [ ] If it touches the JS environment: section 1.2 table updated, C-40 vector updated, V8 parity
      considered (does the intersection still hold?).
- [ ] `TestSDKExamples` run for every SDK (plan 9: "run it for every SDK change") plus
      `go test -tags=integration` where the change touches PG.
- [ ] Docs: `docs/function-runner-plan.md`, `docs/published/60-functions.md`, SDK READMEs,
      `docs/wire-contract.md` (if control routes changed), `docs/environment-variables.md`.
- [ ] Deprecations announced per 2.6.
- [ ] Conformance vector added or changed.

## 7. Known gaps and risks

| ID | Gap or risk | Effect on the contract |
|---|---|---|
| G-1 | **No fuel / instruction metering on wazero.** The Go runner preempts by wall-clock deadline only (`WithCloseOnContextDone`). The Rust host meters fuel and exposes `limits.maxFuel`. | CPU accounting and per-tenant cost attribution are runner-defined; not promised. A function cannot be limited by instructions on this platform. |
| G-2 | **JS-on-wasm runs at interpreter speed.** QuickJS has no JIT, and a JIT cannot run inside a wasm sandbox. Measured: 72 µs warm p50 (plan 2.1) vs about 2 µs for V8 isolates (Rust plan). | Performance is runner-defined; a CPU-heavy TS function is a candidate for the Go/Rust path or a V8 runner, not for QuickJS. |
| G-3 | **QuickJS vs V8 API differences** (section 1.2): V8 has `fetch`, `URL`, `TextEncoder`, timers, `crypto`, full `Intl`, newer regexp. | Code that uses them works on V8 and breaks on QuickJS; the contract is the intersection, enforced only by convention until C-40 exists. |
| G-4 | **Core module vs component split.** The Rust host runs only WASI 0.2 components and refuses core modules; every SDK artifact here is a core module. Same tag `wasm` means different things on the two platforms. | `wasm` artifacts are not portable to the Rust host. Components need their own tag (`component`), their own `DescribeReader` on the Go platform (costing P5), and have no `dispatch` import (no WIT interface), so `dispatches[]` would be refused for them. |
| G-5 | **Describe is strict everywhere** (publish, promote, runner). | Any new key breaks older platforms/runners; ordering rule 2.3 is the only protection. |
| G-6 | **`memoryMb` is runner-defined.** wazero enforces a hard per-call cap; a JVM runner can only weight admission; V8 caps the heap. A leaking function on a JVM runner can take the runner down. | Do not promise memory isolation to function authors; pool by trust. |
| G-7 | **Limits are platform settings, not artifact content.** An artifact cannot assert "I need 256 MB". | A function that needs more memory than the default fails at runtime until someone raises the setting. PROPOSED: `requires.memoryMb` as an admission hint (not an enforcement promise). |
| G-8 | **JS stack/heap behaviour unverified** (section 1.2 resource note). No QuickJS heap limit is set in the engine. | A runaway TS function ends as a wasm-level failure, not a catchable error. |
| G-9 | **Same-source-for-both-targets is not satisfied** for fn-rust (no component backend) or fn-ts (no ESM/V8 target). | A runner swap to a component host requires an SDK feature first (section 3.4). |
| G-10 | **Unknown op is discovered per call** (`UNKNOWN_OP`), not at load. | Deploy-time safety needs `requires` (2.3). |
| G-11 | **Request frame has fields the plan does not document** (`host`, `remoteAddr`, `public`). | Plan 5.2 is behind the code. Frozen here as additive-optional. |
| G-12 | **Signer mismatch trap.** Webhook verification uses one service account per function; dispatch deliveries are signed by the connection's account (F24); `docs/published/60-functions.md` and plan 8.7 still say the application's account in places. | A function that is a dispatch target of a connection other than its application's fails every delivery with 401 until the function's signer is set (C-24). Needs documentation and ideally a publish/promote-time warning. |
| G-13 | **All three SDKs are pre-1.0** and fn-rust has two version numbers (`VERSION` 0.0.0, Cargo 0.1.0). | "SDK major is the only break" is not yet true by semver. |
| G-14 | **No runner conformance suite exists**; correctness of the other runners rests on per-runner tests. | Section 5 is the fix. |
| G-15 | **Instances are recycled** (10 000 calls, idle trim, discard on failure) and the Rust host creates an isolate per request. | Module-level state is a cache at best. Authors must not rely on it (rule 4.3). |
| G-16 | **Op-level semantics are partly Go-runner behaviour**: whether the Java/Rust hosts implement `http.fetch`'s redirect re-check, `db`'s 10 000/8 MiB cap, dedup of emit, etc. is runner-side and UNVERIFIED for them. | Only conformance can promise it. |
| G-17 | **Webhook `none` endpoints and CORS** are runner-implemented. A new runner's CORS behaviour (preflight matching by `Access-Control-Request-Method`) is unspecified outside the Go code. | Add C-vectors for CORS when a second runner appears. |

## 8. Sources and what was verified

Read for this document: `docs/function-runner-plan.md` (all), `docs/runner-plumbing-costing-2026-10-19.md`
(all), `docs/published/60-functions.md`, `docs/java-function-runner-plan.md` (sections 0-6 and
the risk/phase tables), `../flowcatalyst-rust/docs/function-runner-plan.md` (status, section 4a,
H4, H9, G, section 7 headings), `internal/functions/abi/{meta,describe,frame}.go`,
`internal/functions/control/control.go`, `internal/functions/runtimes/runtimes.go`,
`internal/functions/engine/check.go` (load codes), `internal/functions/runner/{auth,invoke,capabilities}.go`
(relevant spans), `internal/functions/echorunner`, `clients/fn-js-engine/src/lib.rs`,
`clients/fn-ts/{README.md,package.json,bin/fc-fn-build.js,src/index.ts,log.ts,http.ts,ops.ts}`,
`clients/fn-go/{README.md,abi_wasip1.go,abi_other.go}` and the exported identifier list,
`clients/fn-rust/{Cargo.toml,README.md,src/host.rs}`, `pkg/fcsdk/webhook/validator.go`.

Not run: no function was built or executed, no runner started, and the `function=all` pipeline is not
in this repository. Items marked UNVERIFIED are the ones that would need a probe or a read of a
repository not in scope (Java host, Rust host internals).
