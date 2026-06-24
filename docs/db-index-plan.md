# Database Index Plan

_Analysis of API access patterns vs. existing indexes, and a prioritized plan for what
to change. Scope: the platform Postgres schema (`internal/migrate/sql`), the sqlc query
set (`internal/sqlc/queries`), and the hand-rolled hot-path SQL in `internal/outbox`,
`internal/stream`, `internal/platform/scheduler`, and the repository layers._

## TL;DR

1. **The headline is a wrong-table bug, not a missing index.** The dispatch-job read
   repository (`FindWithFilters`, `DistinctValues`, `FindByEventID`) queries the **write**
   table `msg_dispatch_jobs`, which was *deliberately stripped of query indexes* in
   migration 015. It should query the projection `msg_dispatch_jobs_read` — exactly the way
   the events repo already reads `msg_events_read`. **Fix the table first; index second.**
2. After repointing, the read projections need a handful of filter/sort indexes they don't
   have today. The single most defensible standalone add is a **`created_at` index on
   `msg_events_read`** (its main list is `ORDER BY created_at DESC` with no supporting index).
3. The write tables and the SKIP-LOCKED claim paths are already well-indexed. Leave them lean.
4. Most of the schema is low-cardinality control-plane tables — no action.

---

## 1. Architecture recap (why the table split matters)

The messaging core uses **CQRS with monthly range partitioning**:

| Concern | Write table | Read projection |
|---|---|---|
| Events | `msg_events` | `msg_events_read` |
| Dispatch jobs | `msg_dispatch_jobs` | `msg_dispatch_jobs_read` |

- Both pairs are `PARTITION BY RANGE (created_at)`, monthly (migration 019). PK is
  `(id, created_at)`.
- **Write tables** carry only *transactional* indexes — partial indexes that serve the
  poll/claim/recover loops. Migration 015 is explicit about this: it **dropped**
  `status`, `client_id`, `subscription_id`, `created_at`, `scheduled_for`, and
  `message_group` indexes from `msg_dispatch_jobs` with the note _"Rich query indexes
  belong on `msg_dispatch_jobs_read` (the projection)."_
- **Read projections** are denormalized (they add `application`/`subdomain`/`aggregate`,
  `is_completed`/`is_terminal`) and are meant to serve every filtered list the
  admin/monitoring UI issues.

The stream projector (`internal/stream`) keeps the projections current via SKIP-LOCKED
claim loops on `projected_at`.

**The contract: all rich, user-facing filtered reads go to `*_read`; the write tables
serve only the engine.** The events repo honors this. The dispatch-job repo does not.

---

## 2. Existing index inventory (the parts that matter)

### Write tables — lean, partial, correct

`msg_events`
- `idx_msg_events_client_id (client_id)`
- `idx_msg_events_created_at (created_at)`
- `idx_msg_events_unprojected (created_at) WHERE projected_at IS NULL` ← projector claim
- `idx_msg_events_unfanned (created_at) WHERE fanned_out_at IS NULL` ← fan-out claim
- `UNIQUE idx_msg_events_deduplication (deduplication_id, created_at)`

`msg_dispatch_jobs`
- `idx_dispatch_jobs_pending_poll (message_group NULLS LAST, sequence, created_at) WHERE status='PENDING'` ← poller claim
- `idx_dispatch_jobs_blocked_groups (message_group, status) WHERE status IN ('FAILED','ERROR')` ← block-on-error
- `idx_dispatch_jobs_stale_queued (queued_at) WHERE status='QUEUED'` ← stale recovery
- `idx_msg_dispatch_jobs_unprojected (created_at) WHERE projected_at IS NULL` ← projector claim

These line up **exactly** with the hot-path SQL (verified against `poller.go`,
`stale_recovery.go`, `fan_out.go`, `events.go`, `dispatch_jobs.go`). No action.

### Read projections — partial coverage

`msg_events_read`: `type`, `client_id`, `time`, `application`, `subdomain`, `aggregate`,
`correlation_id`. **No `created_at` index.**

