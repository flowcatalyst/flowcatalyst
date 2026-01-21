# FlowCatalyst CQRS Architecture

## Overview

FlowCatalyst implements strict Command Query Responsibility Segregation (CQRS). The write side and read side are completely separate, with domain events bridging the two.

```
┌──────────────────────────────────────────────────────────────────────────┐
│                              WRITE SIDE                                   │
│                                                                          │
│   Client ──► Command ──► UseCase ──► UnitOfWork.commit()                │
│                                            │                             │
│                                            ├──► Aggregate (MongoDB)      │
│                                            ├──► Domain Event (Outbox)    │
│                                            └──► Audit Log                │
│                                                                          │
│   ⛔ NO CRUD. All state changes through UseCases only.                   │
└──────────────────────────────────────────────────────────────────────────┘
                                     │
                                     │ Domain Events
                                     │ (fc-router)
                                     ▼
┌──────────────────────────────────────────────────────────────────────────┐
│                         PROJECTION PROCESSORS                             │
│                                                                          │
│   Event Subscription ──► Projector ──► Postgres (Read Models)           │
│                                                                          │
│   • One projector per read model                                         │
│   • Idempotent (safe to replay)                                         │
│   • Maintains checkpoint for exactly-once processing                     │
└──────────────────────────────────────────────────────────────────────────┘
                                     │
                                     ▼
┌──────────────────────────────────────────────────────────────────────────┐
│                              READ SIDE                                    │
│                                                                          │
│   Client ──► BFF Endpoint ──► ReadRepository ──► Postgres               │
│                    │                                                     │
│                    └──► Cache (Redis / moka)                            │
│                                                                          │
│   • Read-only queries                                                    │
│   • Optimized for UI consumption                                         │
│   • No business logic                                                    │
└──────────────────────────────────────────────────────────────────────────┘
```

---

## Write Side

### Principles

1. **UseCases Only** - Every state change goes through a UseCase
2. **No CRUD** - No direct create/update/delete on aggregates
3. **Events Always** - Every successful operation emits a domain event
4. **Atomic Commit** - Aggregate + Event + Audit in single transaction

### Flow

```
Command (DTO)
    │
    ▼
UseCase
    ├── Validate input
    ├── Load aggregate (if exists)
    ├── Apply business rules
    ├── Create/modify aggregate
    │
    ▼
UnitOfWork.commit(aggregate, event, command)
    ├── Insert/Update aggregate in MongoDB
    ├── Insert event in outbox collection
    ├── Insert audit log entry
    └── All in single MongoDB transaction
```

### Technology

| Component | Technology |
|-----------|------------|
| API | axum |
| Database | MongoDB |
| Patterns | UseCase, UnitOfWork, DomainEvent |
| Auth | JWT (fc-platform) |

### Why No CRUD

CRUD operations bypass business rules:

```rust
// ❌ WRONG - Direct CRUD
user.status = "active";
repo.update(&user).await?;
// No validation, no event, no audit

// ✅ CORRECT - Through UseCase
let result = activate_user_use_case
    .execute(ActivateUserCommand { user_id }, ctx)
    .await;
// Validates preconditions, emits UserActivated event, audit logged
```

The platform enforces this by making `UseCaseResult::success()` crate-private. Only `UnitOfWork` can create a successful result, forcing all operations through the transactional, event-emitting path.

---

## Domain Events

### Event Flow

```
UnitOfWork.commit()
    │
    ▼
MongoDB Outbox Collection
    │
    ▼
fc-outbox (Outbox Processor)
    │
    ▼
fc-router (Message Router)
    │
    ▼
Subscribers (Projectors, External Systems)
```

### Event Structure (CloudEvents)

```json
{
  "id": "0HZXEQ5Y8JY5Z",
  "type": "platform:iam:user:created",
  "source": "platform:iam",
  "specversion": "1.0",
  "subject": "platform.user.0HZXEQ5Y8JY5Z",
  "time": "2025-01-02T10:30:00Z",
  "datacontenttype": "application/json",
  "messagegroup": "platform:user:0HZXEQ5Y8JY5Z",
  "executionid": "0HZXEQ5Y8JY6A",
  "correlationid": "0HZXEQ5Y8JY6B",
  "causationid": "0HZXEQ5Y8JY6C",
  "principalid": "admin-user-id",
  "data": {
    "principalId": "0HZXEQ5Y8JY5Z",
    "email": "user@example.com",
    "emailDomain": "example.com",
    "name": "Test User",
    "scope": "CLIENT",
    "clientId": "client-123",
    "isAnchorUser": false
  }
}
```

