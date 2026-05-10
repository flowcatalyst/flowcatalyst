# Partitioning

The high-volume messaging tables are RANGE-partitioned monthly on
`created_at`. Cleanup is `O(1)` (drop a partition) instead of running a
batched `DELETE` that competes with ingest for I/O. This document covers
how the partitioning is set up and how it's maintained.

## Partitioned tables

Seven parents, all `PARTITION BY RANGE (created_at)`:

| Table | Source migration |
|---|---|
| `msg_events` | `019_partition_messaging_tables.sql` |
| `msg_events_read` | `019_partition_messaging_tables.sql` |
| `msg_dispatch_jobs` | `019_partition_messaging_tables.sql` |
| `msg_dispatch_jobs_read` | `019_partition_messaging_tables.sql` |
| `msg_dispatch_job_attempts` | `019_partition_messaging_tables.sql` |
| `msg_scheduled_job_instances` | `022_partition_scheduled_job_history.sql` |
| `msg_scheduled_job_instance_logs` | `022_partition_scheduled_job_history.sql` |

Migrations 019 and 022 run on **every profile** (production *and* fc-dev's
embedded Postgres) so dev mirrors prod's table shape. Partition-related
schema bugs — UNIQUE constraints missing the partition key, queries that
don't include `created_at` in `WHERE` — fail in dev rather than in prod.

Bootstrap creates partitions for `(now − 1 month)` through `(now + 3
months)`. Forward-rolling and retention are handled at runtime by
`fc_stream::PartitionManagerService` — the bootstrap window is small on
purpose.

## Architecture: who manages partitions

**Same code, every environment.** The Rust
`fc_stream::PartitionManagerService` runs in production (fc-server's
stream-processor subsystem) and in fc-dev. It:

- Keeps **3 forward** monthly partitions ahead of the current month
  (`PartitionManagerConfig::months_forward`, default 3).
- Drops partitions whose date range is older than **90 days**
  (`PartitionManagerConfig::retention_days`, default 90).
- Uses a fixed naming convention `<parent>_YYYY_MM`, parsed back to
  determine partition age.
- Ticks once on startup, then every 24 hours.

We deliberately don't depend on a Postgres extension (pg_partman,
pg_cron, etc.). RDS extension allowlists move on AWS's schedule, not
ours, and we've already had one near-miss: RDS PG18 dropped
`pg_partman_bgw` from its allowlist. Keeping maintenance in pure Rust
gives us identical behaviour everywhere, no per-env extension setup, no
parameter-group reboot, and PG-version independence beyond what sqlx
itself supports.

## Concurrency

In production, fc-server gates the stream processor on leadership (Redis
lock via `fc_standby`), so the partition manager runs on exactly one
node. `CREATE TABLE IF NOT EXISTS` and `DROP TABLE IF EXISTS` are
idempotent regardless, so a brief overlap during failover is safe.

