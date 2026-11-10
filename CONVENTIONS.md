# Conventions

This file is the contract for adding code to `flowcatalyst-go`. It exists so
business developers can extend the platform without having to reverse-engineer
patterns from existing code, and so that existing code doesn't drift into 17
different ways of doing the same thing.

If you're about to write a new aggregate, a new use case, or a new HTTP
endpoint, read this file first. Everything here is enforced by code review,
not by the compiler — please don't invent a new shape because it feels nicer
in the moment.

The committed OpenAPI lockfile (`api/openapi.lock.json`) is the canonical wire
spec for HTTP shapes, signing, and TSID format — see docs/wire-contract.md.
When in doubt about a request/response field name, the lockfile wins.

---

## 1. Stack — what's already chosen

These choices are made. Don't re-litigate them per-PR.

| Concern              | Choice                                                   | Rationale                                                                 |
|----------------------|----------------------------------------------------------|---------------------------------------------------------------------------|
| HTTP router          | `github.com/go-chi/chi/v5`                               | stdlib-shaped; one obvious way to wire routes.                            |
| Postgres driver      | `github.com/jackc/pgx/v5` + `pgxpool`                    | typed protocol, no `database/sql` overhead.                               |
| Query generation     | `sqlc` (config: `sqlc.yaml`)                             | generated typed queries in `internal/sqlc/dbq/`. Never write raw SQL in repos. |
| Migrations           | `pressly/goose/v3` against `internal/migrate/sql/NNN_*.sql` | each file starts with `-- +goose Up`. Forward-only. Embedded into binaries. |
| DI                   | plain constructor functions returning `*T`               | no wire, no fx, no DI container.                                          |
| Transactions / UoW   | `pkg/fcsdk/usecasepgx.UnitOfWork`                        | use-case envelope (`usecaseop`) — see §3.                                           |
| Errors (domain)      | `pkg/fcsdk/usecase.Error` with a `Kind`                  | typed; maps to HTTP status via `httperror.Write`.                         |
| Errors (HTTP)        | `internal/platform/shared/httperror`                     | constructors return `*usecase.Error` shaped for the wire envelope.        |
| Logging              | stdlib `log/slog`                                        | structured; no `logrus`/`zap`/`zerolog`.                                  |
| AuthN context        | `internal/platform/shared/auth`                          | `auth.FromContext(ctx)` in handlers.                                      |
| ID generation        | `pkg/fcsdk/tsid`                                         | Crockford Base32 TSID; same wire format across all four SDKs.             |
| Time                 | `time.Now().UTC()`                                       | UTC everywhere; never `time.Now()` without `.UTC()`.                      |
| Test framework       | stdlib `testing` + `github.com/stretchr/testify/{assert,require}` | `require` for fatal, `assert` for non-fatal.                              |

**Do not** introduce a competing library for any row above. If you think one is
needed, open an issue first.

---

## 2. Layout of an aggregate

Every aggregate (Application, EventType, Subscription, etc.) follows this exact
layout under `internal/platform/<aggregate>/`:

```
internal/platform/<aggregate>/
├── entity.go         ← the in-memory type + invariants (no I/O)
├── entity_test.go    ← unit tests of the entity
├── repository.go     ← Postgres-backed CRUD; implements usecase.Persist[T]
├── api/
│   └── api.go        ← HTTP layer: State, RegisterRoutes, handlers
└── operations/
    ├── events.go     ← domain event types (one file for all of them)
    ├── create.go     ← one file per operation
    ├── update.go
    ├── delete.go
    └── activate.go   ← etc.
```

**Hard rules:**
- **One operation per file.** `create.go` contains the create command DTO, use case
  type, constructor, `Validate`, `Authorize`, `Execute`. Don't bundle
  multiple operations.
- **One event types file.** All domain events for the aggregate live in
  `operations/events.go` so they're easy to scan together.
- **No business logic in `api/api.go`.** Handlers do: read auth → decode body
  → build execution context → call the operation via `usecaseop.Run` → write
  response. That's it.
- **No SQL outside `repository.go`.** Repos use the sqlc-generated `dbq`
  package; never construct SQL strings in `operations/`.

The eventtype aggregate at `internal/platform/eventtype/` is the canonical
example. Copy its shape when adding a new aggregate.

---

## 3. The use-case envelope (`usecaseop`) — read this before writing a use case

