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
└── tools/
    ├── analyzer/                       # custom go vet analyzers (uowseal, idconv)
    └── dump-spec/                      # OpenAPI spec dump for the lock file
```

---

## Library choices, with rationale

### HTTP framework: `chi` + `huma/v2`

- **chi** for routing — minimal, idiomatic `net/http`, no surprises, mature, widely used.
- **huma v2** for OpenAPI emission from handlers. Generates spec from typed handlers, register operations with `huma.Register(api, op, handler)`.

Alternative considered: `echo` (heavier, less idiomatic), `gin` (older, less idiomatic), hand-author OpenAPI YAML + `oapi-codegen` (works but loses the spec-from-handlers feel).

### Database: `sqlc` + `pgx`

Typed generated queries where they fit, hand-written pgx where they do not. Both use the same `pgxpool.Pool`.

**`sqlc`** generates the typed query layer used by the repositories under `internal/platform/*/repository.go`:
- Queries are plain SQL in `internal/sqlc/queries/`; sqlc checks them against the schema defined by the migrations in `internal/migrate/sql/` and generates typed Go in `internal/sqlc/dbq/` (config: `sqlc.yaml`).
- **Generated code is committed**, so PR diffs show schema and query changes explicitly and there is no build-time database dependency. `make sqlc` regenerates it with the pinned sqlc; `make sqlc-verify` (run in CI) fails on drift.
- Compile-time checking catches column renames. Dynamic WHERE clauses are written as sqlc queries with optional-parameter predicates or, where that is awkward, as hand-written pgx.

**Raw `pgx/v5`** for code where generated queries are not a fit:
- `internal/stream/projector.go`: `FOR UPDATE SKIP LOCKED` claim queries; we want the SQL to be obviously the SQL.
- `internal/stream/partition_manager.go`: DDL emission (`CREATE TABLE ... PARTITION OF ...`).
- `internal/router/` and `internal/queue/postgres/`: hot-path reads.
- Recursive CTEs (`WITH RECURSIVE`), JSON path operators (`@>`, `?`, `jsonb_path_query`) and non-trivial window functions.
- One-off admin queries in `internal/platform/shared/*` (integrity scan, projections service helpers).

**Migrations** are plain SQL files run by goose (see below). sqlc reads them but does not own them.

### JSON: stdlib

All code, including the message router, uses stdlib `encoding/json`. (An earlier plan to use `goccy/go-json` and generated `easyjson` marshalers on the router hot path was not carried out; `goccy/go-json` appears in `go.mod` only as an indirect dependency.)

**Hard wire-contract constraint:** the router signs the payload bytes it receives — it does **not** re-serialize. Take the bytes from the queue, compute the HMAC, send. **JSON library choice is irrelevant to signing as long as we never re-serialize before signing.** Shared signature vectors (`clients/rust-sdk/tests/fixtures/wire/webhook-signatures.json`, checked by `pkg/fcsdk/webhook/vectors_test.go`) enforce this — see [`wire-contract.md`](./wire-contract.md#webhook-signatures).

If anywhere in the platform we do `Marshal(event) → sign(bytes)` (e.g., outbox event payload signing), the serializer's exact byte output becomes part of the contract with every verifier — which means stdlib only, and a fixed struct-tag posture (field ordering, null/omitempty, number representation). Audit before committing.

### Migrations: `goose`

- `pressly/goose/v3` applies the numbered files in `internal/migrate/sql/` (`NNN_subject.sql`, each starting with `-- +goose Up`), embedded into the binaries and run by `internal/migrate`.
- The ledger is goose's `goose_db_version`. Databases from earlier trackers (`_fc_migrations`, the previous platform's `_schema_migrations`) are upgraded to it on first boot without re-applying anything.
- Production is forward-only; see `CONVENTIONS.md` section 6 for the rules on `Down` sections.

### Validation

Hand-written, in each operation's `Validate` phase (`usecaseop.Operation.Validate`), using the shared helpers in `internal/platform/shared/validate`. There is no struct-tag validation library. HTTP-layer DTO checks stay minimal so the rule lives next to the operation, not the route.

### Auth

Most of the auth subdomain is RFC-compliant OAuth/OIDC protocol mechanics. The OIDC **client** side (bridging to external IDPs) leans on libraries; the OAuth/OIDC **provider** side is hand-rolled, for exact control of the wire contract.

- **`golang-jwt/jwt/v5`** for JWT encode/decode (RS256, with an HS256 dev fallback). Used directly by `authservice` (OAuth/OIDC tokens + JWKS) and `sessiontoken` (session cookies).
- **`github.com/coreos/go-oidc/v3`** + **`golang.org/x/oauth2`** for the OIDC **bridge** (FlowCatalyst as an OIDC client of Entra / Keycloak / Google). Reads `EmailDomainMapping` to route users to the right external IDP.
- **Hand-rolled OAuth/OIDC provider** (`internal/platform/auth/oauthapi`) — FlowCatalyst as an OIDC/OAuth **provider**, issuing access/refresh/ID tokens to SDK consumers (`client_credentials` grant) and users (`authorization_code` + PKCE). Owns the token / authorize / introspect / revoke / userinfo endpoints plus `.well-known/openid-configuration` and JWKS. JWT mint/validate lives in `auth/authservice`; auth-code, refresh-token, and pending-auth artifacts persist in `oauth_oidc_payloads` via `auth/grantstore`. Tokens carry FlowCatalyst-specific claims (`scope`, `clients[]`, `roles[]`, `applications[]`, `email`). Originally built on `ory/fosite`; removed 2026-05-28 (see [ADR-0001](adr/0001-session-token-vs-oauth.md)) because its storage-backed model didn't fit the platform's custom claim shapes, multi-key JWKS rotation, `plain` PKCE, and per-client rate limiting. `client_credentials` is otherwise SDK/service-account-only, but `handleClientCredentialsGrant` (`token.go`) carries one deliberate, narrowly-scoped exception: a regular USER principal holding the seeded `platform:developer` role can mint a token as themselves (`client_id` = their own principal id, no `OAuthClient` row) via a dedicated, rotatable secret on `iam_principals` — self-service local testing against a deployed environment without provisioning a service account. The developer-role check is re-verified live at every mint, not just "does a secret exist," so revoking the role cuts off new tokens immediately. **First-party CLI client:** `fcdev login` signs a person in through the built-in public client `fcdev-cli` (seeded and re-asserted at every start by `seed.SeedFcdevCLIClient`; `authorization_code` + `refresh_token` only, PKCE required, `apiAccess` so the access token carries the user's own roles and permissions). It is the only client allowed a variable port on its registered loopback redirects (`oauthapi.MatchLoopbackRedirectURI`, RFC 8252 7.3: `127.0.0.1`/`[::1]` literals, `http`, exact path, nothing else), and its refresh family has its own ABSOLUTE lifetime (`FC_FCDEV_LOGIN_REFRESH_TTL_SECS`, default 7 days; rotation never extends it). The token response gained the optional `refresh_expires_in`. See [fcdev-profiles.md](fcdev-profiles.md) and `platform-bug-fixes.md` F25.
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
- No row is ever given up on. A retryable failure returns the row to `PENDING` and holds its
  message group (ungrouped: its type, or the row alone when the rest of its batch went through)
  with an in-memory exponential backoff (1 s doubling to 5 min, jitter); attempts are not capped.
- A non-retryable failure (`BAD_REQUEST`, `FORBIDDEN`) keeps its status, its `error_message`
  prefixed `[blocking] `, and with block-on-error blocks its group until an operator unblocks or
  skips it (`[skipped by operator] ` replaces the marker). Before the first claim of each
  leadership the blocked set is rebuilt from the `[blocking] ` rows only; refusals without a marker
  (written by earlier versions, whose groups had moved on) are marked `[skipped before upgrade] `
  and never block, and rows left in a retryable failure status are swept back to `PENDING`. The
  markers, not timestamps, tell the cases apart.
- Claims pass over held, paused and blocked groups (`ClaimFilter`), and a group put back once in a
  claim stays put back for the rest of it, so a later row never overtakes a held one.

### MCP server

`internal/mcp`, built on the official `github.com/modelcontextprotocol/go-sdk`.
The SDK handles the `initialize` handshake, capability negotiation, and both
transports:
- stdio (default for `fcdev mcp`) — JSON-RPC over stdin/stdout, logs to stderr.
- streamable-HTTP — mounted at `/mcp` (default `127.0.0.1:8090`, `FC_MCP_BIND`/`FC_MCP_PORT`).
  `FC_MCP_AUTH_TOKEN`, when set, is required of every caller (`Authorization: Bearer <token>`); a non-loopback bind
  without it is logged as an error at every server start, and `fcdev mcp --http` refuses to start.

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
