# Migration bootstrap

`internal/migrate` applies the embedded `sql/NNN_*.sql` files with goose. `bootstrap()` runs first and makes
`goose_db_version` agree with the schema that is actually present, so that an already-migrated database is not
re-migrated (019/022 drop and recreate the messaging tables) and a migration is never recorded as applied
without its effect existing.

## Rules

1. **goose's ledger wins.** If it records any migration, no other tracker adds to it. Entries whose effect probe
   fails are removed (`repairLedger`) so goose applies them. It only un-marks, never marks.
2. **`_fc_migrations`** (Go's own pre-goose tracker, same numbering): seeded, probed versions only if their
   effect is visible, then dropped.
3. **Another implementation's tracker (`_schema_migrations`), or a populated schema with no tracker**
   (`tnt_clients` exists): the tracker's numbers are **not** used. The implementations number identically through
   030 and differently from 031 (the other side's 034 is its function registry; ours is 059), so copying numbers
   marks migrations applied that never ran. Only versions whose effect is visible are seeded
   (`probes.go`: `effectProbes`, plus `impliedBy` for 009 and 012-018). Everything else, including every
   data-only migration, is left for goose, which applies it (`WithAllowMissing`, because a lower migration may be
   pending beneath a higher one already visible).
4. A fresh database is left alone.

## Consequences for migration authors

- Add an effect probe to `probes.go` with every migration that has a schema effect. It must describe the
  **final** state after later migrations. `TestProbesHoldOnFullyMigratedDatabase` fails if a probe is false on a
  fully migrated database.
- Migrations from 031 must be re-runnable (`IF NOT EXISTS`, guarded data fixes): on a foreign database any
  migration without a visible effect runs for real. 035, 038 and 039 were made re-runnable for this (Up half only;
  goose keeps no checksums, so already-applied databases are unaffected).
- Data-only migrations (055, 063, 069-077, 079, 080) have no probe, so on a foreign database they run. They are
  idempotent. Go 071 (prune unemitted event types) also removes `platform:iam:user:password-reset-requested`,
  which the other implementation emits and re-seeds.

## Tests

`bootstrap_pg_test.go` (build tag `integration`, scratch databases on the embedded Postgres only): fresh,
Go database with lost ledger (data survives), foreign database at 070 (representative fingerprint), foreign
database at 015, a tracker that overclaims, a ledger poisoned by the old number-copying, and a Go ledger behind
a complete foreign tracker (Go older than 072). `TestBootstrap_RealForeignMigrations` replays the other
implementation's real files, read-only, when `FC_FOREIGN_MIGRATIONS_DIR` points at its `migrations/` directory.
