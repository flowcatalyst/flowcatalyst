# Function trace logs: design (2026-10-21)

Status: owner-approved plan (2026-10-21), contract types committed, build in progress. Modelled on the
Integral platform's *process logs* (`inhance/InhanceMono/packages_root/packages/integral-service-v2/docs/process-log-design.md`):
opt-in, named, time-boxed capture that is a cheap no-op the rest of the time. Functions are first-party
code (`docs/function-stability-contract.md` section 0.1).

A later piece offers the same capability to framework applications (the camel ingest control plane is
the first user); it reuses the table shape and the gate semantics below but not the code.

## 1. Locked decisions

| # | Decision |
|---|---|
| 1 | **Gated, never always-on.** Nothing is stored unless an operator switched a context on. A function's stdout/stderr and `log` calls keep going to the normal log pipeline (slog) exactly as today. |
| 2 | **Grain: (function, context).** A context is a name. The function declares its own in the describe document (`traceContexts`); one built-in context, `http`, needs no declaration. |
| 3 | **`http` is the built-in request/response capture.** While it is live, each invocation of the function also records one `kind:"http"` record (request, response, timing, outcome) and the `kind:"log"` records of everything the invocation printed or logged. |
| 4 | **Time-boxed.** Enabling sets `enabled_until = now + minutes`. Caps (settings): `FC_FUNCTIONS_TRACE_MAX_MINUTES` = 60 for declared contexts, `FC_FUNCTIONS_TRACE_HTTP_MAX_MINUTES` = 15 for `http` (bodies are sensitive). Live iff `enabled_until > now`, **compared when read** (platform and runner clocks), so expiry needs no cron. |
| 5 | **Sampling.** `sampleRate` N (1 to 1000, default 1) captures one invocation in N, decided once per invocation per context (counter based), so a captured call is captured whole. |
| 6 | **Business key.** Records carry `reference` (the author's business key: trip number, order number; at most 200 chars) and `correlationId` (the request's `X-Correlation-Id`, else empty) beside the `invocationId`. "Show me everything about trip X" is the main use. |
| 7 | **Durable independent of the call.** Records are queued in the runner and shipped in batches to the platform whatever the call's outcome. The queue is bounded; overflow drops the oldest records and counts them. Tracing never blocks or fails a call. |
| 8 | **Retention 30 days** (`FC_FUNCTIONS_TRACE_RETENTION_DAYS`), pruned (a) for the function when a context is enabled, chunked, and (b) by a daily backstop across functions. |
| 9 | **Permissions.** `platform:function:trace:manage` (switch contexts on and off), `platform:function:trace:view` (read records; request/response bodies are withheld), `platform:function:trace:view-bodies` (read bodies; also required to enable `http`). |
| 10 | **Audit.** Enabling and disabling are use cases with events `platform:function:trace:enabled` and `platform:function:trace:disabled` (the standard usecaseop envelope and audit log). |
| 11 | **Propagation.** A toggle bumps the function's pool revision in the same transaction (as settings writes do), which wakes the runners' long-poll on `fng_desired`: effective in about a second, not a polling interval. |
| 12 | **Not in v1:** a per-client filter, live push to the UI (the UI polls), table partitioning (volume is low by design), and Java/Rust runner support (a hand-off doc follows). |

## 2. Contract: host operations (ABI v1, additive)

Two new ops. A runner that predates them answers `UNKNOWN_OP`, which every SDK treats as "not tracing"; no
existing module changes.

