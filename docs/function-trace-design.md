# Function trace logs: design (2026-10-21)

Status: BUILT and verified end to end on 2026-10-21 (isolated fcdev: TypeScript function, runner, platform, CLI and the admin UI in a browser). Modelled on the
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
| 4 | **Time-boxed.** Enabling sets `enabled_until = now + minutes`. Caps (settings): `FC_FUNCTIONS_TRACE_MAX_MINUTES` = 60 for declared contexts, `FC_FUNCTIONS_TRACE_HTTP_MAX_MINUTES` = 15 for `http` (bodies are sensitive). Live iff `enabled_until > now`, **compared when read** (platform and runner clocks), so expiry needs no cron. A declared context may instead stay on "until switched off" (`indefinite`, permission `platform:function:trace:indefinite`), which still has a hard ceiling, `FC_FUNCTIONS_TRACE_INDEFINITE_MAX_DAYS` = 7; `http` never does (its 15-minute cap always applies). Switching off sets `enabled_until = now`. |
| 5 | **Sampling.** `sampleRate` N (1 to 1000, default 1) captures one invocation in N, decided once per invocation per context (counter based), so a captured call is captured whole. |
| 6 | **Business key.** Records carry `reference` (the author's business key: trip number, order number; at most 200 chars) and `correlationId` (the request's `X-Correlation-Id`, else empty) beside the `invocationId`. "Show me everything about trip X" is the main use. |
| 7 | **Durable independent of the call.** Records are queued in the runner and shipped in batches to the platform whatever the call's outcome. The queue is bounded; overflow drops the oldest records and counts them. Tracing never blocks or fails a call. |
| 8 | **Retention 30 days** (`FC_FUNCTIONS_TRACE_RETENTION_DAYS`) from the platform's `created_at` (never the runner's `at`), pruned in chunks (a) for the function in the background when a context is enabled, (b) every 10 minutes for each function with a live window, and (c) by an hourly sweep across functions bounded by `FC_FUNCTIONS_TRACE_PRUNE_MAX_ROWS` (5 M). |
| 9 | **Permissions.** `platform:function:trace:manage` (switch contexts on and off), `platform:function:trace:view` (read records' metadata, and of an `http` record its method, path without query, status, timings and sizes), `platform:function:trace:view-bodies` (read the sensitive parts: headers, query strings, bodies, log lines, author trace `data`; also required to enable `http`), `platform:function:trace:indefinite` (leave a declared context on until switched off; granted to `platform:messaging-admin`, the role that manages tracing, and super-admin's wildcard; not to viewers or publishers). `platform:viewer` does **not** hold `trace:view` (2026-10-25: trace records hold what functions printed and business keys; the roles that operate functions keep it). |
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

Both are cheap (a map lookup) when off. A `reference` given on a `trace` call is that record's only. Op 14 `trace.reference` (meta `{"reference":"TRIP-123"}`, empty success; `TOO_LARGE` over 200 characters) sets the invocation's business key, which the runner stamps on every later record of the call (`trace`, `http` and `log`); the SDKs' `setReference` sends it (added 2026-10-25, additive like 12 and 13).

Go constants in `internal/functions/abi` (`OpTrace`, `OpTracing`, `TraceCall`, `TracingCall`,
`TracingResult`, `TraceMaxMessage`, `TraceMaxData`, `TraceMaxReference`).

## 3. Contract: describe document

New optional key, validated by `abi.Describe.Validate`:

```json
"traceContexts": [ { "name": "orders", "description": "What the order mapping decided, line by line." } ]
```

`name` matches `^[a-z][a-z0-9-]{0,39}$` and is not `http`; at most 20; unique; `description` at most 200
bytes of UTF-8 (optional; every SDK counts bytes, like `abi.Describe.Validate`). Like every describe key, an older platform or runner rejects an unknown key.

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
`outcome` is `ok`, `timeout`, `trap`, `out_of_memory`, `bad_response`, `busy` or `error` (a 429 busy refusal is not
captured: the capture starts once the call holds a concurrency slot, so refusals take no sampling turn either).
Redacted request and response headers: any (case-insensitive) name containing `auth`, `key`, `token`, `secret`,
`password`, `signature`, `cookie` or `session` (so `authorization`, `x-forwarded-authorization`, `api-key`, `apikey`,
`x-functions-key`, `ocp-apim-subscription-key`, `x-goog-api-key`, `set-cookie`, and `x-flowcatalyst-signature`, so a
reader cannot replay a signed delivery); the value becomes `"[redacted]"`. In `rawQuery` the values of `access_token`,
`code`, `token`, `sig`, `signature`, `X-Amz-*`, `key`, `apikey`, `api_key`, `password`, `secret` and any name matching
the header patterns become `[redacted]`.

