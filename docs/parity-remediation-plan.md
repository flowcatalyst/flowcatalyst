# FlowCatalyst Go — Rust Parity Remediation Plan

_Created 2026-05-29. Source: full read-only parity audit (Go `flowcatalyst-go` vs Rust reference `flowcatalyst-rust`). This plan tracks closing the behavioural/operational gaps found in that audit._

## Decisions baked in (from project owner)

- **Port:** `8080` is the canonical default. We do **not** chase Rust's `3000`; instead fix Go's internal inconsistency and the docs.
- **Binaries:** keep the single `fc-server` binary with `FC_*_ENABLED` toggles. No standalone service binaries required.
- **Compatibility:** Go is a **replacement that must drop into existing, populated databases** without breaking existing systems that use the SDK / public APIs. External contracts — SDK, public APIs, config wire-shapes (`queueName`/`queueUri`), and any **shared DB/queue/outbox schema** — MUST stay interoperable. BFF/internal shapes (casing, list wrappers) may deviate. Go migrations must be safe to apply over an already-provisioned upstream schema.
- **Backends:** router stays SQS/NATS/Postgres; outbox = Postgres **+ add MongoDB**. SQLite/ActiveMQ/MySQL are out of scope.
- **Ops:** in scope now — AWS Secrets Manager DB mode, ALB self-registration, Docker/compose.

## Guiding constraints (every phase)

1. **Drop-in safety is the prime directive.** Anything that reads/writes a *shared* table, a *public/SDK* API, or a *config wire payload* must be byte/shape-compatible with the running Rust system. Internal/BFF shapes are free to differ.
2. **Migrations must be idempotent + additive + guarded** — no-op cleanly on an already-populated upstream DB; no destructive `ALTER`/`DROP` on shared tables.
3. **Crypto outputs must remain cross-readable** (already true; one prefix fix pending).
4. Every behavioural fix gets a **golden parity test** (extend `parityharness`).

---

## Phase 0 — De-risk & verify `[S]`

| ID | Task | Why |
|---|---|---|
| V1 | Confirm router config wire field names (`queueName`/`queueUri` vs `name`/`uri`) and exact shape. | Owner confirmed it MUST be interoperable → definite fix (S2); nail the exact shape/aliases. |
| V2 | Confirm whether the permission-string mismatch (`"READ_EVENT_TYPES"` vs `platform:messaging:event-type:view`) is a live lockout for non-anchor principals. | If real, Phase 4 becomes urgent. |
| V3 | Diff Go outbox schema/queries vs SDK `clients/typescript-sdk/migrations/postgresql/001_create_outbox_messages.sql`. | Confirms exact column contract for S3. |
| V4 | Audit existing WebAuthn credential blob format in a populated DB. | Determines whether drop-in locks out existing passkey users (A4b). |

## Phase 1 — Drop-in schema & wire compatibility `[L]` (FOUNDATIONAL)

| ID | Task | Target | Source of truth |
|---|---|---|---|
| S1 | Postgres queue table → match upstream `queue_messages` (`visible_at BIGINT`, batch receipt handle, `message_group_id` index). | `internal/queue/postgres` | Rust `postgres.rs:35-58` |
| S2 | Config wire-shape → accept `queueName`/`queueUri` (+ `name`/`uri` aliases). | `internal/router/config.go`, `config_sync.go` | Rust `config_sync.rs:97-117` |
| S3 | Outbox table → SDK customer schema (`type`, `payload TEXT`, `retry_count`, `error_message`, `client_id`, `payload_size`, `headers`); delete-on-success. | `internal/outbox/postgres` | SDK migration + Rust `postgres.rs:336-356` |
| S4 | Migration drop-in audit: idempotent/guarded; reconcile Go sqlc column expectations against upstream schema; decide migration-ledger strategy. | `internal/migrate/sql`, `internal/sqlc` | Rust `migrations/` |
| S5 | OAuth secret stored-string: prepend `"encrypted:"` on write; fix stale "Argon2" comment. | `auth/operations/oauth_client.go`, `auth/repository.go:45` | Rust `oauth_clients_api.rs:251` |

## Phase 2 — OIDC finish `[M]` (priority #1)

- **O1** `end_session_endpoint` + persist `post_logout_redirect_uris`. (Rust `oidc_login_api.rs:1491`)
- **O2** `POST /auth/refresh`. (Rust `auth_api.rs:539`)
- **O3** `?provider=` direct-IDP authorize branch. (Rust `oauth_api.rs:504`)
- **O4** in-memory per-client governor on `/oauth/token` + RFC-6749 429 body. (Rust `oauth_api.rs:791`)
- **O5** enforce `max_age` (expose `iat` to session validation). (Rust `oauth_api.rs:425`)
- **O6** `GET /auth/check-domain` query variant. (Rust `auth_api.rs:424`)
- **O7** document `/auth/*` in `api/openapi.lock.json`.

## Phase 3 — Message router behavioural parity `[L]` (priority #3)