Platform use cases must guarantee that every successful domain event is
committed atomically with its aggregate state, and that validation and
authorization always run. The envelope is `pkg/fcsdk/usecaseop`:

- A business operation is a `usecaseop.Operation[C, E]` value with named
  phases: `Validate` (command shape, pure), `Authorize` (resource-level access,
  required) and `Execute` (invariant checks; returns a `Plan[E]`).
- `usecaseop.Run(ctx, uow, op, cmd, ec)` runs the phases in that order, then
  applies the plan: aggregate row, domain event, the plan's dispatch jobs and
  the audit log, in one transaction.
- `Plan` is sealed (its only method is unexported). The sole constructors are
  `usecaseop.Save` / `Delete` / `Emit` / `SaveAll` / `Sync`, and the sole
  consumer is `Run`, so there is no path to a persisted aggregate that skips
  the event and audit rows.
- `Authorize` must be set. An operation that is intentionally open sets it to
  `usecaseop.Public`, a visible and greppable decision. A literal that omits
  `Authorize` fails closed at runtime (`USECASE_MISCONFIGURED`) and is flagged
  at build time by the `uowseal` analyzer (`tools/analyzer/uowseal`, run by
  `make analyze`).
- `usecaseop.TxOperation` is the multi-aggregate sibling, for one logical
  command that genuinely spans several aggregates.

The older `commit.Save` / `Committed[E]` / `internal/usecase` / `uow.Commit`
shapes no longer exist; do not look for them.

### The contract

An operation is a constructor that closes over its repositories:

```go
func VerbAggregate(repo *aggregate.Repository) usecaseop.Operation[VerbCommand, VerbEvent] {
    return usecaseop.Operation[VerbCommand, VerbEvent]{
        Name:      "VerbAggregate",
        Validate:  func(ctx context.Context, cmd VerbCommand) error { ... },
        Authorize: func(ctx context.Context, cmd VerbCommand) error { ... },
        Execute: func(ctx context.Context, cmd VerbCommand, ec usecase.ExecutionContext) (usecaseop.Plan[VerbEvent], error) {
            // find the entity, check invariants, mutate, build the event
            return usecaseop.Save(entity, repo, event), nil
        },
    }
}
```

1. **Validate** the shape (`return usecase.Validation(...)`).
2. **Authorize** with the resource-level check (the handler does the coarse
   permission check).
3. **Execute**: find the entity, check business invariants (`usecase.NotFound`,
   `usecase.Conflict`, `usecase.BusinessRule`), mutate it and build the event.
4. Return a `Plan` from `usecaseop.Save` / `Delete` / `SaveAll` / `Emit` /
   `Sync`. `Run` does the commit.

### Worked example — see `internal/platform/eventtype/operations/create.go`

`CreateEventType(repo)` validates the four-part code, authorizes with
`auth.CheckScopeAccess`, rejects a duplicate code with `usecase.Conflict`,
builds the entity and an `EventTypeCreated` event, and returns
`usecaseop.Save(et, repo, event)`.

### When to use which plan constructor

| Constructor            | Use when                                                            |
|------------------------|---------------------------------------------------------------------|
| `usecaseop.Save`       | One aggregate, one event. The 90% case.                             |
| `usecaseop.Delete`     | Deleting an aggregate. Repo deletes; event records the deletion.    |
| `usecaseop.SaveAll`    | Many aggregates of one type, one summary event.                     |
| `usecaseop.Sync`       | Per-row saves and deletes plus a rollup event (bulk-sync endpoints). |
| `usecaseop.Emit`       | Event with no aggregate state change (e.g. login, fire-now).        |

`Plan.Dispatch(jobs...)` adds dispatch jobs to any plan.

### Consumer-app SDK

`pkg/fcsdk/usecaseop` is the same envelope consumer apps use through the Go
SDK; platform code and consumer code share it. See `pkg/fcsdk/README.md`.

---

## 4. The HTTP handler pattern

Every handler is the same five steps. Copy this shape.

