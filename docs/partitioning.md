# Partitioning

The high-volume messaging tables are RANGE-partitioned monthly on
`created_at`. Cleanup is `O(1)` (drop a partition) instead of running a
batched `DELETE` that competes with ingest for I/O. This document covers
how the partitioning is set up, who manages forward/retention in each
environment, and what infrastructure-as-code needs to be in place.

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
months)`. Forward-rolling and retention are handled at runtime — see
below — so the bootstrap window is small on purpose.

## Architecture: who manages partitions where

| Environment | Forward + retention managed by |
|---|---|
| **Production** (RDS, fc-server) | `pg_partman_bgw` (in-database worker) |
| **fc-dev** (embedded Postgres) | `fc_stream::PartitionManagerService` (Rust task) |

Both implementations:
- Keep **3 forward** monthly partitions ahead of the current month.
- Drop partitions whose date range is older than **90 days**.
- Use the same naming convention: `<parent>_YYYY_MM`.

The Rust manager auto-defers when it sees pg_partman has registered every
parent in `partman.part_config` (logged as
`Partition manager: pg_partman has all 7 parents registered; deferring to
bgw`). So production runs partman exclusively; fc-dev runs Rust exclusively;
neither requires an explicit env-var toggle.

## Production setup (one-time per environment)

`pg_partman` is a Postgres extension and must be installed before migration
023 runs. The setup happens in two places: cluster-level via IaC, then a
post-deploy step in the database itself.

### 1. RDS parameter group (IaC)

Add `pg_partman` to `shared_preload_libraries`. Point the bgw at the right
database and authenticating role:

```
shared_preload_libraries = 'pg_partman_bgw'
pg_partman_bgw.interval  = 3600   # seconds; bgw runs maintenance every hour
pg_partman_bgw.role      = inhance_admin
pg_partman_bgw.dbname    = flowcatalyst
```

Apply, then **reboot** the instance — `shared_preload_libraries` is a
restart-only parameter.

### 2. Install the extension (per database, one-time)

Connect through the bastion as a user with `CREATE EXTENSION` privilege
and run:

```sql
CREATE SCHEMA IF NOT EXISTS partman;
CREATE EXTENSION IF NOT EXISTS pg_partman SCHEMA partman;
GRANT ALL ON SCHEMA partman TO inhance_admin;
```

The bgw worker is dormant until the extension is created; once it is,
migration 023 (`023_partman_takeover.sql`) registers our seven parents
with `partman.create_parent(...)` and bgw takes over from there.

### 3. Deploy and migrate

Deploy fc-server. On startup it runs migrations including 023, which:
1. Drops every existing child partition under each parent (assumes empty
   tables — this migration is dev-environment-only safe to run; once
   production is live it should be guarded or replaced with an
   in-place adoption).
2. Calls `partman.create_parent(...)` for each parent (native, monthly,
   `premake = 3`).
3. Sets `retention = '90 days', retention_keep_table = false`.
4. Calls `partman.run_maintenance()` once to materialise the initial
   forward partitions.

The Rust `PartitionManagerService` starts, sees all seven parents in
`partman.part_config`, and exits. Logs:

```
Partition manager: pg_partman has all 7 parents registered; deferring to bgw
```

## Operating partman: changing config

Every partman setting lives in `partman.part_config`. Changes go through
**migrations** — they're plain SQL `UPDATE`s and the migration tracker
keeps them idempotent. Examples:

```sql
-- Extend retention on events to 180 days
UPDATE partman.part_config
SET retention = '180 days'
WHERE parent_table = 'public.msg_events';
```

```sql
-- Add a new partitioned table to partman's care
SELECT partman.create_parent(
    p_parent_table := 'public.msg_new_table',
    p_control      := 'created_at',
    p_interval     := '1 month',
    p_premake      := 3
);
UPDATE partman.part_config
SET retention = '90 days', retention_keep_table = false
WHERE parent_table = 'public.msg_new_table';
```

```sql
-- Hand a parent back from partman (e.g. you want different cadence)
SELECT partman.undo_partition('public.msg_dispatch_job_attempts');
DELETE FROM partman.part_config WHERE parent_table = 'public.msg_dispatch_job_attempts';
```

`pg_partman_bgw.*` GUCs (interval, role, dbname) are *not* migration-
driven — those live in the IaC parameter group.

## fc-dev (embedded Postgres) details

fc-dev uses `postgresql_embedded` and does **not** install pg_partman (the
extension isn't available in the embedded build). The Rust
`PartitionManagerService` runs instead:

- Configured in `bin/fc-dev/src/main.rs` (`partition_manager_enabled: true`).
- Default cadence: `months_forward = 3`, `retention_days = 90`, ticks once on
  startup then every 24 hours.
- Tunable via `PartitionManagerConfig` if a developer needs different
  retention locally.

## Manually checking partition state

```sql
-- All partitions of msg_events
SELECT child.relname
FROM pg_inherits i
JOIN pg_class    p ON i.inhparent = p.oid
JOIN pg_class    child ON i.inhrelid = child.oid
WHERE p.relname = 'msg_events'
ORDER BY child.relname;

-- partman config (production only)
SELECT parent_table, partition_interval, premake, retention,
       retention_keep_table, infinite_time_partitions
FROM partman.part_config
ORDER BY parent_table;

-- Last bgw run (production only)
SELECT pid, query, state, query_start
FROM pg_stat_activity
WHERE backend_type = 'background worker'
  AND application_name LIKE '%partman%';
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
- **`pg_partman extension is not installed`** during 023 — IaC step
  hasn't run, or the parameter-group reboot was skipped. See *Production
  setup* above.
- **Both Rust manager and bgw running** — shouldn't happen because the
  Rust manager auto-defers when partman has the parents registered. If it
  does, both will issue the same idempotent CREATE/DROP statements; no
  data corruption, just doubled work. Check
  `partman.part_config WHERE parent_table = 'public.msg_events'` returns
  a row.

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
  `migrations/022_partition_scheduled_job_history.sql`,
  `migrations/023_partman_takeover.sql`
- Rust manager: `crates/fc-stream/src/partition_manager.rs`
  (`PARTITIONED_PARENTS` list, `partman_owns_all_parents` auto-defer probe)
- Migration runner: `crates/fc-platform/src/shared/database.rs`
  (`run_migrations`, core vs production split)
