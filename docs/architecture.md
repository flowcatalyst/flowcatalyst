# Architecture

Module layout and library choices.

---

## Module layout

```
flowcatalyst-go/
├── go.mod                              # module github.com/flowcatalyst/flowcatalyst-go
├── go.sum
├── api/                                # committed OpenAPI lockfile (openapi.lock.json) + field fixtures
├── cmd/                                # binary entry points
│   ├── fc-server/main.go               # unified production server (FC_*_ENABLED subsystem toggles)
│   ├── fcdev/                          # local dev monolith (embedded Postgres, start/init/fresh/mcp/outbox/fn/ingest/upgrade)
│   ├── decrypt-check/                  # operator key-verification utility
│   └── fcrunner-echo/                  # test/echo function runner helper
├── internal/                           # non-importable internals
│   ├── common/                         # shared message/dispatch types
│   ├── ids/                            # typed entity IDs (PrincipalID, ClientID, ApplicationID, OAuthClientID)
│   ├── logging/                        # slog JSON init + context trace field helpers
│   ├── migrate/                        # goose runner + embedded SQL migrations (internal/migrate/sql/)
│   ├── sqlc/                           # SQL queries (queries/) and generated typed accessors (dbq/)
│   ├── netguard/                       # outbound webhook SSRF guard (ValidateURL + post-DNS DialContext)
│   ├── secretref/                      # read-time AWS Secrets Manager & SSM Parameter Store reference resolver
│   ├── secrets/                        # in-process secret providers (env, AES-256-GCM encrypted file)
│   ├── standby/                        # Redis-backed leader election
│   ├── server/                         # unified server orchestrator (envcfg.go, wire_*.go, run.go, subsystems.go)
│   ├── queue/                          # queue abstraction
│   │   ├── queue.go                    # Consumer/Publisher interfaces
│   │   ├── sqs/                        # AWS SQS FIFO impl
│   │   ├── postgres/                   # pg_queue_messages impl (used by fcdev and Postgres-queue deployments)
│   │   └── nats/                       # NATS JetStream impl
│   ├── router/                         # message router
│   │   ├── manager.go
│   │   ├── pool.go
│   │   ├── mediator.go
│   │   ├── circuit_breaker.go
│   │   ├── ratelimit.go
│   │   ├── config_sync.go
│   │   ├── notification.go
│   │   ├── traffic.go
│   │   └── api/                        # router's HTTP endpoints (health/metrics/dashboard)
│   ├── stream/                         # CQRS stream processor
│   │   ├── projector.go
│   │   ├── fanout.go
│   │   ├── partition_manager.go
│   │   └── health.go
│   ├── outbox/                         # outbox processor
│   │   ├── processor.go
│   │   ├── group_distributor.go
│   │   ├── group_state.go
│   │   ├── http_dispatcher.go
│   │   ├── admin.go
│   │   ├── postgres/                   # pgx/v5 backend
│   │   ├── mongo/                      # mongo-driver/v2 backend
│   │   └── sqlite/                     # modernc.org/sqlite pure-Go backend
│   ├── functions/                      # in-process Wasm & QuickJS function runner (wazero)
│   │   ├── abi/
│   │   ├── budget/
│   │   ├── control/
│   │   ├── engine/
│   │   ├── jsengine/
│   │   ├── runner/
│   │   └── runtimes/
│   ├── tsid/                           # TSID generator (Crockford Base32)
│   ├── testpg/                         # embedded-Postgres integration test harness (build tag `integration`)
│   ├── ui/                             # server-rendered console (/ui, templ + Datastar, mounted beside the SPA)
│   │   ├── guard/
│   │   ├── pages/
│   │   ├── shell/
│   │   └── uiassets/
│   ├── platform/                       # the platform domain (the bulk of the code)
│   │   ├── application/                # one subdir per subdomain
│   │   │   ├── entity.go
│   │   │   ├── repository.go
│   │   │   ├── api/                    # api.go + dto.go (huma routes)
│   │   │   └── operations/             # events.go + one file per command
│   │   ├── appdocs/
│   │   ├── appupdate/
│   │   ├── appusers/
│   │   ├── audit/
│   │   ├── auth/
│   │   ├── branding/
│   │   ├── client/
│   │   ├── connection/
│   │   ├── cors/
│   │   ├── credentialpublish/
│   │   ├── dispatch/
│   │   ├── dispatchjob/
│   │   ├── dispatchpool/
│   │   ├── docsapi/
│   │   ├── emaildomainmapping/
│   │   ├── event/
│   │   ├── eventtype/
│   │   ├── fieldauth/
│   │   ├── fieldidentity/
│   │   ├── function/
│   │   ├── functiondomain/
│   │   ├── identityprovider/
│   │   ├── idp/
│   │   ├── imageproc/
│   │   ├── ingestion/
│   │   ├── keyrotation/
│   │   ├── loginattempt/
│   │   ├── mfa/
│   │   ├── notify/
│   │   ├── openapispecs/
│   │   ├── passwordreset/
│   │   ├── platformconfig/
│   │   ├── portalauth/
│   │   ├── portalidentity/
│   │   ├── principal/
│   │   ├── process/
│   │   ├── publicapi/
│   │   ├── pushgw/
│   │   ├── resetapproval/
│   │   ├── role/
│   │   ├── scheduledjob/
│   │   ├── scheduler/
│   │   ├── sdksync/
│   │   ├── seed/
│   │   ├── serviceaccount/
│   │   ├── subscription/
│   │   ├── webauthn/
│   │   └── shared/                     # cross-subdomain services
│   │       ├── apicommon/              # standard response envelopes + pagination helpers
│   │       ├── apiroute/               # huma route registration helpers
│   │       ├── auth/                   # checks, authorization context, scope filtering
│   │       ├── bff/                    # BFF API handlers
│   │       ├── clientaddr/             # trusted-proxy client IP extraction
│   │       ├── database/               # pgxpool init, DB credential rotation hooks
│   │       ├── dispatchqueue/          # queue naming and addressing
│   │       ├── email/                  # outbound email delivery
│   │       ├── encryption/             # AES-256-GCM app-key encryption + keyed hashes
│   │       ├── httpcompat/             # huma error transformer (*usecase.Error → wire envelope)
│   │       ├── httperror/              # error constructors + raw net/http writer
│   │       ├── me/                     # /api/me/* handlers
│   │       ├── middleware/             # session/bearer auth + security middleware
│   │       ├── payloadstore/           # externalized event/dispatch payload storage
│   │       ├── platformsink/           # UnitOfWork commit sink for platform aggregates
│   │       ├── ratelimit/              # per-IP and per-client rate limiters
│   │       ├── repocommon/             # shared pgx/sqlc repository helpers
│   │       ├── sdk/                    # SDK sync helpers
│   │       ├── syncdiff/               # declarative sync diffing
│   │       ├── validate/               # command validation helpers
│   │       └── versioncache/           # bounded local LRU + distributed principal version cache
│   └── mcp/                            # MCP server (stdio + streamable HTTP)
├── pkg/                                # public Go module (github.com/flowcatalyst/go-sdk)
│   └── fcsdk/
│       ├── auth/                       # OIDC + JWT validation
│       ├── cache/                      # memory/postgres/redis backends
│       ├── client/                     # platform API client
│       ├── lock/                       # distributed lock
│       ├── outbox/                     # outbox abstractions for consumer apps
│       ├── outboxpgx/                  # pgx v5 transactional outbox writer
│       ├── outboxsql/                  # database/sql transactional outbox writer
│       ├── scheduledjobs/              # consumer-side scheduled job runner
│       ├── sync/                       # declarative definition syncing
│       ├── tests/                      # nested module for SDK embedded-Postgres integration tests
│       ├── tsid/                       # Crockford Base32 TSID generator
│       ├── usecase/                    # DomainEvent, ExecutionContext, typed *usecase.Error
│       ├── usecaseop/                  # Operation[C, E], sealed Plan[E], Run (shared by platform & SDK consumers)
│       ├── usecasepgx/                 # pgx UnitOfWork implementation
│       └── webhook/                    # webhook HMAC signature verification
├── clients/                            # polyglot platform SDKs, function guest SDKs, and field session libraries
├── ingestor/                           # Java 25 / Apache Camel ingestion runtime & JVM function API
└── tools/
    ├── analyzer/                       # custom go vet analyzers (uowseal, idconv)
    ├── dump-spec/                      # OpenAPI spec dump & app/admin splitter for lockfile + SDK codegen
    ├── field-fixtures/                 # shared cryptographic test vectors for field libraries
    └── ingestor-fleet/                 # multi-runtime fault-injection fleet test harness
```

