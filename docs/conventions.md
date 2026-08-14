# Conventions

Engineering conventions for this codebase.

Rules numbered here are non-negotiable. Breaking one is a code review block, not a style preference.

---

## 1. HTTP tier convention

Exactly three programmable tiers.

- **`/bff/*`** — frontend-only. Cookie/session auth. Response shapes tuned to screens; not for external SDKs.
- **`/api/*`** — public, programmable. Bearer token auth. Authorization enforced by **permission checks inside handlers**, not by URL prefix.
- **`/auth/*`, `/oauth/*`, `/.well-known/*`, `/api/dispatch/*`, `/api/monitoring/*`, `/api/me/*`, `/api/public/*`** — platform-owned. Do not move.

**Every write handler under `/api/*` MUST call exactly one authorization check.** The URL prefix is not a second line of defense (no `/api/admin/*`). A missing permission call on a write handler is a privilege-escalation bug.

Permission check naming, in `internal/platform/shared/auth/checks.go`:

```go
RequireAnchor(ctx)              // anchor scope only
IsAdmin(ctx)                    // anchor scope or ADMIN_ALL
CanReadEventTypes(ctx)          // GET event types
CanReadEventsRaw(ctx)           // GET event payloads (sensitive)
CanCreateEventTypes(ctx)
CanUpdateEventTypes(ctx)
CanDeleteEventTypes(ctx)
CanWriteEventTypes(ctx)         // any of create/update/delete
// ... etc, one set per resource
```

Conventions:
- `CanRead<Resource>` — GET (list, get-by-id, filters)
- `CanRead<Resource>Raw` — GET for sensitive payloads
- `CanCreate<Resource>` — POST creating one entity
- `CanUpdate<Resource>` — PUT/PATCH
- `CanDelete<Resource>` — DELETE
- `CanWrite<Resource>` — any of create/update/delete (checks for *any* of the three)
- `RequireAnchor` — anchor-only endpoints
- `IsAdmin` — full admin access

Don't invent new names — follow the existing checks in `checks.go`.

---

## 2. The UoW invariant

Every aggregate write goes through a use case, which ends in `uow.Commit(...)`. **Compile-time enforced via the seal pattern** — see [`usecase-pattern.md`](./usecase-pattern.md) for the type machinery.

What this means for every `*UseCase.Execute`:
1. The happy path returns the result of `uow.Commit(...)`, `uow.CommitDelete(...)`, `uow.EmitEvent(...)`, or `uow.CommitAll(...)`.
2. The only other legal tail is `usecase.Failure(err)`.
3. You cannot construct a success in any other way. The compiler rejects it.

**Layering for writes:**

| Layer | Lives in | Knows about | Does NOT know about |
|---|---|---|---|
| Handler | `internal/platform/<name>/api.go`, `internal/platform/shared/*_api.go` | HTTP types (huma/chi), DTOs, permission checks | SQL, transactions, pgx types |
| Use case | `internal/platform/<name>/operations/*.go` | Domain entities, repositories (as interfaces), `UnitOfWork`, domain events | HTTP, SQL strings, transaction types |
| Domain | `internal/platform/<name>/entity.go`, `operations/events.go` | Plain data, invariants, factory/behavior methods | `pgx`, transaction types, any driver |
| Repository | `internal/platform/<name>/repository.go` | SQL, pgx types, row structs, transaction handles | HTTP, permission checks, domain events |

**Aggregates do not persist themselves.** `Persist[Principal]` is implemented on `PrincipalRepository`, not on `Principal`.

**One write path per aggregate.** Every aggregate has exactly one place its rows are written: its repository's `Persist` and `Delete` methods. No handler, use case, or service writes to that aggregate's tables directly.

---

## 3. Exceptions: infrastructure-processing paths

These bypass UseCase/UnitOfWork because wrapping them would emit recursive domain events. **The list is closed.** Adding anything to it requires a design discussion.

