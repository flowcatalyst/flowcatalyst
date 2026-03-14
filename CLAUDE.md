# FlowCatalyst Rust - Development Guidelines

## Database Access Rules

### N+1 Query Prevention
Never call a query inside a loop. This is the #1 performance issue in this codebase.

**Banned pattern:**
```rust
for item in items {
    item.children = self.load_children(&item.id).await?; // N queries!
}
```

**Required pattern — batch load with IN clause:**
```rust
let ids: Vec<&str> = items.iter().map(|i| i.id.as_str()).collect();
let all_children = sqlx::query_as::<_, ChildRow>(
    "SELECT * FROM children WHERE parent_id = ANY($1)"
)
.bind(&ids)
.fetch_all(&self.pool).await?;

// Group by parent_id in memory
let mut map: HashMap<String, Vec<Child>> = HashMap::new();
for c in all_children {
    map.entry(c.parent_id.clone()).or_default().push(c.into());
}
```

**For inserts — use UNNEST, not loops:**
```rust
// Bad: N inserts
for item in items { sqlx::query("INSERT...").bind(&item).execute(&pool).await?; }

// Good: 1 insert
sqlx::query("INSERT INTO t (a, b) SELECT * FROM UNNEST($1::text[], $2::text[])")
    .bind(&a_values).bind(&b_values).execute(&pool).await?;
```

### Concurrent Independent Queries
When a handler needs data from multiple tables, use `tokio::try_join!` instead of sequential awaits:
```rust
let (clients, events, pools) = tokio::try_join!(
    repo.find_clients(),
    repo.find_events(),
    repo.find_pools(),
)?;
```

### Shallow Queries for Filter/List Endpoints
If a handler only needs a few fields (e.g., id + name for a dropdown), don't load junction tables or child entities. Add a `find_*_shallow()` method that skips hydration.

## SQLx Migration (In Progress)
We are migrating from SeaORM to raw SQLx. New repositories should use `sqlx::PgPool` with handwritten SQL. Pattern:
- Row structs: `#[derive(sqlx::FromRow)]` in the repository file
- Queries: `sqlx::query_as::<_, FooRow>("SELECT ...")` — visible SQL, no ORM magic
- Domain entities stay in `*/entity.rs`, row mapping stays in `*/repository.rs`
- Connection: use `shared::database::create_pool()` for SQLx repos

## Caching
- **Token validation**: `AuthService` caches validated JWT claims (DashMap, 30s TTL)
- **Permission resolution**: `AuthorizationService` caches role→permissions (DashMap, 60s TTL)
- Both caches exist to avoid repeated RSA verification and DB queries on every authenticated request

## Static Asset Serving
Vite hashed assets (`/assets/*`) are served with `Cache-Control: public, max-age=31536000, immutable`. Non-hashed files (index.html) use default caching with SPA fallback.

## Use Case / Operations Pattern

### UseCase Trait Contract
Every write operation MUST implement the `UseCase` trait, which enforces three steps:
1. **`validate`** — Input validation (field presence, format, length). Return `Ok(())` if none needed.
2. **`authorize`** — Resource-level authorization (ownership, access checks). Return `Ok(())` if none needed.
3. **`execute`** — Business logic: load aggregate, check business rules, build domain event, call `unit_of_work.commit()`.

Handlers call `use_case.run(command, ctx)` which executes validate → authorize → execute in order.

### No Direct DB Writes Outside Operations
All write operations (create, update, delete, state transitions) MUST go through a use case in `*/operations/`.
Handlers (BFF, SDK, admin API) are thin adapters that:
1. Check permissions (role/permission-level authorization)
2. Build a Command from the request DTO
3. Create an `ExecutionContext::from_auth(&auth.0)`
4. Call `use_case.run(command, ctx).await.into_result()?`
5. Convert the result to an HTTP response

**Never call `repo.insert()`, `repo.update()`, or `repo.delete()` directly from a handler.**
The use case layer ensures: validation, authorization, domain events, audit logs, and atomic commits via UnitOfWork.

### Exceptions: Platform Infrastructure Processing
The **only** operations that bypass UseCase/UnitOfWork are the platform's own internal
infrastructure — the machinery that moves messages through the pipeline. These cannot
generate events/audit logs (that would be recursive):

- **Event ingest**: `POST /api/sdk/events/batch` — stores events received from consumer apps
- **Dispatch job ingest**: `POST /api/sdk/dispatch-jobs/batch` — stores dispatch jobs from consumer apps
- **Stream processing**: `events_raw` CQRS projection into `msg_events`
- **Dispatch job delivery lifecycle**: status transitions during webhook delivery (pending → in_progress → success/failed), attempt recording
- **Outbox processing**: polling `outbox_messages` and forwarding to platform API

These go directly to the repository. They are the platform's internal plumbing.

**Everything else goes through UseCase with domain events + audit logs:**
- All control plane CRUD: Event Types, Subscriptions, Connections, Dispatch Pools, Clients, Principals, Roles, Applications, Service Accounts, Identity Providers, Email Domain Mappings, CORS Origins, Auth Configs
- Human-initiated dispatch job actions: resend, ignore, cancel
- Sync operations (emit a summary event, e.g., `EventTypesSynced`)
- Consumer app operations via SDK (e.g., `ShipOrder`, `CancelOrder`)

### Events vs Audit Logs
Both are generated from the same `UnitOfWork.commit()` call. They are two views of the same fact:
- **Domain Events** — "what happened", consumed by other systems (subscriptions, webhooks). Can be purged after delivery/TTL.
- **Audit Logs** — "who did what, when", consumed by humans (admin UI, compliance). Retained long-term.

All UseCase operations emit both. The UnitOfWork handles this automatically.

### Reads Are Fine in Handlers
Read operations (list, get, filter) can call repositories directly from handlers.
Only writes need the use case layer.
