# sqlc

FlowCatalyst's data layer goes through [sqlc](https://github.com/sqlc-dev/sqlc) — query-first, compile-time-checked SQL. SQL strings live in `.sql` files; sqlc reads the schema (our embedded migrations) and generates strongly-typed Go functions. Schema or column-name typos fail at codegen time instead of at runtime against the wrong table.

This is the equivalent of what Rust's sqlx gives the upstream crate. The "queries match Rust word-for-word" parity property is preserved: each `.sql` file's text is the same SQL the Rust source emits.

## Layout

```
sqlc.yaml                          ← config (engine, paths, type overrides)
internal/sqlc/queries/             ← *.sql files, one per aggregate
internal/sqlc/dbq/                 ← generated code (DO NOT EDIT)
internal/migrate/sql/              ← schema source (the migration set)
```

`internal/sqlc/dbq` is checked in. CI runs `make sqlc-verify` to confirm it's in sync with the queries; the build fails if not.

## Generation workflow

```bash
make sqlc           # regenerate after editing any .sql or schema file
make sqlc-verify    # run in CI; fails if generated code is stale
```

`make sqlc` will `go install` the sqlc binary first if it's not on `PATH`.

## Adding a new query

1. Find (or create) the right `.sql` file in `internal/sqlc/queries/`. One file per aggregate. Each query has a comment directive:
   ```sql
   -- name: ClientFindByID :one
   SELECT id, name, identifier, ...
   FROM tnt_clients
   WHERE id = $1;
   ```
2. Use `@name` for explicit parameter names rather than `$1` placeholders when the inferred name isn't clear:
   ```sql
   WHERE name ILIKE @pattern OR identifier ILIKE @pattern
   ```
3. Run `make sqlc`. The new method appears as `(*dbq.Queries).ClientFindByID(ctx, id)`.
4. Wire it into the repository wrapper (see "Repository wrapper pattern" below).

### Query directives

| Directive | Generated signature |
|-----------|---------------------|
| `:one`    | `(Row, error)` — exactly one row expected; `pgx.ErrNoRows` on empty |
| `:many`   | `([]Row, error)` |
| `:exec`   | `error` — used for INSERT / UPDATE / DELETE |
| `:execrows` | `(int64, error)` — exec + affected row count |

Query method names should be prefixed with the aggregate (`Client...`, `Role...`, `Principal...`) so the single `*dbq.Queries` interface stays unambiguous.

## Repository wrapper pattern

`*dbq.Queries` exposes the SQL surface but emits sqlc-generated row types (e.g. `TntClient`), not the aggregate's entity (`client.Client`). The repository wrapper in `internal/platform/<aggregate>/repository.go` does three things:

1. Constructs `*dbq.Queries` from a `*pgxpool.Pool`.
2. Calls the generated method.
3. Projects the row onto the aggregate entity.

Compact template (matches `internal/platform/client/repository.go`):

```go
type Repository struct{ q *dbq.Queries }

func NewRepository(pool *pgxpool.Pool) *Repository {
    return &Repository{q: dbq.New(pool)}
}

func (r *Repository) FindByID(ctx context.Context, id string) (*Client, error) {
    row, err := r.q.ClientFindByID(ctx, id)
    if errors.Is(err, pgx.ErrNoRows) {
        return nil, nil
    }
    if err != nil {
        return nil, fmt.Errorf("client repo: %w", err)
    }
    return rowToClient(row)
}

func (r *Repository) Persist(ctx context.Context, c *Client, tx *usecasepgx.DbTx) error {
    return r.q.WithTx(tx.Inner()).ClientUpsert(ctx, dbq.ClientUpsertParams{
        ID: c.ID,
        // ...
    })
}
```

`*dbq.Queries.WithTx(pgx.Tx)` binds the queryer to a transaction. Pass it the `pgx.Tx` you get from `tx.Inner()` (the UoW seal token). The transactional + non-transactional methods share the same generated code — only the underlying connection differs.