Response: `202 {"accepted":N,"dropped":M}`. The platform enforces: at most 500 records and 1 MiB per batch;
**per function at most 200 records/second** (the excess counts as dropped); message cut to 4096 characters;
`data` over 65536 bytes (32768 for `trace`) replaced by `{"truncated":true,"bytes":N}`; an unknown address is
dropped; a record is accepted only if the function's context row exists and `at <= enabled_until + 10s`.
The rate cap is per platform process (an in-memory bucket each; N replicas admit up to N x 200/s): it bounds a
runaway, it is not a quota. An `at` more than a minute ahead of arrival is clamped to arrival (it then meets the
gate as "now"); one more than an hour behind is dropped. A record may carry `id`, the runner's UUID for it (the
same on every retry); the platform stores it as `ftl_r<id>` and skips one it already holds, so a retried batch is
stored once. Without an `id` (an older runner) the platform assigns one. The platform sets `createdAt`.

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
Migration `092_function_trace_remedies.sql` (no transaction): `fng_trace_contexts.indefinite`; retention indexes
`(created_at)` and `(function_id, created_at)` built concurrently, the `(at)` index dropped; old far-future
sentinel rows converted; `trace:indefinite` to `platform:messaging-admin`; `trace:view` removed from `platform:viewer`.
New permissions are granted to the same roles that hold the matching function permissions (follow how `085` grants).

## 7. Contract: platform API (`/api/functions/{id}/...`)

| Call | Permission | Result |
|---|---|---|
| `GET /trace` | view | `{ "contexts": [ { "name", "description", "builtIn", "enabledUntil" (RFC 3339 or null), "indefinite", "sampleRate", "live", "maxMinutes", "indefiniteMaxDays" } ] }` : the union of `traceContexts` of the function's READY versions, plus `http` (`indefiniteMaxDays` 0) |
| `PUT /trace/{context}` body `{ "minutes": 5, "sampleRate": 1 }` | manage (and view-bodies for `http`) | the context's state as above. `minutes` 1 to the context's cap, else 400 `TRACE_WINDOW_TOO_LONG`; unknown context 400 `TRACE_CONTEXT_UNKNOWN`. Upserts, bumps the pool revision, prunes the function's logs older than retention (chunked, after commit) |
| `PUT /trace/{context}` body `{ "indefinite": true, "sampleRate": 1 }` | manage and indefinite | declared contexts only (`http`: 400 `TRACE_INDEFINITE_NOT_ALLOWED`); the window ends `indefiniteMaxDays` later |
| `DELETE /trace/{context}` | manage | sets `enabled_until` to now (records queued before are still accepted within the 10 s grace), bumps the pool revision; idempotent (a second call keeps the first end) |
| `GET /trace-log` | view | query `context kind level reference correlationId invocationId version since until limit cursor`; `since`/`until` RFC 3339 or unix ms; `limit` default 100, max 500; newest first; keyset cursor on `(at,id)`. `{ "records": [ {id, at, version, invocationId, correlationId, reference, context, kind, level, message, data, runnerId} ], "nextCursor": null or string }`. Without view-bodies the sensitive parts are withheld and the record has `redacted: true`: an `http` record's data keeps only `request.{method,path,bodyBytes,bodyTruncated}`, `response.{status,bodyBytes,bodyTruncated}`, `durationMs` and `outcome` (bodies null, `bodiesRedacted: true`); a `log` record has an empty message and null data; a `trace` record null data. The same rule applies to `/ui` and the CLI. |

Errors use the platform's standard problem shape.

## 8. Runner behaviour (`internal/functions/runner`)

