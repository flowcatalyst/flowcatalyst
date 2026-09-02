# Owner rulings — Go work list

_Owner decisions taken 2026-09-01/02 while reviewing platform behaviour for the next
implementation. Each item is a ruled behaviour change for THIS repo. IDs index the
owner's decision ledger; keep them in commit messages for traceability. Before
changing anything, verify the current behaviour first — a few items may have been
fixed since the review's snapshot. Every behaviour change gets a test that pins the
new behaviour._

## P0 — correctness / data-safety

1. **[A-01] BLOCK_ON_ERROR: ACK the untried siblings, and build the re-queue path.**
   Today a terminally failed ordered head NACKs its buffered siblings back to the
   broker. Ruled: the router ACKs them instead, and the platform owns the group's
   recovery. Required, in the same tranche (data loss otherwise):
   - the ACKed siblings are marked platform-side so the group is visibly pending;
     the failed head is marked `FAILED` for review;
   - new use cases + routes: `CancelDispatchJob` (`FAILED → CANCELLED`, "ignore")
     and `CompleteDispatchJob` (`FAILED → COMPLETED`);
   - *resend* is a use case that marks the group's records back to `PENDING`; the
     poller/scheduler re-publishes them in order.
   Do **not** ship the ACK flip before the platform half exists.
2. **[R-16] Scheduler must publish `poolCode` and `dispatchMode`** (from the
   subscription) on every dispatch job. Router side: a message arriving with either
   absent is malformed — ACK + notice. An *unknown* pool code still falls back to
   `DEFAULT-POOL` with the routing warning; only absence is dropped.
3. **[PR-3/PR-4] Kill the principal existence oracle.** Coarse per-operation
   permission gate BEFORE any load (no permission ⇒ 403, nothing touched). After
   the gate, an out-of-scope target answers **404 byte-identical to not-found** —
   the by-id read's cross-tenant 403 becomes 404 (deliberate wire change), and every
   `/{id}/…` sub-route (roles, application-access, developer-credential) carries the
   same scope check. Applies to every id-addressed route on scoped aggregates.
4. **[X-06] Strict enum reads everywhere.** An unknown stored enum value is a loud
   read error (distinct code, row id logged; a list containing the row fails too),
   never a silent default. Enforce at the write boundary with PG enum types or
   `CHECK` constraints (migration scans for violations first). Specifically ruled:
   an unknown service-account auth type must **reject**, never become `NONE`
   (unauthenticated webhook).
5. **[R-43] Health must never require auth.** The router BasicAuth public-path
   bypass compares the full URL path and misses the mount prefix, so
   `/router/health/live`, `/router/metrics`, `/router/openapi.json` 401 when auth
   is on. Match relative to the mount.
6. **[R-27] Start consumers under the process-lifetime context.** The boot-scoped
   10 s context kills the first poll loop on return; the watchdog resurrects it
   ~65–95 s later with a spurious stall warning.

## P1 — router behaviour rulings

7. **[R-05] Never follow redirects.** Disable redirect-following on the delivery
   client; any 3xx is a permanent ACK-drop with a warning naming the `Location`
   (301/302/303 silently converting POST → body-less GET is the bug).
8. **[R-57] 5xx classification: reject everything except 502/503/504.**
   502/503/504 + transport = unavailable → hold at broker with backoff; every other
   5xx (500, 501, 505, …) = the app ran and answered → reject into the review flow.
   (Today only exactly 500 rejects.)
9. **[R-12] Circuit-breaker key = origin + path** (query string excluded).
10. **[R-13] Ordered message without a `messageGroupId` is malformed** — ACK +
    notice. Remove the shared global `""` group.
11. **[R-17] Park malformed broker payloads.** On the Postgres broker a payload
    that fails to decode is quarantined to `queue_messages_failed` (latest-failure
    policy) and the poll continues — never fail the batch / re-claim forever.
12. **[R-06] Non-HTTP mediation types and host-less targets**: still ACK-dropped,
    but raise a CONFIGURATION warning and record **no breaker success**.
13. **[R-26/R-49] In-flight deliveries detach and finish.** Consumer restarts,
    config changes and leadership loss never abort a delivery in the air — it
    completes and resolves its broker action afterward; buffered siblings follow the
    normal release rules. Group processors are long-lived: rebuilds/reloads affect
    polling, not processing. Shutdown drain = finish the in-hand message (within the
    budget), release the rest of the buffer to the broker; never cancel workers at
    drain start.
14. **[R-30] Hold last-known-good config for a failed source** (consumers keep
    running on it) instead of removing its pools/queues; CONFIGURATION warning
    while the source is failing, cleared on recovery.
15. **[R-33/R-34] Leadership-gate `POST /config/reload`** (follower: 409 or
    200-noop, never starts consumers) **and default-broker pools** (start only under
    leadership; recreate on loss→regain).
16. **[R-59] Evict idle synthesised `{client}-DEFAULT-POOL`s** after a TTL with no
    routed message; processors finish their buffers first; re-synthesise on demand.