## Type overrides

Default sqlc Postgres → Go mappings are pgtype-heavy. `sqlc.yaml` overrides them to the types the rest of the codebase already uses:

| Postgres | Go (override) |
|----------|---------------|
| `TIMESTAMPTZ NOT NULL` | `time.Time` |
| `TIMESTAMPTZ NULL` | `*time.Time` |
| `JSONB` | `json.RawMessage` (callers control encoding) |
| `VARCHAR` / `TEXT` | `string` (default) |
| nullable columns | `*<type>` (e.g. `*string`) |

Add column-level overrides in `sqlc.yaml` if a specific JSONB column should map to a typed Go value (e.g. `[]string` for an `additional_client_ids` column). See the existing overrides in `sqlc.yaml` for the format.

## Dynamic queries (the WHERE-clause builder problem)

sqlc generates one Go function per `.sql` query. It does **not** handle dynamic WHERE clauses well — `WHERE foo = $1 AND (bar = $2 OR ...)` with optional fragments needs either:

**Option A — optional-null pattern (sqlc-native):**
```sql
-- name: PrincipalSearch :many
SELECT ... FROM iam_principals
WHERE ($1::text IS NULL OR scope = $1::text)
  AND ($2::text IS NULL OR type = $2::text)
  AND ($3::bool IS NULL OR active = $3::bool);
```
Works with sqlc out of the box — params become nullable. The cost is that Postgres has to evaluate every predicate even when it's a no-op; on indexed columns this is usually fine, but very wide searches can lose index selectivity.

**Option B — hand-built `strings.Builder`:**
For complex search endpoints with 5+ optional filters, search-text + tenant scoping + pagination, write the SQL by hand in `repository.go` (alongside the sqlc wrapper) using a parameter accumulator. This is what `internal/platform/application/repository.go::FindWithFilters` already does. The trade-off: no compile-time schema check on those specific queries — same risk class as before sqlc — but the rest of the repo's SQL is still checked.

**Which one to use:** start with A; drop to B only when the static `IS NULL OR ...` pattern measurably hurts a hot path. For per-tenant search endpoints with a half-dozen optional filters, A is usually fine.

## Migration: porting an existing repository to sqlc

The mechanical steps that worked for the client repo (recorded so the rest of the bulk migration is consistent):

1. **Read the current `repository.go`.** Note the SQL strings, table joins, JSONB columns, and any dynamic WHERE clauses.
2. **Create `internal/sqlc/queries/<aggregate>.sql`.** Copy each SQL statement under a `-- name:` directive. Rename the `$N` placeholders to `@name` parameters where the inferred name isn't obvious.
3. **Run `make sqlc`.** Confirm the generated file appears in `internal/sqlc/dbq/`.
4. **Rewrite `repository.go`.** Replace the hand-rolled pgx calls with `r.q.<Method>(ctx, ...)`, project the row type onto the aggregate entity in a small `rowToEntity` helper.
5. **`go build ./...`** — the sqlc Querier interface is strict; if you missed a method or got a param type wrong, you'll know.
6. **Boot fc-dev + smoke the relevant endpoints.** This is where sink-side bugs (event ID size, column-name mismatches, ON CONFLICT against composite unique indexes) tend to surface for the first time.

## What sqlc does NOT solve

- **The platformsink's `INSERT INTO msg_events` + `INSERT INTO aud_logs`** are hand-rolled in `internal/platform/shared/platformsink/sink.go`. They could be migrated, but they only run as part of `usecasepgx.Commit` and have no row-projection concern. Low priority.
- **Migrations themselves** stay raw SQL — sqlc only reads them as schema, doesn't manage them.
- **Cross-DB portability.** Generated code is Postgres-specific (uses `pgx/v5`). Consumer-app SDKs that target sqlite/mysql/mongo continue to use their own outbox repository implementations.