- Keep, per function, the trace windows from the desired document (replace atomically on apply).
- At the start of each invocation compute the set of live contexts for it (window live now and sampled in); keep it in the invocation state. The `http` capture happens after the response is written, on every outcome (success, 4xx/5xx the function sent, timeout, trap, out of memory, bad response) for calls that resolved to a function.
- `trace` and `tracing` ops are answered in the host-call path (`capabilities.go`); off means a map lookup and no allocation.
- Records go to one bounded queue per runner (`FC_FN_TRACE_QUEUE`, default 5000), one FIFO per function: when full it drops the oldest record of the function with the most queued (counted), and a batch takes the functions in turn. One invocation queues at most 500 `log` and 500 `trace` records. A batcher flushes every 2 s or at 200 records, measuring a batch by the records' encoded JSON bytes, with retry and backoff on failure and without ever blocking a call: a 413 splits the batch (a lone record that is still too large is dropped), a 401/403 is retried with a fresh token (the control client invalidates its cached token and retries once at once), 408/429/5xx/transport errors back off and retry, any other 4xx drops the batch. On shutdown it waits (bounded) for captures still running, then flushes what it can (bounded time).
- Metrics: `fc_fn_trace_records_total{kind}`, `fc_fn_trace_dropped_total`, `fc_fn_trace_flush_failures_total`.
- Runtime-neutral: the capture lives in the generic invoke layer so it works for the wasm and js runtimes (and any future one).

## 9. SDKs

The three SDKs expose the same operations; where the handler's first argument lives differs by language.

| SDK | API |
|---|---|
| TypeScript (`@flowcatalyst/fn`) | `req.trace(context, message, data?, { level?, reference? })`, `req.tracing(context): boolean`, `req.setReference(ref)` (op 14: stamps every later record of the invocation, `http` and `log` included). Declaration: `fn.traceContexts({ name: "description" })`. |
| Go (`fn`) | `fn.Trace(ctx, context, msg, data, opts ...TraceOption)`, `fn.Tracing(ctx, context) bool`, `fn.SetReference(ctx, ref)`, `fn.WithReference(r)`, `fn.WithLevel(l)`. Declaration: `fn.TraceContexts(fn.TraceContext{...})`. |
| Rust (`flowcatalyst-fn`) | `ctx.trace(context, msg, data, TraceOpts)`, `ctx.tracing(context) -> bool`, `ctx.set_reference(r)`. Declaration: `app.trace_context(name, description)`. |

Declarations validate what `abi.Describe.Validate` checks and fail at registration (build and describe), like `dispatches`.
None of the calls can fail or throw: a runner without the ops, `NOT_DECLARED` or any host error is swallowed
(`NOT_DECLARED` is written once per context to stderr as a warning; on `wasm32-unknown-unknown`, which has no stderr, as a WARN log record).
`msg` is cut to 4096 bytes and `reference` to 200 characters (code points; TypeScript never splits a surrogate pair) client-side; `data` is left to the host's `TOO_LARGE`. `setReference` also keeps the value locally and sends it on each `trace` call, so an older runner without op 14 still stamps trace records.

## 10. UI and CLI

- **UI** (function detail): a *Diagnostics* section. Contexts list with a toggle and a duration choice (5, 15, 30, 60 within the cap) and sample rate, and a countdown while live; a log viewer under it with filters (context, kind, level, reference, correlation id, invocation) that refreshes every 5 s while any context is live and can be paused; `http` records expand to request and response panels (text or base64 labelled, truncation noted, "bodies hidden" when the viewer lacks view-bodies); records of one invocation group together. Controls follow the permissions.
- **CLI:** `fcdev fn trace on <address> <context> --for 5m [--sample N] [--no-limit]`, `fcdev fn trace off <address> <context>`, `fcdev fn trace status <address>`, `fcdev fn trace log <address> [--context --kind --level --reference --correlation --invocation --since] [-f]`, `fcdev fn trace run <address> <context> [--for 5m] [--keep]` (follows a window that already covers the run without touching it; otherwise opens one and on exit restores what it found: the earlier window with its time left, or off; a window changed during the run is left alone).

## 11. Work packages

| WP | Scope | Needs |
|---|---|---|
| contract | abi ops and describe key, `control` types and client call (done, committed first) | |
| A platform | migration, entities and repository, use cases and events, permissions and role grants, API, ingest endpoint with the limits above, desired-state field, retention prune, OpenAPI lock and generated types | contract |
| B runner | gate, ops 12/13, http capture, queue and batcher, metrics, tests with the fixture runner | contract |
| C SDKs and author docs | fn-ts, fn-go, fn-rust, the stability contract and `published/60-functions.md` | contract |
| D UI and CLI | Diagnostics section, `fcdev fn trace` | A's API and generated types |

Hand-offs after: Java runner and Rust platform notes (additive contract), and the framework-level process log module for applications.

## 12. As built: differences from this design, and follow-ups