The repo builds two primary Go executables (`fc-server` and `fcdev`) rather than a separate binary per subsystem. Split production topologies run multiple `fc-server` instances with different `FC_*_ENABLED` environment toggles; `fcdev` wraps the same `server.Run` orchestrator against an embedded Postgres for local development.

---

## Library choices, with rationale

### HTTP framework: `chi` + `huma/v2`

- **chi** for routing — minimal, idiomatic `net/http`, no surprises, mature, widely used.
- **huma v2** for OpenAPI 3.1 emission from handlers. Generates the spec directly from typed input/output structs via `internal/platform/shared/apiroute` wrappers, locked in CI against `api/openapi.lock.json`.

Alternative considered: `echo` (heavier, less idiomatic), `gin` (older, less idiomatic), hand-author OpenAPI YAML + `oapi-codegen` (works but loses the spec-from-handlers workflow).

### Use-case & transaction envelope: `pkg/fcsdk/usecaseop` + `usecasepgx`

State-changing domain operations are expressed as `usecaseop.Operation[C, E]` values with `Validate`, `Authorize` (enforced statically by `tools/analyzer/uowseal` and fail-closed at runtime), and `Execute` phases. `Execute` returns a sealed `usecaseop.Plan[E]` (`Save`, `Delete`, `SaveAll`, `Sync`, or `Emit`), which `usecaseop.Run` applies atomically in one `pgx` transaction via `usecasepgx.UnitOfWork` (aggregate persistence + domain event + optional dispatch jobs + audit row). Living in `pkg/fcsdk/` allows platform code and Go SDK consumer applications to share the exact same transactional envelope. See [`usecase-pattern.md`](./usecase-pattern.md) and [`../CONVENTIONS.md`](../CONVENTIONS.md).

