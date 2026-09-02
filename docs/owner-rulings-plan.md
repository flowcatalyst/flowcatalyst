# Owner rulings — implementation plan

_Companion to [`owner-rulings-todo.md`](./owner-rulings-todo.md). Every item there was
verified against this tree before being planned. Item numbers below are the todo doc's;
IDs are the owner's decision-ledger ids and belong in commit messages._

## 0. Status — what has landed (2026-09-02)

Work ran across TWO concurrent sessions in one tree; commits below are whichever session
landed them, not whichever planned them.

| Ruling(s) | Commit | Note |
|---|---|---|
| R-57, R-12, R-06 | `2e2e466` | R-06's third case ("unparseable outright") was found broken during the fix |
| A-19, X-03 | `63e93e1` | X-03 ruled separately on 2026-09-01 (quarterly partitions, 3y retention) |
| X-04 warnings-through-store, INFO TTL | `ff757f9` | |
| A-27 Disposition refactor, R-53, R-04 snapshot | `6ff6bf8` | A-27 built from this session's patch |
| PR-3/PR-4 + IDOR, X-02, X-08 | `e6a33ba` | |
| A-01 router settled hook | `4a160e0` | |
| R-30/R-33/R-34/R-43/R-56/R-59 + R-13/R-16 strict gate | `e7a29d8` | |
| A-01 platform half (settled endpoint, cancel/complete, reaper, wiring) | `718a821` | **Closes the live data-loss gap end to end** |

**Uncommitted at time of writing:** X-06 phase 1 (`serviceaccount`, `loginattempt`) and
phase 2 in progress. **Still open:** X-11 + R-26/R-49 drain-and-detach; X-06 phases 2/3;
the P3 conventions epic (X-05, X-07, X-09, X-10), which wants its own plan doc.

### Corrections to this document's own earlier claims

- **R-13 was NOT already done** — see the correction in §1. Half the ruling had shipped.
- **Owner item 16 was misfiled.** `loginattempt.ParseOutcome`'s unknown→`SUCCESS` default was
  filed as undercounting lockout failures. It does not: `ParseOutcome` has exactly one
  non-test caller (the attempt-history *display* path), and the backoff counters query
  `outcome = 'FAILURE'` as raw SQL against the column. Real hygiene fix, not a vulnerability.
- **Owner item 17 carries a deliberate tradeoff.** The `LastSuccessAt` lower bound
  (~400 days) means an identifier dormant longer than that reads as "never succeeded", so
  its failure-counting window collapses to `loginbackoff`'s 30-day fallback — a narrower
  window, i.e. a weaker backoff, for sparse failures against long-dormant accounts.
  A bounded-then-unbounded fallback was considered and **rejected**: a nonexistent or
  never-successful identifier has no success row, so the unbounded query would fire on
  every attempt — a full partition scan per request, exactly what an enumeration attack
  would trigger. The cheap alternative, if the weakening is unacceptable, is to change what
  `loginbackoff` does with `nil` (count from the lookback horizon rather than 30 days),
  which costs no extra query but also lengthens backoff for genuinely-unknown identifiers.
  **Owner decision pending.**

### Process lessons (earned, not theoretical)

Three separate pieces of work today were reported complete while the part that proved they
worked was missing or unverified: an R-59 eviction suite that passed all seven guard-rail
cases and failed only the one proving eviction happens; a set of principal integration tests
that had never executed; and `RunReaper` — fully implemented, tested, and called from
nowhere. A fourth was a design error in this document's own author (the item-17 fallback
above). **Only quote test results you watched run** (`-count=1`, never a cached `ok`), and
verify a brief's premise before implementing its consequence.

---

## 1. Verification result — a third of the list is already done

Verified 2026-09-02 against `d08d88c`. **No work needed** on these; the todo doc's
"may have been fixed since the snapshot" caveat was right:

| Item | ID | Evidence |
|---|---|---|
| 31 | A-18 | `auth/login/endpoint.go:498` returns `usecase.Internal` → 500 |
| 33 | A-20 | `server/subsystems.go:507,544` prunes on `MaxWindow()+margin` |
| 34 | A-04 | `common/mediation.go:65` `Success(status int)` |
| 35 | A-11 | `router/mediator.go:249` returns before the breaker switch |
| 6 | R-27 | consumers parent on `manager.go:203` process root, not `bootCtx` |
| 7 | R-05 | `mediator.go:201` `ErrUseLastResponse`; 3xx → `ErrorConfig` |
| 11 | R-17 | `queue/postgres/postgres.go:226` quarantines to `queue_messages_failed` |
| 12 | R-06 | `mediator.go:310,346` pre-flight ACK + CONFIG warning, no breaker success |
| 17 | R-18 | `queue/sqs/sqs.go:277` sets no `MessageDeduplicationId` — leave alone |

**CORRECTION (2026-09-02):** R-13 was initially recorded here as already fixed. That was
wrong — it read only half the ruling. "Remove the shared global `""` group" is indeed done
(`pool.go:275`), but "an ordered message without a `messageGroupId` is malformed — ACK +
notice" is **not**: such a message is silently dispatched down the concurrent path today.
R-13 belongs with R-16 in the strict-routing gate, not in the done list.

**Partially done** — the remaining half is scoped in the tranches below: R-16 (scheduler
half done, router half not), R-36 (consumer liveness wired; pool success rate still
wrongly gates readiness), R-26/R-49 (shutdown correct; config-rebuild still aborts
in-flight), R-30 (holds last-known-good; no warning), R-57 (500 rejects; 505+ wrongly
releases), X-04 (store + API filters exist; three gaps).

## 2. Two things the review did not know

**a. A-01's ACK flip already shipped.** The ruling says "do not ship the ACK flip before
the platform half exists". `pool.go:664` already ACKs the buffered siblings, and
`ackBuffered`'s own comment concedes the consequence: *"nothing marks those job rows for
review, so they sit at QUEUED/PROCESSING rather than FAILED."* This is not a future risk
to sequence around — it is live silent message loss whenever a BLOCK_ON_ERROR head fails.
A-01's platform half is therefore the **highest-priority item on the list**, and the
sequencing instruction is moot.

**b. PR-3/PR-4 is sitting on top of live IDOR, not just an existence oracle.** The ruled
fix (403 → 404 on out-of-scope) assumes a scope check runs. On four principal routes none
runs at all:

- `GET /api/principals/{id}/version` (`api.go:335`) — coarse permission only
- `GET /api/principals/{id}/roles` (`api.go:1534`) — coarse permission only; returns another
  tenant's role assignments
- `GET /api/principals/{id}/application-access` (`api.go:1680`)
- `GET /api/principals/{id}/available-applications` (`api.go:1724`) — also leaks the target
  tenant's app entitlements

and `POST /{id}/roles` / `DELETE /{id}/roles/{role}` bypass authorization entirely on their
idempotent no-op path (`api.go:1576`, `api.go:1631`): the scope check lives *inside* the
`AssignRoles` use case, which is skipped when the role is already present/absent, yet the
handler still returns a full `PrincipalResponse`. `addRole` has **no coarse permission check
at the handler top at all** (verified: routes at `api.go:101-107` are plain registrations,
authorization is per-handler only).

**c. X-02 is the same class of bug.** `dispatchpool.SyncDispatchPools` and
`principal.SyncPrincipals` sweep **platform-wide** with no client/application containment
(`FindWithFilters(ctx, nil, nil)` / `FindAll(ctx)`), so any caller with app-sync access can
archive pools or strip `SDK_SYNC` roles belonging to unrelated tenants. The ruling files
this under P3 "bigger, phase them"; the containment half is a P0 security fix and is
promoted accordingly.

## 3. Tranches

Each tranche is independently shippable, lands as one commit carrying its ruling IDs, and
pins every behaviour change with a test. Tranches marked ⟂ are mutually independent and can
run concurrently; the rest are ordered.

### T1 — Router boot, auth and leadership ⟂
_Items 5, 15, 14. IDs R-43, R-33, R-34, R-30. No wire change._

