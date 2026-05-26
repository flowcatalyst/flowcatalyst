# FlowCatalyst Go — Reimplementation Plan

A port of [`flowcatalyst-rust`](../flowcatalyst-rust/) to idiomatic Go, designed as a **drop-in replacement on the same Postgres schema, with byte-compatible HTTP APIs**, so existing SDK consumers and the existing Vue frontend continue to work unchanged.

> **Source of truth.** This document is the canonical plan. Supporting detail lives in [`docs/`](./docs/). Conventions ported from the Rust [`CLAUDE.md`](../flowcatalyst-rust/CLAUDE.md) live in [`docs/conventions.md`](./docs/conventions.md) — read that before writing any code.

---

## 1. Non-negotiables

These are the constraints that shape every decision below. If a future change conflicts with one of these, the change is wrong.

1. **Same Postgres schema, same migrations.** The Go server runs against the existing `flowcatalyst-rust/migrations/*.sql` files unchanged. Schema evolution from this point onward is shared between the two codebases until Rust is retired.
2. **Same HTTP routes, same JSON shapes.** Every `/api/*`, `/bff/*`, `/auth/*`, `/oauth/*`, `/.well-known/*` route returns a response byte-identical (modulo whitespace) to the Rust server. Verified by contract tests — see [`docs/api-parity.md`](./docs/api-parity.md).
3. **Same OpenAPI spec.** The frontend uses `@hey-api/openapi-ts` to generate its TypeScript client from the platform spec. The Go server emits a spec compatible with that generator. The frontend ships unchanged.
4. **No fork of the schema migration sequence.** Migrations live in `flowcatalyst-rust/migrations/` (the canonical location) until cutover; the Go binary references them via path or copies them as part of its build. After cutover, ownership moves here.
5. **The UoW seal is compile-time enforced.** Every write goes through `UseCase.Run()` → `UnitOfWork.Commit()`. The pattern is enforced by Go's type system, not by convention. See [`docs/usecase-pattern.md`](./docs/usecase-pattern.md).

---

## 2. Scope

### In scope
- All seven Rust binaries: `fc-server`, `fc-platform-server`, `fc-router`, `fc-stream-processor`, `fc-outbox-processor`, `fc-mcp-server`, `fc-dev`.
- All twelve Rust crates: `fc-common`, `fc-config`, `fc-queue`, `fc-router`, `fc-standby`, `fc-outbox`, `fc-stream`, `fc-platform`, `fc-secrets`, `fc-sdk`, `fc-mcp`, and (renamed) `fc-test-helpers`.
- All 103 use cases, all 348 HTTP handlers, all queue/outbox/secret backends.
- The Rust SDK becomes a Go SDK (`pkg/fcsdk`) with the same feature surface.

### Out of scope
- **Frontend.** `flowcatalyst-rust/frontend/` is copied into `flowcatalyst-go/frontend/` unchanged. Only its generated API client is regenerated against the Go server's OpenAPI spec — and it must produce byte-identical TypeScript output (this is a contract test).
- **TypeScript and Laravel SDKs.** They consume the public HTTP API. Since the contract is preserved, they ship unchanged.
- **Schema changes.** No new tables, no column renames, no migration additions during the port. Schema changes happen *after* cutover.
- **Architectural redesign.** This is a port, not a rewrite-from-design. Same layering (handler → use case → domain → repository), same aggregate boundaries, same module structure.

---

## 3. Approach

**Strangler-fig cutover, per-binary.** All seven binaries share a Postgres database. Once the Go `fc-platform-server` passes parity tests, you can swap traffic to it while Rust still runs the router/stream/outbox. Then cut those over one at a time. Rust binaries remain available as a rollback target for 2 weeks after each cutover.

Cutover order (lowest risk first):
1. `fc-mcp-server` — read-only, trivial blast radius.
2. `fc-platform-server` — read paths first via canary, then writes once contract tests are green.
3. `fc-stream-processor` — idempotent projections; safe to run both for a few minutes during cutover.
4. `fc-outbox-processor` — at-most-once with idempotency keys; cutover by stopping Rust, starting Go.
5. `fc-router` — last; most stateful (in-flight SQS messages, circuit breaker state). Drain to zero in-flight before swapping.
6. `fc-server` — once all the above are green, the monolithic binary is just composition.
7. `fc-dev` — developer experience; built last but launched alongside the team for dogfooding from week 2.

---

## 4. Phases

