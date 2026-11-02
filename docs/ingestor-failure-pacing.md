# Ingestor failure pacing

*Owner request 2026-11-02: "when any exchange has an error we don't get into a tight retry loop forever and
generate logs very quickly. That could lead to massive bills." Rulings below are the main session's.*

## 0. Principle

A failure that repeats is **paced** (each attempt waits longer than the last, up to a cap, with jitter) and
**logged once in full, then summarised** (at most one line a minute per thing that fails, with a count). Nothing
gives up silently: the failure stays visible on the platform (failures, health) the whole time, and the first
success ends the streak and says so. Nothing here changes what is delivered or how often a *working* source runs.

The building blocks are in `ingestor-extension` (committed `1d2776e3`):

- `Backoff` — `initial·2^(n−1)` capped at `max`, equal jitter (`[d/2, d]`); `delay(n, floor)` never shorter
  than the floor. `Backoff.FAILURES` = 5 s → 5 min. `Backoff.REFUSED_LOGIN` = 5 min → 1 h.
- `FailureLog` — `failed(key, message, cause)`: the first of a streak with its stack trace, then at most one
  line per minute per key without it ("N more like it not logged; M in a row"); `recovered(key, message)` ends
  it ("… (after M failures in a row)"); `streak(key)` is the count. `FailureLog.warn(LOG)` for WARN.

Use these; do not write another backoff or rate-limited logger.

## 1. Audit findings this answers (2026-11-02, re-verified in code)

| # | Where | What |
|---|---|---|
| F1 | `functions/context/HostLogger` | a function's log lines go straight to SLF4J, unbounded |
| F2 | `CamelSourceEndpoints.sqs` | `delay=100`, no backoff: a failing receive re-polls ~10×/s, two stack traces each |
| F3 | `IngestionRoute.flush` | platform down: every pending payload, every ingestion, 3 POSTs per ~2 s, forever; stack trace per payload (S3/SQS) |
| F4 | sftp / s3 / database-poll | move/mark/commit failure after a good read: re-read and re-sent every poll, counted healthy |
| F5 | `PolledSource.Poller.run` | an extension's returned wait used as is (≥1 ms); full stack trace every failure; a poll whose dispatch failed is not a failure |
| F6 | `FunctionListener` (≈:337, :347) | a full stack trace per failed invocation |
| F7 | `HttpIngest` (≈:171, :236, :493) | a WARN (one with stack) per request while the platform/secrets fail; secret reveal per request, no negative cache |
| F8 | `functions/reconcile/Reconciler.fetchArtifact` | only `ControlPlaneException` caught: another RuntimeException leaves the version due → ~4 cycles/s |
| F9 | `ReconcileLoop` / `Reconciler.initialise` | a `LinkageError`/`ExceptionInInitializerError` from a function's `init` ends the reconcile thread silently |
| F10 | control plane (desired, heartbeat) | 401/403 (no `platform:function-runner`) retried every ≤30 s forever, a WARN each |
| F11 | sftp | a refused login / host down never shows as failing (Camel skips the poll; commit marks healthy) |
| F12 | Camel consumers | the component's own ERROR/WARN with stack trace on top of `ReportingPollStrategy`'s line |
| F13 | db / s3 / sftp / api-poll | a failing poll repeats at the normal interval, no backoff (database poll opens a new login each time) |

## 2. Rulings

### R1 Every polled source backs off on failures in a row

Applies to every source the runtime polls: sftp, s3, sqs, database-poll, api-poll, and `PolledSource`
(extensions). A **failure** for pacing is: the poll fails; the poll's dispatch fails (`DispatchFailedException`,
the platform breaker of R2 open); a commit after the read fails (R4). A **success** is a poll that read and
dispatched (or read nothing) without any of those.

After failure `n` in a row the next poll is no sooner than `Backoff.FAILURES.delay(n, interval)` where
`interval` is the source's normal wait (`every`, `idleDelay`, sqs's delay). The first success polls at the normal
interval again. Saving the ingestion (a new route) starts from zero, as today.