Differences (accepted):
- Record `at` in the platform's read API is RFC 3339 UTC (the runner ships unix ms). `DELETE /trace/{context}` is 204. `bodiesRedacted` is present only when bodies are withheld.
- SDK surface: TypeScript `req.trace` (handlers receive `req`), Go `fn.Trace(ctx, ...)`, Rust `ctx.trace`; contexts declared with `fn.traceContexts(...)`, `fn.TraceContexts(...)`, `app.trace_context(...)` (section 9).
- The runner queue is bounded by bytes as well as count (`FC_FN_TRACE_QUEUE_BYTES`, 32 MiB), because a record can carry two 64 KiB bodies. A record over the platform's data cap has its bodies halved until it fits.
- The ingest body is accepted with unknown fields at every depth (`httpcompat.RelaxRequestBodiesDeep`); the runner drops a batch the platform refuses.
- `platform:function-deployer` (the CI role) holds none of the trace permissions.

Follow-ups noticed in the end-to-end run, not done:
- `log` records written before `setReference` carry no `reference` (op 14 stamps only later records); stamp them when the invocation ends.
- `durationMs` is whole milliseconds, so fast calls read 0; consider microsecond precision.
- A guest-made 500 (an uncaught exception answered as `FUNCTION_PANIC`) has outcome `ok`; the status and level show the error.

Changed after first use (2026-10-21):
- **No time limit.** `PUT /trace/{context}` also takes `{ "indefinite": true }` (minutes ignored, `minutes` is no longer required). Superseded on 2026-10-25 (below): it is now "until switched off (max N days)", not unbounded.
- **The log is its own page**, `/functions/{id}/traces` (the drawer keeps only the switches and a "View trace logs" button): the records are wide, and the drawer was too narrow for them. The switches are a list of cards rather than a table, which wrapped badly in the drawer.

Remedies (owner-approved, 2026-10-25), after a review of the "no time limit" option and related findings:
- **Indefinite windows are bounded and deliberate.** They need `platform:function:trace:indefinite` (seeded; granted to
  `platform:messaging-admin` and, by its wildcard, super-admin; not to viewers or publishers), are refused for `http`
  (`TRACE_INDEFINITE_NOT_ALLOWED`; the http minute cap always applies), and end by themselves after
  `FC_FUNCTIONS_TRACE_INDEFINITE_MAX_DAYS` (7): stored as `indefinite = true` with `enabled_until = now + ceiling`
  (column added by migration 092, which converts the old 9999 sentinel). The UI and SPA call it "Until switched off
  (max N days)" and ask for confirmation; `platform:function:trace:enabled` always carries `indefinite` and the ceiling.
- **Retention reads `created_at`.** A runner clock far off can no longer keep records for ever or delete them early;
  ingest clamps a future `at` to arrival and drops one over an hour old. The sweep is hourly and bounded by
  `FC_FUNCTIONS_TRACE_PRUNE_MAX_ROWS` (5 M): 120 M rows a day, against the worst case of 17.28 M a day for one function
  at the 200/s cap on one process (about seven such functions per process), and every function with a live window is
  also pruned every 10 minutes. The prune after an enable runs in the background (detached, 5-minute bound, one per
  function at a time), so enabling cannot hang on it.
- **Redaction by pattern**, for headers and query parameters (section 5), including the delivery signature.
- **Who sees what.** `trace:view` alone shows records' metadata and an http record's method, path, status, timings and
  sizes; headers, query strings, bodies, log lines and trace `data` need `trace:view-bodies` (API, `/ui`, CLI and SPA).
  `platform:viewer` no longer holds `trace:view` (decision 9).
- **Switching off** sets `enabled_until = now`, so records captured just before and still queued are accepted within
  the 10 s grace. `fcdev fn trace run` no longer clobbers someone else's window (section 10).
- **Runner shipping**: byte-exact batches split on 413, 401/403 retried with a fresh token, the capture installed after
  the concurrency acquire, shutdown waits for in-flight captures, per-invocation cap for author trace records, per-function
  fairness in the queue (section 8).
- **Idempotent ingest**: the runner's record `id` (UUID) makes a retried batch store once; its absence is tolerated.
  The 200/s limiter stays per process (documented, kept).
- **SDKs**: `setReference` reaches the host (op 14), a per-call reference applies to that record only, description
  limits count bytes everywhere.
- **`/ui` follow stream** resolves the caller again every tick (`pages.ReauthWith` over the UI's authenticator) and
  stops when the session or the permission is gone; declared contexts are parsed once per version (cached).