Each phase has a definition of done. Don't skip — phase N+1 depends on phase N being green.

### Phase 0 — Foundation (week 1)

- [ ] Initialize Go module: `github.com/flowcatalyst/flowcatalyst-go` (or final org name).
- [ ] Repo layout matching [`docs/architecture.md`](./docs/architecture.md): `cmd/`, `internal/`, `pkg/`.
- [ ] Toolchain config: `golangci-lint`, `gofumpt`, `goimports`, `go vet`, custom analyzer for UoW seal (see [`docs/usecase-pattern.md`](./docs/usecase-pattern.md)).
- [ ] CI: `go test ./...`, `golangci-lint`, integration tests gated on `testcontainers-go` Postgres.
- [ ] Logger: `log/slog` JSON handler with the same field names as Rust `tracing` JSON output (`correlation_id`, `causation_id`, `principal_id`).
- [ ] OpenAPI spec contract test scaffold — fails on first commit, passes when spec parity is reached.

**Done when:** `go test ./...` runs (with zero tests), CI is green, a placeholder `cmd/fc-platform-server/main.go` binds to `:3000` and serves `/health`.

### Phase 1 — Core abstractions (weeks 2–3)

- [ ] `internal/usecase/` — `Result[E]`, sealed `success[E]`/`failure[E]`, type-state wrappers `validated[C]`/`authorized[C]`, `UnitOfWork` interface, `Persist[A]` interface, `HasID` interface. See [`docs/usecase-pattern.md`](./docs/usecase-pattern.md).
- [ ] `internal/usecase/uow_postgres.go` — `PgUnitOfWork`, `TxScopedUnitOfWork`. Same SQL as Rust UoW (insert into `msg_events` + `iam_audit_logs` in one tx).
- [ ] `internal/common/` — port `fc-common` types (`Message`, `QueuedMessage`, `DispatchMode`, `MediationResult`, …) as plain Go structs with JSON tags.
- [ ] `internal/tsid/` — port the Crockford Base32 TSID generator. Cross-check output against Rust generator with golden tests.
- [ ] `internal/config/` — port `fc-config` (TOML loader). `BurntSushi/toml`.
- [ ] `internal/secrets/` — port `fc-secrets`. Provider registry: env, encrypted file, AWS Secrets Manager, AWS SSM, Vault.
- [ ] `internal/standby/` — port `fc-standby`. Redis SET NX + lease loop, exposes `IsLeader()` and a `<-chan LeadershipChange`.
- [ ] `internal/queue/` — port `fc-queue`. Backend interface + concrete impls for SQS, Postgres, SQLite, NATS, AMQP/ActiveMQ. Registered at runtime, not behind build tags.

**Done when:** golden tests pass against Rust outputs for TSID and `Message` JSON. `internal/usecase` has a unit test proving `success[E]` cannot be constructed from another package.

### Phase 2 — fc-router (weeks 4–6, can run parallel to Phase 3)

- [ ] `internal/router/manager.go` — per-pool drain goroutines.
- [ ] `internal/router/pool.go` — message-group FIFO via per-group queue + `chan struct{}` work signal.
- [ ] `internal/router/ratelimit.go` — sharded `*rate.Limiter` map with hot-swap on config reload.
- [ ] `internal/router/circuitbreaker.go` — port the Rust circuit breaker state machine (`Closed`/`Open`/`HalfOpen` + sliding window).
- [ ] `internal/router/mediator.go` — HTTP webhook delivery with HMAC-SHA256 signing.
- [ ] `internal/router/configsync.go` — config hot-reload from URL list.
- [ ] `internal/router/notification.go` — Teams webhook batching for warnings.
- [ ] `cmd/fc-router/main.go` — env-var wiring matching Rust binary's flags.

**Done when:** the Go router can be pointed at the dev SQS+Postgres setup and deliver a webhook with the same headers, signature, retry behavior, and circuit-breaker state transitions as the Rust router. Verified via test that captures Rust output, replays the same input through Go, and diffs.

### Phase 3 — fc-platform domain ports (months 2–5)

This is the long pole. 27 subdomains × ~3–5 use cases each = ~103 use case files to port. Plus 348 HTTP handlers.

**Order** (low-coupling subdomains first, high-coupling last). Each subdomain ships behind a feature flag (`FC_GO_DOMAIN_<NAME>=true`) on a single shared binary so traffic can canary per-domain.

