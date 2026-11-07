# Ingestor: closing the known gaps (2026-11-07)

Owner approved 2026-11-07, after the production-readiness review. None of these can create a duplicate job
or lose a record today (the lease fencing and record identities hold); they shorten pauses, close one
duplicate path after 7 days, and add the tests that prove the fleet behaviour. Background:
`docs/ingestion-design.md`, `ingestor/README.md` ("Leases, stopping and shutting down"),
`docs/ingestor-failure-pacing.md`.

No API, wire-contract or migration change in any item. Each fix comes with a test that fails without it
(prove it red first) and a mutation check.

## A. Shutdown and stopping (Java runtime)

### A1. Shutdown must not hang on a stuck reconcile

**Today:** `RuntimeModule.shutdown` calls `Reconciler.stop(ingestion -> true, STOP_WITHIN)`, which takes
`Reconciler.lock`. A reconcile round stuck starting a route (an extension that connects in `doStart`) holds
that lock, and Camel's context lock, so shutdown waits until ECS kills the process (60 s), no goodbye is sent,
and every lease runs out (a pause of up to lease + poll).

**Change:** shutdown waits for the lock for at most `SHUTDOWN_LOCK_WAIT` (3 s). If it does not get it, it
falls back to the watchdog's lock-free path: `revoke` every started ingestion (consumers suspended on the
captured `Route`s, no lock taken), then waits, within what is left of `STOP_WITHIN`, for each revoked route's
in-flight count (`camel.getInflightRepository().size(routeId)`, lock-free) to reach 0. A revoked ingestion
whose consumer suspended in time and whose in-flight count reached 0 counts as **cleanly stopped** for the
goodbye (its lease is given back); any other is left to run out, as now.

**Also check:** whether `RuntimeLoop.close()` / the framework `Background` loop stop waits for a round stuck
in reconcile. If it can block, bound it the same way (the total shutdown budget stays at or under ~25 s:
ECS gives 60 s, fcdev 15 s; fcdev's 15 s may cut the goodbye on a slow stop, which is acceptable).

**Safety argument to keep true:** a suspended consumer takes nothing more from its source; with in-flight 0
nothing of that ingestion can create jobs or move a watermark, so giving the lease back cannot let two
runtimes run it. A route the stuck round starts later is never in the clean list (it was not revoked; it did
not exist), so its lease is not given back; the context close after `shutdown` stops it, and platform fencing
still holds.

### A2. Stop routes in parallel, not one after another

**Today:** `Reconciler.removeAll` starts one virtual thread per route, but each calls
`RouteController.stopRoute(id, 10 s)`, which holds Camel's context lock for its whole graceful wait, so
stops run serially. With several slow routes few finish within the shutdown's 12 s, and fewer leases are
given back. Rebalance stops have the same delay.

**Change:** two phases inside `removeAll` (used by reconcile, `stop`, `rebuild`):
1. **Suspend intake at once:** suspend every route's consumer in parallel, the way `revoke` does
   (`Live::suspend`, no context lock).
2. **Drain in parallel:** wait, per route, for its in-flight count to reach 0 (poll every ~50 ms), each
   bounded by `ROUTE_STOP` and all by `within`.
3. **Then stop:** `stopRoute` / `removeRoute` as today. With intake suspended and nothing in flight this
   is quick, so its serial lock no longer matters.

The "stopped cleanly" definition is unchanged (stopped, in-flight 0 before removal, no exception, not
revoked). Behaviour of a batch cut off by suspension must stay safe: an SFTP/file/S3 batch's unprocessed
files stay at the source (not moved); an SQS batch's unprocessed messages are not deleted and come back after
the visibility timeout (deduplicated by record identity). Verify this for the file, S3, SQS and
database-poll consumers and say what you found.

### A3. A test for the shutdown order

`RuntimeModule.shutdown` must: stop the function host, stop the watchdog, close the loop, stop the routes,
send the goodbye with the clean list, then flush traces. No test asserts the order. Add one with a recording
fake platform (and a route stuck in start, for A1).

## B. Sources and logging (Java runtime)

### B1. SQS: delete the message ourselves, and report a failed delete

**Today:** `CamelSourceEndpoints.sqsEndpoint` sets `deleteAfterRead=true`; Camel deletes in its own
on-completion and a failed delete only shows in Camel's log. The record identity is `<queue>|<messageId>`
(`IngestionRoute.identity`), and a redelivered message keeps its message id, so a repeat is deduplicated, but
`ing_job_keys` keeps identities for 7 days (`ingestion.JobKeysRetention`) while SQS keeps a message up to 14.
A message whose delete keeps failing comes back after 7 days as a duplicate job.

**Change:**
- `deleteAfterRead=false`; delete the message (receipt handle header) after the jobs are accepted, at the
  same point a file is moved or a database row marked. A failed delete throws `CommitFailedException`, so it
  is reported and paced like the other sources' commit failures (`ReportingPollStrategy.commitFailed`).
- Ask SQS for `ApproximateReceiveCount`. When a message has been received more than `SQS_RECEIVES_WARN`
  (5) times, report it through the same paced failure path (one line per ingestion per window, not per
  message), naming the queue and count, so a poison or never-deleted message is visible.
- README: a source queue should have a dead-letter queue (`maxReceiveCount`) so nothing outlives the 7-day
  identity window.

### B2. Camel's own error lines go through our pacing

**Today:** a failed poll is reported (paced) by `ReportingPollStrategy`, but Camel also logs its own line
(`Sqs2Consumer` "Error while polling", the S3 rollback line, the file rollback WARN) on every failed poll.

**Change:** first verify whether setting the consumer's `exceptionHandler` (an option every
`DefaultConsumer` endpoint accepts) to a `FailureLog`-backed handler replaces Camel's "Error while polling"
line. If it does, set it on every source endpoint. For lines a component logs directly (find them), lower
those specific loggers in the runtime's slf4j-simple configuration, but only where `ReportingPollStrategy`
already reports the same failure. Prove with a test that a source failing every poll for N polls produces at
most the paced number of lines in total. If a line cannot be hooked, say so; do not silence anything that is
not reported elsewhere.