### Ordering Guarantees

- `messagegroup` ensures events for the same aggregate are processed in order
- fc-router delivers events in order per message group
- Different message groups can be processed in parallel

---

## Projection Processors

### Purpose

Transform domain events into optimized read models stored in Postgres.

### Principles

1. **Single Responsibility** - One projector per read model
2. **Idempotent** - Same event processed twice produces same result
3. **Replayable** - Can rebuild read model from event history
4. **Checkpoint Tracking** - Tracks last processed event for exactly-once semantics

### Projector Structure

```
Projector
    ├── Subscribe to event types
    ├── For each event:
    │   ├── Check if already processed (idempotency)
    │   ├── Transform event data
    │   ├── Upsert into Postgres read model
    │   └── Update checkpoint
    └── Handle failures (retry, dead letter)
```

### Read Model Design

Read models are denormalized for query performance:

| Write Side (MongoDB) | Read Side (Postgres) |
|---------------------|----------------------|
| `principals` collection | `user_list_view` table |
| Normalized aggregate | Denormalized for UI |
| Business invariants | Query optimization |
| Single entity | Joined/flattened data |

### Example: User List Projection

**Events consumed:**
- `platform:iam:user:created`
- `platform:iam:user:updated`
- `platform:iam:user:activated`
- `platform:iam:user:deactivated`
- `platform:iam:user:deleted`
- `platform:iam:user:roles-assigned`

**Read model table:**

```sql
CREATE TABLE user_list_view (
    id              TEXT PRIMARY KEY,
    email           TEXT NOT NULL,
    email_domain    TEXT NOT NULL,
    name            TEXT NOT NULL,
    scope           TEXT NOT NULL,
    client_id       TEXT,
    client_name     TEXT,          -- Denormalized from client
    is_active       BOOLEAN NOT NULL,
    role_names      TEXT[],        -- Denormalized role list
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL,

    -- Indexes for common queries
    CONSTRAINT idx_email UNIQUE (email)
);

CREATE INDEX idx_user_list_client ON user_list_view(client_id);
CREATE INDEX idx_user_list_scope ON user_list_view(scope);
CREATE INDEX idx_user_list_active ON user_list_view(is_active);
CREATE INDEX idx_user_list_domain ON user_list_view(email_domain);
```

### Checkpoint Table

```sql
CREATE TABLE projection_checkpoints (
    projector_name  TEXT PRIMARY KEY,
    last_event_id   TEXT NOT NULL,
    last_event_time TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL
);
```

---

## Read Side

### BFF (Backend for Frontend)

Each UI has a dedicated BFF that:
- Serves queries optimized for that UI's needs
- Aggregates data from multiple read models
- Handles caching
- Contains no business logic

### Read Repository Pattern

```
BFF Endpoint
    │
    ▼
ReadRepository (trait)
    │
    ├── Cache check (moka / Redis)
    │   ├── Hit → Return cached
    │   └── Miss → Query DB
    │
    ▼
Postgres (sqlx)
    │
    ▼
DTO (optimized for UI)
```

### Technology

| Component | Technology | Purpose |
|-----------|------------|---------|
| BFF API | axum | HTTP endpoints |
| Database | Postgres | Read model storage |
| Query layer | sqlx | Compile-time checked SQL |
| L1 Cache | moka | In-process, hot data |
| L2 Cache | Redis | Distributed, shared |

### Caching Strategy

| Data Type | L1 (moka) | L2 (Redis) | TTL |
|-----------|-----------|------------|-----|
| User profile | ✓ | ✓ | 5 min |
| User list | ✗ | ✓ | 1 min |
| Lookup tables | ✓ | ✓ | 1 hour |
| Search results | ✗ | ✓ | 30 sec |

### Cache Invalidation

Two strategies:

1. **TTL-based** - Cache expires after fixed time
2. **Event-based** - Projector invalidates cache when processing event

```
Event arrives
    │
    ▼
Projector
    ├── Update Postgres
    └── Invalidate Redis cache key
```

---

## Technology Stack Summary

### Write Side