| Wave | Subdomains | Why this order |
|---|---|---|
| 3a | `cors`, `email_domain_mapping`, `connection`, `event_type` | Leaf aggregates, minimal cross-domain calls |
| 3b | `subscription`, `dispatch_pool`, `process`, `application` | Depend on 3a |
| 3c | `role`, `service_account`, `client`, `principal` | IAM core; depend on 3b |
| 3d | `auth` (oauth/oidc_login/config/identity_provider/idp), `webauthn` | Depends on principal + IAM |
| 3e | `scheduled_job`, `platform_config`, `audit`, `login_attempt`, `password_reset` | Cross-cutting + admin |
| 3f | `dispatch_job`, `event`, `seed` | Infrastructure-processing tables (bypass UoW; see [`docs/conventions.md`](./docs/conventions.md) §3) |
| 3g | `shared/` services + `scheduler/` (poller, dispatcher, stale_recovery) | Plumbing that depends on everything else |

For each subdomain, the port produces (in order):
1. `internal/platform/<name>/entity.go` — pure Go structs, JSON tags, status enums as `string` typedef + `Valid()` method.
2. `internal/platform/<name>/repository.go` — `pgx`-based repo with handwritten SQL copied from the Rust repo's SQL strings.
3. `internal/platform/<name>/operations/*.go` — one file per use case. Implements `UseCase[Command, Event]`. `Execute` returns the result of `uow.Commit(...)`.
4. `internal/platform/<name>/api.go` — huma handler registrations. Permission checks. Build command, call `UseCase.Run(...)`, convert result to HTTP.

**Definition of done per subdomain:**
- Contract test green: Rust and Go return byte-identical JSON for the same input across all routes the subdomain owns.
- Integration test green: full happy path + every documented error case, against real Postgres via testcontainers.
- Behind a feature flag in `cmd/fc-platform-server/main.go` so it can be enabled per-environment.

### Phase 4 — fc-stream & fc-outbox (weeks 14–16)

- [ ] `internal/stream/projector.go` — three parallel projection loops (events, dispatch jobs, fan-out). `FOR UPDATE SKIP LOCKED` claim queries via `pgx`.
- [ ] `internal/stream/partition_manager.go` — monthly RANGE-partition manager. Port the Rust SQL DDL emitters verbatim.
- [ ] `internal/outbox/processor.go` — buffer + group distributor + HTTP dispatcher. Multi-backend (Postgres / SQLite / MySQL / Mongo).
- [ ] `cmd/fc-stream-processor/main.go`, `cmd/fc-outbox-processor/main.go`.

**Done when:** running the Go stream processor against a database with pending events produces identical projection rows to the Rust one. Outbox: same.

### Phase 5 — fc-sdk and fc-mcp (weeks 17–18)

- [ ] `pkg/fcsdk/` — public SDK: outbox helpers, platform API client, auth (OIDC + JWT validation), cache, lock, scheduled-jobs runner, webhook signature verification, optional `chi`/`huma` integration.
- [ ] `internal/mcp/` + `cmd/fc-mcp-server/` — read-only MCP server using `mark3labs/mcp-go`. stdio and streamable-HTTP transports.

**Done when:** the SDK's outbox unit-of-work writes byte-identical `outbox_messages` rows to the Rust SDK. MCP server returns the same tool listings and tool call results.

### Phase 6 — fc-dev + fc-server (weeks 19–20)

- [ ] `cmd/fc-server/main.go` — composes all subsystems behind toggle env vars (`FC_PLATFORM_ENABLED`, `FC_ROUTER_ENABLED`, …). Leader-aware via the standby package.
- [ ] `cmd/fc-dev/main.go` — embedded Postgres via `fergusstrange/embedded-postgres`. Bundles the same subcommands: `start`, `init`, `fresh`, `mcp`, `outbox`, `upgrade`.
- [ ] Frontend asset embedding via `embed.FS`. SPA fallback + cache headers (`Cache-Control: public, max-age=31536000, immutable` for hashed `/assets/*`, default for non-hashed).
- [ ] Single-binary release builds for darwin/arm64, darwin/amd64, linux/amd64, linux/arm64, windows/amd64.

**Done when:** `fc-dev start` brings up the full system (embedded PG + all subsystems) and the existing frontend works against it with no code change.

### Phase 7 — Cutover (weeks 21–22)

For each binary, in the order listed in §3:
1. Deploy Go binary alongside Rust binary in staging. Both connected to the same database.
2. Run shadow traffic for 24h. Compare outputs.
3. Promote Go to primary in staging. Rust runs as warm standby for 48h.
4. If green, promote in prod (canary 1% → 10% → 50% → 100% over a week).
5. Decommission Rust binary 2 weeks after 100% on Go.

