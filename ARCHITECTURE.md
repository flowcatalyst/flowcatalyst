# FlowCatalyst — System Architecture

## System Context (C4 Level 1)

FlowCatalyst is an event-driven integration platform. Applications publish domain events, the platform routes them through subscriptions to webhook endpoints, with rate limiting, ordering, and retry guarantees.

```
┌───────────────────┐         ┌──────────────────────────────────────┐
│  Consumer Apps    │────────>│          FlowCatalyst                │
│  (SDK clients)    │  events │                                      │
│                   │<────────│  Platform API   Scheduler   Router   │
│                   │ webhooks│                                      │
└───────────────────┘         └───────────┬──────────┬───────────────┘
                                          │          │
                              ┌───────────┴──┐  ┌────┴────────┐
                              │  PostgreSQL   │  │  SQS FIFO   │
                              │  (all state)  │  │  (dispatch)  │
                              └──────────────┘  └─────────────┘
                                          │
                              ┌───────────┴──┐  ┌─────────────┐
                              │  Redis       │  │  Entra/      │
                              │  (HA leader) │  │  Keycloak    │
                              └──────────────┘  │  (OIDC IDPs) │
                                                └─────────────┘
```

### External Systems

| System | Protocol | Purpose |
|--------|----------|---------|
| PostgreSQL | TCP/TLS | All state: tenants, IAM, events, dispatch jobs, audit logs, OAuth tokens |
| SQS FIFO | AWS SDK | Dispatch job delivery (scheduler → router) |
| Redis | TCP/TLS | Leader election for active/standby HA |
| OIDC Identity Providers | HTTPS | External authentication (Entra ID, Keycloak) |
| Webhook Endpoints | HTTPS | Message delivery to subscriber connections |
| Teams Webhooks | HTTPS | Operational alerts from router |
| AWS Secrets Manager | AWS SDK | Database credentials in production |

---

## Container View (C4 Level 2)

### Binaries

```
┌─────────────────────────────────────────────────────────────────┐
│                    fc-server (unified binary)                    │
│                                                                  │
│  ┌────────────┐ ┌────────────┐ ┌──────────┐ ┌───────────────┐  │
│  │ Platform   │ │ Scheduler  │ │ Router   │ │ Stream        │  │
│  │ API        │ │            │ │ (SQS)    │ │ Processor     │  │
│  │ (Axum)     │ │ (poller)   │ │          │ │ (CQRS)        │  │
│  └────────────┘ └────────────┘ └──────────┘ └───────────────┘  │
│                                                                  │
│  Each subsystem toggled via env: FC_PLATFORM_ENABLED=true, etc. │
└─────────────────────────────────────────────────────────────────┘

┌──────────────┐  ┌───────────────┐  ┌──────────────────┐
│ fc-router    │  │ fc-dev        │  │ fc-outbox-       │
│ (standalone) │  │ (dev monolith │  │ processor        │
│              │  │  SQLite queue)│  │ (multi-DB)       │
└──────────────┘  └───────────────┘  └──────────────────┘
```

| Binary | Purpose | Subsystems | Database |
|--------|---------|------------|----------|
| **fc-server** | Unified production server | Platform + Scheduler + Router + Stream + Outbox (all toggleable) | PostgreSQL |
| **fc-router** | Standalone message router | SQS consumption + HTTP delivery | None (config via HTTP) |
| **fc-dev** | Development monolith | Platform + embedded SQLite queue + Router | PostgreSQL |
| **fc-outbox-processor** | Application outbox dispatcher | Polls app outbox tables, forwards to platform SDK API | SQLite/PostgreSQL/MongoDB |
| **fc-platform-server** | Standalone platform API | REST API only | PostgreSQL |
| **fc-scheduler-server** | Standalone scheduler | Job polling + SQS publishing | PostgreSQL |

### Crate Dependency Graph

```
fc-server
 ├─ fc-platform       ← domain logic, API handlers, use cases
 ├─ fc-router         ← message routing engine
 ├─ fc-scheduler      ← dispatch job polling + ordering
 ├─ fc-stream         ← CQRS read-model projections
 ├─ fc-outbox         ← application outbox processing
 ├─ fc-standby        ← Redis leader election
 ├─ fc-queue          ← queue abstraction (SQS, SQLite, ActiveMQ, NATS)
 ├─ fc-config         ← TOML config + env var loading
 ├─ fc-secrets        ← secret resolution (AWS SM, Vault, encrypted files)
 └─ fc-common         ← shared types (Message, DispatchMode, PoolConfig)
```

---

## Component View (C4 Level 3)

### Event Lifecycle

