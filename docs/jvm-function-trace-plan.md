# JVM function tracing in the ingestor: implementation spec (2026-11-06)

Status: APPROVED by the owner 2026-11-06 (scope: parity with the Go runner **plus** the function's own outbound
calls). Platform side (new `outbound` kind) is done separately; this spec is the Java side.

## 0. Why

Function tracing (`docs/function-trace-design.md`) was built for the Go runner (wasm/js) on 2026-10-21. The
ingestor hosts JVM functions (`docs/jvm-functions-in-ingestor.md`) and implements **none** of it: the desired
document's `trace` field is not read, invocations are never captured, `traceContexts` in a describe is parsed
and dropped, and `FunctionContext` has no trace API. An operator who switches tracing on for a Java function
today gets nothing. Separately, a JVM function's outbound calls go through the framework `ServiceClient`
(`AllowlistHttpCaller` → `OutboundClients`), whose `HttpCapture` hook nobody binds for functions; those calls
are where a vendor integration's credentials appear (CTrack: password in a login query, session token in every
later path), so they are captured too, with the redaction the ingestion trace uses.

## 1. Read first

- `docs/function-trace-design.md` sections 1-5 and 8: THE contract (desired-state `trace`, sampling, records,
  `POST /control/functions/trace-log`, limits, shipping behaviour). Match the Go runner's behaviour unless this
  spec says otherwise. Go reference: `internal/functions/runner/trace.go`, wire types
  `internal/functions/control/control.go` (`TraceState`, `TraceRecord`, `TraceBatch`, `TraceHTTPData`, limits).
- `docs/ingestion-trace-design.md` section 6: redaction rules, implemented in
  `ingestor/ingestor-runtime/src/main/java/io/flowcatalyst/ingestor/runtime/TraceRedactor.java`.
- The ingestor's existing shipper for ingestion traces, `runtime/TraceShipper.java`, and the capture
  `runtime/Capture.java` (`http(HttpCapture.Exchange)` builds exactly the outbound record shape below).
- JVM host: `functions/http/FunctionListener.java` (`invokeEndpoint`), `functions/http/InvocationRunner.java`,
  `functions/context/{HostFunctionContext,HostLogger,LogBudget,ContextFactory,AllowlistHttpCaller,InvocationInfo}.java`,
  `functions/control/{Control,Describe,HttpControlPlane,ControlPlane}.java`, `functions/FunctionHost.java`,
  `functions/reconcile/*`. Function API: `ingestor/function-api/src/main/java/io/flowcatalyst/function/`
  (`FunctionContext`, `testing/FakeFunctionContext`).

## 2. Work items

### J1. Desired state: windows