1. **R-43** `router/api/auth.go:22-56` — `IsPublicPath` matches absolute paths but the
   middleware is mounted under `/router` (`server/run.go:207`), and chi does not rewrite
   `r.URL.Path`. Health, metrics and openapi.json 401 whenever router auth is on. Match
   relative to the mount prefix.
2. **R-34** — default-broker pools start unconditionally at `server/run.go:308`, before the
   leadership gate exists. Worse: on leadership **loss → regain**, `Shutdown` zeroes
   `m.pools`, then `startPools` finds `ConfigSource == nil` and logs "no pools will start"
   (`router/server.go:227`) — *a default-broker router that loses and regains leadership
   stops processing permanently until restarted.* Fix both halves: gate the bootstrap, and
   give `startPools` a re-bootstrap path for the `ConfigSource == nil` case.
3. **R-33** — `handlers_misc.go:150` `configReload` calls `Manager.Reconfigure` with no
   leadership check, so a follower starts consumers. `s.Leader` is already on `State` —
   gate on it (409 for a follower).
4. **R-30** — `config_sync.go:224-236` already holds last-known-good (it simply skips
   `Reconfigure`), but only `slog.Warn`s. Thread a `*WarningService` into the watcher and
   raise/clear a CONFIGURATION warning.

### T2 — Delivery classification ⟂
_Items 8, 9, 23. IDs R-57, R-12, A-27. Behaviour change; conformance-visible._

Order matters within the tranche:

1. **A-27 first, as a pure refactor.** Extract `dispositionOf(outcome, …)` out of the
   `switch` in `pool.go:973-1128`, leaving metrics/ack/flush side effects in `processOne`.
   `mediation_conformance_test.go:1-15` explicitly names this as the blocker for asserting
   message fate ("the decision lives in an inline switch inside pool.go's delivery loop with
   nothing a test can call"). Land with **no behaviour change**, then have the conformance
   runner assert disposition — which gives 2 and 3 a safety net.
   This is also the trigger for **router-go-idiom-plan item 5** (restructure the outcome
   enum into a struct carrying broker action + group effect); do it here, in the same
   refactor, since the enum is already being opened.
2. **R-57** — `pool.go:1074-1094` rejects only exactly `500`. Invert to the ruled allowlist:
   502/503/504 + transport = release; every other 5xx = reject into review.
3. **R-12** — `mediator.go:244` keys the breaker on the **full `MediationTarget` including
   query string**, so `?id=1` and `?id=2` get independent breakers and a dead host never
   trips one. Ruled key is **origin + path, query excluded** — note this is *not* host-only;
   `HostKeyFromURL` is the wrong helper to reach for here, a new origin+path key is needed.

### T3 — A-01 platform recovery ⟂ (highest priority — live data loss)
_Item 1. ID A-01._

The router half is done and the platform half is missing, so ACKed siblings currently
strand at QUEUED/PROCESSING forever. Prerequisites in `internal/platform/dispatchjob/`
(none of which exist today): `DispatchJob.IDStr()`, `Repository.Persist`, and an
`operations/` package. Every status query must carry `created_at` alongside `id` — the
table is partitioned by it (`sqlc/queries/dispatchjob.sql:41-49`).

1. `CancelDispatchJob` (FAILED → CANCELLED) and `CompleteDispatchJob` (FAILED → COMPLETED)
   as `usecaseop.Operation`s, modelled on `scheduledjob/operations/ops.go:229` `statusFlip`.
   Both are terminal → terminal operator overrides, so `Execute` must explicitly reject any
   source status other than FAILED with a 409 (`DispatchFailed.IsTerminal()` is already true).
2. **Resend** — marks the group's records back to PENDING. The existing
   `POST /api/dispatch-jobs/requeue` does this via a bare `UPDATE` straight from the handler
   (`repository.go:434`), contradicting the module's own doc comment (`entity.go:9`:
   "human-initiated dispatch-job actions (resend, ignore, cancel) DO go through use cases").
   Migrate it onto the envelope in this tranche rather than adding a third pattern beside it.