- **Event ingest**: `POST /api/events/batch` — stores events received from consumer apps.
- **Dispatch job ingest**: `POST /api/dispatch-jobs/batch`.
- **Stream processing**: `events_raw` projection into `msg_events`.
- **Dispatch job delivery lifecycle**: status transitions during webhook delivery (pending → in_progress → success/failed), attempt recording.
- **Outbox processing**: polling `outbox_messages` and forwarding to platform API.
- **Auth/OIDC token storage**: refresh tokens, authorization codes, OIDC pending-auth state, login state. Login/logout *outcomes* (`UserLoggedIn`, `UserLoggedOut`) DO go through UoW; only the token plumbing bypasses.
- **Built-in role seeding**: startup-time hydration via `internal/platform/shared/seed/`.
- **Scheduled-job firings**: every cron tick writes to `msg_scheduled_job_instances` directly. SDK callbacks (`POST /api/scheduled-jobs/instances/:id/log`, `.../complete`) write to `msg_scheduled_job_instance_logs` directly. *Definitions* (create/update/pause/resume/archive/delete/sync) DO go through UoW.

These go directly to the repository. They are the platform's internal plumbing.

---

## 4. Database access rules

### 4.1 No N+1 queries

**Banned:**

```go
// Bad: N queries
for _, item := range items {
    item.Children, _ = repo.LoadChildren(ctx, item.ID)
}
```

**Required:**

```go
ids := make([]string, len(items))
for i, it := range items { ids[i] = it.ID }

rows, err := tx.Query(ctx, `SELECT * FROM children WHERE parent_id = ANY($1)`, ids)
// scan into []ChildRow, group by parent_id in memory using map[string][]Child
```

For inserts, use `UNNEST`, not loops:

```go
// Bad: N inserts in a loop
// Good:
_, err := tx.Exec(ctx,
    `INSERT INTO t (a, b) SELECT * FROM UNNEST($1::text[], $2::text[])`,
    aValues, bValues)
```

### 4.2 Concurrent independent queries use `errgroup`

```go
var g errgroup.Group
var clients []Client
var events []Event
var pools []DispatchPool

g.Go(func() error { var err error; clients, err = repo.FindClients(ctx); return err })
g.Go(func() error { var err error; events, err = repo.FindEvents(ctx); return err })
g.Go(func() error { var err error; pools, err = repo.FindPools(ctx); return err })

if err := g.Wait(); err != nil { return err }
```

### 4.3 Prefer `pgx.RowToStructByName` + `errors.Is(err, pgx.ErrNoRows)`

`QueryRow().Scan()` silently zeroes when no row exists. Banned unless the query is mathematically guaranteed to return a row (`SELECT COUNT(*)`, `SELECT EXISTS(...)`).

```go
// Bad: returns ErrNoRows but it's easy to ignore
row := pool.QueryRow(ctx, `SELECT id FROM foo WHERE bar = $1`, bar)
var id int64
_ = row.Scan(&id) // silently zero!

// Good:
rows, _ := pool.Query(ctx, `SELECT id FROM foo WHERE bar = $1`, bar)
res, err := pgx.CollectOneRow(rows, pgx.RowTo[int64])
if errors.Is(err, pgx.ErrNoRows) { /* handle missing */ }
if err != nil { return err }
// use res
```

The only acceptable use of `QueryRow().Scan()` is on aggregate queries that always return one row.

### 4.4 Shallow queries for list/filter endpoints

If a handler only needs id + name (e.g., a dropdown), don't load junction tables. Add a `FindXShallow()` method that skips hydration.

---

## 5. Caching

- **Token validation**: `AuthService` caches validated JWT claims, 30s TTL.
- **Permission resolution**: `AuthorizationService` caches role → permissions, 60s TTL.

Use `github.com/dgraph-io/ristretto` or a hand-rolled `sync.Map` with TTL. The caches exist to avoid repeated RSA verification and DB queries on every authenticated request.

---

## 6. Static asset serving