`Control.FunctionDoc` gains `List<TraceState> trace` (nullable in JSON → empty list), `record TraceState(String
context, long enabledUntilMs, int sampleRate)`. The host keeps, per function address, the windows of the last
applied document (replaced atomically when a document is applied). A window is live iff `enabledUntilMs >
clock.millis()`, compared on every use (the host's `Clock`). `sampleRate` < 1 is treated as 1.

### J2. Describe: declared contexts

`Describe` gains `List<TraceContext> traceContexts` (`record TraceContext(String name, @Nullable String
description)`). Validate as `abi.Describe.Validate` does: name `^[a-z][a-z0-9-]{0,39}$`, not `http`, at most 20,
unique; description at most 200 bytes UTF-8. A version whose describe fails this fails to load, like any other
describe problem.

### J3. Per-invocation gate

At the start of an invocation (in `FunctionListener.invokeEndpoint`, **after** the concurrency permit is
acquired, so a 429 busy refusal takes no sampling turn), compute the invocation's live contexts: each live window
of the function whose sampler admits this invocation. Sampler: one `AtomicLong` per (function address, context);
admitted iff `counter.getAndIncrement() % sampleRate == 0`; decided once per invocation per context. Build an
`InvocationTrace` holding: address, version, invocation id, correlation id (`X-Correlation-Id` or empty), the
live set, the current reference (mutable, see J5), per-invocation counters (≤500 `log`, ≤500 `trace`, ≤100
`outbound` records), and the redactor source (J7). Bind it as a `ScopedValue` (e.g. `InvocationTrace.CURRENT`)
in `InvocationRunner.start` next to `InvocationDeadline.CURRENT` / `InvocationInfo.CURRENT`. When no context is
live, bind nothing (or a shared NONE): the off path must be a map lookup and no allocation beyond it.

### J4. Built-in `http` context

While `http` is live for the invocation:

- **One `kind:"http"` record** after the outcome is known, on every outcome once the permit was held: the
  function's result, a timeout (504 → outcome `timeout`), a thrown exception (500 → `error`), an out-of-range
  status (502 → `bad_response`), the request not encodable (500 → `error`). Data = `TraceHTTPData`
  (`internal/functions/control/control.go`): `request.{method, path, rawQuery, headers, body, bodyEncoding,
  bodyBytes, bodyTruncated, caller:{kind,id}}`, `response.{status, headers, body, ...}` (the response the
  listener actually sent), `durationMs`, `outcome`. Bodies cut at 65536 bytes (`FC_FN_TRACE_BODY_BYTES`,
  default 65536), `utf8` when valid UTF-8 without NUL else `base64`; if the record's data exceeds 65536 bytes
  halve the bodies until it fits (as the Go runner). Level: `error` when outcome is not `ok` or status ≥ 500,
  `warn` for 4xx, else `info`. Message: `<METHOD> <path> -> <status> (<ms> ms)`.
  Redaction: headers by `HttpCapture.isSensitiveHeader` (the platform credential is already dropped from
  the function's view by `requestHeaders`; the capture redacts what remains, and `X-Flowcatalyst-Signature`
  is covered by the pattern); `rawQuery` with the **ingestion rule** (every value `[redacted]` except paging
  and time-window names, `TraceRedactor`); bodies as `TraceRedactor.body` does (JSON/form credential fields);
  then the function's secret values and concealed values (J7) replaced everywhere.
- **`kind:"log"` records**: each line the function logs through `ctx.logger()` (`HostLogger`) during the
  invocation, level mapped (`TRACE/DEBUG→debug`, `INFO→info`, `WARNING→warn`, `ERROR→error`, `ALL→debug`),
  message cut to 4096 characters, secret/concealed values replaced, ≤ 500 per invocation. A line is recorded
  whatever the SLF4J level (tracing is when debug lines are wanted, as the Go runner records every log call
  of a live invocation); the host's own log output and `LogBudget` are unchanged by tracing.
- **`kind:"outbound"` records (NEW kind, see section 3)**: bind `HttpCapture.CURRENT` around the call into the
  function (only when `http` is live) with a sink that turns each `HttpCapture.Exchange` into one record, ≤ 100
  per invocation. Calls to the platform (control plane client) are not recorded.

### J5. Author API (`function-api`)

On `FunctionContext`, all `default` methods (no-op defaults keep existing implementations compiling), none
of which ever throws:

```java
default void trace(String context, String message) { trace(context, message, null, null); }
default void trace(String context, String message, @Nullable Map<String, ?> data) { trace(context, message, data, null); }
default void trace(String context, String message, @Nullable Map<String, ?> data, @Nullable TraceOptions options) {}
default boolean tracing(String context) { return false; }
default void setReference(String reference) {}
default void conceal(String value) {}
```

`record TraceOptions(@Nullable String level, @Nullable String reference)` with `level` one of
`debug|info|warn|error` (default `info`; anything else → `info`). Host behaviour (in `HostFunctionContext`,
reading `InvocationTrace.CURRENT`):

- `trace`: context must be declared (J2) — `http` or undeclared → nothing recorded, one WARN per (function,
  context) in the host log (`NOT_DECLARED` / not writable). Records only when the context is live for this
  invocation. `message` cut to 4096 characters; `data` serialized with the host's Jackson mapper, over 32768
  bytes → the record is not written (debug log); per-call `reference` (cut to 200 code points) applies to that
  record only. ≤ 500 per invocation. Values replaced as in J7.
- `tracing(context)`: true iff declared, live now and sampled in for this invocation.
- `setReference(ref)`: cut to 200 code points; stamped on every later record of the invocation (`http`,
  `log`, `trace`, `outbound`) and on the `http` record written at the end.
- `conceal(value)`: J7. Works outside an invocation too (e.g. in `init`).

`FakeFunctionContext` records `trace` calls (context, message, data, options), honours a settable set of
"live" contexts for `tracing`, keeps the last reference and the concealed values, with accessors for tests.

### J6. Shipping

`FunctionTraceShipper`, modelled on `runtime/TraceShipper` (copy its semantics, not necessarily its code):
records carry `id` (random UUID, stable across retries), `at` (host clock, unix ms); body `{runnerId, pool,
records}` POSTed to `/control/functions/trace-log` via a new `ControlPlane.traceLog(Control.TraceBatch)`
(`HttpControlPlane` implementation using the same authenticated client as `heartbeat`; 202 `{accepted,dropped}`).
Queue bounded by count (`FC_FN_TRACE_QUEUE`, default 5000) and bytes (`FC_FN_TRACE_QUEUE_BYTES`, 32 MiB), one
FIFO per function, full → drop the oldest record of the function with the most queued (counted); a batch takes
functions in turn; flush every 2 s or at 200 records; batch ≤ 500 records and under 1 MiB of encoded JSON; 413
splits (a lone record still too large is dropped); 401/403 retried once with a fresh token; 408/429/5xx and
transport errors back off and retry (bounded attempts); any other 4xx drops the batch. Never blocks a call. On
host shutdown, flush what it can within a bound. Log dropped counts, never content. Metrics as the ingestor's
other shippers do, named `fc_fn_trace_records_total{kind}`, `fc_fn_trace_dropped_total`,
`fc_fn_trace_flush_failures_total`.

### J7. Values replaced in every record

Per function (address), in memory: the plain values of the function's declared secrets from the desired
document (`FunctionDoc.secrets()` values) and a bounded deque (32, oldest out) of **concealed** values: those
given to `ctx.conceal(value)` and those **learned** from outbound answers (`TraceRedactor.credentialsIn` on each
captured response body, before the outbound record is built). Values shorter than 4 characters are ignored.
Dropped when the function is no longer in the desired document. Every record's message, data, headers, bodies,
query and URL path pass through a `TraceRedactor` built from these values. Make `TraceRedactor` (and the
methods needed: `text`, `headers`, `body`, `url`, `address`, `credentialsIn`) `public` so the functions packages
can use it; do not fork it.

## 3. The `outbound` record (new kind; the platform accepts it)

`context: "http"`, `kind: "outbound"`, level `info` (status < 400), `warn` (≥ 400), `error` (transport
failure). Message as `Capture.http`: `<METHOD> <scheme://host[:port]/path> -> <status> (<ms> ms)` or
`... failed: <FAILURE>`. Data — the ingestion trace's `http` record shape, so the same rendering applies:

```json
{ "client": "<service name>",
  "request":  { "method": "GET", "url": "https://ct.example/REST/Vehicles/[redacted]/GetVehicles?limit=10",
                "headers": {...}, "body": null, "bodyEncoding": "utf8", "bodyBytes": 0, "bodyTruncated": false },
  "response": { "status": 200, "headers": {...}, "body": "...", "bodyEncoding": "utf8", "bodyBytes": 57, "bodyTruncated": false },
  "durationMs": 12, "attempt": 1, "failure": "TIMEOUT" }
```

`response` is null for a transport failure; `failure` present only then. Bodies via `TraceRedactor.body` (32 KiB
each). Data must stay under 65536 bytes (the platform cap for `http`/`outbound`); otherwise halve the bodies.

## 4. Tests (all required; `ingestor/` must pass `./mvnw -o clean verify`)

- Desired parsing with and without `trace`; describe `traceContexts` validation (each rule).
- Sampler: rate 1 captures all; rate 3 captures 1 in 3; decided once per invocation.
- End-to-end through `FunctionListener` with a fixture function (look at existing `FunctionListenerTest` and its
  fixtures) and an in-memory `ControlPlane` capturing trace batches: with `http` live, one call yields an `http`
  record (request/response/outcome/caller), `log` records for its logger lines, an `outbound` record for an
  outbound call to a local `HttpServer`; nothing when no window or expired window; timeout → outcome `timeout`;
  exception → `error`.
- CTrack shape: the function logs in with `GET /Membership/Login?username=u&password=<pw from config, NOT a
  secret>`, reads `SessionToken` from the JSON answer, calls `GET /Vehicles/{session}/GetVehicles`: no shipped
  record contains the password or the session (learned path); and a second test where the session was obtained
  in `init` (no capture) and declared with `ctx.conceal`: the later outbound record's URL has `[redacted]` in the
  path.
- A declared secret value appearing in a log line, a trace message and a response body is replaced.
- Author API: `trace` on an undeclared context records nothing and warns once; `tracing` true only when live and
  sampled; `setReference` stamps later records; data over 32768 bytes not written; per-invocation caps.
- Shipper: batching, 413 split, 401 retry, 5xx retry/backoff, other 4xx drop, queue overflow drops oldest of the
  busiest function, ids stable across retries.
- `FakeFunctionContext` accessors.
- Mutation check: disable the learning step and confirm the learned CTrack test fails; restore (restore with `cp`
  + `touch`, not `mv`, so Maven recompiles).

## 5. Docs (Java side)

- `docs/jvm-functions-in-ingestor.md`: replace the "Not built yet: function traces" note with what is built.
- `docs/published/60-functions.md`: a short "Java" row/paragraph for the author API (trace, tracing,
  setReference, conceal, declaring `traceContexts` in the describe).
- Javadoc on every new public type and method, in the style of the surrounding code (plain, no marketing).

## 6. Out of scope / constraints

- Do not change Go code, the platform, the SPA or `/ui` (done separately), and do not touch git (no commits,
  no stash, no checkout).
- No broad process kills; use absolute paths; do not prefix commands with `cd`.
- Keep the off path cheap: no window for a function means no per-invocation allocation beyond a lookup.