Mechanism is the implementer's choice, inside the runtime (not per component config the user can see). Camel
4.22's `ScheduledPollConsumer` backoff (`backoffErrorThreshold` / `backoffMultiplier`) skips a fixed number of
polls; if used, set the skip count per streak so the wait matches the schedule above, or gate the poll instead.
Record which in the code.

**SQS:** use long polling, `waitTimeSeconds=20` (an empty queue waits on SQS instead of re-polling every 100 ms;
cost and calls drop ~200×), and keep `delay` small for a busy queue; failures pace by R1. **SFTP:** set
`throwExceptionOnConnectFailed=true` so a refused login or a down host is a failed poll (F11), paced by R1, and
reported (a refused login as `AUTH`).

### R2 A platform that is not taking jobs holds every ingestion (runtime-wide breaker)

In the job sink (where `IngestionRoute.flush` submits): when a submit fails after its attempts with a failure that
is the **platform's** (connect/timeout, 5xx, 429, 401/403 after the token is refreshed once), the breaker opens
until `now + Backoff.FAILURES.delay(n)` (n = consecutive platform failures), or the platform's `Retry-After` when
it sent a longer one (honour it up to 15 min). While open, a submit fails **at once** with a transient
`DispatchFailedException` ("the platform is not taking jobs; held until …") without calling the platform, so the
source keeps its payload and R1 paces it. The first submit after the hold is a probe; success closes the breaker.
A failure that is the **ingestion's** (409 paused target or anything else per-ingestion, a refusal) does not open
it; that ingestion paces by R1 alone.