```go
func (s *State) create(w http.ResponseWriter, r *http.Request) {
    // 1. Authn context.
    ac := auth.FromContext(r.Context())
    if err := auth.CanWriteApplications(ac); err != nil {
        httperror.Write(w, err)
        return
    }

    // 2. Decode the command DTO.
    var body operations.CreateCommand
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
        return
    }

    // 3. Build the execution context.
    ec := usecase.NewExecutionContext(ac.PrincipalID)

    // 4. Run the operation. Returns (E, error): the committed event.
    event, err := usecaseop.Run(r.Context(), s.UoW, operations.CreateApplication(s.Repo), body, ec)
    if err != nil {
        httperror.Write(w, err)
        return
    }

    // 5. Write the response. Use apicommon constructors for the standard
    //    envelopes so frontend can rely on the shape.
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated)
    _ = json.NewEncoder(w).Encode(apicommon.CreatedResponse{ID: event.ApplicationID})
}
```

The `State` struct holds only `{Repo, UoW *usecasepgx.UnitOfWork}` (plus
any extra repos a particular aggregate needs for cross-aggregate
validation). No per-operation pointers.

**Do not** put business logic in the handler. If you're tempted, that's a
use case.

**Do not** call `httperror.Write` with a custom error. Use the constructors
in `internal/platform/shared/httperror/` (`NotFound`, `BadRequest`,
`Forbidden`, etc.). They produce the wire-format envelope the frontend
expects.

---

## 5. Repositories

Repos live in the aggregate's package root (e.g. `eventtype/repository.go`)
and follow a fixed shape:

```go
type Repository struct {
    pool *pgxpool.Pool
    q    *dbq.Queries  // sqlc-generated
}

func NewRepository(pool *pgxpool.Pool) *Repository { ... }

// CRUD. Use sqlc-generated queries — no SQL strings in this file.
func (r *Repository) FindByID(ctx context.Context, id string) (*T, error) { ... }
func (r *Repository) FindByCode(ctx context.Context, code string) (*T, error) { ... }

// Persist implements usecase.Persist[T]. UnitOfWork calls this inside
// the committing transaction. The DbTx wraps a pgx.Tx the SDK owns.
func (r *Repository) Persist(ctx context.Context, e *T, tx *usecasepgx.DbTx) error { ... }

// Delete (only if the aggregate supports it).
func (r *Repository) Delete(ctx context.Context, id string, tx *usecasepgx.DbTx) error { ... }
```

**Rules:**
- `Find*` returns `(nil, nil)` on "not found" (not an error). The use case
  decides whether absence is an error.
- `Persist` is the only method called inside a transaction; everything else
  takes its own connection from the pool.
- Sqlc-generated code goes in `internal/sqlc/dbq/`. Re-run `sqlc generate`
  after editing `internal/sqlc/queries/*.sql`.

---

## 5a. OpenAPI spec — committed lockfile + CI gate

The platform's OpenAPI spec is derived from huma's per-route registrations
in `internal/platform/<aggregate>/api/`. The committed contract is
`api/openapi.lock.json` — the parity-spec CI job fails on any drift
between the live spec and this file.

**When you intentionally change the wire shape** (add/remove a field,
change a status code, rename an operation):

1. Run `make api-bump` to regenerate `api/openapi.lock.json`.
2. Commit the diff in the same PR as the code change.
3. The PR review should see the spec diff alongside the code diff —
   that's the human gate on wire-format changes.

**When you do NOT intend to change the wire shape:**

The `TestOpenAPISpecLocked` snapshot test and the `make api-diff` CI step
both fail loudly if the live spec drifts from the lockfile. If you see
that failure unexpectedly, you've probably renamed a Go field or
changed a JSON tag — fix the code, not the lockfile.

**Adding a new aggregate to the spec:** register it in
`tools/dump-spec/main.go` alongside the existing aggregates. The
dump-spec binary builds the spec without touching the database, so it
runs in CI without infra.

---

## 5b. Typed entity ids

`internal/ids` gives `Principal`, `Client` and `Application` (and the OAuth
client's public `client_id`) their own string types, so passing a `ClientID`
where a `PrincipalID` belongs is a compile error. JSON, SQL and OpenAPI still
see plain strings.

- **sqlc:** typed columns are listed one by one in `sqlc.yaml` (never by
  wildcard). A nullable column needs its own `pointer: true` entry; sqlc column
  overrides take the first match and ignore `nullable:`. An unlisted column
  stays a string.
- **Stay strings:** `client_id` on the high-volume `msg_*` tables (set from
  unchecked SDK payloads), the audit log, the OAuth `client_id` columns, and
  anything that can hold the `"*"` wildcard (claims, the client-association
  command).
- **Conversions are the weak point.** `ids.ClientID("x")` always compiles.
  `make analyze` runs `tools/analyzer/idconv`, which rejects converting one id
  kind to another, converting another entity's `.ID` field (a portal identity's
  `ptu_` id must never become a `PrincipalID`), and any conversion outside
  `internal/platform`, `internal/server` and `cmd`. Mark a deliberate exception
  `//idconv:ok <reason>`.
