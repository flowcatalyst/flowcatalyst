# Schema changes on large tables (policy)

Owner rule (2026-10-19): when a migration adds an index or does anything that can hurt while a table
holds a lot of data, do it the online way. On MySQL that meant `pt-online-schema-change`. **On
PostgreSQL you almost never need an external tool**: Postgres has native online equivalents, listed
below. Reach for an external tool (`pg_repack`, `pgroll`, `pg-osc`) only for the cases marked
"table rewrite".

This applies to the Go platform's goose migrations (`internal/migrate/sql`), and equally to the Rust
and Java ports when they mirror a migration.

## Which tables count as large

Anything that grows with traffic: `msg_events` and its partitions, `msg_dispatch_jobs` and
`msg_dispatch_jobs_read` (and their partitions), `msg_dispatch_job_attempts`, audit logs, login
attempts, outbox tables, and any table you cannot say is small. Reference and config tables
(roles, applications, subscriptions, connections) are small; a plain migration is fine there.
fcdev's embedded Postgres is near-empty, so a plain statement is fine there too, but the migration
that ships must still be safe on a big production database.

## The equivalents (what to do instead of a plain statement)

| Change | Plain statement problem | Online way (Postgres) |
|---|---|---|
| Add an index | `CREATE INDEX` blocks writes for the whole build | `CREATE INDEX CONCURRENTLY` (goose: `-- +goose NO TRANSACTION`). Failed build leaves an INVALID index: `DROP INDEX CONCURRENTLY`, rerun |
| Add an index on a **partitioned** table | `CONCURRENTLY` is not allowed on the parent; a plain build takes one ACCESS EXCLUSIVE lock for all partitions | `CREATE INDEX ... ON ONLY <parent>`, then `CREATE INDEX CONCURRENTLY` on each partition, then `ALTER INDEX <parent_idx> ATTACH PARTITION <child_idx>`. Ship it as an ops script (see `scripts/ops/086-dispatch-read-indexes-online.sql`), run **before** the deploy; the migration then uses `IF NOT EXISTS` and does nothing. Rule recorded in `docs/db-index-plan.md` section 6 |
| Drop an index | brief lock | `DROP INDEX CONCURRENTLY` (not on a partitioned parent: drop per partition or accept the brief lock when idle) |
| Add a column | nullable, or a constant default, is instant (PG 11+) | a volatile default (`now()`, `gen_random_uuid()`) rewrites the table: add the column nullable, backfill in batches, then set the default |
| Add `NOT NULL` | scans the table under ACCESS EXCLUSIVE | `ALTER TABLE ... ADD CONSTRAINT c CHECK (col IS NOT NULL) NOT VALID`, then `VALIDATE CONSTRAINT`, then `SET NOT NULL` (PG 12+ skips the scan because of the validated check), then drop the check |
| Add a foreign key or CHECK | validates every row under a strong lock | add `NOT VALID`, then `VALIDATE CONSTRAINT` (weaker lock) |
| Backfill / data fix | one giant `UPDATE` holds locks, bloats, and stalls replicas | batch by key range (for example 5,000 to 50,000 rows), commit each batch, throttle, and make it idempotent and resumable. Prefer a throttled ops script over a migration for big tables |
| Change a column's type, or anything else that **rewrites the table** | ACCESS EXCLUSIVE for the whole rewrite | **table rewrite**: add a new column, backfill in batches, switch the code, drop the old one; or use `pg_repack` / `pgroll` / `pg-osc`. Needs an owner decision and a plan |
| Rename / drop a column the running code uses | breaks old instances during a rolling deploy | expand and contract: add the new shape, deploy code that handles both, then remove the old one in a later release |

## Rules for every migration that touches a large table

1. **Set `lock_timeout`** at the top of the statement block (for example `SET LOCAL lock_timeout = '5s'`)
   so a migration that cannot get its lock fails fast instead of queueing behind a long query and
   blocking everything behind itself. Rerun at a quiet moment.
2. **Say what it costs.** The migration's header comment states the lock it takes, whether it
   rewrites, and the expected duration on production-sized data. If it needs an ops script, say so
   and name the script.
3. **Test on production-shaped data.** An empty embedded database proves nothing about a build time
   or a lock. At least read the plan and count rows on a staging copy, and keep new indexes to what
   `EXPLAIN (ANALYZE, BUFFERS)` shows is needed. Every index is write-path cost on every insert
   and status change (see the index swap reasoning in `docs/db-index-plan.md`).
4. **Never `CREATE INDEX CONCURRENTLY` inside a transaction** (a goose migration is one by
   default). Use the no-transaction annotation, one statement per migration file.
5. **Ops scripts are rerunnable** (`IF NOT EXISTS`, skip finished steps) and are run with `psql`
   outside a transaction, before the deploy that carries the migration.
6. **Mirror it.** The Rust and Java migrations for the same change follow the same rule, and
   their hand-off docs name the ops script.
7. **Remember it is optional where the data is small.** Production with almost no dispatch traffic
   does not need the online script; say so in the change, do not assume either way. Ask how big the
   target table actually is before recommending a procedure.

## History

The 086 index swap (event type, subscription, connection filters) is the first change done under
this rule. The online script was run on a production database that has almost no dispatch traffic
(it was not needed there); the lesson recorded: check the table size first.