`msg_dispatch_jobs_read`: `status`, `client_id`, `application`, `subscription_id`,
`message_group`, `created_at`. **No `code`, `source`, `dispatch_pool_id`, `kind`,
`event_id`, `external_id`.**

### Outbox (customer-side, SDK)

The `internal/outbox` claim (`WHERE status=0 ORDER BY message_group, created_at … FOR
UPDATE SKIP LOCKED`) runs against `outbox_messages`, which is defined in the **SDK**
migrations (`clients/typescript-sdk/migrations`, `clients/laravel-sdk`), not the platform
schema. Its index lives there. Out of scope for the platform DB, but worth a parity check
that the SDK ships a `(status, message_group, created_at)` index.

---

## 3. Access-pattern → index gap analysis

### 3.1 Dispatch-job filtered list / facets / drill-down — **P0 (code fix + indexes)**

`dispatchjob/repository.go` runs these against `msg_dispatch_jobs` (write table):

| Method | Predicate | Sort |
|---|---|---|
| `FindWithFilters` | `status`/`status IN`, `client_id`/`IN`, `(client_id IS NULL OR client_id = ANY)`, `dispatch_pool_id`, `subscription_id`, `code`/`IN`/`LIKE`, `source`, `created_at` range | `created_at DESC` + LIMIT/OFFSET |
| `DistinctValues` | `WHERE <col> IS NOT NULL` for `status, code, client_id, dispatch_pool_id, subscription_id, kind` | `ORDER BY 1` |
| `FindByEventID` | `event_id = $1` | `created_at` |

On the write table these have **zero supporting indexes** (015 removed them) → full
per-partition seq scan + sort on every monitoring page load. The facet filters
(`application`/`subdomain`/`aggregate`) are forced into `code LIKE '%:x:%'` prefix scans
because the write table lacks those columns.

**Fix (prerequisite): repoint these reads to `msg_dispatch_jobs_read`**, mirroring the
events repo. The projection already has `application`/`subdomain`/`aggregate` as real
columns, so the `code LIKE` facet hack can be replaced with indexed equality. Note the
projection omits `payload`/`metadata`/`schema_id`/`payload_content_type`/`data_only` — the
list `SELECT` must drop those (a list view should not be hauling payloads anyway; keep
them on the `FindByID` detail path). Verify the list DTO doesn't depend on `payload`.

**Then add to `msg_dispatch_jobs_read`:**

```sql
-- Drill-down: GET /api/dispatch-jobs/event/{eventId}
CREATE INDEX idx_msg_dispatch_jobs_read_event_id ON msg_dispatch_jobs_read (event_id);

-- Facet dropdowns (DistinctValues) + equality filters not yet covered
CREATE INDEX idx_msg_dispatch_jobs_read_dispatch_pool_id ON msg_dispatch_jobs_read (dispatch_pool_id);
CREATE INDEX idx_msg_dispatch_jobs_read_code              ON msg_dispatch_jobs_read (code);

-- Dominant filter+sort combos (optional but high value): fold the ORDER BY into the index
CREATE INDEX idx_msg_dispatch_jobs_read_client_created ON msg_dispatch_jobs_read (client_id, created_at DESC);
CREATE INDEX idx_msg_dispatch_jobs_read_status_created ON msg_dispatch_jobs_read (status,    created_at DESC);
```

`source` and `kind` are only used by `DistinctValues` (filter-option dropdowns, low QPS) —
skip unless the dropdowns prove slow. The composite `(client_id, created_at DESC)` and
`(status, created_at DESC)` make the existing single-column `client_id`/`status` indexes
redundant for these queries; consider dropping the singles once the composites are in and
`pg_stat_user_indexes` confirms the singles aren't used elsewhere.

### 3.2 Events filtered list — **P1 (one real index)**