```
1. App publishes event
   POST /api/sdk/events/batch
        │
        ▼
2. Platform stores event
   INSERT INTO msg_events (CloudEvents 1.0)
        │
        ▼
3. Dispatch service matches subscriptions
   SELECT * FROM msg_subscriptions WHERE event_type matches
        │
        ▼
4. Dispatch jobs created (one per matching subscription)
   INSERT INTO msg_dispatch_jobs (status=PENDING)
        │
        ▼
5. Scheduler polls PENDING jobs
   SELECT * FROM msg_dispatch_jobs WHERE status='PENDING'
   Filters: paused connections (cached), blocked groups
        │
        ▼
6. Scheduler publishes to SQS
   MessagePointer: {id, poolCode, messageGroupId, mediationTarget, dispatchMode}
        │
        ▼
7. Router consumes from SQS
   Dedup → route to pool → rate limit → circuit breaker → HTTP POST
        │
        ▼
8. Webhook endpoint processes message
   POST {mediationTarget} with {"messageId":"<id>"}
        │
        ▼
9. Router ACKs/NACKs based on response
   2xx → ACK (delete from SQS)
   5xx → NACK (retry after delay)
```

### CQRS Projection

```
msg_events (write model)          msg_events_read (read model)
┌──────────────────────┐          ┌─────────────────────────┐
│ id, type, source,    │  stream  │ id, type, source,       │
│ data (JSONB),        │ ──────>  │ application, subdomain, │
│ correlation_id, ...  │processor │ aggregate, event_name   │
└──────────────────────┘          └─────────────────────────┘

msg_dispatch_jobs (write)         msg_dispatch_jobs_read
┌──────────────────────┐          ┌─────────────────────────┐
│ id, status, mode,    │  stream  │ (same + terminal flags, │
│ target_url, ...      │ ──────>  │  enriched metadata)     │
└──────────────────────┘          └─────────────────────────┘
```

---

## Domain Model (C4 Level 3 — Platform)

### Entity Relationship Overview

```
CLIENT (tenant)
 ├── PRINCIPAL (user or service account)
 │    ├── ROLE assignments (junction: iam_principal_roles)
 │    ├── CLIENT access grants (junction: iam_client_access_grants)
 │    └── APPLICATION access (junction: iam_principal_application_access)
 │
 ├── EVENT_TYPE definitions
 │    └── SPEC_VERSION (schema versions per event type)
 │
 ├── CONNECTION (webhook endpoint)
 │    └── SERVICE_ACCOUNT (auth credentials)
 │
 ├── SUBSCRIPTION (event type → connection binding)
 │    ├── EVENT_TYPE_BINDING (pattern matches, wildcards)
 │    ├── DISPATCH_POOL (rate limit / concurrency config)
 │    └── CONFIG entries (custom key-value)
 │
 └── DISPATCH_JOB (async delivery unit)
      └── DISPATCH_ATTEMPT (individual delivery attempts)

APPLICATION
 ├── ROLE definitions (application-scoped)
 │    └── PERMISSION grants (junction: iam_role_permissions)
 └── SERVICE_ACCOUNT (machine credentials)

IDENTITY_PROVIDER (external OIDC)
 └── EMAIL_DOMAIN_MAPPING (domain → IDP + scope + roles)
```

### Domain Aggregates

| Aggregate | Table Prefix | ID Prefix | Key Fields |
|-----------|-------------|-----------|------------|
| Client | `tnt_` | `clt_` | name, identifier, status |
| Principal | `iam_` | `usr_` | type (User/Service), scope (Anchor/Partner/Client), email |
| Role | `iam_` | `rol_` | name, application_code, source (Code/Database/Sdk) |
| Application | `app_` | `app_` | code, type (Application/Integration), active |
| ServiceAccount | `iam_` | `svc_` | code, webhook_auth_type, auth_token (encrypted) |
| EventType | `msg_` | `evt_` | code (app:subdomain:aggregate:event), status |
| Subscription | `msg_` | `sub_` | event type bindings, connection_id, dispatch_mode |
| Connection | `msg_` | `con_` | endpoint URL, status (Active/Paused) |
| DispatchPool | `msg_` | `dpl_` | rate_limit, concurrency |
| DispatchJob | `msg_` | `djb_` | status, mode, target_url, sequence |
| Event | `msg_` | `mev_` | CloudEvents 1.0 (type, source, data) |
| AuditLog | `aud_` | `aud_` | entity_type, operation, principal_id |
| IdentityProvider | `oauth_` | `idp_` | type, oidc_issuer_url, oidc_client_id |
| EmailDomainMapping | `tnt_` | `edm_` | email_domain, identity_provider_id, scope_type |
| CorsOrigin | `tnt_` | `cor_` | origin URL |

