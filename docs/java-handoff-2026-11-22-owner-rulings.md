# Java hand-off: the owner's rulings of 2026-11-22 on the ingestion runtime API

For the Java platform port, and equally for the Rust port. The owner decided on 2026-11-22 how several
behaviours of the ingestion runtime API (`/api/ingestion-runtime/v1/...`) are to change, and they are
built here first; each section below is one decision, named by the owner's number (D1, D2, ...), with
the contract a port must mirror: the codes and statuses, the schema, the lock keys and the order of the
checks. Both implementations share one database, so a lock key or a column is a contract between them,
not a detail. Authority, in order: this document, `docs/wire-contract.md` (the ingestion runtime sections),
`docs/ingestion-design.md` section 7 and `docs/ingestion-trace-design.md` section 4, then the Go code each
section names. The bug-fix ledger (`docs/platform-bug-fixes.md`) has one F entry per decision with its
commit and tests.

Sections are per decision and self-contained; a decision built later is added as a section of its own.

## D9. A first poll takes a transaction lock on the runtime id before its row is read

Go: commit `2befa23a`, F54 in `docs/platform-bug-fixes.md`.

**Before.** A poll and a lease release read the runtime's row with `SELECT ... FOR UPDATE` to apply the
instance guard (`409 RUNTIME_ID_IN_USE`). A runtime id with no row yet has nothing to lock, so two
processes making the first poll of a new id at the same moment both read no row, both passed the guard,
and both were answered the same polled ingestions until the next poll refused one.

**Contract** (Go: `internal/platform/ingestion/runtime.go` `previousPoll`, used by `Poll` and `release.go`
`Release`; the query `IngestionRuntimeLock` in `internal/sqlc/queries/ingestion_runtime.sql`).

- In the poll's and the release's transaction, before the runtime's row is read, take
  `SELECT pg_advisory_xact_lock(hashtext('ingestion-runtime:' || runtimeId))`: the single-argument
  (bigint) form, its key the `int4` that `hashtext` returns, widened, of the text `ingestion-runtime:`
  followed by the runtime id exactly as sent. This exact expression, because a Java and a Go platform polled by one
  fleet must contend on the same key. It is released when the transaction ends.
- Lock order, always: the runtime id's lock, then the runtime's row (`FOR UPDATE`), then the lease lock
  (`pg_advisory_xact_lock(hashtext('ingestion-leases'))`). Nothing takes them in another order.
- No wire, schema or status change. The second of two first polls now reads the first's row: refused
  `409 RUNTIME_ID_IN_USE` when the two sent different `instanceId`s (as any later poll would be); when
  either sent none, both are accepted as before (the instance guard checks only a poll that names one).
  With D1, a second first poll from another account is refused `403 RUNTIME_ID_NOT_OWNED` whatever the
  instances.