- **R1** branch `ProcessPool.submit` on `DispatchMode` (IMMEDIATE → concurrent). (Rust `pool.rs:332`)
- **R2** route by `message.pool_code` + DEFAULT-POOL fallback. (Rust `manager.rs:1095`)
- **R3** failure-rate circuit breaker. (Rust `circuit_breaker_registry.rs:136`)
- **R4** external-requeue dedup. (Rust `manager.rs:1042`)
- **R5** config-sync multi-URL + retry. (Rust `config_sync.rs:193-301`)
- **R7** stalled-consumer auto-restart. (Rust `lifecycle.rs:186`)
- **R8** align Prometheus metric names/labels. (Rust `router_metrics.rs`)

_(R6/R10 handled in Phase 1; R9 out of scope.)_

## Phase 4 — IAM / authz correctness & security `[M]` (urgency set by V2)

- **A1** permission resolution + wildcard matcher (if V2 confirms). (`shared/auth/auth.go`)
- **A2** per-resource scope checks on by-ID mutations (systemic sweep). (Rust `check_scope_access`)
- **A3** authorization on connection update/delete. (`connection/api/api.go:182,195`)
- **A4a** WebAuthn credential delete ownership check. (`webauthn/api/api.go:338`)
- **A4b** WebAuthn credential-blob compatibility (if V4 confirms).
- **A5** password-reset flow with hex SHA-256 token hashing. (Rust `password_reset_api.rs:157`)

## Phase 5 — SDK `/sync` self-registration contract `[L]`

Implement 8 application-scoped sync endpoints + 6 missing use-cases (subscriptions, dispatch-pools, processes, scheduled-jobs, principals; app-scoped roles sync). Match Rust payloads. (Rust `shared/sdk_sync_api.rs:881`)

## Phase 6 — Dispatch + cron scheduler `[L]`

- **SC1** wire up cron scheduler (zero callers today).
- **SC2** fix `fire()` to write real instance columns.
- **SC3** dispatcher/retry engine (poller→dispatcher, IN_FLIGHT→DELIVERED→requeue, 202 contract). (Rust `dispatcher.rs`)
- **SC4** run-now inserts an instance. (Rust `fire_now.rs:101`)
- **SC5** cron syntax 6–7 field + validation.
- **SC6** skip-to-latest downtime semantics; monotonic `mark_fired`.
- **SC7** leader-gate cron + dispatch-job schedulers.
- **SC8** `FC_SCHEDULED_JOB_*` / `FC_SCHEDULER_*` config.
- **SC9** API field gaps (`hasActiveInstance`, `clientId="platform"`, FireNow shape, `correlationId`).

## Phase 7 — Stream processor `[M]`

- **ST1** preserve source `created_at` into read-model. (`events.go:72`)
- **ST2** populate `is_terminal`.
- **ST3** leader-gate projections.
- **ST4** partition retention/drop + `is_partitioned` guard + window/cadence. (Rust `partition_manager.rs:229`)
- **ST5** per-projection batch sizes + env knobs; rename toggle to `FC_STREAM_PARTITION_MANAGER_ENABLED`.

## Phase 8 — Outbox processor `[L]` (schema in Phase 1)

- **OB1** add MongoDB backend. (Rust `mongo.rs`)
- **OB2** crash recovery. **OB3** leader-gating.
- **OB4** API batching. **OB5** per-item 2xx `{results:[]}` response handling.
- **OB6** max-retries cap + group-blocking. **OB7** bounded concurrent groups.
- **OB8** env-var alignment/aliases.

## Phase 9 — MCP server `[L]`

- **M1** MCP library + `initialize` handshake. **M2** stdio transport.
- **M3** remaining 12 tools. **M4** resources.
- **M5** OAuth client-credentials + token cache. **M6** `fc-dev` credential bootstrap.
- **M7** JSON output fix, localhost bind, default port. **M8** remove stale artifact + fix docs.

## Phase 10 — Ops surface `[M]`

- **P1** AWS Secrets Manager DB mode. **P2** ALB self-registration.
- **P3** Dockerfile + docker-compose (+ healthchecks).
- **P4** env-var alias layer for drop-in (router `AUTH_MODE`/`AUTH_BASIC_*`, outbox `FC_API_BASE_URL`/`FC_API_TOKEN`/`FC_OUTBOX_DB_URL`, …).
- **P5** port canonicalization to 8080 (`internal/config/config.go`); README/docs staleness.

## Cross-cutting

- **Shared leader-gating helper** reused by SC7 / ST3 / OB3.
- **Parity test harness** (extend `parityharness`) with golden assertions.

## Open micro-decisions

1. MCP default port — keep `8090` or match Rust `3100`.
2. Env-var aliasing (P4) — accept Rust names as aliases (lean: yes).
3. WebAuthn blobs (A4b) — convert-on-read vs migration (only if V4 finds existing passkeys).

## Recommended sequence

Phase 0 → Phase 1 (prerequisites for safe deploy). Then parallel tracks: Phase 2 (OIDC) + Phase 3 (router) + Phase 4 (IAM/security). Then 5 → 6 → 7 → 8 → 9, with Phase 10 + the test harness alongside.
