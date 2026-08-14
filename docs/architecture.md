# Architecture

Module layout and library choices.

---

## Module layout

```
flowcatalyst-go/
├── go.mod                              # module github.com/flowcatalyst/flowcatalyst-go
├── go.sum
├── docs/                               # this directory
├── cmd/                                # binary entry points
│   ├── fc-server/main.go
│   ├── fc-platform-server/main.go
│   ├── fc-router/main.go
│   ├── fc-stream-processor/main.go
│   ├── fc-outbox-processor/main.go
│   └── fcdev/                          # dev monolith; `mcp` subcommand runs the MCP server
│       ├── main.go
│       └── subcommands/                # start, init, fresh, mcp, outbox, upgrade
│
│   NOTE: today only `cmd/fc-server` (unified, FC_*_ENABLED toggles) and
│   `cmd/fcdev` are built. The standalone service binaries above are an
│   aspirational layout — by project decision the MCP server ships inside
│   fc-server / `fcdev mcp`, not as a separate fc-mcp-server binary.
├── internal/                           # non-importable internals
│   ├── common/                         # shared message/dispatch types
│   ├── config/                         # config loader
│   ├── secrets/                        # secret provider registry
│   ├── standby/                        # Redis-backed leader election
│   ├── queue/                          # queue abstraction
│   │   ├── queue.go                    # Consumer/Publisher interfaces
│   │   ├── sqs/                        # AWS SQS impl
│   │   ├── postgres/
│   │   ├── sqlite/
│   │   ├── nats/
│   │   └── amqp/
│   ├── router/                         # message router
│   │   ├── manager.go
│   │   ├── pool.go
│   │   ├── mediator.go
│   │   ├── circuitbreaker.go
│   │   ├── ratelimit.go
│   │   ├── configsync.go
│   │   ├── notification.go
│   │   ├── traffic.go
│   │   └── api/                        # router's HTTP endpoints (health/metrics)
│   ├── stream/                         # CQRS stream processor
│   │   ├── projector.go
│   │   ├── fanout.go
│   │   ├── partition_manager.go
│   │   └── health.go
│   ├── outbox/                         # outbox processor
│   │   ├── processor.go
│   │   ├── buffer.go
│   │   ├── distributor.go
│   │   ├── dispatcher.go
│   │   ├── postgres/                   # backend repos
│   │   ├── sqlite/
│   │   ├── mysql/
│   │   └── mongo/
│   ├── usecase/                        # use-case machinery
│   │   ├── result.go                   # Result[E], sealed success/failure
│   │   ├── usecase.go                  # UseCase[C, E] interface
│   │   ├── runner.go                   # Run() with type-state ordering
│   │   ├── unit_of_work.go             # UnitOfWork[E] interface
│   │   ├── uow_postgres.go             # PgUnitOfWork, TxScopedUnitOfWork
│   │   ├── persist.go                  # Persist[A] interface, DbTx newtype
│   │   ├── domain_event.go             # DomainEvent interface
│   │   ├── execution_context.go        # ExecutionContext
│   │   ├── tracing_context.go
│   │   └── error.go                    # UseCaseError sum type
│   ├── tsid/                           # TSID generator (Crockford Base32)
│   ├── platform/                       # the platform domain (the bulk of the code)
│   │   ├── application/                # one subdir per subdomain
│   │   │   ├── entity.go
│   │   │   ├── repository.go
│   │   │   ├── api.go
│   │   │   ├── operations/
│   │   │   │   ├── create.go
│   │   │   │   ├── update.go
│   │   │   │   ├── delete.go
│   │   │   │   ├── events.go
│   │   │   │   └── ...
│   │   │   └── application_test.go
│   │   ├── audit/
│   │   ├── auth/
│   │   ├── client/
│   │   ├── connection/
│   │   ├── cors/
│   │   ├── dispatchjob/
│   │   ├── dispatchpool/
│   │   ├── emaildomainmapping/
│   │   ├── event/
│   │   ├── eventtype/
│   │   ├── identityprovider/
│   │   ├── idp/
│   │   ├── loginattempt/
│   │   ├── passwordreset/
│   │   ├── platformconfig/
│   │   ├── principal/
│   │   ├── process/
│   │   ├── role/
│   │   ├── scheduledjob/
│   │   ├── scheduler/
│   │   ├── seed/
│   │   ├── serviceaccount/
│   │   ├── shared/                     # cross-subdomain services
│   │   │   ├── auth/                   # checks, authorization service, middleware
│   │   │   ├── database/               # pool init, secrets-manager rotation
│   │   │   ├── httperror/              # error → HTTP status mapping
│   │   │   ├── apicommon/              # PaginatedResponse, PaginationParams
│   │   │   ├── ratelimit/              # per-IP rate limiter
│   │   │   ├── server/                 # huma server assembly
│   │   │   ├── bff/                    # BFF API handlers
│   │   │   ├── sdk/                    # SDK sync API
│   │   │   └── monitoring/             # health, metrics, monitoring APIs
│   │   ├── subscription/
│   │   └── webauthn/
│   └── mcp/                            # MCP server
├── pkg/                                # public, importable by consumers
│   └── fcsdk/                          # the Go SDK
│       ├── client/                     # platform API client
│       ├── outbox/                     # outbox + UoW for consumer apps
│       ├── auth/                       # OIDC + JWT validation
│       ├── webhook/                    # webhook signature verification
│       ├── cache/                      # memory/postgres/redis backends
│       ├── lock/                       # distributed lock
│       ├── scheduledjobs/              # consumer-side runner
│       ├── tsid/                       # re-export of internal/tsid
│       └── http/                       # chi/huma integration
├── frontend/                           # Vue SPA (embedded into binaries at build time)
├── tests/
│   ├── parity/                         # API contract tests
│   ├── golden/                         # JSON golden files
│   └── integration/                    # cross-package integration tests
└── tools/
    ├── analyzer/                       # custom go vet analyzer for UoW seal
    └── parityharness/                  # replay-based API contract harness
```