`event/repository.go` correctly reads `msg_events_read` and filters on `type`/`IN`,
`source`, `subject`, `client_id`/`IN`, `(client_id IS NULL OR ANY)`, `application`/
`subdomain`/`aggregate` (IN), `correlation_id`, `created_at` range — then
`ORDER BY created_at DESC LIMIT/OFFSET`.

The equality filters are individually indexed, but the **sort column `created_at` is
not** (the table indexes `time`, the event timestamp, not `created_at`, the ingest
timestamp). The default "recent events" view (no time filter) therefore can't walk an
index in `created_at DESC` order and falls back to scan-and-sort across partitions.

```sql
CREATE INDEX idx_msg_events_read_created_at ON msg_events_read (created_at);

-- Optional, for the two highest-traffic filter+sort combos (tenant + type views):
CREATE INDEX idx_msg_events_read_client_created ON msg_events_read (client_id, created_at DESC);
CREATE INDEX idx_msg_events_read_type_created   ON msg_events_read (type,      created_at DESC);
```

**Suspected dead index:** `idx_msg_events_read_time (time)` — nothing filters or sorts on
`time` (the repo uses `created_at`). Verify against `pg_stat_user_indexes` and drop if
unused. (The write table `msg_events` already has `created_at` indexed, so the
`FindRecentRaw` debug view is fine.)

### 3.3 Login-attempt brute-force throttle — **P2 (medium)**

`loginattempt.go` runs `SELECT COUNT(*), MAX(attempted_at) FROM iam_login_attempts WHERE
outcome='FAILURE' AND identifier=$1 AND ip_address=$2 AND attempted_at >= $3` on every
login. Today only single-column indexes exist (`identifier`, `outcome`, `attempted_at`,
`principal_id`) — the planner picks one and filters the rest. A targeted partial composite
serves the throttle directly:

```sql
CREATE INDEX idx_iam_login_attempts_throttle
    ON iam_login_attempts (identifier, attempted_at) WHERE outcome = 'FAILURE';
```

Low data volume today, so this is a "before it bites" add, not urgent. The
`iam_rate_limit_events` path is already well-served by
`idx_iam_rate_limit_events_lookup (bucket, key, occurred_at DESC)` — no change.

### 3.4 Watch item — dispatch-job re-projection scan

The projector claim is `WHERE projected_at IS NULL OR updated_at > projected_at ORDER BY
created_at … FOR UPDATE SKIP LOCKED` (`dispatch_jobs.go`). `idx_msg_dispatch_jobs_unprojected`
covers the `projected_at IS NULL` arm (new jobs) but **the `updated_at > projected_at` arm
is not indexable** (column-to-column comparison) — so under heavy status-churn the
projector re-scans live partitions. Not fixable with a plain index. If this shows up in
`pg_stat_statements`, the fix is a `needs_projection boolean` flag column with a partial
index, set on write and cleared by the projector — a schema change, out of scope for an
index-only pass. Flagging, not scheduling.

---

## 4. Things that are already fine (no action)

- **Hot-path claim loops** (outbox claim, event projection, fan-out, pending-job poller,
  stale recovery) — every one maps to a dedicated partial index. Verified.
- **Control-plane tables** (`tnt_clients`, `iam_principals`, `iam_roles`, `msg_subscriptions`,
  `msg_dispatch_pools`, `msg_connections`, `msg_event_types`, `oauth_clients`, IdPs,
  email-domain mappings, processes, …) — low cardinality, accessed by PK or by their
  unique `code`/`identifier`/`email_domain` columns, which are already indexed. `FindAll
  ORDER BY code` over a few hundred rows needs nothing.
- **Audit** (`aud_logs`) — filters on `entity_type/entity_id`, `principal_id`, `client_id`,
  `performed_at` range, sort `performed_at DESC`; covered by existing
  `(entity_type, entity_id)`, `performed_at`, `principal`, `client_id` indexes. Adequate at
  audit volume.
- **Scheduled-job instances** — `(scheduled_job_id)`, `(client_id)`, `(status)`, `(active)`
  indexes cover the history list and the "any active instance?" check.