3. **Marking the ACKed siblings** — the one real design question, see §4.

No new group-release plumbing is needed: flipping the head off FAILED already releases the
rest via `GroupHoldingStatusSQL` on the poller's next pass (`scheduler/poller.go:251`).

### T4 — Principal authorization ⟂ (security)
_Item 3. IDs PR-3, PR-4 + the unruled IDOR above._

1. **The IDOR first, on its own commit** — it needs no wire change and should not wait
   behind the 403→404 sweep. Add the missing scope check to the four read routes; move the
   `addRole`/`removeRole` authorization *out* of the use case's skipped path so it runs
   before the idempotent early return; give `addRole` its coarse permission check.
2. **Then the ruled shape**: coarse permission gate before any load (403, nothing touched);
   after it, out-of-scope answers **404 byte-identical to not-found**. Build one shared
   load-then-scope helper and apply it to principal first.
3. **Then the sweep** — `auth.CheckScopeAccess` produces this shape at ~30 call sites across
   8 modules (connection, subscription, scheduledjob, dispatchjob, dispatchpool, application,
   eventtype, principal). One module per commit.

This changes the wire (403 → 404 on cross-tenant by-id reads): `make api-bump`, and the
lockfile diff is the human gate per CONVENTIONS.md §5a.

### T5 — X-02 sync containment (security half only)
_Item 27. ID X-02._ Promoted out of P3.

Scope `archiveUnlisted`/`removeUnlisted` to `clientId + applicationId`, and refuse a
platform-scope (`clientId: null`) sweep unless the caller is an anchor.
`scheduledjob/operations/sync.go` already has exactly this anchor gate — copy it. The two
unbounded sweeps are `dispatchpool/operations/sync.go` (global by design comment) and
`principal/operations/sync_principals.go` (`Authorize: Public`, `FindAll`).

### T6 — X-06 strict enum reads
_Item 4. ID X-06. Three phases; phase 1 is security._

1. **Phase 1 (security, small).** `serviceaccount.ParseAuthType` (`entity.go:27-40`) returns
   `AuthNone` on an unknown value — a typo'd `authType` silently produces an
   **unauthenticated webhook**, and the read path (`repository.go:301`) casts the raw column
   with no validation at all. Reject at both boundaries. Same treatment for
   `loginattempt.ParseOutcome`, which defaults unknown → `OutcomeSuccess` and would
   *undercount* lockout failures (`loginattempt.go:56`).
2. **Phase 2.** ~34 `Parse*` functions silently default; ~25 are called from repository row
   mappers, so a bad stored value is coerced on **every read**. Change the signature to
   return an error, with a distinct code and the row id logged; a list containing a bad row
   fails too. `common.ParseOutboxItemType` is the existing correct idiom to copy.
3. **Phase 3.** Write-boundary enforcement: ~30 enum-ish columns across ~20 tables, all
   plain `VARCHAR` — there are **zero** `CHECK` constraints and zero PG enum types in the
   schema today. Migration must scan for existing violations before adding constraints.

### T7 — A-19 real global lock ⟂
_Item 32._ `loginbackoff.go:139-155` uses `GlobalLockSecs` only to populate `RetryAfterSecs`.
The actual gate is the `GlobalWindowSecs` sliding window (default 3600s) which outlives the
advertised lock (default 900s), so a caller who waits exactly as long as told is still
refused. Make it a real lock.

### T8 — Observability
_Items 18-22. IDs X-04, R-36, R-52/R-53, R-56, R-04._

1. **X-04** — three gaps, not one. `QueueHealthMonitor` holds only a `*Notifier` and calls it
   directly (`queue_health.go:91,118`), so backlog warnings webhook but never reach
   `/warnings` or health counts — route it through the store as `StallDetector` already does
   (`inflight.go:379`). Emit the three dead categories (CONNECTION, RATE_LIMIT,
   CIRCUIT_BREAKER are declared at `notification.go:21-23` and constructed nowhere).
   `Notifier.SetMinSeverity` is implemented but never called — add `FC_NOTIFY_MIN_SEVERITY`.
   `severity`/`category` filters already exist (`handlers_warnings.go:60`); INFO TTL is new.