### Use Case Pattern

All control-plane writes follow the UseCase trait:

```
Handler (thin adapter)
 1. Check permission (role-level)
 2. Build Command from request DTO
 3. Create ExecutionContext from auth
 4. Call use_case.run(command, ctx)
      │
      ▼
UseCase.run(command, ctx)
 1. validate()  → field format, length, presence
 2. authorize() → resource-level ownership checks
 3. execute()   → load aggregate, apply business rules
      │
      ▼
UnitOfWork.commit()
 → INSERT/UPDATE entity
 → INSERT domain event into outbox_messages
 → INSERT audit log
 → All in single PostgreSQL transaction
```

**Infrastructure exceptions** (bypass UseCase, per CLAUDE.md):
- Event ingest: `POST /api/sdk/events/batch`
- Dispatch job ingest: `POST /api/sdk/dispatch-jobs/batch`
- Dispatch job status transitions (scheduler lifecycle)
- Outbox processing

---

## Authentication & Authorization

### JWT Token Flow

```
Login (password or OIDC callback)
      │
      ▼
AuthService.generate_access_token()
 → RS256 signed JWT (RSA keys, key rotation support)
 → Claims: sub, scope, roles, clients, applications
      │
      ▼
API Request with Bearer token
      │
      ▼
AuthService.validate_token()
 → Check cache (DashMap, 30s TTL)
 → Verify RSA signature (current key, then previous for rotation)
 → Extract AccessTokenClaims
      │
      ▼
AuthorizationService.has_permission()
 → Load role→permissions (DashMap cache, 60s TTL)
 → Wildcard matching (e.g., platform:*:*:read)
```

### OIDC Login Flow

```
1. POST /auth/check-domain {email}
   → Look up EmailDomainMapping by domain
   → Return auth method (internal / OIDC)

2. If OIDC: redirect to IDP
   → Build authorization URL with state + nonce
   → Store OidcLoginState in DB (single-use)

3. IDP callback: GET /auth/oidc/login/callback?code=...&state=...
   → Consume state (DELETE RETURNING — atomic, race-free)
   → Exchange code for tokens at IDP
   → Validate ID token (JWKS per issuer, cached)
   → Reject #EXT# guest accounts
   → Sync principal from IDP claims
   → Emit UserLoggedIn domain event
   → Issue FlowCatalyst session JWT
```

### Multi-Tenancy

```
UserScope::Anchor   → Access all clients (platform admin)
UserScope::Partner  → Access assigned clients (integration partner)
UserScope::Client   → Access home client only (end user)
```

Token `clients` claim:
- Anchor: `["*"]`
- Partner: `["clt_abc:acme", "clt_def:globex"]`
- Client: `["clt_abc:acme"]`

---

## Scheduler

### Adaptive Polling

```
Poll msg_dispatch_jobs WHERE status='PENDING'
    │
    ├─ Full batch (200 jobs) → re-poll immediately
    ├─ Partial batch → 500ms pause
    └─ Empty → 1s pause

Filters applied:
 1. Connection pause cache (refreshed every 60s)
 2. Blocked message groups (batch query per poll)
 3. Dispatch mode filtering
```

### Dispatch Flow

```
PendingJobPoller.poll()
 → SELECT PENDING jobs (batch_size=200)
 → Filter paused connections (PausedConnectionCache, 60s TTL)
 → Group by message_group
 → Check blocked groups (batch query)
 → Submit to MessageGroupDispatcher
      │
      ▼
MessageGroupDispatcher
 → 1 in-flight per group (semaphore)
 → Queue remaining in MessageGroupQueue (VecDeque)
      │
      ▼
dispatch_single_job()
 → Build MessagePointer {id, poolCode, messageGroupId, dispatchMode, ...}
 → Publish to SQS (QueuePublisher trait)
 → Batch update status → QUEUED (single SQL: WHERE id IN (...))
```

---

## Message Router

See [crates/fc-router/ARCHITECTURE.md](crates/fc-router/ARCHITECTURE.md) for detailed router internals.

**Summary:** Consumes from SQS → deduplicates → routes to process pools → rate limits → circuit breaks per endpoint → HTTP POST to webhook → ACK/NACK.

Key features:
- Per-endpoint circuit breakers (sliding window, 50% failure threshold)
- IMMEDIATE mode: concurrent processing (no group ordering)
- BLOCK_ON_ERROR / NEXT_ON_ERROR: sequential per group with cascading NACKs
- Governor async rate limiting (zero-CPU wait)
- HdrHistogram metrics (O(1) percentile queries)
- Dynamic config sync (pools + queues hot-reloaded)
- Capacity-gated polling (backpressure when pools full)