- **Junction tables** — all keyed by their FK parent (`subscription_id`, `role_id`,
  `oauth_client_id`, `email_domain_mapping_id`, …). Fine.

## 5. Dead / unused query paths (don't index; consider removing)

- `DispatchJobFindPendingForPool` (sqlc) / `FindPendingForPool` (repo) — **no caller.** The
  live per-pool dispatch is the raw SQL in `scheduler/poller.go`. Don't add a
  `(dispatch_pool_id … ) WHERE status='PENDING'` index for it; delete the dead query
  instead.
- `FindByExternalID` / `DispatchJobFindByExternalID` — **no live caller** despite the
  "idempotent ingest" comment. Don't add an `external_id` index speculatively. **If/when
  ingest wires read-after-write idempotency, it must hit the write table** (the projection
  lags), so that future index goes on `msg_dispatch_jobs` as a partial/unique on
  `external_id`, decided at that time.

---

## 6. Rollout notes (partitioned tables)

- All `*_read` and `*_events`/`*_dispatch_jobs` tables are partitioned. `CREATE INDEX` on
  the parent takes an `ACCESS EXCLUSIVE` lock and builds each partition serially. For a
  table with live data, do it online: `CREATE INDEX CONCURRENTLY` on each existing
  partition, then `CREATE INDEX … ON ONLY <parent>` + `ALTER INDEX <parent_idx> ATTACH
  PARTITION <child_idx>` for each, so the parent index is marked valid without a global
  rebuild. The partition manager will create the index on future partitions automatically
  once it exists on the parent.
- For fc-dev's embedded Postgres (assumed near-empty), a plain `CREATE INDEX IF NOT EXISTS`
  in a new goose migration is fine.
- **Validate before committing.** Drive the monitoring UI / `/api/dispatch-jobs` +
  `/api/events` lists, then read `pg_stat_statements` for the actual offending plans and
  `pg_stat_user_indexes.idx_scan` to confirm which existing indexes are unused (the `time`
  and low-selectivity single-column candidates). Add the composites only where `EXPLAIN
  (ANALYZE, BUFFERS)` shows a sort/scan the index removes.

---

## 7. Prioritized worklist

| # | Pri | Change | Type | Status |
|---|-----|--------|------|--------|
| 1 | **P0** | Repoint `dispatchjob/repository.go` reads (`FindWithFilters`, `DistinctValues`, `FindByEventID`) from `msg_dispatch_jobs` → `msg_dispatch_jobs_read`; replace `code LIKE` facets with real `application`/`subdomain`/`aggregate` columns; trim payload/metadata from the list SELECT | code | ✅ done |
| 2 | **P0** | `msg_dispatch_jobs_read`: add `event_id`, `dispatch_pool_id`, `code` indexes | migration 036 | ✅ done |
| 3 | **P1** | `msg_events_read`: add `created_at` index | migration 036 | ✅ done |
| 4 | **P1** | Composites for dominant filter+sort: `(client_id, created_at DESC)` and `(status, created_at DESC)` on `msg_dispatch_jobs_read`; `(client_id, created_at DESC)` and `(type, created_at DESC)` on `msg_events_read` | migration 036 | ✅ done |
| 5 | **P2** | `iam_login_attempts`: partial `(identifier, attempted_at) WHERE outcome='FAILURE'` | migration 037 | ✅ done |
| 6 | **P2** | Drop dead `idx_msg_events_read_time` + the single-col `client_id`/`status`/`type` indexes made redundant by 036's composites | migration 037 | ✅ done |
| 7 | **P3** | Delete dead `FindPendingForPool` / `FindByExternalID` query paths (+ the orphaned `DispatchJobFindByEventID` sqlc query) | code | ✅ done |
| — | watch | Dispatch-job re-projection `updated_at > projected_at` scan — revisit with a `needs_projection` flag if `pg_stat_statements` flags it | schema | watch |