| Op | Name | Call meta | Result |
|---|---|---|---|
| 12 | `trace` | `{"context":"orders","level":"info","msg":"resolved transport order","data":{"lines":3},"reference":"TRIP-123"}` | success with empty meta. **Never an error for a context that is not live** (a silent no-op). `level` optional (`debug` `info` `warn` `error`, default `info`); `data` optional JSON object; `reference` optional. Errors: `NOT_DECLARED` (context neither declared in the version's describe nor `http`), `BAD_REQUEST` (malformed meta, `http` used by a guest: the built-in context is not writable by functions), `TOO_LARGE` (`msg` over 4096 bytes or `data` over 32768 bytes). |
| 13 | `tracing` | `{"context":"orders"}` | result meta `{"live":true}`. `live` is true iff the context is enabled for this function now **and** this invocation was sampled in. `NOT_DECLARED` as above. Lets the guest skip expensive work. |

Both are cheap (a map lookup) when off. `reference` given on any `trace` call is remembered for the invocation and also stamped on its `http` and `log` records.

Go constants in `internal/functions/abi` (`OpTrace`, `OpTracing`, `TraceCall`, `TracingCall`,
`TracingResult`, `TraceMaxMessage`, `TraceMaxData`, `TraceMaxReference`).

## 3. Contract: describe document

New optional key, validated by `abi.Describe.Validate`:

```json
"traceContexts": [ { "name": "orders", "description": "What the order mapping decided, line by line." } ]
```

`name` matches `^[a-z][a-z0-9-]{0,39}$` and is not `http`; at most 20; unique; `description` at most 200
characters (optional). Like every describe key, an older platform or runner rejects an unknown key.

## 4. Contract: desired state

`control.Function` gains `trace`: the function's windows that are live when the document is rendered:

```json
"trace": [ { "context": "http", "enabledUntilMs": 1761000000000, "sampleRate": 1 } ]
```

Absent when none. The runner compares `enabledUntilMs` with its own clock on every use. Expired entries
simply stop matching (they leave the document the next time it is re-rendered).

## 5. Contract: shipping records (runner to platform)

`POST /control/functions/trace-log`, runner-control permission (`platform:function:runner:control`), body:

```json
{ "runnerId": "…", "pool": "default",
  "records": [ {
    "at": 1761000000123, "address": "billing.invoice-pdf", "version": 7,
    "invocationId": "…", "correlationId": "…", "reference": "TRIP-123",
    "context": "orders", "kind": "trace", "level": "info",
    "message": "resolved transport order", "data": { "lines": 3 } } ] }
```

`kind` is `trace` (author), `http` (built-in capture) or `log` (stdout/stderr and `log` calls during an
`http`-live invocation). For `kind:"http"`, `data` is:

```json
{ "request":  { "method":"POST", "path":"/orders", "rawQuery":"a=1", "headers":{"content-type":["application/json"]},
                "body":"…", "bodyEncoding":"utf8", "bodyBytes":1234, "bodyTruncated":false,
                "caller":{"kind":"principal","id":"prn_…"} },
  "response": { "status":200, "headers":{…}, "body":"…", "bodyEncoding":"utf8", "bodyBytes":57, "bodyTruncated":false },
  "durationMs": 12, "outcome": "ok" }
```

`bodyEncoding` is `utf8` when the bytes are valid UTF-8, else `base64`. Bodies are cut at
`FC_FN_TRACE_BODY_BYTES` (default 65536) with `bodyTruncated` true and `bodyBytes` the real size.
`outcome` is `ok`, `timeout`, `trap`, `out_of_memory`, `bad_response`, `busy` or `error`. Redacted request and response
headers (case-insensitive name): `authorization`, `proxy-authorization`, `cookie`, `set-cookie`,
`x-api-key`, `x-auth-token`, and any name containing `secret`, `token` or `password`; the value becomes `"[redacted]"`.

Response: `202 {"accepted":N,"dropped":M}`. The platform enforces: at most 500 records and 1 MiB per batch;
**per function at most 200 records/second** (the excess counts as dropped); message cut to 4096 characters;
`data` over 65536 bytes (32768 for `trace`) replaced by `{"truncated":true,"bytes":N}`; an unknown address is
dropped; a record is accepted only if the function's context row exists and `at <= enabled_until + 10s`.
The platform assigns `id` and `createdAt`.

## 6. Storage

Migration `087_function_trace.sql`, re-runnable (`IF NOT EXISTS`):

```
fng_trace_contexts(function_id text, context text, enabled_until timestamptz null, sample_rate int not null default 1,
                   enabled_by text null, updated_at timestamptz not null, primary key (function_id, context))
fng_trace_log(id text primary key, function_id text not null, version int not null, invocation_id text not null,
              correlation_id text null, reference text null, context text not null, kind text not null,
              level text not null, message text not null, data jsonb null, at timestamptz(6) not null,
              runner_id text null, created_at timestamptz not null)
```

Indexes on `fng_trace_log`: `(function_id, at desc, id desc)`; `(function_id, reference, at desc) where reference is not null`;
`(function_id, correlation_id, at desc) where correlation_id is not null`; `(function_id, invocation_id)`;
`(function_id, context, at desc)`; `(at)` for the retention scan. No foreign keys (the function table's convention). Append only: no update path.
New permissions are granted to the same roles that hold the matching function permissions (follow how `085` grants).

## 7. Contract: platform API (`/api/functions/{id}/...`)

| Call | Permission | Result |
|---|---|---|
| `GET /trace` | view | `{ "contexts": [ { "name", "description", "builtIn", "enabledUntil" (RFC 3339 or null), "sampleRate", "live", "maxMinutes" } ] }` : the union of `traceContexts` of the function's READY versions, plus `http` |
| `PUT /trace/{context}` body `{ "minutes": 5, "sampleRate": 1 }` | manage (and view-bodies for `http`) | the context's state as above. `minutes` 1 to the context's cap, else 400 `TRACE_WINDOW_TOO_LONG`; unknown context 400 `TRACE_CONTEXT_UNKNOWN`. Upserts, bumps the pool revision, prunes the function's logs older than retention (chunked, after commit) |
| `DELETE /trace/{context}` | manage | sets `enabled_until` null, bumps the pool revision; idempotent |
| `GET /trace-log` | view | query `context kind level reference correlationId invocationId version since until limit cursor`; `since`/`until` RFC 3339 or unix ms; `limit` default 100, max 500; newest first; keyset cursor on `(at,id)`. `{ "records": [ {id, at, version, invocationId, correlationId, reference, context, kind, level, message, data, runnerId} ], "nextCursor": null or string }`. Without view-bodies, `data.request.body` and `data.response.body` are null and the record has `bodiesRedacted: true`. |

Errors use the platform's standard problem shape.

## 8. Runner behaviour (`internal/functions/runner`)

- Keep, per function, the trace windows from the desired document (replace atomically on apply).
- At the start of each invocation compute the set of live contexts for it (window live now and sampled in); keep it in the invocation state. The `http` capture happens after the response is written, on every outcome (success, 4xx/5xx the function sent, timeout, trap, out of memory, bad response) for calls that resolved to a function.
- `trace` and `tracing` ops are answered in the host-call path (`capabilities.go`); off means a map lookup and no allocation.
- Records go to one bounded queue per runner (`FC_FN_TRACE_QUEUE`, default 5000), drop-oldest with a counter. A batcher flushes every 2 s or at 200 records, with retry and backoff on failure and without ever blocking a call. On shutdown it flushes what it can (bounded time).
- Metrics: `fc_fn_trace_records_total{kind}`, `fc_fn_trace_dropped_total`, `fc_fn_trace_flush_failures_total`.
- Runtime-neutral: the capture lives in the generic invoke layer so it works for the wasm and js runtimes (and any future one).

## 9. SDKs

| SDK | API |
|---|---|
| TypeScript (`@flowcatalyst/fn`) | `ctx.trace(context, message, data?, { level?, reference? })`, `ctx.tracing(context): boolean`, `ctx.setReference(ref)` (stamps later calls of the invocation). Definition option `traceContexts: [{ name, description }]`. |
| Go (`fn`) | `ctx.Trace(context, msg string, data map[string]any, opts ...TraceOption)`, `ctx.Tracing(context) bool`, `fn.WithReference(r)`, `fn.WithLevel(l)`, `ctx.SetReference(r)`. Describe: `TraceContexts`. |
| Rust (`flowcatalyst-fn`) | `ctx.trace(context, msg, data, opts)`, `ctx.tracing(context) -> bool`, `ctx.set_reference(r)`. Describe: `trace_contexts`. |

None of these can fail or throw: a runner without the ops, `NOT_DECLARED` or any host error is swallowed
(`NOT_DECLARED` is written once per context to stderr as a warning).

## 10. UI and CLI

- **UI** (function detail): a *Diagnostics* section. Contexts list with a toggle and a duration choice (5, 15, 30, 60 within the cap) and sample rate, and a countdown while live; a log viewer under it with filters (context, kind, level, reference, correlation id, invocation) that refreshes every 5 s while any context is live and can be paused; `http` records expand to request and response panels (text or base64 labelled, truncation noted, "bodies hidden" when the viewer lacks view-bodies); records of one invocation group together. Controls follow the permissions.
- **CLI:** `fcdev fn trace on <address> <context> --for 5m [--sample N]`, `fcdev fn trace off <address> <context>`, `fcdev fn trace status <address>`, `fcdev fn trace log <address> [--context --kind --level --reference --correlation --invocation --since] [-f]`.

## 11. Work packages

| WP | Scope | Needs |
|---|---|---|
| contract | abi ops and describe key, `control` types and client call (done, committed first) | |
| A platform | migration, entities and repository, use cases and events, permissions and role grants, API, ingest endpoint with the limits above, desired-state field, retention prune, OpenAPI lock and generated types | contract |
| B runner | gate, ops 12/13, http capture, queue and batcher, metrics, tests with the fixture runner | contract |
| C SDKs and author docs | fn-ts, fn-go, fn-rust, the stability contract and `published/60-functions.md` | contract |
| D UI and CLI | Diagnostics section, `fcdev fn trace` | A's API and generated types |

Hand-offs after: Java runner and Rust platform notes (additive contract), and the framework-level process log module for applications.