### B3. The "N log lines dropped" summary is written on time

**Today:** `LogBudget` writes a function's "N lines dropped" summary on its next log call after the minute,
so a function that floods and then falls silent never reports its last minute.

**Change:** one shared timer (a single virtual thread or scheduled task for the whole host, not one per
function) writes pending summaries once a minute. Closing a function writes its pending summary.

### B4. Exchanges run on virtual threads: assert it where it happens

Add a test that runs a real polled source (file or database poll) and a queue-style source if practical, and
records `Thread.currentThread().isVirtual()` inside the route's processing. Today only Camel's thread
manager is asserted.

## C. Platform and tests (Go)

### C1. "No runtime carries this kind" uses the configured lease

`ingestion.RuntimeLiveWindow` is a fixed 3 minutes (`internal/platform/ingestion/kinds.go`), while liveness
everywhere else is one configured lease (`FC_INGESTION_LEASE_SECONDS`, default `DefaultLease`). Use the
configured lease. Test with a non-default lease.

### C2. The runtime API test suite does not depend on test order

`internal/platform/ingestion/runtimeapi` tests share one database and poll with platform-wide reach, so a
test's leftovers change another's fair share. Give each test its own database cloned from the migrated one
(`testpg.CloneDB` exists). Prove it with `-shuffle=on` over several seeds. Do the same for
`internal/platform/ingestion` if it shows the same dependence (check with `-shuffle=on` first).

## D. A fleet test (new)

An automated, local test with no AWS: one isolated platform and three runtime processes, steady input, and
three faults. It proves what no unit test does and guards A1/A2.

- **Isolation:** its own Postgres cluster (`E2E_DB_MODE=own` style: never the developer's cluster on 15432,
  never the fcdev on 8080), own ports, own HOME/state dir, clean environment. Build on
  `scripts/e2e-platform.sh` where it fits.
- **Sources:** polled sources that need no external service: database-poll ingestions reading tables in the
  test's own Postgres that a feeder fills at a steady rate (rows with an increasing id), and if practical an
  api-poll ingestion against a tiny local HTTP feeder. Enough ingestions (≥ 6) that all three runtimes hold
  some.
- **Timings:** shortest legal lease (120 s) and a short poll interval, so a run takes minutes, not an hour.
- **Faults, in order, with recovery between:**
  1. `kill -9` one runtime (by PID).
  2. A rolling restart: SIGTERM each runtime in turn, start it again, wait for it to poll.
  3. Freeze the platform (`SIGSTOP` by PID) for 60 s, then `SIGCONT`.
- **Assertions:**
  - every fed record became exactly one job (no duplicate external ids, none missing);
  - the longest gap in job creation per ingestion stays within a bound per fault (kill: lease + poll + margin;
    rolling restart: about one poll interval + margin when goodbyes work; freeze: freeze length + poll +
    margin);
  - `fc_ingestion_unheld` (platform metrics endpoint) rises during the kill and returns to 0;
  - at the end every ingestion is held by exactly one live runtime.
- **Output:** a short report (per fault: gap per ingestion, duplicates, missing) and a non-zero exit on any
  failed assertion. A `make` target that runs it. Processes it started are stopped by PID on exit, including
  on failure.

## Status

| Item | Status |
|---|---|
| A1–A3 | built (merged 2026-11-07). The module's shutdown fits the framework's bound (`app.shutdown.drain`: 20 s in the image, 10 s under fcdev); the watchdog suspends only the source's intake |
| Extension API (owner, 2026-11-07) | built: `PolledSource.polling()`, suspendable pollers, `Payloads.stopping()`, `Payloads.dispatchTaken()` (destructive reads); API major stays 1. mix and webiq check `stopping()` (mono-repo branch `feat/ingest-extensions-stopping`, pushed, PR to merge); ctrack as a patch for the owner. Deploy the runtime before the extensions |
| B1–B4 | built (merged) |
| C1–C3 | built (merged); C3 is the "ingestions unheld" WARN line |
| D | built (merged): `make ingestor-fleet-test` |
| `fc_ingestion_unheld` alarm | the gauge is not scraped in AWS: an iac log metric filter + alarm on the C3 line, on iac main, applied in nonprod 2026-11-07; prod with the next iac release |
| Runtime load test | not started; scope not yet decided |