**Test to port.** `robustness_pg_test.go` `TestInstanceGuard_TwoFirstPollsOfANewIDAreDecidedInTurn`: a
session holds the lease lock, so the first poll of a new id waits after its upsert; the second poll of the
same id starts and waits too; the lease lock is let go; the first is answered the polled source and the
second is refused `RUNTIME_ID_IN_USE`, and the stored instance is the first's. Without the lock the second
reads no row (the first's insert is not yet visible) and both are answered.

## D1. A runtime id belongs to the account that first registered it

Go: commit `766860f7`, F55 in `docs/platform-bug-fixes.md`.

**Before.** Every account holding `platform:ingestion:runtime:operate` (anchor tier, bound to no application)
could poll, report, release leases, read and write state, post jobs and traces and fetch secrets as any
runtime id: the platform recorded no account on a runtime.

**Schema.** Migration 125 (`125_ingestion_runtime_principal.sql`; take the next free number in a port):
`ALTER TABLE ing_runtimes ADD COLUMN IF NOT EXISTS principal_id VARCHAR(17)` (nullable, no default, no
foreign key; `lock_timeout = '5s'`; Down drops it). Effect probe: the column exists. NULL means the row was
written before the migration.

**Registration** (Go: `internal/platform/ingestion/runtime.go` `Poll`, query `IngestionRuntimeUpsert`). The
poll's upsert writes the caller's principal id (`auth` context `PrincipalID`) on insert, and on conflict
`principal_id = COALESCE(ing_runtimes.principal_id, EXCLUDED.principal_id)`: the first poll registers the
id, the next poll of a NULL row binds it, nothing ever changes it. Only the poll registers; no other route
writes the column. A call with no principal is a bug in the caller (an error, not a refusal).

**The refusal.** `403 {"error":"RUNTIME_ID_NOT_OWNED","message":"runtime id <id> is registered to another
service account"}` (a `usecase.Authorization`; the message does not name the owner). Refused when the
runtime's row exists, its `principal_id` is not NULL, and it differs from the caller's principal. An id with
no row, or with a NULL principal, is refused by nobody; the poll then registers it.

**Order of checks**, per route (each later check runs only if the earlier passed; a refused call reads and
writes nothing):

| Route (operation id) | Order |
|---|---|
| poll (`ingestionRuntimePoll`) | runtime permission (the `INGESTION_RUNTIME_*` 403s) → contract version (426) → in one transaction: the database clock, the runtime id's lock (D9), the row `FOR UPDATE`, **owner (403)**, instance guard (409 `RUNTIME_ID_IN_USE`), upsert, the rest of the poll |
| release (`ingestionRuntimeReleaseLeases`) | runtime permission → at most 1000 ids → in one transaction: clock, the id's lock, the row `FOR UPDATE`, **owner (403)**, instance guard (409), lease lock, release, goodbye |
| report, state read and write, jobs, traces | runtime permission → (traces only: `400 RUNTIME_ID_REQUIRED` for a blank id) → **owner (403)**, read outside any transaction (`IngestionRuntimeOwner`) → every check the route had before |
| secret (`ingestionRuntimeGetSecret`) | runtime permission → **owner (403)** → the fetch's checks as before |

Fair share, the instance guard and "carries the kind" are unchanged: a refused poll writes nothing (not
`last_seen_at`, not kinds, not a lease), so it is never counted live nor a carrier; the kinds a runtime
carries are written only by its owner's polls.

**What frees an id.** Only retention: `DELETE FROM ing_runtimes WHERE last_seen_at < now - 7 days` removes the
row and the binding with it (a refused poll does not update `last_seen_at`, so it cannot keep another
account's binding alive). A goodbye (`left_at`) does not free the id. There is no route to re-assign one.

**Audit and log.** Secret route: the refusal is a refused fetch as before, `aud_logs` `SECRET_FETCH_REFUSED`
on the ingestion with `reason` `RUNTIME_ID_NOT_OWNED` (and the WARN `security.ingestion_secret_fetch`
line, counter `outcome=refused`). Every other route: one `aud_logs` row, `entity_type` `IngestionRuntime`,
`entity_id` the runtime id, `operation` `RUNTIME_ID_REFUSED`, `principal_id` the caller, no client or
application, `operation_json` `{"runtimeId","operation" (the route's operation id),"ingestionId" (state and
jobs only),"reason":"RUNTIME_ID_NOT_OWNED","message"}`; and a WARN log line
`security.ingestion_runtime_id_refused` (`event`, `runtimeId`, `principalId`, `operation`, `reason`,
`ingestionId?`). An audit row that cannot be written is logged and does not change the answer.

**Wire.** The poll and the release list `403` in the OpenAPI lock; the runtime id parameters' descriptions
say the id must belong to the caller's account. No shape changes.

**Tests to port.** `internal/platform/ingestion/runtime_owner_pg_test.go` (the first poll registers; another
account's poll, with or without an instance, live owner or not, and its release are refused and change
nothing; the owner check comes before the instance guard; a goodbye does not free the id; a NULL row is
refused to nobody and bound by the next poll; retention frees the id; a call without a principal is an
error) and `internal/platform/ingestion/runtimeapi/owner_pg_test.go` (all eight routes refuse another
account with the code and the audit row of each kind; nothing it sent took effect; the owner goes on; an
account with a runtime id of its own is refused the lease another account's runtime holds).

## D2. Posting trace records for a polled ingestion needs the lease, as the jobs call does

Go: commit `21269198`, F56 in `docs/platform-bug-fixes.md`.

**Before.** `POST /api/ingestion-runtime/v1/traces` kept a record when its `windowId` was the ingestion's
current window and its time was in range; nothing checked that the poster runs the ingestion, so any runtime
could add records to the trace of a polled ingestion another runtime held.

**Contract** (Go: `internal/platform/ingestion/trace_access.go` `WithTraceAccess`, `runtimeapi/traces.go`).
The checks are exactly the jobs call's, per ingestion the batch names, in the jobs call's order; the first
that fails gives the ingestion its code:

1. the ingestion exists (`INGESTION_NOT_FOUND`);
2. it is live, active with a live version (`INGESTION_NOT_LIVE`);
3. the runtime carries its kind, from the kinds the runtime's row holds (`RUNTIME_KIND_NOT_CARRIED`);
4. for a polled (single-owner) source, the runtime holds its lease now (`expires_at > now`, the database's
   clock): `SELECT ingestion_id FROM ing_leases WHERE ingestion_id = $1 AND runtime_id = $2 AND expires_at
   > $3 FOR SHARE`, no row being `LEASE_NOT_HELD`; a shared source has no lease and skips this;
5. its client is `ACTIVE` (`CLIENT_NOT_ACTIVE`).

Steps 1 to 5 and the insert of the records run in **one transaction**, the lease rows held `FOR SHARE`
until it commits (the jobs call's lock: a claim by another runtime waits; the holder's own concurrent
requests do not), the named ingestions taken in id order. A record of an ingestion that fails is counted in
`dropped`; the answer stays `202 {"accepted","dropped"}` with no code (a batch spans ingestions, and
tracing never fails a runtime's work); the codes go only to the platform's debug log line (`refused`: code
→ count). The lease is the one held when the batch arrives, not when a record was captured, so records a
runtime queued through an outage are taken once it holds the lease again, and none while it does not.

**Order within the call.** Runtime permission → `400 RUNTIME_ID_REQUIRED` → D1's owner check (`403
RUNTIME_ID_NOT_OWNED`) → `400 TRACE_BATCH_TOO_LARGE` → an empty batch answers `202` with zeros → per record,
the window rules as before (window id, `at`, kind, capture id, hard stop) → the access checks above, for
the ingestions of the records that passed → the per-ingestion rate cap, now counted only over the records
that passed the access checks → the insert (`ON CONFLICT (id) DO NOTHING`, as before).

**Wire.** No status, code or shape change; more records count as `dropped`.

**Tests to port.** `internal/platform/ingestion/trace_access_pg_test.go`
`TestWithTraceAccess_TheJobsCallsChecks` (every code; the lease run out and taken back);
`internal/platform/ingestion/runtimeapi/traces_pg_test.go` `TestTraces_APolledIngestionNeedsTheLease` (the
holder's records kept, another runtime's dropped for the polled ingestion and kept for the shared one;
records captured before an outage dropped while the lease is lost and kept after the holder's next poll).
The existing trace tests now poll as their runtime first, so it carries the kind.

## D10. The lease of an EVENT target's ingestion is checked inside the insert's transaction

Go: commit `6b1df73b`, F57 in `docs/platform-bug-fixes.md`.

**Before.** For an ingestion whose target is `EVENT`, the jobs call
(`POST /api/ingestion-runtime/v1/ingestions/{id}/jobs`) checked the lease of a polled source and the
client's status, then inserted the events through the event repository on a connection of its own. A
request that lost its lease between the check and the insert still stored its events, and the next lease
holder could make the same events again (doubled where the records carry no `externalId`).

**Contract** (Go: `internal/platform/ingestion/runtimeapi/events.go` `createEvents`,
`internal/platform/ingestion/jobs.go` `WithRuntimeLease`, `internal/platform/event/repository.go`
`PrepareIngest` and `PreparedIngest.Insert`). For an `EVENT` target, in this order:

1. the checks before, unchanged (the runtime permission, D1's owner, the ingestion found, live, its target
   resolved, the kind carried; then the lease held and the client active, read without a lock);
2. each record made into an event (its `error` when it cannot be one);
3. outside any transaction: the duplicate lookup by deduplication id and, where the payload table is
   written, the payloads prepared (the payload store written), exactly as `POST /api/events/batch` does;
4. **one transaction**: the database's clock; for a polled source the lease row
   `SELECT ingestion_id FROM ing_leases WHERE ingestion_id = $1 AND runtime_id = $2 AND expires_at > $3
   FOR SHARE` (no row: `409 LEASE_NOT_HELD`, the transaction rolls back, no event is stored); the client's
   status (`409 CLIENT_NOT_ACTIVE` unless `ACTIVE`); then the events' inserts, the same statements as the
   batch path's, sent in that transaction. Commit.
5. after the commit: the stored ids read back by deduplication id, as before.

This is the jobs call's transaction (`WithJobKeys`) without the job keys. The transaction runs even when no
record became an event, as the jobs call's does, so a request that lost its lease is refused whatever its
records. No wire, schema or code change: the same `409`s, now also when the lease is lost after the first
check.

**Test to port.** `internal/platform/ingestion/runtimeapi/events_pg_test.go`
`TestJobs_EventTargetLeaseCheckedWithTheInsert`: the runtime API's clock answers a time inside the lease to
the first check and one past it to the insert's transaction; the request is refused `LEASE_NOT_HELD` and no
event is stored. With the lease held throughout, the events are stored.

## D11. Only a real NUL escape is replaced in a trace record

Go: commit `8661d369`, F58 in `docs/platform-bug-fixes.md`.

**Before.** A trace record's `data` (valid JSON, at most 80 KiB) had every occurrence of the six characters
`\u0000` replaced by U+FFFD, as text. In a record holding the text `\\u0000` (an escaped backslash, then
`u0000`), the second backslash and `u0000` were taken for an escape and replaced, leaving `\` followed by
U+FFFD, an invalid escape; the batch's one insert failed and the call answered `500 REPO` for every record.

**Contract** (Go: `internal/platform/ingestion/runtimeapi/traces.go` `replaceNULEscapes`, called by
`traceData` on data that is valid JSON and within the size limit). Walk the JSON's bytes; in valid JSON a
backslash occurs only inside a string and always starts an escape, so:

- at a backslash followed by `u0000` (the escape of NUL), write U+FFFD as itself (UTF-8 `EF BF BD`) and
  skip the six characters;
- at any other backslash, copy it and the character after it, and continue after both (so the second
  character of `\\` is never read as the start of an escape);
- copy every other byte.

Equivalently, decode the JSON, replace each NUL in every string (keys included) with U+FFFD, and encode
it again; but the Go platform stores the bytes as sent otherwise, so a port that re-encodes must not change
anything else a reader would see (key order, number spelling, other escapes). Examples: `{"a":"\u0000"}` →
`{"a":"�"}`; `{"a":"\\u0000"}` unchanged; `{"a":"\\\u0000"}` → `{"a":"\\�"}`. No other character is
touched. No wire, schema or status change.

**Tests to port.** `internal/platform/ingestion/runtimeapi/traces_test.go` `TestReplaceNULEscapes` (the
cases above, in a key and an array, several in one string) and `FuzzReplaceNULEscapes` (for any text, its
JSON with the escapes replaced is valid JSON and decodes to the text with each NUL replaced by U+FFFD);
`traces_pg_test.go` `TestTraces_OnlyARealNULEscapeIsReplaced` (a batch with the four kinds of record is
stored whole, each as expected).

## D4. A function's dispatch may name a connection only if the function may sign as its account

Go: F59 in `docs/platform-bug-fixes.md`.

**Before.** `POST /control/functions/dispatch-jobs` (the host's `dispatch.create`, ABI op 11) resolved the declared
connection (the function's application's, then a shared one, most specific first) and created the job with it; the
connection's service account signs the delivery. A shared connection whose account belongs to another application
signed the function's jobs as that application. The Java port's ingest routes already run a delivery-signing guard
(`DeliverySigningGuard`, `SigningReach`); its function route did not, as Go's did not.

**The rule** (Go: `internal/platform/serviceaccount/signing.go` `SigningIdentity.SigningProblem`). Work done as an
identity `{application, own account?}` may be delivered signed by account A when:

1. A does not exist (it signs nothing; the delivery is declined with that reason): not refused;
2. A is the identity's own account: allowed, whatever A's application (none included);
3. A's `application_id` is the identity's application: allowed;
4. otherwise refused: A belongs to another application, or to none.

Whether A is active, and which clients it is linked to, are not asked. This is `SigningReach.mayUse` for a caller that
is an application (its rules 2 and 3), plus the own account. A function's identity is its application
(`fng_functions.application_id`) and its own service account (`fng_functions.service_account_id`, F35) when set. The
account is read by id only (`SELECT id, code, application_id FROM iam_service_accounts WHERE id = $1`), the
connection's `service_account_id`, exactly as the delivery's signer reads it; a connection with an empty
`service_account_id` is not refused (its delivery is declined).

**The refusal.** `403 {"error":"DISPATCH_SIGNER_NOT_ALLOWED","message":"connection <code> signs its jobs with an account
function <address> may not sign as: service account <code> belongs to another application; a function's dispatch may
name only a connection whose account is its application's or its own"}` (or `... belongs to no application, and work
of an application is signed only by the application's own accounts; ...`). A `usecase.Authorization`; the guest hears
`NOT_ALLOWED`.

**Order of checks** on the control route, each only if the earlier passed; nothing is written before the last:

| Step | Refusal |
|---|---|
| runner permission and anchor scope | 403 |
| `functionId`, `dispatch.target`, `dispatch.path` present | 400 `FUNCTION_ID_REQUIRED`, `DISPATCH_TARGET_REQUIRED` |
| the function and the invoked version exist; describe parses | 404; 500 `CORRUPT_DESCRIBE` |
| (target, path) declared by this version | 403 `DISPATCH_NOT_DECLARED` |
| target owned (application, then client), exists, live with a webhook POST at the path | 403/404/409 as before |
| the declared pool | `DISPATCH_POOL_*` as before |
| the declared connection: named, found | 400 `CONNECTION_REQUIRED`, 404 `CONNECTION_NOT_FOUND` |
| **its account: the function may sign as it** | **403 `DISPATCH_SIGNER_NOT_ALLOWED`** |
| the job's own fields (payload, code, mode, metadata, ...) | 400 `DISPATCH_*` as before |
| payload prepared, job inserted | |

**At publish** (`POST /api/functions/{id}/versions`, Go `operations.Publish`): after the existing describe checks
(`EMIT_NOT_OWNED`, `DISPATCH_NOT_OWNED`), every declared dispatch's connection is resolved the same way; one that
exists and fails the rule refuses the publish `403 DISPATCH_SIGNER_NOT_ALLOWED` before the version is stored. One that
does not exist yet passes (the call checks it every time, as the target). Promote does not check it.

**Wire.** A new code on two routes; no shape, schema or lock change.

**Tests to port.** `internal/platform/serviceaccount/signing_test.go` `TestSigningProblem` (every case of the rule);
`internal/platform/function/control/dispatch_signer_test.go` (the allowed and refused accounts through a shared and
through the application's connection, nothing written when refused, an account that does not exist, the guest's
`NOT_ALLOWED`); `internal/platform/function/operations/dispatch_signer_pg_test.go` `TestPublish_DispatchSigner`.