---

## Stream Processor (CQRS)

```
EventProjectionService
 → Polls msg_events for unprojected rows
 → Projects to msg_events_read (app/subdomain/aggregate breakdown)
 → Batch processing with configurable sizes
 → SQL CTE-based projections

DispatchJobProjectionService
 → Polls msg_dispatch_jobs for unprojected rows
 → Projects to msg_dispatch_jobs_read (terminal state flags)
```

---

## Outbox Processor

For consumer applications that integrate via the SDK outbox pattern:

```
Application DB                    Platform API
┌──────────────┐                 ┌──────────────┐
│ outbox_      │  outbox         │ /api/sdk/    │
│ messages     │──processor────> │ events/batch │
│ (SQLite/PG/  │  HTTP batch     │ dispatch-    │
│  MongoDB)    │                 │ jobs/batch   │
└──────────────┘                 └──────────────┘

Per-group sequential processing
Global concurrency control (max_concurrent_groups)
Recovery task for stalled messages
Retry with exponential backoff
```

---

## Infrastructure

### Database Schema (Key Tables)

| Category | Tables | Purpose |
|----------|--------|---------|
| **Tenants** | tnt_clients, tnt_anchor_domains, tnt_cors_allowed_origins, tnt_email_domain_mappings | Multi-tenant organization management |
| **IAM** | iam_principals, iam_roles, iam_permissions, iam_principal_roles, iam_client_access_grants, iam_principal_application_access | Identity, roles, permissions |
| **Messaging** | msg_events, msg_event_types, msg_event_type_spec_versions, msg_subscriptions, msg_connections, msg_dispatch_pools, msg_dispatch_jobs, msg_dispatch_job_attempts | Event store + dispatch pipeline |
| **Read Models** | msg_events_read, msg_dispatch_jobs_read | CQRS projections |
| **Auth** | oauth_oidc_login_states, oauth_oidc_payloads, oauth_clients | OAuth/OIDC state |
| **Applications** | app_applications, app_application_client_configs | Application definitions |
| **Audit** | aud_logs | Audit trail |
| **Outbox** | outbox_messages | Transactional outbox |

### ID Format (TSID)

All entities use time-sorted IDs with type prefixes:
```
clt_0HZXEQ5Y8JY5Z    (Client)
usr_0HZXEQ6A2B3C4    (Principal/User)
evt_0HZXEQ7D5E6F7    (Event Type)
sub_0HZXEQ8G8H9I0    (Subscription)
djb_0HZXEQ9J1K2L3    (Dispatch Job)
```

30 entity type variants, Crockford Base32 encoded, 13 characters + prefix.

### Configuration

Environment variables support both Rust (`FC_*`) and TypeScript names for deployment compatibility:

| Rust Name | TS Alias | Default |
|-----------|----------|---------|
| `FC_API_PORT` | `PORT` | 3000 |
| `FC_DATABASE_URL` | `DATABASE_URL` | postgresql://localhost:5432/flowcatalyst |
| `FC_PLATFORM_ENABLED` | `PLATFORM_ENABLED` | true |
| `FC_ROUTER_ENABLED` | `MESSAGE_ROUTER_ENABLED` | false |
| `FC_SCHEDULER_ENABLED` | `DISPATCH_SCHEDULER_ENABLED` | false |
| `FC_STANDBY_ENABLED` | `STANDBY_ENABLED` | false |
| `FC_STANDBY_REDIS_URL` | `REDIS_URL` | redis://127.0.0.1:6379 |

Database credentials can be resolved from:
1. `FC_DATABASE_URL` — full connection string
2. `DB_HOST` + `DB_SECRET_ARN` — AWS Secrets Manager (TS compatibility)
3. `DB_HOST` + `DB_USERNAME` + `DB_PASSWORD` — explicit

### Technology Stack

| Layer | Technology |
|-------|-----------|
| Web | Axum 0.8, tower-http |
| ORM | SeaORM 1.1 (migrating to raw SQLx) |
| Database | PostgreSQL, SQLite (dev queue) |
| Queue | AWS SQS FIFO, SQLite, ActiveMQ, NATS |
| Auth | jsonwebtoken (RS256), HMAC-SHA256 webhook signing |
| Caching | DashMap (in-process), Redis (leader election) |
| Metrics | Prometheus, HdrHistogram |
| Rate Limiting | governor (RFC-compliant token bucket) |
| Config | TOML + env vars, dynamic HTTP sync |
| Secrets | AWS Secrets Manager, Vault, encrypted files |
| Logging | tracing + tracing-subscriber (JSON or text) |
| API Docs | utoipa (OpenAPI/Swagger) |