**Done when:** all seven binaries are Go in production. Rust workspace is archived (not deleted; kept for reference for 6 months).

---

## 5. Engineering ground rules

These mirror the Rust [`CLAUDE.md`](../flowcatalyst-rust/CLAUDE.md) — translated to Go. Full detail in [`docs/conventions.md`](./docs/conventions.md).

1. **Layering** — handler → use case → domain → repository. No SQL outside `repository.go`. No HTTP types in `operations/`. No domain types in `repository.go`. Enforced by `go-arch-lint` or equivalent.
2. **UoW invariant** — every write goes through a `UseCase.Run()` that ends in `uow.Commit(...)`. The seal pattern in `internal/usecase` enforces this at compile time. See [`docs/usecase-pattern.md`](./docs/usecase-pattern.md).
3. **Reads in handlers are fine.** GET endpoints call repositories directly.
4. **N+1 is banned.** Batch loads using `WHERE id = ANY($1)` and group in memory. Same rule as Rust.
5. **No mocking of the database in tests.** Use `testcontainers-go` for real Postgres. (Same reason as Rust: prior incidents where mocks passed but migrations broke.)
6. **`fetch_one` equivalent (`QueryRow.Scan`) is forbidden unless mathematically guaranteed.** Use `pgx.RowToStructByName` + `errors.Is(err, pgx.ErrNoRows)` and handle the missing case.
7. **Concurrent reads use `errgroup.Group`.** Equivalent of Rust's `tokio::try_join!`.
8. **Permission checks live in `internal/platform/shared/auth/checks.go`** and follow the same naming convention as Rust (`CanReadEventTypes`, `CanWriteSubscriptions`, `RequireAnchor`, etc.). Every write handler must call exactly one.
9. **No `interface{}` / `any` in domain code** except for `serde_json::Value` equivalents (`json.RawMessage`). Generics over `interface{}` is a smell.
10. **JSON tags use `camelCase`** to match Rust's `#[serde(rename_all = "camelCase")]`. Enums use `SCREAMING_SNAKE_CASE` constants.

---

## 6. Library choices (the short list)

Full rationale and alternatives in [`docs/architecture.md`](./docs/architecture.md).

| Concern | Library |
|---|---|
| HTTP router | `github.com/go-chi/chi/v5` |
| OpenAPI from handlers | `github.com/danielgtaylor/huma/v2` |
| Postgres driver | `github.com/jackc/pgx/v5` (+ `pgxpool`) |
| Type-safe SQL | `github.com/go-jet/jet/v2` for CRUD repositories; raw pgx for `internal/router/`, `internal/stream/`, partition DDL, recursive CTEs |
| Migrations | `github.com/golang-migrate/migrate/v4` |
| JSON (default) | stdlib `encoding/json` |
| JSON (router hot path) | `github.com/goccy/go-json` drop-in; `github.com/mailru/easyjson` codegen for `common.Message`, `common.QueuedMessage`, `common.MediationOutcome` |
| Validation | `github.com/go-playground/validator/v10` |
| JWT | `github.com/golang-jwt/jwt/v5` |
| OIDC client (external IDP bridge) | `github.com/coreos/go-oidc/v3` + `golang.org/x/oauth2` |
| OIDC/OAuth provider (issuing our own tokens) | `github.com/ory/fosite` — replaces ~12k LOC of Rust protocol mechanics with ~2k LOC of storage adapter. Chosen over zitadel/oidc because fosite has much broader independent OSS adoption beyond its parent company (Ory), lowering single-vendor concentration risk. |
| JWT / JWK / JWS primitives | `github.com/go-jose/go-jose/v4` (originally Square, now community-maintained). Used transitively by fosite; explicit usage in our own signing/verification helpers. |
| WebAuthn | `github.com/go-webauthn/webauthn` |
| Argon2 | `golang.org/x/crypto/argon2` |
| AWS | `github.com/aws/aws-sdk-go-v2` |
| Redis | `github.com/redis/go-redis/v9` |
| AMQP | `github.com/rabbitmq/amqp091-go` |
| NATS | `github.com/nats-io/nats.go` |
| MongoDB | `go.mongodb.org/mongo-driver/v2` |
| SMTP | `github.com/wneessen/go-mail` |
| Rate limit | `golang.org/x/time/rate` |
| Circuit breaker | hand-ported (state machine is ~300 LOC) |
| Cron | `github.com/robfig/cron/v3` |
| UUID | `github.com/google/uuid` |
| Histograms | `github.com/HdrHistogram/hdrhistogram-go` |
| Prometheus | `github.com/prometheus/client_golang` |
| Logging | stdlib `log/slog` |
| Test containers | `github.com/testcontainers/testcontainers-go` |
| MCP | `github.com/mark3labs/mcp-go` |
| Embedded Postgres (fc-dev) | `github.com/fergusstrange/embedded-postgres` |
| Hot reload (dev) | `github.com/air-verse/air` |