---

## Library choices, with rationale

### HTTP framework: `chi` + `huma/v2`

- **chi** for routing — minimal, idiomatic `net/http`, no surprises, mature, widely used.
- **huma v2** for OpenAPI emission from handlers. Generates spec from typed handlers, register operations with `huma.Register(api, op, handler)`.

Alternative considered: `echo` (heavier, less idiomatic), `gin` (older, less idiomatic), hand-author OpenAPI YAML + `oapi-codegen` (works but loses the spec-from-handlers feel).

### Database: `go-jet` + raw `pgx`

Hybrid: type-safe codegen where it pays, raw pgx where it doesn't.

**`go-jet/v2`** for CRUD repositories under `internal/platform/*/repository.go`:
- Generates typed model structs from the live schema (one per table).
- DSL handles dynamic WHERE clauses naturally — `find_with_filters(application?, client_id?, status?, ...)` becomes `SELECT(...).FROM(...).WHERE(builder.Build())` instead of N query variants.
- Compile-time checking catches column renames.
- Codegen runs from `tools/gen/jet.go` against an ephemeral Postgres (migrated to HEAD) on every migration change. **Generated code is committed** to `internal/db/gen/` — PR diffs show schema changes explicitly, no build-time DB dependency, and the model code is grep-able. CI has a `make verify-jet` step that regenerates and `git diff --exit-code` to catch drift.

**Raw `pgx/v5`** for code where the DSL isn't a fit:
- `internal/stream/projector.go` — `FOR UPDATE SKIP LOCKED` claim queries; we want the SQL to be obviously the SQL.
- `internal/stream/partition_manager.go` — DDL emission (`CREATE TABLE ... PARTITION OF ...`). Not what jet is for.
- `internal/router/` and `internal/queue/postgres/` — hot-path reads, where DSL allocation overhead matters.
- Anywhere recursive CTEs (`WITH RECURSIVE`), JSON path operators (`@>`, `?`, `jsonb_path_query`), or non-trivial window functions are awkward in jet.
- One-off admin queries in `internal/platform/shared/*` (integrity scan, projections service helpers).

Net split: roughly 75% jet, 25% raw pgx by query count. Both use the same `pgxpool.Pool` — jet has a `qrm.DB` adapter for pgx.