2. **R-36** — consumer liveness is already wired via `ConsumerStatsProvider`. The remaining
   half is the ruled *exclusion*: `HealthReport` (`health.go:212-263`) still degrades on
   `poolsUnhealthy`, which flows into `/health/ready`'s 503. Take pool success rate out of
   readiness, keep it as warning + metric.
3. **R-52/R-53** — `GroupFlushRegistry` already exposes `Stats`, `SuppressedUntil` and
   `Clear`; **none has a caller** (`group_flush.go:94-125`). Add the monitoring endpoint,
   the operator clear, and a suppressed-ACK pool metric (today a suppression is
   indistinguishable from a successful delivery at `pool.go:1023`).
4. **R-56** — `leaderAdapter.InstanceID()` (`api.go:358`) returns `Cfg.StandbyLockKey`, which
   is *identical on every replica*. The real per-process id is `Election.cfg.InstanceID`
   (`standby/election.go:137`), which has no exported getter — add one.
5. **R-04** — note the router UI is **not** the Vue SPA: it is a standalone embedded
   `router/api/dashboard.html` (1570 lines) served at `/monitoring/dashboard`. Blocked-groups
   needs new API surface first — `Pool.MessageGroupCount()` returns only a count, with no way
   to enumerate which groups are held or why. Do this after R-52/R-53, which builds half of it.

### T9 — Router group machinery
_Items 2, 13, 16. IDs R-16, R-26/R-49, R-59._

1. **R-16** — the scheduler half is **already done** (`scheduler/dispatcher.go:81` publishes
   both `PoolCode` and `DispatchMode`). Only the router-side strictness remains: today
   absence is silently defaulted (`manager.go:551` → DEFAULT-POOL; `message.go:47` →
   `NEXT_ON_ERROR`) while a *garbage* value warns. Ruled: absence = malformed → ACK + notice;
   unknown pool code keeps the DEFAULT-POOL fallback + routing warning.
2. **R-59** — no TTL eviction exists, and the live behaviour is worse than the ruling
   assumes: `Reconfigure`'s removal loop (`manager.go:862-868`) has no fallback-pool
   exemption, so **any unrelated config edit destroys every active per-client fallback pool,
   mid-delivery**. Fix the exemption *and* add the ruled idle TTL (`BreakerRegistry.Evict`,
   `circuit_breaker.go:273`, is the idiom to copy).
3. **R-26/R-49** — shutdown and leadership loss are already correct (`server.go:267-296`
   drains before cancelling; work runs under `workCtx`, not `pollCtx`). The gap is
   config-driven rebuild and the stall watchdog, which cancel `workCtx` directly
   (`manager.go:908-911`, `manager.go:1182`) and abort a delivery in the air. Detach it.

### T10 — router-go-idiom item 1 (per-group goroutine)
Its stated trigger is "next time the group machinery is opened", which T9 does. Recommend
landing it as its **own commit after T9** rather than interleaved: it is the plan's only
high-risk item and the only one it calls a *safety* change, and bundling it with behaviour
rulings makes a regression unattributable. Same work period, separate blame.

### T11 — P3 conventions (separate epic)
_Items 24-30, minus the X-02 containment already pulled into T5._

Sized, not scheduled — these are multi-week and want their own plan doc:

- **X-08** (item 28) is a **one-line fix, do it immediately**: `PrincipalsSynced.Subject()`
  (`principal/operations/events.go:359`) concatenates unconditionally, emitting
  `"platform.principals."` with a trailing dot for application-less syncs. The same file's
  `Execute` computes a correctly-guarded subject 150 lines earlier — but the outbox row uses
  the interface method (`outboxpgx/sink.go:112`), so the buggy one is what hits the wire and
  breaks downstream `ExtractEntityID`.
