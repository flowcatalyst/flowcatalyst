# Outbox Processor

The outbox processor lives on the **consumer-application side**, not inside the FlowCatalyst platform. Its job is to read messages that an application has written to its own outbox table (in the same transaction as its business write) and forward them to the platform's HTTP API. Source: `crates/fc-outbox/`, binary `bin/fc-outbox-processor/`, also embeddable in `fc-server` for self-contained deployments.

The pattern is the standard transactional outbox: an application that wants to emit a FlowCatalyst event writes a business row and an outbox row inside the same database transaction, then trusts a background process to deliver the outbox row eventually. This trades immediate delivery for crash-safety — the message is durable before any HTTP call is attempted.

---

## Position in the system

```
┌────────────────────────────────────────────┐
│   Consumer application                     │
│                                            │
│   BEGIN;                                   │
│     INSERT INTO orders (...);              │
│     INSERT INTO outbox_messages (...);   ◄─── same tx
│   COMMIT;                                  │
│                                            │
│   ┌──────────────────────────────────────┐ │
│   │  fc-outbox-processor                 │ │
│   │  (deployed alongside the app)        │ │
│   │                                      │ │
│   │  poll → buffer → group distributor   │ │
│   │       → HTTP POST                    │ │
│   └─────────────┬────────────────────────┘ │
└─────────────────┼──────────────────────────┘
                  │  POST /api/events/batch
                  │  POST /api/dispatch-jobs/batch
                  │  POST /api/audit-logs/batch
                  ▼
        ┌─────────────────────────┐
        │  FlowCatalyst Platform  │
        └─────────────────────────┘
```

Three things matter:

1. **The outbox table lives in the application's database**, not the platform's. The processor is a sidecar/companion to each application, not a centralised platform component.
2. **Three item types share the same table** (default `outbox_messages`): `EVENT`, `DISPATCH_JOB`, `AUDIT_LOG`. The processor dispatches each type to its own platform endpoint.
3. **Per-group FIFO**. Messages with the same `message_group` deliver in order. Different groups deliver in parallel.

---

## Module layout

| File | Owns |
|---|---|
| `lib.rs` | Module exports. |
| `enhanced_processor.rs` | `EnhancedOutboxProcessor` — top-level orchestrator and `start()` / `stop()` lifecycle. |
| `repository.rs` | `OutboxRepository` trait and `OutboxTableConfig` (table name per type). |
| `postgres.rs`, `sqlite.rs`, `mysql.rs`, `mongo.rs` | Per-backend `OutboxRepository` implementations (feature-gated). |
| `buffer.rs` | `GlobalBuffer` — bounded in-memory queue between repository and distributor. |
| `group_distributor.rs` | `GroupDistributor` — routes messages to per-group processors. |
| `message_group_processor.rs` | `MessageGroupProcessor` — per-group FIFO accumulator + batcher. |
| `http_dispatcher.rs` | `HttpDispatcher` — POSTs batches to the platform API, classifies responses per item. |
| `recovery.rs` | `RecoveryTask` — re-pends stuck `IN_PROGRESS` items. |

Layered flow:

```
                  poll loop
                     │
                     ▼
   ┌─────────────┐
   │ OutboxRepo  │  ── fetch_pending_by_type ──▶ Postgres / SQLite / MySQL / Mongo
   └──────┬──────┘
          │
          ▼
   ┌──────────────────┐
   │  GlobalBuffer    │  ── bounded VecDeque; if full, poll waits
   └──────┬───────────┘
          │
          ▼
   ┌──────────────────┐
   │ GroupDistributor │  ── one MessageGroupProcessor per active group
   └──────┬───────────┘
          │
          ▼
   ┌──────────────────────────────────────────────────────────────┐
   │  MessageGroupProcessor (one per group)                       │
   │                                                              │
   │   accumulate up to api_batch_size items                      │
   │   semaphore(1) → at most one dispatch in flight per group    │
   │   HttpDispatcher.send(batch)                                 │
   │     200 with per-item results → repo.mark_with_status        │
   │     retryable error            → repo.increment_retry_count  │
   │     terminal error             → repo.mark_with_status FAILED│
   └──────────────────────────────────────────────────────────────┘
```

Independently, a `RecoveryTask` polls for items stuck in `IN_PROGRESS` past `processing_timeout` and resets them to `PENDING`. This is the safety net for processor crashes mid-batch.

---

## OutboxRepository trait

The repository is the cross-backend abstraction. Methods (`crates/fc-outbox/src/repository.rs`):

