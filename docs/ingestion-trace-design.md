# Ingestion traces: design (2026-10-30)

Status: APPROVED by the owner on 2026-10-30, not built. Modelled on the function trace logs
(`docs/function-trace-design.md`): opt-in, time-boxed, sampled capture that is a cheap no-op the rest of
the time. It answers "the ingestion is running but nothing shows": an operator switches tracing on for an
ingestion and sees, for a minute, what each runtime's polls and exchanges did, the HTTP calls an
extension or an api poll made included, with their requests and responses.

## 1. Locked decisions

| # | Decision |
|---|---|
| 1 | **Gated, never always-on.** Nothing is captured unless an operator switched tracing on for the ingestion. Off, tracing costs a map lookup per poll or exchange and nothing else. Tracing never blocks, slows or fails ingestion. |
| 2 | **Grain: (ingestion, runtime).** A *window* is switched on per ingestion. Each runtime that runs the ingestion captures on its own. |
| 3 | **The window counts from pickup.** A runtime learns of a window at its next sync (`POST /poll`, every `INGEST_POLL_INTERVAL`: 1 minute in production, 15 s under fcdev). Its capture lasts `windowSecs` (default 60; 60, 300 or 900; cap 900) **from the moment that runtime first sees the window**, so a 1-minute window is a minute of capture however long the sync took. The platform stores a **hard stop** = enable time + `windowSecs` + `FC_INGESTION_TRACE_PICKUP_GRACE` (default 2 minutes, covering one production sync); a runtime never captures past it and the platform refuses records after it (+10 s grace). Switching off sets the hard stop to now. |
| 4 | **Sampling by time.** A runtime starts a capture for an ingestion only when its last capture of that ingestion started `sampleEverySecs` or more ago (default 5; 1 to 60). A capture is **one poll cycle** (a polled source's poll, everything it called and dispatched included) **or one exchange** (a push, file or queue message through the route), captured whole. So at most about 12 captures per runtime per minute at the default. |
| 5 | **Empty polls are visible.** A captured poll that found nothing is recorded (its HTTP calls show why). Without tracing, every runtime also reports **polls** per window and the **last poll time**, so the Activity page shows "polling, nothing new" instead of nothing. |
| 6 | **Bodies are captured, secrets never.** Request and response bodies are kept (cut at 32 KiB each), with the redaction of section 6. Bodies are customer data: reading them needs `view-bodies`. |
| 7 | **Retention 7 days** (`FC_INGESTION_TRACE_RETENTION_DAYS`) from the platform's `created_at`, pruned by the existing hourly ingestion retention sweep. |
| 8 | **Permissions.** `platform:ingestion:trace:manage` (switch on and off), `platform:ingestion:trace:view` (records' metadata: kind, time, outcome, an HTTP call's method, URL path, status, duration and sizes), `platform:ingestion:trace:view-bodies` (headers, query strings, bodies, payload excerpts, error text; also needed to switch tracing on). Granted like the function trace permissions: to the roles that manage ingestions (`manage`, `view`, `view-bodies`) and the roles that operate them (`view`); `platform:viewer` gets none. Super-admin by its wildcard. |
| 9 | **Audit.** Switching on and off are use cases with events `platform:ingestion:trace:enabled` and `platform:ingestion:trace:disabled` (usecaseop envelope, audit log). |
| 10 | **Activity is its own page**, `/ui/ingestions/{id}/activity`: the counters, last poll, last exchange, last failure, the runtimes, the trace switch and the trace records. The drawer's Activity tab keeps a short summary and links to it; the list row gets an "Activity" action. |
| 11 | **Not in v1:** tracing a test run, a global cross-ingestion trace view, live push (the page polls every 5 s while a window is live). |

## 2. Contract: the sync answer (platform to runtime)

`PollResponse` gains a top-level list, **not** a field of `RuntimeIngestion` (the runtime rebuilds a route
when its ingestion record changes; switching tracing must not restart routes):

```json
"traces": [ { "ingestionId": "ing_…", "windowId": "itw_…", "windowSecs": 60, "sampleEverySecs": 5,
              "hardStop": "2026-10-30T11:30:00Z" } ]
```

Listed: the windows whose `hardStop` is after now, for ingestions this answer lists to this runtime (a polled
ingestion only to its lease holder). Absent when none. Additive within contract version 2: the runtime reads
poll answers with unknown properties ignored, so an older runtime ignores it.

Runtime semantics: a `windowId` seen for the first time starts a local window ending at
`min(now + windowSecs, hardStop)`; the same `windowId` seen again changes nothing; a different `windowId`
for the ingestion replaces the window (a new start); an ingestion no longer listed has no window (switched
off, or ended). The local clock and `hardStop` are compared on every use.

## 3. Contract: report additions (runtime to platform)

`ExchangeWindow` gains `polls` (int, polls finished in the window, empty ones included) and `lastPollAt`
(RFC 3339, the end of the last poll in the window, optional). A window with `polls > 0` is reported even
when nothing else happened. Additive within version 2 (the report body is relaxed for unknown fields).
Stored on `ing_exchange_metrics` (new columns, a re-sent window replaces as today).

## 4. Contract: shipping records (runtime to platform)

`POST /api/ingestion-runtime/v1/traces`, the runtime permission (`auth.RequireIngestionRuntime`), body
relaxed for unknown fields (`httpcompat.RelaxRequestBodiesDeep`, registered in `tools/dump-spec` like poll
and report):

```json
{ "runtimeId": "fcdev-8080",
  "records": [ {
    "id": "8d3c…-uuid", "ingestionId": "ing_…", "version": 1, "windowId": "itw_…",
    "captureId": "c-uuid", "at": 1761823500123,
    "kind": "poll", "level": "info", "message": "poll: 1 call, 0 payloads", "data": { … } } ] }
```

- `kind`: `poll` (one per captured poll cycle, written when it ends), `exchange` (one per payload through the
  route in a capture), `http` (one per outbound HTTP attempt in a capture), `error` (a failure inside a
  capture that is not one of the above). Records of one capture share `captureId`.
- `level`: `info`, `warn`, `error`.
- `message`: one line, cut to 1000 characters.
- `data` by kind:
  - `poll`: `{ "startedAt", "durationMs", "httpCalls", "payloads", "records", "jobs", "outcome": "ok"|"failed", "error"?, "nextDelayMs"? }`
  - `exchange`: `{ "source": {…headers the source set: file name, message id, sender…}, "identity"?, "payloadBytes",
    "payload": {body excerpt as in http bodies}, "records", "jobs", "outcome": "completed"|"rejected"|"failed", "error"? }`
  - `http`: `{ "client", "request": { "method", "url" (scheme, host, path, redacted query), "headers", "body",
    "bodyEncoding", "bodyBytes", "bodyTruncated" }, "response": { "status", "headers", "body", "bodyEncoding",
    "bodyBytes", "bodyTruncated" } | null, "durationMs", "attempt", "failure"? }`
  - `error`: `{ "error", "kind"? }`
- Bodies: `bodyEncoding` `utf8` when valid UTF-8, else `base64`; cut at 32 KiB with `bodyTruncated` and the
  real `bodyBytes`.

Platform rules: at most 500 records and 1 MiB per batch; per ingestion at most 50 records per second per
platform process (excess counted as dropped); a record is accepted only if `windowId` is the ingestion's
window and `at <= hardStop + 10 s`, else dropped; `at` more than a minute ahead is clamped to arrival, more
than an hour behind is dropped; `data` over 80 KiB is replaced by `{"truncated":true,"bytes":N}`; a record
`id` the platform already holds is skipped (a retried batch stores once; stored as `itr_<uuid>`). The platform
sets `created_at`. Answer `202 {"accepted":N,"dropped":M}`.

## 5. Storage (migration 094, re-runnable)

```
ing_trace_windows(ingestion_id text primary key, window_id text not null, window_secs int not null,
                  sample_every_secs int not null, hard_stop timestamptz not null, enabled_by text null,
                  enabled_at timestamptz not null, updated_at timestamptz not null)
ing_trace_log(id text primary key, ingestion_id text not null, version int not null, window_id text not null,
              capture_id text not null, runtime_id text not null, kind text not null, level text not null,
              message text not null, data jsonb null, at timestamptz(6) not null, created_at timestamptz not null)
ing_exchange_metrics: + polls int not null default 0, + last_poll_at timestamptz null
```

Indexes on `ing_trace_log`: `(ingestion_id, at desc, id desc)`, `(ingestion_id, capture_id)`,
`(created_at)` for retention. No foreign keys (the ingestion tables' convention); deleting an ingestion
deletes its window and records (in the delete use case, like its other runtime rows). Permissions seeded and
granted in the migration as section 1 (8) says, following how `087`/`092` grant the function ones.

## 6. Redaction (runtime, before anything leaves it)

- **Headers**: a name containing (case-insensitive) `auth`, `key`, `token`, `secret`, `password`,
  `signature`, `cookie` or `session` has its value replaced by `[redacted]` (the framework already does this
  for the headers it hands out, section 8; the runtime applies the same rule again to source headers).
- **Query**: the values of `access_token`, `code`, `token`, `sig`, `signature`, `X-Amz-*`, `key`, `apikey`,
  `api_key`, `password`, `secret`, and any name matching the header patterns, become `[redacted]`.
- **Bodies**: a JSON body (and a form body, `application/x-www-form-urlencoded`) has the value of every field,
  at any depth, whose name matches the header patterns or is `client_secret`, `access_token`,
  `refresh_token`, `id_token`, `assertion`, `credential`, `private_key` replaced by `[redacted]`. So MiX's
  token call shows `grant_type`, `username`, `scope`, `client_id` and a redacted `password` and
  `client_secret`, and its answer a redacted `access_token`. A body that is neither JSON nor a form is kept
  as is (cut at 32 KiB).
- **URLs**: user info is removed.
- The secret values the runtime fetched for the ingestion are also replaced wherever they appear verbatim in
  a captured header, query or body (a last line of defence for a secret sent in an unexpected place).

## 7. Platform API (`/api/ingestions/{id}/...`)

| Call | Permission | Result |
|---|---|---|
| `GET /trace` | trace:view | `{ "live": bool, "windowId", "windowSecs", "sampleEverySecs", "enabledAt", "hardStop", "enabledBy", "maxWindowSecs": 900 }` (`live` = hard stop after now; fields null when there never was one) |
| `PUT /trace` body `{ "windowSecs": 60, "sampleEverySecs": 5 }` | trace:manage and trace:view-bodies, and the ingestion's reach | starts a new window (a new `windowId`, so runtimes start a fresh minute), answers as GET. `windowSecs` one of 60, 300, 900 else 400 `TRACE_WINDOW_INVALID`; `sampleEverySecs` 1 to 60 else 400 `TRACE_SAMPLE_INVALID`. Event `platform:ingestion:trace:enabled`. |
| `DELETE /trace` | trace:manage | hard stop = now; idempotent; 204. Event `platform:ingestion:trace:disabled` (only when a live window ended). |
| `GET /trace-log` | trace:view | query `kind runtimeId captureId since until limit cursor`; newest first; `limit` default 100, max 500; keyset cursor on `(at, id)`. `{ "records": [ {id, at, version, windowId, captureId, runtimeId, kind, level, message, data} ], "nextCursor" }`. Without view-bodies: an `http` record's data keeps `client`, `request.{method, url path only, bodyBytes, bodyTruncated}`, `response.{status, bodyBytes, bodyTruncated}`, `durationMs`, `attempt`, `failure`; an `exchange` record keeps counts and `outcome`; `poll` keeps everything but `error`; `error` keeps nothing in data; such a record carries `redacted: true`. The same rule applies to the page and fcdev. |

The activity read (`GET /api/ingestions/{id}` or the activity view model) gains `polls` per period and
`lastPollAt`.

## 8. Framework: the HTTP capture hook (flowcatalyst-framework, framework-httpclient)

A per-call capture scope, so nothing changes for a caller that does not bind it:

```java
package io.flowcatalyst.framework.httpclient;

public final class HttpCapture {
  /** Bound around work whose outbound calls should be captured. */
  public static final ScopedValue<Sink> CURRENT = ScopedValue.newInstance();

  @FunctionalInterface
  public interface Sink { void captured(Exchange exchange); }

  /** One attempt: what was sent and what came back. Header values a sensitive name carries are already "[redacted]". */
  public record Exchange(String service, RequestMethod method, URI url,
                         Map<String, List<String>> requestHeaders, byte @Nullable [] requestBody,
                         int status, Map<String, List<String>> responseHeaders, byte @Nullable [] responseBody,
                         HttpClientException.@Nullable Kind failure, long nanos, int attempt) {}
}
```

`ServiceClient`, after each attempt (where it calls `observe`), when `HttpCapture.CURRENT.isBound()`, builds
an `Exchange` (the full target URL with the query; the request headers as sent, auth included, with the
sensitive header rule of section 6 applied; the request body; the response status, headers and body as
received; or the failure kind with status 0 and no response) and calls the sink, swallowing and logging
(WARN) anything it throws. Unbound, it costs one `isBound()` check. Tested with a fake transport: bound and
unbound, a retry (two exchanges, attempts 1 and 2), a transport failure, redacted auth headers. Released as
the next framework tag (`v0.1.3`); the ingestor's `flowcatalyst-framework.version` moves to it after the tag is
published.

## 9. Runtime (ingestor)

- **Windows**: `PlatformApi.poll` reads `traces`; a `TraceWindows` store keeps per ingestion
  `{windowId, localUntil, sampleEvery, lastCaptureStart}` with the semantics of section 2.
- **Sampler**: `TraceWindows.begin(ingestionId)` returns a `Capture` (id, window, record buffer) when the
  window is live and the sample interval has passed, else null; a map lookup when off.
- **Polls**: the extension API's `PolledSource.Poller.run()` (and the runtime's `ApiPollEndpoint` and
  scheduled/database polls) asks the reporter for a capture before each poll. With one, it runs the poll inside
  `ScopedValue.where(HttpCapture.CURRENT, capture::http).where(Capture.CURRENT, capture)`; the exchanges a poll
  dispatches run on the same thread, so the route sees the capture and writes their `exchange` records into
  it. When the poll ends the `poll` record is written. Every poll, captured or not, increments the window's
  `polls` and sets `lastPollAt` (section 3). This is a change inside `PolledSource`'s runtime-provided code
  plus new default methods on its reporter interface: the extension API stays major 1, and extension jars
  (MiX, WebIQ) need no rebuild.
- **Exchanges** not inside a poll capture (HTTP push, files, queues): `IngestionRoute.begin` asks for a
  capture per exchange and writes the `exchange` record at completion or failure.
- **Shipping**: a bounded queue (`INGEST_TRACE_QUEUE`, 2000 records, and 16 MiB) drained every 2 s or at 200
  records to `POST /traces`, retried with backoff on 408/429/5xx/transport errors, a 401/403 retried once with a
  fresh token, other 4xx drop the batch, overflow drops the oldest and counts it. Never on the ingestion's
  thread; never blocks a poll or an exchange. Flushed (bounded) on shutdown.
- **Redaction** of section 6 happens when a record is made.

## 10. UI and CLI

- **Activity page** `/ui/ingestions/{id}/activity` (permission: ingestion view; the trace parts follow the
  trace permissions): header with the ingestion's name, status and a link back; summary cards (last 24 hours
  and 7 days: polls, completed, failed, records, jobs; last poll; last exchange; last failure); the runtimes
  table (status, version, reported); the **Trace** panel: the switch with the window choice (1, 5, 15 minutes)
  and "one capture every N seconds", a note that capture starts when each runtime picks it up (within a sync
  interval), a countdown to the hard stop, and the records newest first grouped by capture (a poll with its HTTP
  calls and exchanges nested; each HTTP call expands to request and response panels, JSON pretty-printed,
  truncation and redaction marked), filters (kind, runtime), refreshing every 5 s while live, "load more" by
  cursor. Patterns: `internal/ui/pages/function_trace*.go` (the function trace page).
