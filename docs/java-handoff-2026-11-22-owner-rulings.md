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

**Before.** A poll and a lease release read the runtime's row with `SELECT ... FOR UPDATE` to apply the
instance guard (`409 RUNTIME_ID_IN_USE`). A runtime id with no row yet has nothing to lock, so two
processes making the first poll of a new id at the same moment both read no row, both passed the guard,
and both were answered the same polled ingestions until the next poll refused one.

**Contract** (Go: `internal/platform/ingestion/runtime.go` `previousPoll`, used by `Poll` and `release.go`
`Release`; the query `IngestionRuntimeLock` in `internal/sqlc/queries/ingestion_runtime.sql`).

- In the poll's and the release's transaction, before the runtime's row is read, take
  `SELECT pg_advisory_xact_lock(hashtext('ingestion-runtime:' || runtimeId))`: the single-argument
  (bigint) form, the key the `int4` that `hashtext` returns, on the text `ingestion-runtime:` followed by
  the runtime id exactly as sent. This exact expression, because a Java and a Go platform polled by one
  fleet must contend on the same key. It is released when the transaction ends.
- Lock order, always: the runtime id's lock, then the runtime's row (`FOR UPDATE`), then the lease lock
  (`pg_advisory_xact_lock(hashtext('ingestion-leases'))`). Nothing takes them in another order.
- No wire, schema or status change. The second of two first polls now reads the first's row: refused
  `409 RUNTIME_ID_IN_USE` when the two sent different `instanceId`s (as any later poll would be); when
  either sent none, both are accepted as before (the instance guard checks only a poll that names one).

**Test to port.** `robustness_pg_test.go` `TestInstanceGuard_TwoFirstPollsOfANewIDAreDecidedInTurn`: a
session holds the lease lock, so the first poll of a new id waits after its upsert; the second poll of the
same id starts and waits too; the lease lock is let go; the first is answered the polled source and the
second is refused `RUNTIME_ID_IN_USE`, and the stored instance is the first's. Without the lock the second
reads no row (the first's insert is not yet visible) and both are answered.