---

## 7. Timeline & resourcing

|  | Solo (1 engineer) | Pair (2 engineers) | Squad (3 engineers) |
|---|---|---|---|
| Phase 0 | 1 wk | 1 wk | 1 wk |
| Phase 1 | 2 wk | 1.5 wk | 1 wk |
| Phase 2 (router) | 3 wk | 2 wk | 2 wk |
| Phase 3 (platform, 27 subdomains) | 14 wk | 8 wk | 5 wk |
| Phase 4 (stream + outbox) | 3 wk | 2 wk | 1.5 wk |
| Phase 5 (sdk + mcp) | 2 wk | 1.5 wk | 1 wk |
| Phase 6 (fc-dev + fc-server) | 2 wk | 1.5 wk | 1 wk |
| Phase 7 (cutover, including bake time) | 3 wk | 3 wk | 3 wk |
| **Total** | **30 wk** (~7 months) | **20 wk** (~5 months) | **15 wk** (~3.5 months) |

Phases 2 and 3 can run in parallel — the router doesn't depend on platform use cases. Phase 3 waves can also run in parallel within a wave (3a's four subdomains are independent of each other).

**Risk multipliers** (apply to the platform/use case work specifically — not router/stream/outbox):
- ×1.0 if the engineers have written both Rust and Go production code.
- ×1.3 if they've only written Go.
- ×1.5 if they've only written one of the two and need to read Rust to port from.

---

## 8. Risks (ranked)

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| OpenAPI spec drift breaks frontend codegen | Medium | High | Contract test in CI from day 1: diff Go spec against Rust spec, fail on any drift in route/parameter/response shape. See [`docs/api-parity.md`](./docs/api-parity.md). |
| UoW seal weaker in Go than in Rust | Low (with type-state) | Medium | Type-state + seal + custom analyzer + code review. See [`docs/usecase-pattern.md`](./docs/usecase-pattern.md). |
| Subtle JSON diffs (field order, optional-null vs missing) break SDKs | High | High | Byte-level contract tests on representative payloads. Use `omitempty` to match Rust's `skip_serializing_if`. Where Rust omits the field for `None`, Go must too. |
| Webhook signature differences | Low | High | Test vector pinned in `pkg/fcsdk/webhook/testdata/` — both Rust and Go must produce the byte-identical HMAC for the same input. |
| Outbox / dispatch ordering bugs after cutover | Medium | High | Drain to zero in-flight before swapping (§3, item 5). Shadow traffic in staging for 48h. |
| Postgres connection pool sizing differs | Low | Medium | `pgxpool` defaults differ from `sqlx`. Port the pool config (max conns, idle, max lifetime) verbatim. |
| Migration drift if both binaries run migrations | Medium | High | Only Rust runs migrations until cutover. Go reads `_schema_migrations` but doesn't write. After cutover, ownership transfers. |
| `tokio::select!` semantics translate imperfectly to Go `select` | Low | Low | Audit the 4–5 multi-branch selects in `fc-router` carefully. Most are 2-branch (work + shutdown), trivially direct. |
| Frontend openapi-ts regenerates with different TypeScript | Medium | Medium | Pin the openapi-ts version; commit the generated client; run codegen in CI and fail on diff. The Vue code itself doesn't change. |
| WebAuthn ceremony state shape differs | Low | Medium | Drain in-flight registration/authentication ceremonies (≤2 min lifetime) before cutover. No persistent state to migrate. |
| Bus factor — only one engineer knows the Rust internals | Medium | Medium | Pair on first 2–3 subdomain ports. Document non-obvious Rust patterns inline in Go (`// rust: see fc-platform/src/...`). |

---

## 9. What "drop-in" means precisely