### Database: `sqlc` + `pgx`

Typed generated queries where they fit, hand-written pgx where they do not. Both use the same `pgxpool.Pool`.

**`sqlc`** generates the typed query layer used by the repositories under `internal/platform/*/repository.go`:
- Queries are plain SQL in `internal/sqlc/queries/`; sqlc checks them against the schema defined by the migrations in `internal/migrate/sql/` and generates typed Go in `internal/sqlc/dbq/` (config: `sqlc.yaml`).
- **Generated code is committed**, so PR diffs show schema and query changes explicitly and there is no build-time database dependency. `make sqlc` regenerates it with the pinned sqlc; `make sqlc-verify` (run in CI) fails on drift.
- Compile-time checking catches column renames and enforces typed entity IDs (`internal/ids`). Dynamic WHERE clauses are written as sqlc queries with optional-parameter predicates or, where that is awkward, as hand-written pgx.

**Raw `pgx/v5`** for code where generated queries are not a fit:
- `internal/stream/projector.go`: `FOR UPDATE SKIP LOCKED` claim queries; we want the SQL to be obviously the SQL.
- `internal/stream/partition_manager.go`: DDL emission (`CREATE TABLE ... PARTITION OF ...`).
- `internal/router/` and `internal/queue/postgres/`: hot-path reads and queue operations.
- Recursive CTEs (`WITH RECURSIVE`), JSON path operators (`@>`, `?`, `jsonb_path_query`) and non-trivial window functions.
- One-off admin queries in `internal/platform/shared/*`.

**Migrations** are plain SQL files run by goose (see below). sqlc reads them but does not own them.

### JSON: stdlib

All code, including the message router, uses stdlib `encoding/json`.

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
- **`github.com/lestrrat-go/jwx/v4`** for JWK/JWS/DPoP handling (with `go-jose/v4` pulled in transitively by `go-oidc`).
- **`go-webauthn/webauthn`** for passkeys. Its `SessionData` shape lets the in-flight ceremony be persisted.
- **`x/crypto/argon2`** for password hashing (Argon2id).
- **`crypto/aes`** + **`crypto/cipher`** for AES-256-GCM (cookie sessions, secret encryption, keyed rotation).

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

Backend implementations:
- `internal/queue/sqs` — `aws-sdk-go-v2/service/sqs` (production FIFO queues)
- `internal/queue/postgres` — the `pg_queue_messages` table (used by `fcdev` and Postgres-backed deployments)
- `internal/queue/nats` — `nats-io/nats.go` JetStream