```
Runtime:          tokio
HTTP:             axum
Database:         MongoDB
Patterns:         UseCase, UnitOfWork, DomainEvent
Auth:             JWT (jsonwebtoken)
Serialization:    serde
```

### Event Infrastructure

```
Outbox:           MongoDB (fc-outbox processor)
Router:           fc-router (Rust)
Protocol:         CloudEvents 1.0
Ordering:         Message groups
```

### Read Side

```
Runtime:          tokio
HTTP:             axum (BFF endpoints)
Database:         Postgres
Query layer:      sqlx (compile-time checked)
L1 Cache:         moka (in-process)
L2 Cache:         Redis
Serialization:    serde
```

### Projection Processors

```
Runtime:          tokio
Event source:     fc-router subscription
Database:         Postgres (sqlx)
Checkpointing:    Postgres table
```

---

## Service Boundaries

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  fc-platform    │     │   fc-router     │     │  fc-projector   │
│  (Write API)    │────►│  (Event Bus)    │────►│  (Processors)   │
│                 │     │                 │     │                 │
│  • UseCases     │     │  • Routing      │     │  • Subscriptions│
│  • Aggregates   │     │  • Delivery     │     │  • Transforms   │
│  • MongoDB      │     │  • Ordering     │     │  • Postgres     │
└─────────────────┘     └─────────────────┘     └─────────────────┘
                                                        │
                                                        ▼
                                                ┌─────────────────┐
                                                │    fc-bff       │
                                                │  (Read API)     │
                                                │                 │
                                                │  • Queries      │
                                                │  • Caching      │
                                                │  • UI-optimized │
                                                └─────────────────┘
```

### Deployment

| Service | Instances | Database |
|---------|-----------|----------|
| fc-platform | 2+ (HA) | MongoDB |
| fc-router | 2+ (HA) | MongoDB |
| fc-outbox | 1 (leader election) | MongoDB |
| fc-projector | 1 per projector | Postgres |
| fc-bff | 2+ (HA) | Postgres (read-only) |

---

## Consistency Model

### Write Side

- **Strong consistency** within MongoDB transaction
- Aggregate + Event + Audit committed atomically

### Read Side

- **Eventual consistency** with write side
- Typical lag: 10-100ms
- Maximum lag: bounded by projector health checks

### Handling Eventual Consistency in UI

1. **Optimistic UI** - Update UI immediately, reconcile on next read
2. **Read-your-writes** - After command, poll read side until updated
3. **Event subscription** - UI subscribes to events for real-time updates

---

## Error Handling

### Write Side Errors

| Error Type | HTTP Status | Retry |
|------------|-------------|-------|
| ValidationError | 400 | No |
| BusinessRuleViolation | 409 | No |
| NotFoundError | 404 | No |
| ConcurrencyError | 409 | Yes (with backoff) |
| CommitError | 500 | Yes |

### Projection Errors

| Error Type | Action |
|------------|--------|
| Transient (DB connection) | Retry with backoff |
| Permanent (bad event data) | Dead letter queue |
| Schema mismatch | Alert, manual intervention |

### Read Side Errors

| Error Type | Action |
|------------|--------|
| Cache miss | Query database |
| Database timeout | Return cached (stale) if available |
| Database down | Circuit breaker, return error |

---

## Monitoring

### Key Metrics

| Metric | Source | Alert Threshold |
|--------|--------|-----------------|
| Command latency P99 | fc-platform | > 500ms |
| Event processing lag | fc-projector | > 5 seconds |
| Projection error rate | fc-projector | > 1% |
| Read query latency P99 | fc-bff | > 100ms |
| Cache hit ratio | fc-bff | < 80% |

### Health Checks

| Service | Check |
|---------|-------|
| fc-platform | MongoDB connection, auth service |
| fc-router | MongoDB connection, queue depth |
| fc-projector | Postgres connection, checkpoint freshness |
| fc-bff | Postgres connection, Redis connection |

---

## References

- [CQRS Pattern (Martin Fowler)](https://martinfowler.com/bliki/CQRS.html)
- [Event Sourcing vs Event-Driven](https://www.eventstore.com/blog/event-sourcing-and-cqrs)
- [CloudEvents Specification](https://cloudevents.io/)
- [Transactional Outbox Pattern](https://microservices.io/patterns/data/transactional-outbox.html)