**Migrations** stay as plain SQL files. Jet does not own migrations.

### JSON: stdlib by default, fast-path libraries for the router

Layered, to keep most of the codebase boring while the message router gets the throughput it needs.

| Where | Library | Why |
|---|---|---|
| `internal/platform/*`, `pkg/fcsdk/*`, all HTTP handlers, all OpenAPI surfaces | stdlib `encoding/json` | Boring, well-known, zero extra dependency. Throughput is plenty for a control plane. |
| `internal/router/*`, `internal/queue/*` (the message hot path) | `github.com/goccy/go-json` as a drop-in via a tiny `internal/router/jsonx/` wrapper | ~3× stdlib throughput. Drop-in (same struct tags). Pure Go, no CPU-arch restrictions, no JIT, no platform-specific build paths. |
| `internal/common.Message`, `internal/common.QueuedMessage`, `internal/common.MediationOutcome` | `github.com/mailru/easyjson` generated marshalers, alongside stdlib fallback | ~8× stdlib for these specific types. They (de)serialize on every webhook. Commit the generated `_easyjson.go` files. |

Considered and rejected: `bytedance/sonic`. Faster than goccy on amd64 (JIT), but has CPU-arch restrictions (amd64+arm64 only, JIT only on amd64) and is harder to debug. The throughput delta over goccy isn't worth the operational complexity given Go's `GOARCH=arm64` macOS dev environment + Linux server mix.