Subsystems wire the configured backend in `internal/server/` without build tags so a single binary image supports any deployment topology.

### Outbox backends

`internal/outbox/repository.go` defines the `Repository` interface. Backends:
- `internal/outbox/postgres` — `pgx/v5`
- `internal/outbox/mongo` — `go.mongodb.org/mongo-driver/v2`
- `internal/outbox/sqlite` — `database/sql` + `modernc.org/sqlite` (pure Go, no CGo; WAL + `BEGIN IMMEDIATE` single-writer claims)

### Secrets & external secret references

Secret handling splits cleanly between read-time external references and local storage:
- **`internal/secretref`** (`docs/external-secret-references.md`) — resolves `aws-sm://...` (AWS Secrets Manager, with optional `#jsonKey` extraction) and `aws-ps://...` (AWS SSM Parameter Store) references at read time with a short TTL cache, concurrent-fetch coalescing, and owner allow-lists (`FC_SECRET_REFERENCE_ALLOW`). The platform database stores only the reference, never the plaintext secret.
- **`internal/secrets`** — in-process `Provider` registry supporting `env://` environment lookups and `encrypted:` AES-256-GCM local files, alongside `internal/platform/shared/encryption` for database-stored encrypted secrets (`FLOWCATALYST_APP_KEY`).

### Standby / leader election

`internal/standby`:
- Uses `redis/go-redis/v9` (`SET NX EX` with periodic lease refresh and monotonic leadership terms) when `FC_STANDBY_ENABLED=true`.
- Gates single-active pollers (such as outbox leadership) and exposes leadership state to health/metrics surfaces.

### Router internals

Concurrency and delivery model (`internal/router/`):
- `Manager` polls configured queues and submits messages to the target `Pool` (`pool_code`, with `DEFAULT-POOL` fallback) while tracking pipeline ownership in `InFlightTracker`.
- Each `Pool` enforces concurrency via a semaphore, per-pool token-bucket rate limiting (`RateLimiter`), and per-endpoint `CircuitBreaker` state machines.
- **Ordered groups:** strict per-message-group FIFO via `map[string]*groupQueue` protected by `sync.Mutex`. Failed retryable messages re-insert at the front so later messages in the same group never overtake them.
- **Unordered (`IMMEDIATE`) messages:** queued in an in-memory slice (`immQ`) drained by reusable `immediateWorkers` goroutines rather than spawning a goroutine per buffered message.
- **Outbound safety:** HTTP deliveries dial through `internal/netguard` (`netguard.Default` blocks loopback, link-local/cloud metadata, and private networks unless explicitly permitted; `netguard.Internal` scopes exemptions strictly to platform-owned targets like the function runner).
- Webhook payloads are signed over the exact received bytes with HMAC-SHA256 (`crypto/hmac` + `crypto/sha256`).

### Stream processor

Three independent `FOR UPDATE SKIP LOCKED` claim loops (`internal/stream/`):
- `eventProjector` — `msg_events` → `msg_events_read`.
- `fanOut` — matches subscriptions and inserts `msg_dispatch_jobs`.
- `dispatchJobProjector` — `msg_dispatch_jobs` → `msg_dispatch_jobs_read`.

Plus a `partitionManager` goroutine that runs on a 60-minute tick to ensure upcoming monthly partitions exist for the partitioned message tables (`stream.PartitionedTables`) — no `pg_partman` or `pg_cron` extension required.

### Outbox processor

Pipeline in `internal/outbox/`:
- `Processor` claims pending rows (`ClaimPending`) → `GroupDistributor` routes items by `message_group` with `GroupStateManager` → `HTTPDispatcher` sends batches to the FlowCatalyst HTTP API.
- Backpressure via `atomic.Int64` in-flight counter; polling pauses at `MaxInFlight`.
- No row is ever given up on. A retryable failure returns the row to `PENDING` and holds its message group (ungrouped: its type, or the row alone when the rest of its batch went through) with an in-memory exponential backoff (1 s doubling to 5 min, with jitter); attempts are not capped.
- A non-retryable failure (`BAD_REQUEST`, `FORBIDDEN`) keeps its status, its `error_message` prefixed `[blocking] `, and with `BlockOnError` blocks its group until an operator unblocks or skips it (`[skipped by operator] ` replaces the marker). Before the first claim of each leadership term, the blocked set is rebuilt from `[blocking] ` rows, legacy unmarked refusals are marked `[skipped before upgrade] `, and rows left in a retryable failure status are swept back to `PENDING`.
- Claims pass over held, paused, and blocked groups (`ClaimFilter`), and a group put back once in a claim stays put back for the rest of it, so a later row never overtakes a held one.
- Ungrouped rows of one type are sent in requests cut at `BatchSize` or `MaxRequestBytes` (8 MiB, `FC_OUTBOX_MAX_REQUEST_BYTES`), in order; a larger row goes alone, a `413` splits a request in two, and a single row refused `413` is terminal. Grouped rows are always sent one per request so neither cutting nor splitting can reorder a message group.