A consumer of the existing system (the Vue frontend, a TypeScript SDK user, a webhook subscriber, a Laravel SDK user, an `fc-outbox-processor` from a customer's app) must observe **no change** when traffic moves from the Rust binary to the Go binary. Concretely:

- **HTTP routes:** same paths, same methods, same query parameter names, same path parameter positions.
- **Request bodies:** same field names (camelCase), same nullable/required posture, same enum string values (SCREAMING_SNAKE_CASE).
- **Response bodies:** same field names, same field ordering does NOT matter (JSON unordered), same omission posture for nullables (`null` vs missing must match Rust), same enum strings, same TSID format, same timestamp format (RFC3339 with microseconds).
- **HTTP status codes:** same status codes for same outcomes, including the difference between 400 / 403 / 404 / 409 / 422.
- **Error response shape:** same JSON envelope (`{ "code": "...", "message": "...", "details": {...} }`).
- **Auth:** same JWT shape (claims, alg=RS256), same key IDs, same JWKS endpoint format.
- **Webhook headers:** same `X-Fc-Signature`, same `X-Fc-Event-Type`, same `X-Fc-Message-Id`, same canonicalization of the signed payload.
- **OAuth flows:** same authorization code, refresh token, and PKCE handling. Same OIDC callback URL shape.
- **Outbox row shape:** same column types in `outbox_messages`; SDK clients run unchanged.

[`docs/api-parity.md`](./docs/api-parity.md) is the authoritative document on parity testing.

---

## 10. Decisions

Resolved before Phase 0:

1. ✅ **Module path:** `github.com/flowcatalyst/flowcatalyst-go`.
2. ✅ **SQL strategy:** `go-jet/v2` for CRUD repositories; raw `pgx` for `internal/router/`, `internal/stream/`, partition DDL, recursive CTEs. Generated jet model code is committed. See [`docs/architecture.md`](./docs/architecture.md#database-go-jet--raw-pgx).
3. ✅ **Generics in repository layer:** generic `Persist[A HasID]` interface in `internal/usecase`; each domain repository is written independently (no generic `Repository[T]` base). Generic UoW signatures (`Commit[A, E, C]`).
4. ✅ **Testing framework:** stdlib `testing` + `github.com/stretchr/testify/assert` + `github.com/stretchr/testify/require`. `testcontainers-go` for integration. No Ginkgo.
5. ✅ **OpenAPI tooling:** `huma/v2`.
6. ✅ **MCP transport:** `github.com/mark3labs/mcp-go` (covers stdio + streamable-HTTP). Verified during planning.
7. ✅ **Repo strategy:** `flowcatalyst-go/` is a new git repo, sibling to `flowcatalyst-rust/`. One Go module at the root; all binaries and packages live as subdirectories of `cmd/`, `internal/`, `pkg/`. After cutover, `flowcatalyst-rust/` is archived and this repo is renamed to `flowcatalyst/`.
8. ✅ **JSON strategy:** stdlib `encoding/json` default; `goccy/go-json` drop-in in `internal/router/` + `internal/queue/`; `mailru/easyjson` codegen for `common.Message`, `common.QueuedMessage`, `common.MediationOutcome`.
9. ✅ **HMAC audit:** complete. Only one site signs serialized JSON (`MediationPayload = {messageId: string}`); JSON library choice is irrelevant for parity. Test vector pinned in [`docs/api-parity.md`](./docs/api-parity.md#hmac-signing-sites-audit-result).
10. ✅ **Migrations:** Rust binary applies them until cutover; Go reads-only against the existing `_schema_migrations` table. `golang-migrate/migrate/v4` after cutover.

---

## 11. Pointers

- [`docs/conventions.md`](./docs/conventions.md) — engineering conventions ported from Rust `CLAUDE.md`
- [`docs/architecture.md`](./docs/architecture.md) — full crate→package mapping and module layout
- [`docs/usecase-pattern.md`](./docs/usecase-pattern.md) — the seal + type-state pattern, with `event_type/create` as worked example
- [`docs/api-parity.md`](./docs/api-parity.md) — parity strategy and contract testing
- [`../flowcatalyst-rust/CLAUDE.md`](../flowcatalyst-rust/CLAUDE.md) — the source-of-truth conventions document for the Rust system
- [`../flowcatalyst-rust/ARCHITECTURE.md`](../flowcatalyst-rust/ARCHITECTURE.md) — system overview
- [`../flowcatalyst-rust/docs/`](../flowcatalyst-rust/docs/) — per-component architecture deep-dives