If fc-stream is fully down for several days, no forward partitions get
created — but the bootstrap leaves +3 months of headroom and runtime
maintains that, so a multi-day outage doesn't break ingest. If you want
extra defensiveness, the partition manager could be split into a tiny
standalone binary (it's a single poll loop). Not currently warranted.

## Changing config

`PartitionManagerConfig` is constructed in:
- `bin/fc-server/src/main.rs::spawn_stream_processor` (production)
- `bin/fc-dev/src/main.rs` (dev, inside the `start_stream_processor` call)

Defaults today: `months_forward = 3, retention_days = 90, tick_interval =
24h`. To change in code:

```rust
let svc = PartitionManagerService::new(
    pool,
    PartitionManagerConfig {
        months_forward: 6,           // keep 6 months ahead
        retention_days: 180,         // 6 months retention
        tick_interval: Duration::from_secs(60 * 60),  // hourly instead of daily
    },
);
```

If you find yourself wanting different retention per environment without a
rebuild, expose env vars (`FC_STREAM_PARTITION_MONTHS_FORWARD` etc.) at
the `spawn_stream_processor` boundary — easy retrofit.

## Adding a new partitioned table

1. **Migration**: create the table with `PARTITION BY RANGE (created_at)`,
   composite PK `(id, created_at)`, and bootstrap the initial monthly
   partitions (now-1 to now+3) using the same idempotency guard 019/022
   use (`SELECT EXISTS … FROM pg_partitioned_table` early-return).
2. **Add to `PARTITIONED_PARENTS`** in
   `crates/fc-stream/src/partition_manager.rs` so the manager covers it.
3. **Schema rule**: any UNIQUE constraint on the new table must include
   `created_at`. Postgres rejects partitioned-table UNIQUEs that don't
   include the partition key.

## Manually checking partition state

```sql
-- All partitions of msg_events
SELECT child.relname
FROM pg_inherits i
JOIN pg_class    p ON i.inhparent = p.oid
JOIN pg_class    child ON i.inhrelid = child.oid
WHERE p.relname = 'msg_events'
ORDER BY child.relname;
-- Expect: msg_events_<last_month> through msg_events_<this_month + 3>.

-- Confirm a parent is partitioned
SELECT c.relname, pt.partattrs
FROM pg_partitioned_table pt
JOIN pg_class c ON c.oid = pt.partrelid
WHERE c.relname = 'msg_events';

-- Inspect a specific partition's range
SELECT pg_get_expr(c.relpartbound, c.oid)
FROM pg_class c
WHERE c.relname = 'msg_events_2026_05';
```

Operationally you should also see periodic log lines from the partition
manager:

```
INFO  Partition manager started months_forward=3 retention_days=90
INFO  Partition manager tick created=1 dropped=0
```

## Common failure modes

- **`unique constraint on partitioned table must include all partitioning
  columns`** — a UNIQUE index or PRIMARY KEY in a migration doesn't
  include `created_at`. All partitioned-table UNIQUE/PK constraints must
  contain the partition key. Fix the migration's index definition.
- **`relation … already exists`** during 019/022 re-run — those
  migrations are idempotent (guarded on `pg_partitioned_table`); if you
  see this, check that the migration tracker (`_schema_migrations`) is
  recording them. Re-running an already-applied migration is a no-op.
- **Insert fails with "no partition of relation … found for row"** — a
  row's `created_at` is past the last forward partition. The manager
  only creates partitions for `[now, now + months_forward]`, so an
  intentionally-future-dated insert can fall outside the ring. Either
  back-date the insert or extend `months_forward`.

## Editing existing migrations: don't

The migration tracker (`_schema_migrations`) is keyed by id, not content.
Once a migration's row exists, its SQL is never re-read on subsequent
deploys. Editing a shipped migration is therefore a silent no-op on every
DB that already ran the original. **Always write a new migration** for
schema changes — `NNN_alter_<table>_add_<column>.sql` with
`ALTER TABLE ... ADD COLUMN IF NOT EXISTS ...`.

The runner detects drift via a sha256 of each migration's SQL stored in
`_schema_migrations.checksum`. On a later run with edited content, you'll
see:

```
WARN  Migration content changed since it was applied. The new SQL has NOT
been executed — migrations are immutable once shipped. If you intended a
schema change, write a new migration. If the edit was benign (e.g.
comment-only) you can silence this warning with:
UPDATE _schema_migrations SET checksum = '<current>' WHERE migration_id = '<id>'.
```

The warning is visible but doesn't fail startup — the bad path is silent
drift, not "deploy refused".

## Code references

- Migrations: `migrations/019_partition_messaging_tables.sql`,
  `migrations/022_partition_scheduled_job_history.sql`
- Rust manager: `crates/fc-stream/src/partition_manager.rs`
  (`PARTITIONED_PARENTS` allow-list, `tick`, `ensure_forward_partitions`,
  `drop_old_partitions`)
- Migration runner: `crates/fc-platform/src/shared/database.rs`
  (`run_migrations`)