## 9. Implementation notes (P2/P3 shipped)

- **Migration `037_login_throttle_and_index_cleanup.sql`**:
  - Adds `idx_iam_login_attempts_failure_throttle (identifier, attempted_at) WHERE
    outcome='FAILURE'` — serves all three hot failure-counting queries
    (`CountRecentFailures`, `FailureCountByIdentifierSince`,
    `FailureStatsByIdentifierIPSince`); the per-IP variant filters `ip_address` as a
    cheap residual.
  - Drops `idx_msg_events_read_time` (dead — nothing filters/sorts on event-time;
    verified by code audit) and the four singles now strictly covered by 036's
    `(col, created_at DESC)` composites: `msg_dispatch_jobs_read (client_id)` / `(status)`
    and `msg_events_read (client_id)` / `(type)`. These are structurally redundant (prefix
    of a composite), so the drop is safe without runtime stats. `message_group` and the
    events facet singles (`source`/`subdomain`/`aggregate`/`correlation_id`) are **kept** —
    no composite covers them.
  - Verified end-to-end: the dispatchjob + event integration suites boot embedded PG,
    apply 001→037 (incl. the partitioned-index drops + partial index), and pass.
- **P3 dead-code removal** — deleted the zero-caller `FindByExternalID` and
  `FindPendingForPool` repo methods + their row adapters, and removed the three orphaned
  sqlc queries (`DispatchJobFindByExternalID`, `DispatchJobFindPendingForPool`, and the
  P0-orphaned `DispatchJobFindByEventID`) from `dispatchjob.sql`. Regenerated `dbq` with
  the pinned **sqlc v1.31.1** (matches the existing generated header) so the diff is
  exactly those removals — no spurious churn. If ingest idempotency is wired later it
  should read the **write** table on `external_id` (the projection lags), as noted in §5.

## 8. Implementation notes (P0/P1 shipped)

- **Migration `036_read_projection_indexes.sql`** adds the 5 dispatch-projection
  indexes + 3 events-projection indexes above. Verified: it applies cleanly against a
  real partitioned Postgres (the dispatchjob integration test boots embedded PG, runs
  every migration including 036, and passes).
- **`dispatchjob/repository.go`**: `FindWithFilters` + `DistinctValues` + `FindByEventID`
  now read `msg_dispatch_jobs_read` via a shared slim `readSelect`/`readRow`. Facet
  filters use the projection's real `application`/`subdomain`/`aggregate` columns
  (indexed equality) instead of leading-wildcard `code LIKE`. This is also a **parity
  fix** — these mirror Rust's `DispatchJobReadResponse`, which reads the projection.
- **Behavioral note (eventual consistency):** the dispatch-job list / by-event / facets
  now reflect the projection, which lags the write table by the projector's cycle — same
  as the events list already did. A just-created job appears once projected. The detail
  view (`FindByID`) still reads the write table, so it's immediately consistent.
- **Debug view preserved:** `GET /bff/debug/dispatch-jobs` needs the un-projected
  payload, so it now uses a new `FindRecentRaw` that reads `msg_dispatch_jobs` directly
  (mirrors the events repo's `FindRecentRaw`).
- **Facet indexes deferred:** `subdomain`/`aggregate` on `msg_dispatch_jobs_read` are
  *not* indexed — they're almost always drilled alongside `application`/`client_id`
  (both indexed), so the planner filters them as a cheap residual. Equality on these is
  already strictly better than the old leading-wildcard `code LIKE` (which no index could
  serve). Add single-column indexes only if `pg_stat_statements` shows standalone
  subdomain/aggregate filtering.
- **Not regenerated:** the now-unused sqlc query `DispatchJobFindByEventID` remains in
  `internal/sqlc/queries/dispatchjob.sql` (its generated method is harmless dead code);
  `sqlc` wasn't run to avoid an unrelated `@latest`-version diff. Clean it up on the next
  intentional `make sqlc`.