**Hard wire-contract constraint:** the router signs the payload bytes it receives — it does **not** re-serialize. Take the bytes from the queue, compute the HMAC, send. **JSON library choice is irrelevant to signing as long as we never re-serialize before signing.** A test vector in `tests/golden/webhook/` enforces this — see [`wire-contract.md`](./wire-contract.md#webhook-signatures).

If anywhere in the platform we do `Marshal(event) → sign(bytes)` (e.g., outbox event payload signing), the serializer's exact byte output becomes part of the contract with every verifier — which means stdlib only, and a fixed struct-tag posture (field ordering, null/omitempty, number representation). Audit before committing.

### Migrations: `golang-migrate`

- Compatible with the existing `_schema_migrations` table format.
- File naming: `001_initial.sql` → already matches our existing convention.

### Validation: `go-playground/validator/v10`

- Struct tag-based validation. `validate:"required,email"`, `validate:"oneof=CURRENT ARCHIVED"`, etc.
- Used in `Validate()` step of use cases. Not for HTTP-layer DTO checks — those happen inside `UseCase.Validate()` so the rule lives next to the operation, not the route.

### Auth

Most of the auth subdomain is RFC-compliant OAuth/OIDC protocol mechanics. The OIDC **client** side (bridging to external IDPs) leans on libraries; the OAuth/OIDC **provider** side is hand-rolled, for exact control of the wire contract.

- **`golang-jwt/jwt/v5`** for JWT encode/decode (RS256, with an HS256 dev fallback). Used directly by `authservice` (OAuth/OIDC tokens + JWKS) and `sessiontoken` (session cookies).
- **`github.com/coreos/go-oidc/v3`** + **`golang.org/x/oauth2`** for the OIDC **bridge** (FlowCatalyst as an OIDC client of Entra / Keycloak / Google). Reads `EmailDomainMapping` to route users to the right external IDP.
- **Hand-rolled OAuth/OIDC provider** (`internal/platform/auth/oauthapi`) — FlowCatalyst as an OIDC/OAuth **provider**, issuing access/refresh/ID tokens to SDK consumers (`client_credentials` grant) and users (`authorization_code` + PKCE). Owns the token / authorize / introspect / revoke / userinfo endpoints plus `.well-known/openid-configuration` and JWKS. JWT mint/validate lives in `auth/authservice`; auth-code, refresh-token, and pending-auth artifacts persist in `oauth_oidc_payloads` via `auth/grantstore`. Tokens carry FlowCatalyst-specific claims (`scope`, `clients[]`, `roles[]`, `applications[]`, `email`). Originally built on `ory/fosite`; removed 2026-05-28 (see [ADR-0001](adr/0001-session-token-vs-oauth.md)) because its storage-backed model didn't fit the platform's custom claim shapes, multi-key JWKS rotation, `plain` PKCE, and per-client rate limiting. `client_credentials` is otherwise SDK/service-account-only, but `handleClientCredentialsGrant` (`token.go`) carries one deliberate, narrowly-scoped exception: a regular USER principal holding the seeded `platform:developer` role can mint a token as themselves (`client_id` = their own principal id, no `OAuthClient` row) via a dedicated, rotatable secret on `iam_principals` — self-service local testing against a deployed environment without provisioning a service account. The developer-role check is re-verified live at every mint, not just "does a secret exist," so revoking the role cuts off new tokens immediately.
- **`github.com/go-jose/go-jose/v4`** — JWK/JWS primitives, now pulled in only transitively by the OIDC bridge. We don't use it directly (JWKS is hand-rolled in `authservice`).
- **`go-webauthn/webauthn`** for passkeys. Its `SessionData` shape lets the in-flight ceremony be persisted.
- **`x/crypto/argon2`** for password hashing.
- **`crypto/aes`** + **`crypto/cipher`** for AES-GCM (cookie sessions, secret encryption).

**Library longevity:** `go-oidc` is Red Hat / Kubernetes-grade; `x/oauth2` is an official Go subrepository; `go-jose` originated at Square and is now community-maintained; `go-webauthn` is the de-facto Go passkey library. All Apache 2.0 — pinned versions are forever-freely-usable. (The OAuth/OIDC **provider** is no longer a library at all — it's hand-rolled as described above.)

### Queue backends

Backend interface in `internal/queue/queue.go`:

```go
type Consumer interface {
    Poll(ctx context.Context, maxMessages int) ([]QueuedMessage, error)
    Ack(ctx context.Context, receipt string) error
    Nack(ctx context.Context, receipt string, delaySeconds int) error
    Defer(ctx context.Context, receipt string) error
    ExtendVisibility(ctx context.Context, receipt string, seconds int) error
    Healthy() bool
    Stop()
}

type Publisher interface {
    Publish(ctx context.Context, msg Message) error
    PublishBatch(ctx context.Context, msgs []Message) error
}
```

Backend impls:
- `internal/queue/sqs` — `aws-sdk-go-v2/service/sqs`
- `internal/queue/postgres` — the `pg_queue_messages` table
- `internal/queue/sqlite` — same table shape, for `fcdev`
- `internal/queue/amqp` — `rabbitmq/amqp091-go` (this backend is labelled "activemq" in some configs for historical reasons, but the protocol spoken is AMQP)
- `internal/queue/nats` — `nats-io/nats.go` JetStream

Backends registered at runtime in `cmd/*/main.go` via `queue.Register(name, factory)`. **No build tags.** Binary size delta is negligible; deployment is simpler.

### Outbox backends

`internal/outbox/repository.go` defines the `Repository` interface. Backends:
- `internal/outbox/postgres` — `pgx/v5`
- `internal/outbox/sqlite` — `database/sql` + `modernc.org/sqlite` (pure Go; no CGo)
- `internal/outbox/mysql` — `database/sql` + `go-sql-driver/mysql`
- `internal/outbox/mongo` — `go.mongodb.org/mongo-driver/v2`

### Secrets backends

`internal/secrets/provider.go`:

```go
type Provider interface {
    Get(ctx context.Context, key string) (string, error)
    Set(ctx context.Context, key, value string) error
    Delete(ctx context.Context, key string) error
    Name() string
}
```

Provider registry:
- `env` — environment variable lookup
- `encrypted` — AES-256-GCM local file
- `aws-sm` — AWS Secrets Manager
- `aws-ps` — AWS Parameter Store
- `vault` — HashiCorp Vault (HTTP)

Reference format parsing (`aws-sm://name`, `vault://path#key`, …).

### Standby / leader election

`internal/standby/election.go`:

```go
type Election struct { /* unexported */ }

func New(cfg Config) (*Election, error)
func (e *Election) Start(ctx context.Context) error      // spawns the lease loop
func (e *Election) IsLeader() bool                       // atomic read
func (e *Election) Subscribe() <-chan LeadershipChange   // notification chan
func (e *Election) Release(ctx context.Context) error    // graceful step-down
```

Uses `redis/go-redis/v9` SET NX EX with periodic refresh.

### Router internals

Concurrency model:
- One goroutine per pool drain.
- Per-message-group FIFO via `map[string]*groupQueue` protected by `sync.RWMutex`; each group queue is `[]Message` + an in-flight flag.
- A `*rate.Limiter` per pool (rate limit applied before processing each message), hot-swappable on config reload via `atomic.Pointer[rate.Limiter]`.
- Circuit breaker per endpoint URL — a `Closed`/`Open`/`HalfOpen` state machine with a sliding window (`[]bool`) of recent success/failure.
- HTTP delivery via `net/http` client with per-pool transport tuning (max idle conns, etc.).
- HMAC-SHA256 webhook signature using `crypto/hmac` + `crypto/sha256`.

### Stream processor

Three independent goroutines:
- `eventProjector` — `msg_events` → `msg_events_read`.
- `fanOut` — match subscriptions, insert `msg_dispatch_jobs`.
- `dispatchJobProjector` — `msg_dispatch_jobs` → `msg_dispatch_jobs_read`.

Plus a `partitionManager` goroutine that runs on a 60-minute tick to ensure next-month partitions exist for the seven partitioned tables.

All claim queries use `FOR UPDATE SKIP LOCKED`.

### Outbox processor

- `Buffer` — ring buffer with a `chan struct{}` work signal.
- `GroupDistributor` — routes items to per-group queues based on `message_group`.
- `Dispatcher` — sends batches to the FlowCatalyst HTTP API.
- Backpressure via `atomic.Int64` in-flight counter; pause polling at `maxInFlight`.

### MCP server

`internal/mcp`, built on the official `github.com/modelcontextprotocol/go-sdk`.
The SDK handles the `initialize` handshake, capability negotiation, and both
transports:
- stdio (default for `fcdev mcp`) — JSON-RPC over stdin/stdout, logs to stderr.
- streamable-HTTP — mounted at `/mcp` (default `127.0.0.1:8090`, `FC_MCP_BIND`/`FC_MCP_PORT`).

Read-only tool surface: `list_event_types`, `get_event_type`,
`get_schema`, `list_subscriptions`, `get_subscription`, `list_applications`,
`list_roles`, `get_role`, `get_openapi`, `whoami`, `list_my_applications`,
`get_application_capabilities`. Plus resources: 5 fixed collections
(`flowcatalyst://{openapi/platform,applications,roles,event-types,subscriptions}`)
and hierarchical single-entity templates (`…/event-types/{id}`, etc.). Auth is
OAuth2 client_credentials with an in-memory token cache (refresh 60s before
expiry); `fcdev start` bootstraps a local MCP client + credentials file.

---

## Cross-cutting concerns

### Logging

`log/slog` JSON handler. Field names:
- `level`, `time`, `msg`
- `correlation_id`, `causation_id`, `principal_id`, `execution_id`
- `aggregate_type`, `aggregate_id`, `event_type`

Pass loggers via `slog.With(...)` rather than passing them through every function signature. Use `context.Context` for request-scoped fields (correlation_id) via a small `internal/platform/shared/logctx/` helper.

### Metrics

`prometheus/client_golang`:
- HTTP request duration histograms (per route + status code).
- Pool throughput counters (per pool code).
- Webhook delivery latencies — backed by `HdrHistogram/hdrhistogram-go` for fine p99 tracking.
- Circuit breaker state gauges (per endpoint).
- Queue depth, in-flight, and rate-limit-defer counts.

`/metrics` endpoint on each binary, exposed on `FC_METRICS_PORT`.

### Tracing

OpenTelemetry via `go.opentelemetry.io/otel`. Optional, off by default. When enabled, spans wrap each HTTP request and each UoW transaction.

### Configuration

Two layers:
1. **TOML files** — loaded by `internal/config`.
2. **Environment variables** — override TOML, namespaced by `FC_` and `FLOWCATALYST_`.

Precedence: env > file > default.