- **Drawer**: the Activity tab keeps the day and week line and gains "Open activity"; the list row gets an
  "Activity" action.
- **fcdev**: `fcdev ingest trace on <ref> [--for 1m|5m|15m] [--every 5s]`, `fcdev ingest trace off <ref>`,
  `fcdev ingest trace status <ref>`, `fcdev ingest trace log <ref> [--kind] [--since] [-f]` (follow).

## 11. Work packages

| WP | Scope | Repo | Needs |
|---|---|---|---|
| F framework | `HttpCapture` and the `ServiceClient` hook, tests, docs | flowcatalyst-framework | |
| P platform | migration 094, permissions and grants, window use cases and events, trace API, `traces` in the poll answer, `POST /traces` with the limits, report `polls`/`lastPollAt`, retention, Activity page, drawer and list links, `fcdev ingest trace`, OpenAPI lock, docs (`published/80-ingestion.md`) | flowcatalyst-go | |
| R runtime | contract records (`PollAnswer.traces`, `ExchangeWindow.polls/lastPollAt`, the trace record), `TraceWindows`, sampler, poll and exchange capture, redaction, shipper, poll counting, tests; the ingestor README settings | flowcatalyst-go `ingestor/` | F (built locally until released), P's contract (this document) |

Order: F and P in parallel; R after F is installed locally. Release: the framework tag, then the ingestor's
framework pin, then a runtime release.