### Function runner (`internal/functions/`)

Runs operator-published serverless functions in-process on `tetratelabs/wazero` (`docs/function-runner-plan.md`):
- Dual `wazero` runtimes sharing a disk compilation cache: a preemptible runtime (`WithCloseOnContextDone(true)`) for compiled Wasm guests (Go, Rust) and a cooperative runtime for the embedded QuickJS engine (bounded via `fc_js_deadline` + host watchdog).
- Guest linear memory is charged against `internal/functions/budget`.

### Console UI (`internal/ui/` and `internal/consoleredirect/`)

- **Server-rendered console (`internal/ui/`):** `templ` components + Datastar mounted under `/ui`, built on `github.com/flowcatalyst/framework/go/ui`. Generated `*_templ.go` files are committed and verified in CI (`make ui-verify`).
- **Legacy URL redirector (`internal/consoleredirect/`):** standalone fallback handler redirecting root and former SPA browser routes (`/`, `/dashboard`, `/dispatch-jobs`, `/auth/login?...`, etc.) to their `/ui` equivalents while preserving JSON `404` envelopes on unknown API paths.

### MCP server

`internal/mcp`, built on the official `github.com/modelcontextprotocol/go-sdk`:
- **stdio** (default for `fcdev mcp`) — JSON-RPC over stdin/stdout, logs to stderr.
- **streamable-HTTP** — mounted at `/mcp` (default `127.0.0.1:8090`, `FC_MCP_BIND`/`FC_MCP_PORT`). `FC_MCP_AUTH_TOKEN`, when set, is required of every caller (`Authorization: Bearer <token>`); a non-loopback bind without it refuses to start.

Read-only tool and resource surface (`list_event_types`, `get_event_type`, `get_schema`, `list_subscriptions`, `get_subscription`, `list_applications`, `list_roles`, `get_role`, `get_openapi`, `whoami`, `list_my_applications`, `get_application_capabilities`, and `flowcatalyst://...` resources), authenticated via OAuth2 `client_credentials` with an in-memory token cache.

---

## Cross-cutting concerns

### Logging

`log/slog` JSON handler configured in `internal/logging` (`FC_LOG_LEVEL`, default `info`). Standard structured fields:
- `level`, `time`, `msg`
- `correlation_id`, `causation_id`, `principal_id`, `execution_id`
- `aggregate_type`, `aggregate_id`, `event_type`

Request-scoped trace fields are attached and retrieved via `logging.WithCorrelationID` / `logging.FromContext(ctx)` in `internal/logging/logging.go`.

### Metrics

`prometheus/client_golang` counters, gauges, and histograms across HTTP handlers, router pools/circuit breakers/queues, scheduler, stream projections, secret resolution, and credential publishing. Exposed on `/metrics` (and `/ready`) on `FC_METRICS_PORT` (default `9090`).

### Diagnostics & Execution Tracing

Correlation, causation, and execution IDs propagate through `usecase.ExecutionContext` into every domain event and audit log row. Subsystems with complex runtime behaviour — functions (`internal/functions/runner/trace.go`), ingestions (`docs/ingestion-trace-design.md`), and push (`internal/platform/pushgw`) — provide bounded, opt-in diagnostic trace capture inspectable from the UI and `fcdev`.

### Configuration

Configuration is 100% environment-variable driven — loaded and validated at startup by `internal/server/envcfg.go` (`server.LoadEnv()`), with no TOML/YAML server config files. Canonical variables use the `FC_` and `FLOWCATALYST_` prefixes (with legacy aliases preserved for task-definition compatibility). In local development, `fcdev init` writes application bootstrap credentials to `.env`. See [`environment-variables.md`](./environment-variables.md) for the complete reference.