- **Tests:** comparing a typed id with a string in `assert.Equal` compiles and
  then fails at runtime (different types); convert one side.

## 6. Migrations

- One numbered file per migration: `internal/migrate/sql/NNN_subject.sql` where
  `NNN` is a three-digit zero-padded sequence (`001_tenant_tables.sql`). This
  directory is the single canonical source — the embed.FS in the migrate
  package and sqlc both read from here.
- Every file starts with a `-- +goose Up` line. Production is forward-only
  (the runner only ever applies Up). `-- +goose Down` sections exist on most
  migrations from 041 onward, where they document and test the reversal
  (some migration tests exercise them); migrations 001-040 have none. Write a
  Down section when the reversal is simple and safe; never rely on it for a
  deploy.
- Migrations are append-only. **Never** edit a migration that has shipped.
  If you need to change a schema, write a new migration.
- Use `IF NOT EXISTS` / `IF EXISTS` so re-running is idempotent. Goose
  tracks applied versions in `goose_db_version`, but idempotency is still
  defensive against partial-apply scenarios and dev-DB rewinds.
- Don't mix DDL and DML in the same file unless the DML is bootstrap data
  with stable IDs.
- Don't reference tables introduced in later migrations.

If you need to rename a column or drop a constraint, write two migrations: one
to add the new shape, one (later, in a follow-up release) to drop the old
shape. Never break running services with a single migration.

The runner (`internal/migrate.Run`) handles the legacy `_fc_migrations`
tracker transparently: existing databases are upgraded to `goose_db_version`
on first boot without re-applying anything.

---

## 7. Errors

There's exactly one error type for domain failures: `*usecase.Error`. The
`Kind` field controls HTTP status mapping.

Build them with the helpers — never construct the struct manually:

```go
usecase.Validation(code, msg)      // → 400
usecase.Authorization(code, msg)   // → 403
usecase.NotFound(code, msg)        // → 404
usecase.Conflict(code, msg)        // → 409
usecase.BusinessRule(code, msg)    // → 409 (same status as Conflict; the code tells them apart)
usecase.Internal(code, msg, err)   // → 500; preserves wrapped err for logs

httperror.NotFound("Application", id)  // sugar for the common 404 case
httperror.BadRequest("INVALID_JSON", err.Error())
```

The `code` is a short SCREAMING_SNAKE string the frontend can switch on. The
`msg` is human-readable. Don't put dynamic data in the code; put it in the
message.

---

## 8. Logging

stdlib `log/slog` only. Structured key-value pairs, not formatted strings.

```go
slog.Info("event delivered", "msg_id", msg.ID, "target", msg.MediationTarget)
slog.Warn("ack failed", "msg_id", msg.ID, "err", err)
slog.Error("repo find_by_id failed", "id", id, "err", err)
```

Don't log inside use cases — logging is the handler's job for request errors,
and `slog.Error` at infra failure sites. Use cases return typed errors; the
handler decides whether to log.

---

## 9. Channels & goroutines

The trickiest comprehension barrier in Go. Conventions to keep it sane:

1. **Document channel ownership at the declaration site.** One line:
   `// Closed by X; receivers must not close.` See
   `internal/router/pool.go` for examples.
2. **Comment wakeup conditions on every select.** Each case gets a phrase
   like `// shutdown` or `// rate-limit budget restored`. See
   `internal/router/lifecycle.go`.
3. **Pick from this small library of patterns; don't invent new ones:**
   - Fan-out worker pool with ctx-cancel
   - `time.Ticker`-driven background loop
   - Buffered channel as a semaphore (`sem chan struct{}`)
   - `sync.WaitGroup` + `done` channel for "wait with timeout"
4. **`go.uber.org/goleak`** is allowed (and encouraged) in `TestMain` for
   tests that exercise concurrent code. Goroutine leaks become CI failures
   instead of paged-at-3am surprises.
5. **Every goroutine must have an exit path tied to `ctx.Done()`** or a
   close on its input channel. No fire-and-forget goroutines, ever.

---