Vite hashed assets (`/assets/*`) served with `Cache-Control: public, max-age=31536000, immutable`. Non-hashed files (index.html) use default caching with SPA fallback.

Implementation: embed the frontend `dist/` via `//go:embed`, serve through a handler that sets the right headers per path.

---

## 7. Frontend API response handling

**Not the server's concern, but flagging because the contract matters.** Most PUT/PATCH return `204 No Content` — no body. The frontend wrapper is typed `Promise<void>` and refetches after a mutation. Don't change response shapes for PUT/PATCH; where the contract is 204, keep returning 204.

---

## 8. Error handling

- Use stdlib `errors` with `errors.Is` / `errors.As`. No `pkg/errors`.
- Domain errors are typed structs implementing `error`.
- The HTTP error envelope shape is part of the wire contract — see [`wire-contract.md`](./wire-contract.md#error-envelope).
- `UseCaseError` distinguishes `Validation`, `BusinessRule`, `Authorization`, `NotFound`, `Conflict`. Each maps to a specific HTTP status code via a single function in `internal/platform/shared/httperror/`.

---

## 9. Naming

- Go packages: lowercase, no underscores. `eventtype` not `event_type`.
- Files: `entity.go`, `repository.go`, `api.go`, `operations/create.go` — the same structure within each subdomain.
- JSON field names: `camelCase`. Use `json:"foo,omitempty"` where the contract omits the field when unset.
- Enum values: `SCREAMING_SNAKE_CASE` strings (e.g., `"CURRENT"`, `"ARCHIVED"`). Represented as Go `type EventTypeStatus string` with constants.
- Database column names: `snake_case` (unchanged from existing schema).
- Domain event types: format `platform:<domain>:<aggregate>:<verb>` (e.g., `platform:admin:eventtype:created`).

---

## 10. Comments

- Default to no comments.
- Add one only when *why* is non-obvious (hidden constraint, subtle invariant, workaround for a specific bug).
- Don't explain *what* the code does — well-named identifiers do that.
- Never reference the current task/fix/PR ("added for X", "fixes #123") — that belongs in the commit message.

---

## 11. Testing

- **No database mocks.** Use `testcontainers-go` for real Postgres. Reason: prior incidents where mocks passed but migrations failed.
- **One integration test per use case happy path + each documented error case.**
- **Contract tests** live in `tests/parity/` and run in CI for every PR — they ensure no change accidentally breaks the public API.
- **Golden tests** for JSON marshaling of every public DTO. Captures field ordering, omission posture, enum casing. Stored in `tests/golden/*.json`.
- Unit tests for pure functions (validators, parsers, signature canonicalization) — use plain stdlib `testing`. No need for fixtures.
- `t.Parallel()` everywhere unless the test mutates global state.

---

## 12. Concurrency

- One goroutine per logical worker. Don't spawn a goroutine per message — pool drains share a goroutine, message groups share the pool drain.
- Context propagation: every function that crosses a goroutine boundary takes a `context.Context` as its first parameter. Cancellation propagates.
- No `sync.Map` for low-cardinality maps with mixed read/write — use `sync.RWMutex` + plain map. `sync.Map` is for high-read, low-write or per-key-isolated workloads.
- `chan struct{}` for signals, `chan T` for data, `chan error` for fan-in error returns.
- `select` with `<-ctx.Done()` in every loop that could otherwise block forever.

---

## 13. Migrations

Migrations are plain SQL files embedded into every binary (`internal/migrate/sql/`), applied at boot by the migration runner. Only one binary should apply migrations against a given database at a time.

---

## 14. The wire contract trumps everything

If a Go-idiomatic change would alter an HTTP response shape, an OpenAPI definition, a JSON field name, or a database row layout — **don't make it**. API redesign is a separate, versioned effort.

Exceptions get a written sign-off from whoever owns the affected consumer (the frontend lead for `/bff/*`, the SDK maintainer for `/api/*`).