- **X-05** (24): 149 permission codes, 122 use cases, no 1:1 mapping anywhere — the mapping
  lives implicitly in whichever handler calls the constructor. The `uowseal` analyzer
  (`tools/analyzer/uowseal/`) is the right place to enforce it, but only after `Operation`
  gains a `Permission` field; cross-referencing handler call sites needs call-graph analysis.
  Note `auth.go` (32KB) duplicates the `seed/permissions.go` strings — dedupe first.
- **X-07** (25): three different semantics live simultaneously today — reject-409
  (eventtype archive), documented-idempotent-re-emit (subscription pause), and
  undocumented-idempotent-re-emit (dispatchpool, process, client). All three currently write
  an event + audit row on every call. 19 files.
- **X-09** (26): 96 pointer fields in create/update DTOs, 275 nullable TEXT columns.
  `NOT NULL DEFAULT ''` has exactly **one** precedent in the whole tree
  (`026_processes.sql:16`) — this is new convention, not an extension.
- **X-03** (29): no housekeeping for `iam_login_attempts` exists at all, and its PK is a
  bare `id` (must become `(id, occurred_at)`). Precedent is good: `019`/`022` range-partition
  five tables and `stream/partition_manager.go` already creates/drops partitions on a tick —
  extend it rather than writing a new one.
- **X-10** (30): **no branded-type precedent exists** — no `RoleName` type, no branded id
  types; TSIDs are bare `string`s everywhere. Entity factories like `process.New` are the
  closest idiom. Genuinely new ground; "apply opportunistically" is the right framing.

## 4. Owner decisions (2026-09-02)

_These are decisions the owner made in session on 2026-09-02, in answer to an explicit
multiple-choice question — not planner recommendations. Recorded verbatim so they are not
mistaken for assumptions and re-litigated. A concurrent session relabelled this heading as
"working assumptions"; that was based on not having seen the exchange._

| Question put to the owner | Answer chosen |
|---|---|
| How should the ACKed siblings get marked? | **Hook + reaper backstop** |
| How wide should the first PR-3/PR-4 pass go? | **IDOR now, sweep after** |
| Which tranches to start now? | **T1, T2, T3, T4.1** |

- **A-01 marking mechanism: (c) hook + reaper backstop** (recommended below; adopt unless
  the owner objects).
- **PR-3/PR-4 breadth: IDOR first as its own commit, then the 403→404 shape on principal,
  then the remaining 7 modules one commit each.**
- **Started first: T1, T2, T3, T4.1** (concurrently, on disjoint file lanes).

### How the ACKed siblings get marked

The router knows which ids it ACKed; the platform does not. Options considered:

- **(a) Router → platform hook.** The router POSTs the ACKed ids + reason to a new endpoint.
  Matches `ackBuffered`'s own suggestion ("a settled-message hook carrying the reason would
  make the ids recoverable") and marks the group promptly. Fails if the router dies between
  ACK and callback.
- **(b) Platform-side reaper.** Sweeps QUEUED/PROCESSING jobs whose group head is FAILED and
  which have no live delivery, resetting them to PENDING. Robust to router crashes; adds
  latency and needs a liveness heuristic to avoid racing a real in-flight delivery.
- **(c) Both — CHOSEN.** Hook as the fast path, reaper as the backstop. The hook alone
  reintroduces the same "ACK with nothing marking it" hole on any router crash, which is the
  exact failure this tranche exists to close.

Verified while planning, and load-bearing for the design: `GroupHoldingStatusSQL`
(`dispatchjob/group_hold.go`) holds a group at claim time on a `FAILED`/`ERROR` head, and the
poller applies it at `scheduler/poller.go:442-446`. So siblings reset to PENDING stay held
until an operator resolves the head, then flow again in order. **No new hold-back plumbing is
required** — this is why resend/cancel/complete are sufficient to recover a group.

## 5. Sequencing

T3 (data loss), T4.1 (IDOR), T5 and T6.1 are the security/correctness floor and should land
first; they are mutually independent. T1, T2 and T7 are independent of everything and can run
alongside. T8 and T9 follow T2 (both build on the disposition refactor). T10 follows T9.
T11 is a separate epic.