## 10. Testing

- File next to the file under test: `repository.go` → `repository_test.go`.
- Package: same package (whitebox) when testing private helpers, `_test`
  package (blackbox) when verifying the public API.
- `testify/require` for setup assertions (test must abort if these fail).
  `testify/assert` for behaviour assertions.
- DB tests run on an embedded PostgreSQL (`internal/testpg`; files named
  `*_pg_test.go` carry the `integration` build tag, run with
  `make test-integration` or `go test -tags=integration`; a plain `go test`
  skips them silently). No docker-compose database is needed. Never mock pgx. Mock-pgx tests miss real-driver behaviour (e.g. tx scoping,
  `RETURNING` handling). The pain of a real DB in CI is worth it.
- Each test is self-contained: setup, action, assertions, teardown. No
  shared fixtures across tests.

---

## 11. Adding a new aggregate — checklist

Copy from `internal/platform/eventtype/` as the template.

- [ ] Migration: `migrations/NNN_<aggregate>_tables.sql`
- [ ] Sqlc queries: `internal/sqlc/queries/<aggregate>.sql`
- [ ] Run `sqlc generate` to refresh `internal/sqlc/dbq/`
- [ ] `internal/platform/<aggregate>/entity.go` — type + invariants
- [ ] `internal/platform/<aggregate>/entity_test.go`
- [ ] `internal/platform/<aggregate>/repository.go` — `FindBy*`, `Persist`,
      `Delete`
- [ ] `internal/platform/<aggregate>/operations/events.go` — event types
- [ ] One `operations/<op>.go` per command
- [ ] `internal/platform/<aggregate>/api/api.go` — `State`, `RegisterRoutes`,
      handlers
- [ ] Auth helpers in `internal/platform/shared/auth/` if new permission
      types are needed
- [ ] Wire into platform server entrypoint (currently
      `cmd/fc-platform-server/main.go` — see existing aggregate wiring once
      it lands)
- [ ] Tests pass: `go test ./internal/platform/<aggregate>/...`
- [ ] `go vet ./...` clean
- [ ] If this aggregate is exposed to consumer apps, add a typed accessor
      to `pkg/fcsdk/client/` and add a sync category to
      `pkg/fcsdk/sync/` if it should be declaratively reconcilable.

---

## 12. What NOT to do

These come up often enough to call out explicitly:

- **Don't try to build a `Plan` by hand.** `usecaseop.Plan` is sealed; only
  `Save` / `Delete` / `Emit` / `SaveAll` / `Sync` produce one, and only `Run`
  consumes it. If you find yourself working around that, the seal is being broken.
- **Don't construct domain events in handlers.** Events are built in
  `Execute` and committed atomically by `Run`. Handlers see them only as the
  value `usecaseop.Run` returns.
- **Don't use `database/sql`** anywhere on the platform side. pgx only.
  (`pkg/fcsdk/outboxsql` exists for SDK consumers who use `database/sql`
  for their own reasons; the platform itself uses pgx.)
- **Don't hand-edit the OpenAPI spec.** It is generated from the huma
  handlers and committed as `api/openapi.lock.json`; a handler change that
  moves the wire shape must regenerate the lock (see docs/wire-contract.md).
- **Don't introduce `panic` as flow control.** Even in dev code. `panic`
  is for invariants the type system can't express, used at most once or
  twice in this codebase.
- **Don't rename DTO fields without coordinating across the SDKs**
  (TypeScript, Laravel). Wire compatibility matters.
- **Don't skip pre-commit hooks** (`--no-verify`). If a hook fails, fix
  the cause.

---

## 13. When to ask vs. when to just do it

**Just do it:**
- Add a new use case to an existing aggregate following the create.go shape.
- Add a new field to a DTO that the wire spec already has.
- Refactor within a single file.
- Add tests.

**Ask first (open an issue or grab a reviewer in chat):**
- Introduce a new aggregate.
- Add a new top-level package outside `internal/platform/<aggregate>/`.
- Change the wire format of any DTO.
- Touch `internal/sealed`, `pkg/fcsdk/usecase`, or the `Commit*` functions.
- Add a new third-party dependency.
- Change a migration that's already been merged.

---

If something here feels wrong, raise it — but raise it as "I want to change the
convention" rather than working around it in a PR. The whole point of this file
is that the codebase stays coherent across the dozens of aggregates that will
eventually live in it.