```rust
async fn fetch_pending_by_type(item_type: ItemType, limit: usize) -> Result<Vec<OutboxItem>>;
async fn mark_in_progress(item_type: ItemType, ids: &[String]) -> Result<()>;
async fn mark_with_status(item_type: ItemType, ids: &[String], status: ItemStatus, err: Option<&str>) -> Result<()>;
async fn increment_retry_count(item_type: ItemType, ids: &[String]) -> Result<()>;
async fn fetch_recoverable_items(item_type: ItemType, timeout: Duration, limit: usize) -> Result<Vec<OutboxItem>>;
async fn fetch_stuck_items(item_type: ItemType, timeout: Duration, limit: usize) -> Result<Vec<OutboxItem>>;
async fn reset_stuck_items(item_type: ItemType, ids: &[String]) -> Result<()>;
```

`ItemType` is `EVENT | DISPATCH_JOB | AUDIT_LOG`. Each is routed to a table via `OutboxTableConfig`:

```rust
pub struct OutboxTableConfig {
    pub events_table: String,        // default "outbox_messages"
    pub dispatch_jobs_table: String, // default "outbox_messages"
    pub audit_logs_table: String,    // default "outbox_messages"
}
```

By default all three types live in one table with a `type` discriminator column. You can split them across three physical tables (or three databases) by overriding the env vars `FC_OUTBOX_EVENTS_TABLE` / `FC_OUTBOX_DISPATCH_JOBS_TABLE` / `FC_OUTBOX_AUDIT_LOGS_TABLE`. Useful for isolating audit-log volume from event volume.

Status values stored in the table:

| Status | Numeric | Meaning |
|---|---|---|
| `PENDING` | 1 | Available for pickup |
| `IN_PROGRESS` | 9 | Claimed by a processor; waiting for response |
| `SUCCESS` | 2 | Platform accepted |
| `FAILED` | 3 | Terminal failure (bad request, forbidden — won't retry) |
| `BAD_REQUEST` | 4 | 400 from platform (won't retry; investigate) |
| `INTERNAL_ERROR` | 5 | 5xx from platform (will retry until retry limit) |
| `UNAUTHORIZED` | 6 | 401 (will retry — token refresh may have fixed it) |
| `FORBIDDEN` | 7 | 403 (terminal — caller is misconfigured) |
| `GATEWAY_ERROR` | 8 | Network / DNS / connection (will retry) |

The numeric values are stable across schema versions; new statuses are appended, never reordered.

---

## GlobalBuffer

`buffer.rs::GlobalBuffer`. A bounded `Mutex<VecDeque<OutboxItem>>` between the repository fetch and the distributor. The buffer is the **first** backpressure point: if the distributor is slow (downstream API is slow), the buffer fills, and the poll loop yields rather than fetching more rows from the database. Default size: 1000 items.

Without this, a fast poll loop combined with a slow downstream would fill memory with claimed-but-undispatched items. The buffer caps that exposure.

`max_in_flight` (default 5000) is the **second** backpressure point: even if the buffer drains, the processor stops claiming new items once 5000 are in flight across all groups. This caps total memory under any workload shape.

---

## GroupDistributor

`group_distributor.rs::GroupDistributor`. Owns `HashMap<String, GroupEntry>` (active groups), `BatchMessageDispatcher` (HttpDispatcher impl), and stats.

Algorithm:

```
On item arrival from buffer:
    if item.message_group is None:
        dispatcher.dispatch(item, no-group)   // no ordering
    else:
        let proc = groups.entry(group).or_insert_with(|| spawn MessageGroupProcessor)
        proc.enqueue(item)
```

`max_concurrent_groups` (default 10) caps how many groups can be actively dispatching at once. With heavy workload across many groups, the 11th group waits until one of the 10 finishes a batch. This bounds parallel HTTP connections to the platform — useful because the platform has its own admission control and a 500-way fan-out from a single processor isn't friendly.

Groups become inactive when their queue drains; their `MessageGroupProcessor` is dropped, releasing the slot.

---

## MessageGroupProcessor

`message_group_processor.rs::MessageGroupProcessor`. Per-group state:

- A `VecDeque<TrackedMessage>` accumulating incoming items.
- A `Semaphore::new(1)` — at most one in-flight HTTP dispatch per group (this is what enforces FIFO).
- A reference to the `HttpDispatcher` and the `OutboxRepository`.

Per-cycle:

1. Drain up to `api_batch_size` items (default 100) from the queue. All must be of the same `ItemType` — different types go to different endpoints, so they can't share a batch.
2. Acquire the semaphore permit.
3. Call `repository.mark_in_progress(type, ids)`.
4. Call `dispatcher.dispatch(type, items)` → HTTP POST.
5. Parse per-item results, partition by retryable vs terminal vs success.
6. Single round-trip per result class:
   - `repository.mark_with_status(SUCCESS, success_ids)` to mark complete.
   - `repository.increment_retry_count(retryable_ids)` — bumps `retry_count`, sets status back to `PENDING`.
   - `repository.mark_with_status(terminal_status, terminal_ids, error)` — marks `FAILED`/`BAD_REQUEST`/`FORBIDDEN` and stops retrying.
7. Release the semaphore. If more items are queued, loop.

The `Semaphore::new(1)` is the critical piece. Without it, multiple drain iterations could race and the platform might receive items out of order.

---

## HttpDispatcher

`http_dispatcher.rs::HttpDispatcher`. Routes item types to endpoints:

| Item type | Endpoint |
|---|---|
| `EVENT` | `POST {api_base_url}/api/events/batch` |
| `DISPATCH_JOB` | `POST {api_base_url}/api/dispatch-jobs/batch` |
| `AUDIT_LOG` | `POST {api_base_url}/api/audit-logs/batch` |

Authorization: `Authorization: Bearer {api_token}` if `FC_API_TOKEN` is set. Otherwise unauthenticated (acceptable in dev / fc-dev mode; mandatory in prod).

Request payload:

```json
{
  "items": [
    { ...row 1's payload, as-stored... },
    { ...row 2's payload... }
  ]
}
```

Response:

```json
{
  "results": [
    { "id": "evt_...", "status": "SUCCESS" },
    { "id": "evt_...", "status": "BAD_REQUEST", "error": "missing field 'source'" },
    { "id": "evt_...", "status": "INTERNAL_ERROR", "error": "tx aborted" }
  ]
}
```

Per-item granularity matters: one bad event in a batch of 100 shouldn't poison the other 99. The platform's batch endpoints commit each item independently and report individual outcomes.

Retry classification on the processor side:

| Platform status | Processor action |
|---|---|
| `SUCCESS` | Mark `SUCCESS`. Done. |
| `BAD_REQUEST`, `FORBIDDEN` | Mark terminal. Operator must investigate. |
| `INTERNAL_ERROR`, `UNAUTHORIZED`, `GATEWAY_ERROR` | Increment retry; row goes back to `PENDING`. |
| HTTP-level network error | Treat as `GATEWAY_ERROR` for all items in the batch. |

`UNAUTHORIZED` is treated as retryable on the theory that the API token might be on the cusp of rotation and the next call will use the fresh one.

---

## Recovery task

`recovery.rs::RecoveryTask`. Runs on its own interval (default 60 s). Queries every `ItemType` for rows where `status = IN_PROGRESS AND updated_at < (now - processing_timeout)`. Default timeout is 5 minutes.

For each stuck batch, sets `status = PENDING` and `retry_count = retry_count + 1`. The next regular poll picks them up.

This is the leak-stopper for processor crashes mid-batch. Without it, items marked `IN_PROGRESS` before a crash would stay marked forever, and no future poll would ever see them. With it, you get at-least-once delivery even across hard crashes.

---

## Backends

| Backend | Feature | Status |
|---|---|---|
| PostgreSQL | `postgres` | Primary backend. SQLx + handwritten SQL. |
| SQLite | `sqlite` | For local dev or single-instance deploys. |
| MySQL | `mysql` | Supported. |
| MongoDB | `mongo` | Supported — outbox is the **only** crate in the workspace where MongoDB is still wired in (everywhere else it was removed). The platform team kept it for customers whose apps are MongoDB-native. |

Each backend ships its own schema migrations. For Postgres:

```sql
CREATE TABLE outbox_messages (
    id VARCHAR(36) PRIMARY KEY,
    type SMALLINT NOT NULL,           -- EVENT=1, DISPATCH_JOB=2, AUDIT_LOG=3
    status SMALLINT NOT NULL DEFAULT 1,
    payload JSONB NOT NULL,
    message_group VARCHAR(255),
    deduplication_id VARCHAR(255),
    retry_count INT NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX outbox_status_type ON outbox_messages(type, status, created_at)
    WHERE status IN (1, 9);          -- only PENDING and IN_PROGRESS rows
```

A **partial index** on the active states keeps the polling query O(log n) of the active backlog rather than O(log n) of all history.

---

## Configuration

Read by `bin/fc-outbox-processor/src/main.rs` (or `bin/fc-server/src/main.rs::spawn_outbox_processor` when embedded):

| Variable | Default | Description |
|---|---|---|
| `FC_OUTBOX_DB_TYPE` | `postgres` | `sqlite`, `postgres`, `mysql`, `mongo` |
| `FC_OUTBOX_DB_URL` | — (required) | Application database URL |
| `FC_OUTBOX_MONGO_DB` | `flowcatalyst` | MongoDB database name (mongo only) |
| `FC_OUTBOX_EVENTS_TABLE` | `outbox_messages` | Per-type table override |
| `FC_OUTBOX_DISPATCH_JOBS_TABLE` | `outbox_messages` | Per-type table override |
| `FC_OUTBOX_AUDIT_LOGS_TABLE` | `outbox_messages` | Per-type table override |
| `FC_OUTBOX_POLL_INTERVAL_MS` | `1000` | Poll cadence when idle |
| `FC_OUTBOX_BATCH_SIZE` | `500` | Max items per poll (across all types) |
| `FC_API_BASE_URL` | `http://localhost:8080` | Platform API base URL |
| `FC_API_TOKEN` | — | Bearer token; required in prod, optional in dev |
| `FC_API_BATCH_SIZE` | `100` | Items per HTTP POST |
| `FC_MAX_IN_FLIGHT` | `5000` | Cap on claimed-but-undispatched items |
| `FC_GLOBAL_BUFFER_SIZE` | `1000` | Buffer between repo and distributor |
| `FC_MAX_CONCURRENT_GROUPS` | `10` | Active groups dispatching simultaneously |
| `FC_METRICS_PORT` | `9090` | Prometheus / health |
| `RUST_LOG` | `info` | Log level |

Sizing rule of thumb: keep `FC_API_BATCH_SIZE × FC_MAX_CONCURRENT_GROUPS ≪ platform's accept rate`. The platform's batch endpoints commit each item in its own transaction, so a 100-item batch costs 100 commits. A single processor sustaining 100 × 10 / sec = 1000 commits/sec is a lot of platform load; tune `FC_MAX_CONCURRENT_GROUPS` down if you see platform Postgres saturation correlating to outbox throughput.

---

## Standby integration

The outbox processor is a singleton (per consuming-application database). If you run two instances pointing at the same outbox table, both will fetch the same `PENDING` rows and double-dispatch — the platform's deduplication will catch this, but it's wasted work.

For HA, run the processor with `FC_STANDBY_ENABLED=true` and let only the leader poll. The standalone binary supports this directly via the `fc-standby` crate; the embedded variant in `fc-server` uses the cluster-wide leader lock.

```
FC_OUTBOX_ENABLED=true
FC_STANDBY_ENABLED=true
FC_STANDBY_REDIS_URL=redis://redis:6379
FC_STANDBY_LOCK_KEY=app-acme-outbox-leader   # one key per application's outbox
```

The lock key must be **unique per outbox**, not per FlowCatalyst cluster — if you have three applications with three outboxes, each gets its own lock.

---

## How the outbox relates to the platform's own UoW

The platform itself uses a different write path. Its own write operations go through `UnitOfWork::commit` (see [platform-control-plane.md](platform-control-plane.md)), which inserts directly into `msg_events` in the same transaction as the entity change — no outbox needed because the platform owns both tables.

The outbox pattern exists for the case where the **application** and **platform** databases are different (which is the common case: application Postgres on one host, FlowCatalyst Postgres on another). The application can't write atomically across them, so it writes to its own outbox and lets a separate process bridge.

If your application happens to share a database with FlowCatalyst, you can write to FlowCatalyst's `outbox_messages` table directly and skip the outbox processor — but that couples your app's schema to FlowCatalyst's, which is rarely what you want.

---

## Code references

- Entry point (standalone): `bin/fc-outbox-processor/src/main.rs`.
- Entry point (embedded): `bin/fc-server/src/main.rs::spawn_outbox_processor`.
- Orchestrator: `crates/fc-outbox/src/enhanced_processor.rs::EnhancedOutboxProcessor`.
- Repository trait: `crates/fc-outbox/src/repository.rs`.
- Backends: `crates/fc-outbox/src/{postgres,sqlite,mysql,mongo}.rs`.
- Buffer: `crates/fc-outbox/src/buffer.rs`.
- Distributor: `crates/fc-outbox/src/group_distributor.rs`.
- Per-group processor: `crates/fc-outbox/src/message_group_processor.rs`.
- HTTP dispatcher: `crates/fc-outbox/src/http_dispatcher.rs`.
- Recovery: `crates/fc-outbox/src/recovery.rs`.
- SDK helper for *writing* outbox rows from an application: `crates/fc-sdk/` (with the `outbox-postgres` / `outbox-sqlite` features).