Logging: one WARN when the breaker opens ("the platform is not taking jobs (…); holding every ingestion until
…"), one line a minute while it stays open (FailureLog), one INFO when it closes. No per-payload stack traces for
`DispatchFailedException` anywhere.

The in-flush retry (3 attempts, 0.5 s then 1.5 s) stays for a single blip, with jitter added.

### R3 Repeated failures are logged through `FailureLog`

Every place that logs a failure that can repeat uses a `FailureLog` with a key naming what fails:

- source poll and commit failures — key `ingestion id + ":poll"` / `":commit"`; recovered on success;
- route start failures in the runtime reconciler — key `ingestion id + ":start"`;
- extension polls (`PolledSource`) — key `ingestion id + ":poll"` / `":login"`;
- function invocation failures and timeouts (F6) — key `address + "@" + version + ":" + exception class` (or
  `":timeout"`); no recovered call needed (the minute limit and the stale rule end streaks);
- HTTP ingest 503s (F7) — key `ingestion id + ":" + reason`;
- the function control plane's desired/heartbeat failures — key `"control:desired"` / `"control:heartbeat"`.

Camel components' own failure logging (F12) is silenced or routed: a consumer's exception handler must not log a
second stack trace for what `ReportingPollStrategy` (via FailureLog) already logs. The failures route
(`camel-route` kind) gets no extra error-handler logging beyond FailureLog.

### R4 A failed commit is a failure

When a payload was read and dispatched but marking it done fails (sftp move/delete, s3 delete/move, database
`UPDATE` / watermark store, the state API), that is a failure of kind `SOURCE` with a message saying what could
not be marked ("read and sent, but could not be moved/deleted/marked: …; it will be read again"), the
ingestion's health is failing, and R1 paces it. Use each component's hook (file consumer
`onCompletionExceptionHandler`, the S3 consumer's exception handler, the database poll's own commit path). If a
component offers no hook, say so in the report rather than reaching into its internals.

### R5 `PolledSource`

- A returned wait is at least **250 ms** after a successful poll (a backlog still drains), and after a failed
  poll it is `Backoff.FAILURES.delay(n, max(retryDelay(), 1 s))`.
- A poll in which `payloads.dispatch(…)` returned **false** at least once is a failure for pacing (R1), whatever
  the poll returned. It is still not an exception: no stack trace; the route already reported the dispatch.
- A refused login waits `Backoff.REFUSED_LOGIN.delay(n)` (5 min doubling to 1 h; account lockouts) instead of a
  fixed 5 min; the failure text says when it tries again. `setAuthRetry` (if present) still overrides for tests.
- Logging through FailureLog (R3).
- `docs/extensions.md`: fix the example that returns `Duration.ZERO` after a failed dispatch, and document the
  pacing an extension gets for free (R1, R5) and `Backoff` / `FailureLog` for its own calls.

### R6 Function host

- **Log budget per function (F1).** `HostLogger` lines pass a token bucket per function address: **100 lines/s,
  burst 1,000** (settings `INGEST_FUNCTIONS_LOG_RATE`, `INGEST_FUNCTIONS_LOG_BURST`). Lines over budget are
  dropped and counted; once a minute per function that dropped any, one WARN from the host: "function X dropped N
  log lines in the last minute (limit 100/s)". A message longer than **8 KiB** is cut with "… (cut, M bytes)". A
  throwable logged by a function counts as one line and keeps its stack trace (but its message is cut the same).
- **Invocation failures (F6)** through FailureLog (R3).
- **Artifact fetch (F8):** any `RuntimeException` from fetching fails the version as transient (it is retried by
  the existing backoff), logged through FailureLog. A computed wake-up under 1 s waits at least 1 s (no `wait=0`
  long-poll storm).
- **Init errors (F9):** `LinkageError` and `ExceptionInInitializerError` from a function's `init` are a permanent
  `INIT_FAILED` for that version (like an exception). The reconcile loop survives any non-`VirtualMachineError`
  thrown in a cycle: logged through FailureLog, then the outage backoff, then the next cycle.
- **Refused by the control plane (F10):** a 401/403 on desired or heartbeat, after a token refresh, backs off by
  `Backoff.REFUSED_LOGIN` (5 min → 1 h); the WARN names the cause ("the platform refused this runtime as a
  function runner (403): give its service account platform:function-runner") and repeats through FailureLog.

### R7 HTTP ingest (F7)

The 503 WARNs go through FailureLog (R3). A failed secret reveal in `Secrets.require` is remembered for **10 s**
per secret (later calls in that time fail at once with the same error, without calling the platform); a
successful one is unaffected.

### R8 Not in scope

Changing what is retried or delivered; a give-up that drops data; the Go function runner; the platform. Log
format and level stay as they are apart from the summaries above.

## 3. Tests (each ruling has at least one; a deliberate break must fail it)

- R1: a source failing every poll waits longer each time up to the cap (fake clock or scheduled-delay
  inspection), and polls at its interval after one success; SQS endpoint has `waitTimeSeconds=20`; SFTP has
  `throwExceptionOnConnectFailed=true`.
- R2: with the platform failing, N ingestions make **one** platform call per hold, not N×3; Retry-After honoured;
  a 409 for one ingestion does not hold another; closes on the first success; logs open/close once.
- R3: a source failing 600 times in a minute writes 2 lines, one with a stack trace (capture the log).
- R4: a commit failure is reported as SOURCE, health failing, paced.
- R5: an extension returning ZERO after a failed dispatch is paced; a refused login doubles to 1 h.
- R6: a function logging 100,000 lines in a second emits ≤ ~1,100 plus one summary; an 8 KiB+ message is cut; an
  artifact `IllegalArgumentException` fails the version transient and the loop does not spin; `NoClassDefFoundError`
  in init is INIT_FAILED and the loop keeps running; a 403 backs off to minutes.
- R7: N requests during a secrets outage make one reveal call per 10 s and ≤ 2 log lines a minute.

## 4. Status

| Part | Status |
|---|---|
| Building blocks (`Backoff`, `FailureLog`) | committed `1d2776e3` |
| R1–R5, R7 (ingestion) | committed `d12d2954` |
| R6 (function host) | committed `4811de79` |
| WebIQ (mono-repo): stop on refused login/429/refused dispatch | mono-repo `9fd7632` |
| iac: alerts topic, log-volume alarm, optional budget | iac `d04c644` |

**Not met (no hook without reaching into a component):** Camel components' own log lines on a failed poll
(`Sqs2Consumer` "Error while polling", the S3 rollback line, the file rollback WARN) still appear, but only as often
as the paced polls; an SQS delete-after-read failure is not reported as a commit failure. **Limit:** a function's
"N log lines dropped" summary is written on its next log call after the minute (no timer), so a function that floods
and then falls silent reports its last minute only when it logs again.