17. **[R-18] Do not add a `MessageDeduplicationId`** to SQS publishes — ruled: keep
    content-based dedup; duplicate mediation is acceptable. (No change if none is
    set today; do not "fix" this.)

## P2 — observability / operator tooling

18. **[X-04] All warnings through the store; notifier subscribes.** STALL and
    QUEUE_HEALTH go through the warning store too; emit the never-emitted
    CONNECTION / RATE_LIMIT / CIRCUIT_BREAKER categories; the webhook notifier
    becomes a severity-filtered subscriber (`FC_NOTIFY_MIN_SEVERITY`, default
    `WARNING`); INFO entries carry a ~1 h TTL; `/warnings` gains `severity` and
    `category` filters.
19. **[R-36] Wire consumer-liveness into readiness** (`SetConsumerRunning` /
    `RecordConsumerPoll` are currently never called): a router that is not polling
    is not ready. Pool success rate stays OUT of readiness (warning + metric only —
    a failing target is not a failing router).
20. **[R-52/R-53] Group-flush suppression made visible**: monitoring API lists
    active suppressions (group, pool, until) + operator clear to lift one early;
    suppressed ACKs get their own pool metric so a flushed pool reads
    busy-but-suppressed, not idle.
21. **[R-56] `/monitoring/standby-status.instance_id`** returns the election's
    per-process instance id, not the configured lock key.
22. **[R-04] Router UI: blocked-groups view** — message groups currently held, the
    pool each belongs to, how many messages each holds, and that pool's
    rate/concurrency settings. (15-min delivery timeout stays; `ExtendVisibility`
    stays unwired.)
23. **[A-27] Extract `dispositionOf(outcome)`** as a pure function the pool loop
    calls (no behaviour change) so the conformance runner can assert message fate.

## P3 — platform conventions (bigger; phase them, one aggregate at a time)

24. **[X-05] One permission per use case.** `edit` consolidates create+update;
    lifecycle transitions, credential ops, grants and `sync` are separate verbs;
    permission codes derive from use case names; a lint requires every use case to
    declare its permission and every controller to gate with exactly it. Roles may
    hold wildcards (`platform:admin:process:*`); existing umbrella `:write`/`:manage`
    implies every verb on its aggregate for one release. Direction: prefer specific
    domain operations over generic `update`.
25. **[X-07] Verb semantics for state changes.** Target-state verbs (`archive`,
    `activate`, `suspend`) are idempotent — already-there = success, **no event, no
    audit row**. Transition verbs (`resume` = from PAUSED) require their
    precondition, 409 otherwise; `resume` must never un-archive. Invalid from-states
    are 409 regardless.
26. **[X-09] No optional fields on operations.** Every field an operation edits is
    required in the request (absent ⇒ 400 `FIELD_REQUIRED`); `""` is a valid value
    meaning "empty"; columns become `NOT NULL DEFAULT ''` (migration normalises
    NULL → ''). Placeholder text lives in the UI, never the data. Truly-nullable
    domain fields (e.g. subscription `connectionId`) get a specific clearing
    operation; sync payloads state such fields explicitly rather than omitting.
27. **[X-02] Narrow sync sweeps.** `archiveUnlisted`/`removeUnlisted` on scheduled
    jobs, dispatch pools and principal role sync operate on
    `clientId + applicationId` only; a platform-scope (`clientId: null`) sweep is
    refused unless the caller is an anchor.
28. **[X-08] Per-application rollup groups**: sync rollup message group =
    `platform:<aggregate>:<applicationCode>` (never finer); fix the trailing-dot
    subject for application-less principal syncs.
29. **[X-03] `iam_login_attempts`: partition + retain.** Range-partition by quarter
    on `occurred_at`, keep 3 years, housekeeping drops old partitions (no row
    DELETEs) and pre-creates the next + a DEFAULT partition; PK `(id, occurred_at)`;
    local indexes `(identifier, occurred_at)` and `(identifier, ip, occurred_at)`;
    backoff query always carries the `occurred_at >=` bound. Note the 3-year figure
    in the retention policy.
30. **[X-10] Branded types (direction, apply opportunistically):** ids, codes,
    names, URLs and secrets stop being bare `string`s at domain boundaries;
    validate-on-construct wrappers, enums for finite sets with no catch-all.

## Verify-first (ruled earlier; may already be done in this tree)

31. **[A-18]** login endpoint: cookie-mint failure after a correct password answers
    500 (was `BadRequest("MINT_FAILED")`).
32. **[A-19]** `GlobalLockSecs` is a real lock, not just `Retry-After`.
33. **[A-20]** purger sweeps `iam_rate_limit_events` with retention `MaxWindow()`.
34. **[A-04 / router Fix 1]** `common.Success()` carries the real 2xx status
    (constructor takes it).
35. **[A-11 / router Fix 6]** pre-flight rejections (429, open breaker, `ack:false`
    deferral) record no breaker success.
