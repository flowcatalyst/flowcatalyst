# FlowCatalyst Go Port — Handoff

This document is the canonical "where we are, where we're going" reference
for the Rust → Go port. Read it cold to pick up the work.

## 1. The Drop-In Contract

The Go port is a **drop-in replacement** for the Rust binaries. That means:

1. **Same Postgres database.** No new tables, no schema changes. The
   29 migrations in `/migrations/` are the Rust source's own — copied
   verbatim and embedded into every Go binary via `internal/migrate/`.
   Either build can apply them.

2. **Byte-identical HTTP APIs.** Every existing SDK consumer + the Vue
   frontend MUST continue working with zero source changes after cutover.
   This is the load-bearing rule. Specifically:
   - Path: `/api/clients/{id}/activate` (not `/api/clients/{id}:activate`).
   - JSON shape: snake_case for inbound, camelCase for outbound (matches
     the TS+Rust convention).
   - Status codes: 201/204/200 must match Rust for the same operation.
   - Error envelope: `{ "error": "<code>", "error_description": "..." }`.

3. **Byte-identical event types.** The 41 platform event-type codes
   (`platform:iam:user:created` etc.) are emitted into `msg_events` and
   read by consumer SDKs. Codes are pinned in
   `internal/platform/seed/event_types.go` and individual subdomain
   `operations/events.go` files. **Fix the constant, not the consumer.**

4. **Same router behaviour for SDK consumers.** The message router is
   the most sensitive surface — see §5 below. Wire-format compatibility
   is mandatory: same HMAC signing scheme, same retry semantics, same
   header names, same per-message-group FIFO ordering, same circuit-
   breaker thresholds. Existing SDK consumer apps cannot tell whether
   they're being delivered to by the Rust or Go router.

5. **Single Go module.** `github.com/flowcatalyst/flowcatalyst-go` —
   all binaries + libraries live here.

6. **Existing JWT/token compatibility is NOT required.** The user
   explicitly accepted that tokens issued by Rust won't validate after
   cutover. Users + service accounts need to re-auth. New tokens use
   fosite + the existing `oauth_oidc_payloads` storage table.

## 2. Architecture at a Glance

```
cmd/
  fc-server/          — unified production binary (FC_*_ENABLED toggles)
  fc-dev/             — developer monolith with embedded Postgres (cobra subcommands)
  fc-router/          — standalone router binary (264 lines)
  fc-platform-server/ — placeholder; superseded by fc-server
  fc-stream-processor/
  fc-outbox-processor/
  fc-mcp-server/

internal/
  server/             — shared wiring (EnvCfg, WirePlatform, subsystem launchers)
  migrate/            — embed-FS migration runner (applies /migrations/*.sql)
  platform/           — every subdomain (20+ aggregates with operations/api/)
    auth/             — fosite-backed OAuth provider + OIDC bridge
    seed/             — 12 roles + 41 event types + 41 schemas
    scheduler/        — dispatch-job poller + dispatcher + stale recovery
    scheduledjob/     — cron-fired scheduled jobs
    shared/           — auth context, sink, BFF, SDK ingest
  router/             — message router internals (mediator/pool/queue_health/etc.)
  stream/             — CQRS projectors (events, dispatch_jobs, fan_out)
  outbox/             — consumer-app outbox processor
  mcp/                — MCP server
  queue/              — queue abstraction (Postgres, SQS)
  standby/            — Redis-backed leader election
  secrets/            — secret resolver
  sealed/             — compile-time-enforced UoW seal token
  tsid/               — typed TSID generator (prefix per entity)

pkg/fcsdk/            — SDK exported to consumer apps (usecase + usecasepgx)

migrations/           — 29 SQL files (verbatim from Rust)
```

### Key patterns

- **Sealed Unit-of-Work seal.** Every domain write must go through
  `usecasepgx.Commit` (or `CommitDelete`/`CommitAll`/`EmitEvent`). The
  seal is a sealed.Token whose only constructor lives in the `sealed`
  package; no one outside `usecasepgx` can mint one. This guarantees
  every aggregate write emits its DomainEvent + audit row atomically.

- **OAuth via fosite.** `internal/platform/auth/provider/` implements
  fosite's Storage + ClientManager + Hasher (Argon2id) against the
  existing `oauth_oidc_payloads` and `iam_oauth_clients` tables.
  fosite's `compose.Compose(...)` mints the OAuth2Provider with
  client_credentials + refresh_token + revoke + introspect + PKCE +
  authorize_explicit factories registered. **The token endpoint is
  ~80 lines of glue** — fosite does the rest.

- **Embedded Postgres for dev.** `cmd/fc-dev` uses
  `github.com/fergusstrange/embedded-postgres` (pure-Go, downloads PG
  binaries on first run). Same UX as Rust's `pg_embed` feature.

- **Event-type catalog as static Go data.** `internal/platform/seed/`
  ports the Rust `seed/platform_event_types.rs` + `platform_event_schemas.rs`
  as Go literals. The DSL (`obj/reqStr/optStr/reqBool/reqU32/...`)
  mirrors the Rust helpers so transcription stays mechanical.

## 3. Build / Run

```bash
go build ./...                # everything
go test ./...                 # everything (no DB required for current tests)

go run ./cmd/fc-dev --help    # see subcommands
go run ./cmd/fc-dev           # default: start embedded PG + platform API
go run ./cmd/fc-dev fresh --yes
go run ./cmd/fc-server        # production-shape binary (needs external PG)
```

Env toggles for `fc-server` (TS-aliased names also supported):

| Toggle                          | Default | Purpose                       |
| ------------------------------- | ------- | ----------------------------- |
| `FC_PLATFORM_ENABLED`           | `true`  | Run the platform API server   |
| `FC_ROUTER_ENABLED`             | `false` | Run the SQS message router    |
| `FC_SCHEDULER_ENABLED`          | `false` | Run the dispatch scheduler    |
| `FC_STREAM_PROCESSOR_ENABLED`   | `false` | Run the CQRS stream processor |
| `FC_OUTBOX_ENABLED`             | `false` | Run the outbox processor      |
| `FC_STANDBY_ENABLED`            | `false` | Redis leader election         |

## 4. State of the Port

### What's done

- Phases 0–2 complete (foundation, common, tsid, queue, router primitives, secrets, config, standby).
- Phase 3 (every subdomain): 20+ aggregates with their operations, repository, api routes. Includes principal IAM verbs, serviceaccount/application/auth provisioning, full event-type/role/dispatch-pool/subscription/connection/scheduled-job CRUD.
- Phase 4 (stream + outbox): packages exist, internal APIs present.
- Phase 5 (SDK + MCP): SDK ported to `pkg/fcsdk/`. MCP scaffolded.
- OAuth/OIDC runtime via fosite: token, authorize, revoke, introspect, /.well-known/openid-configuration, /.well-known/jwks.json, OIDC bridge for external IDPs.
- WebAuthn HTTP wiring.
- Seed data: 12 platform roles + 1 platform application + 41 event types + 41 JSON schemas.
- Migration runner + 29 embedded SQL files.
- `cmd/fc-server` — unified binary.
- `cmd/fc-dev` — developer monolith with embedded PG + subcommands.

### What's stubbed (Production-blocking)

These are the items the audits surfaced. Each has a `TODO(<name>)` in
the source. Search for the marker to find the exact file:line.

1. ~~**`internal/server/subsystems.go`** — `StartStreamProcessor`,
   `StartOutboxProcessor`, `StartRouter` are signal-only stubs.~~
   **Done.** `StartStreamProcessor` launches events / dispatch_jobs /
   fan_out / partition_manager (per-projection sub-toggles default ON).
   `StartOutboxProcessor` runs the Postgres outbox poller against the
   shared pool; FC_OUTBOX_PLATFORM_URL is required, sqlite/mysql/mongo
   remain in `cmd/fc-outbox-processor`. `StartRouter` delegates to a
   new `internal/router.Server` (NewServer + Run); `cmd/fc-router/main.go`
   now only contributes signal handling + HTTP listener. See
   `EnvCfg`'s new Stream/Outbox/Router fields for the knob set.

2. ~~**Auth middleware.**~~ **Done.** `internal/platform/shared/middleware/middleware.go`
   now introspects the inbound `Authorization: Bearer <jwt>` (or the
   `fc_session` cookie used by the Vue frontend) through the fosite
   provider, builds an `AuthContext` from the JWT's extra claims
   (PrincipalID, Scope, Clients, Roles, Applications, Permissions, Email),
   and attaches it to the request. The `X-FC-Test-Principal` dev fallback
   now requires `FC_AUTH_ALLOW_TEST_HEADERS=true`. To make Permissions
   self-contained in the JWT, `BuildClaims` now flattens role→permissions
   at mint time (so `NewProvider` takes a `*role.Repository`). The
   middleware is mounted globally in `WirePlatform` via `r.Use(...)`.

3. **Init bootstrap depth.** `fc-dev init` writes a placeholder `.env`.
   The Rust impl creates: admin user + default Client + default
   Application + Service Account + OAuth client + anchor domain row.
   Port from `crates/fc-platform/src/shared/bootstrap_admin.rs` +
   `bin/fc-dev/src/init.rs`.

4. ~~**Argon2id PHC salt.**~~ **Done.** Shared
   `internal/platform/auth/passwordhash` package now owns Argon2id
   hashing — PHC envelope (`$argon2id$v=19$m=65536,t=1,p=4$<salt>$<hash>`)
   with per-row random salt. Used by:
   - `principal/operations/create.go::Execute` (user passwords)
   - `principal/operations/reset_password.go` (password reset)
   - `cmd/fc-dev/init.go::hashSecret` (init admin password)
   - `auth/operations/oauth_client.go::generateSecret` (OAuth client
     secret)
   - `auth/provider/hasher.go` (fosite's `ClientSecretsHasher`)
   Verified end-to-end: init mints a CONFIDENTIAL OAuth client with
   PHC-hashed secret → `/oauth/token` exchange with the plaintext
   passes fosite's `Compare`; wrong secret returns `invalid_client`.
   Unit tests in `passwordhash_test.go` cover round-trip,
   per-call salt uniqueness, and 5 invalid-envelope rejections.

### What's stubbed (Lower priority)

5. **Per-row sync events.** `eventtype/operations/sync.go` is wired
   but emits only the rollup event. Rust emits per-row Created/Updated/
   Deleted alongside the rollup. Same pattern needed for role,
   dispatch_pool, subscription, scheduled_job, process, platformconfig
   sync ops (only event_type is ported today).

6. **WebAuthn enumeration defence.** `authenticate/begin` currently
   returns an empty challenge for unknown/federated emails. Rust
   returns deterministic-fake `allowCredentials` keyed by HMAC(email)
   so the response shape is indistinguishable from a real one.

7. **Router gaps** (from `Router parity audit`):
   - ~~RouterError struct~~ **Done.** `internal/router/error.go` with
     the full ErrorKind enum + helper constructors + AsRouterError unwrap.
   - ~~HealthService with rolling-window success rates~~ **Done.**
     `internal/router/health.go` — per-pool rolling counter (30m
     default, amortised O(1) record), consumer poll/stall tracking,
     HealthReport with Healthy/Warning/Degraded bands matching the
     Java/Rust warning-count thresholds (5/20). 8 unit tests.
   - ~~WarningService with TTL cleanup + acknowledgement tracking~~
     **Done.** `internal/router/warning.go` — in-memory store with
     UUID ids, ack state, auto-ack on age, capacity-bounded with
     oldest-10% eviction, optional Notifier forwarding. 5 unit tests.
     The existing router Warning struct in `notification.go` now
     carries id/createdAt/acknowledged so it's the same shape Rust
     uses (no separate stored-vs-emitted distinction).
   - ~~LifecycleManager coordination~~ **Done.**
     `internal/router/lifecycle.go` — owns the warning-cleanup,
     consumer-health, and health-report background loops. Wired in
     `Server.NewServer`/`Server.Run`/`Server.Shutdown`. Manager-coupled
     tasks (memory health monitor, consumer auto-restart, stale-entry
     reaper) deferred until the Go `Manager` grows the matching
     surface (Rust has `check_memory_health`, `restart_consumer`,
     `get_pool_stats`, `reap_stale_entries`, `cleanup_draining_pools`,
     `pool_codes`, `consumer_ids`). Optional `PoolStatsProvider`
     interface lets the health-report logger pick up real pool stats
     once that lands.

   Still pending: Prometheus metrics + HdrHistogram, full `/monitoring/*`
   / `/warnings/*` / `/config/*` HTTP surface (40+ routes),
   Swagger/OpenAPI, TrafficStrategy for ALB integration.

8. **Outbox gaps** (from `Outbox parity audit`): MySQL/Mongo backends,
   GlobalBuffer with BufferFullError, RecoveryTask for stuck PROCESSING
   items, GroupDistributorConfig + DistributorStats, Prometheus
   `/metrics`.

9. **Stream gaps** (from `Stream parity audit`): StreamHealthService
   with per-projection snapshots, `StreamProcessorHandle.Stop()` graceful
   shutdown, `/ready` endpoint.

10. **Scheduler gaps** (from `Scheduler parity audit`):
    `PausedConnectionCache.spawn_refresh_task` integration, separate
    `BlockOnErrorChecker` component.

11. **Specialty routes**: connection activate/pause, dispatch-pool
    suspend (new ops needed, not just route wiring).

12. **AWS Secrets Manager integration** for DB credentials rotation
    (Rust supports DB_SECRET_ARN; Go skips it — explicit TODO in
    `internal/server/envcfg.go::ResolveDatabaseURL`).

13. **ALB target-group registration** on leader transition (Rust feature-gated; not yet ported).

13a. **Embedded NATS as dev broker** — deferred. Considered as a
    replacement for the Postgres-table queue in fc-dev. Held off
    because (a) prod uses SQS, so introducing NATS only in dev breaks
    dev/prod parity; (b) "single-node simple NATS" loses messages on
    restart, JetStream adds a stateful component undoing the
    one-data-dir appeal of embedded PG; (c) nothing today needs broker
    semantics PG polling can't model. Revisit if prod ever migrates
    off SQS, or if true durable-multi-consumer fan-out at the broker
    level becomes a requirement. **If/when this happens, NATS must
    slot in via the existing `internal/queue.Publisher` /
    `internal/queue.Consumer` abstraction** — same contract as PG
    dev + SQS prod backends, not a parallel pattern.

14. ~~**Frontend.**~~ **Done.** Vue 3 source copied to top-level
    `frontend/`; `dist/` + `node_modules/` gitignored.
    `frontend/embed.go` + `frontend/handler.go` provide
    `frontend.Handler() http.Handler` that mirrors Rust's
    `bin/fc-dev/src/main.rs::embedded_asset_handler`: exact-path
    asset → MIME-typed response with `Cache-Control: immutable` for
    `/assets/*`, otherwise SPA fallback to `index.html`. Mounted on
    fc-dev as the chi NotFound handler so every API route takes
    precedence. `frontend.IsAvailable()` lets the caller skip the
    mount cleanly when the binary was built without
    `make frontend`. **Build pipeline:** `make build` now depends on
    `make frontend`, which runs `pnpm install --frozen-lockfile` +
    `pnpm build` in `frontend/`. For backend-only iteration,
    `make go-build` skips the frontend step. Smoke verified end-to-end:
    `GET /` → SPA shell; `GET /assets/*.js` → immutable cache;
    `GET /api/event-types` → still served by the API; SPA history-mode
    routes (`/principals/some-id`) → fall back to `index.html`. The
    Hey-API generated TypeScript client under
    `frontend/src/api/generated/` is currently committed (matches
    Rust); will be regenerated from Go's spec — see item #24a
    (OpenAPI), now landed (framework + 3 aggregates). Binary size
    delta: +7.9MB (matches the 7.4MB dist/ + embed overhead).

14a. **OpenAPI spec generation.** **Framework done.**
    `internal/platform/shared/openapi/` provides `Doc` + `Op()` +
    helper option builders (`Tag`, `PathParam`, `QueryParam`,
    `RequestBody`, `Response`) with reflective schema generation via
    `getkin/kin-openapi/openapi3gen`. Each api package pairs its
    `RegisterRoutes` with an `OpenAPI(doc)` function — three landed
    today (eventtype, principal, subscription), each covering every
    route the package exposes. `WirePlatform` builds the Doc,
    threads it through each registrar, and mounts
    `GET /api/openapi.json` unauthenticated for tooling
    (oasdiff, hey-api codegen). Smoke verified:
    `curl /api/openapi.json` → 200, 22.5KB, 17 paths, 19 component
    schemas. Parity-harness YAML in
    `tests/parity/requests/openapi/spec.yaml` asserts the spec is
    served + has the core OpenAPI 3.0 top-level shape.
    **Still pending** (per-PR work as api packages get touched):
    spec for the other ~17 aggregates (client, role, application,
    serviceaccount, etc.); wiring the parity-spec CI job
    (`.github/workflows/ci.yml`) to actually run `oasdiff` once the
    spec is complete; pointing frontend's `openapi-ts.config.ts` at
    Go's spec URL; Swagger UI at `/api/swagger`. Pattern recommendation
    for future packages: prefer a fused `Mount(r, doc, state)` helper
    that registers route + spec together so they can't drift; today
    `RegisterRoutes` + `OpenAPI` are paired by convention.

15. **Frontend-only `/bff/*` routes.** Beyond `/bff/dashboard` (which
    is ported), Rust exposes `/bff/events`, `/bff/dispatch-jobs`,
    `/bff/roles`, `/bff/event-types`, `/bff/scheduled-jobs`, etc.
    These are thin frontend-tailored views.

16. **OIDC bridge auto-provisioning.** Today the bridge fails with
    `USER_NOT_PROVISIONED` if no FlowCatalyst principal matches the
    IDP's email. Rust auto-creates via the anchor-domain row.

17. **WebAuthn ceremony purger.** `webauthn.CeremonyRepository.PurgeExpired`
    exists (added in the sqlc sweep, matches Rust's `purge_expired`),
    but nothing calls it on a loop. Rust runs it as a background task.
    Wire it alongside the payload purger or as a sibling poller.

18. ~~**sqlc nullable-JSONB override gap.**~~ **Done.** Added a second
    override entry (`db_type: "jsonb"`, `nullable: true`) so nullable
    JSONB columns now generate as `json.RawMessage` like the NOT NULL
    case. `audit.jsonOf()` helper removed; `OperationJson` reads
    directly as `json.RawMessage`. (The remaining `[]byte` in
    `dbq/models.go` is `WebauthnCredential.CredentialID`, which is a
    BYTEA column — the correct mapping.)

19. **Principal find-method surface.** The Go repo exposes only
    `FindByID`, `FindByEmail`, `FindAll`. Rust additionally has
    `find_by_service_account`, `find_active`, `find_users`,
    `find_services`, `find_by_client`, `find_by_scope`,
    `find_with_filters`, `find_anchors`, `find_by_application`,
    `find_with_role`, `search`, `find_names_by_ids`,
    `count_by_email_domain`. None are called by the current Go API
    surface, but they'll be needed as `/api/principals` filter
    endpoints + the OIDC bridge auto-provisioning land. Each is a
    small sqlc query; the schema is already mapped correctly.

20. ~~**`app_applications.service_account_id` is a principal id, not
    a SA row id.**~~ **Done.** `attach_service_account.go` now takes
    a `*principal.Repository` dependency and resolves the SA's
    linked principal id via `PrincipalFindByServiceAccount` (new sqlc
    query) before writing `app.ServiceAccountID = saPrincipal.ID`.
    `internal/server/wire.go` updated to pass the principal repo.

21. ~~**Password-hash verifiers must base64-decode.**~~ Superseded by
    the PHC envelope (§4 #4) — `passwordhash.Verify` parses the
    envelope and does the right thing. No more base64-stopgap.

22. **Pre-existing build failures (not my work).** Two packages don't
    compile against the current SDK client surface — flagged here
    because `make ci` will fail on them:
    - `cmd/fc-mcp-server/main.go` references `client.Config` which
      doesn't exist on `pkg/fcsdk/client` today.
    - `pkg/fcsdk/sync/synchronizer.go` references `client.FlowCatalystClient`,
      `client.SyncRoleItem`, `client.SyncRolesRequest`, `client.SyncResult`,
      etc. — none of which exist. `pkg/fcsdk/sync` was scaffolded ahead
      of the client expansion and the client expansion never happened.
    Either flesh out `pkg/fcsdk/client` to match what the consumers
    expect, or delete the consuming code if it's not on the immediate
    roadmap. `go build ./cmd/fc-dev ./cmd/fc-server ./internal/...`
    is clean — only these two paths fail.

23. **Flaky TSID test.** `internal/tsid.TestUniquenessSerial` /
    `TestUniquenessParallel` intermittently fail with `duplicate TSID
    generated`. Generator runs at >10k IDs/test which can hit
    millisecond-bucket collisions on a fast machine. Either lower the
    sample count or change the generator to incorporate a
    sub-millisecond counter. Not my work — flagged for follow-up.

## 5. The Message Router — Drop-in Specifics

The router is the most sensitive subsystem for drop-in compatibility
because **consumer apps' SDKs are wire-coupled to its behaviour**.
Specifically:

### Wire format

- **HMAC-SHA256 signing.** Auth token in `Authorization: Bearer fc_<token>`
  header is the existing scheme. Go side: `internal/platform/scheduler/auth.go::DispatchAuthService.Sign`
  produces the same token shape; the consumer SDK's verifier checks
  `HMAC(secret, jobID)`.

- **Per-message-group FIFO.** `MessageGroupDispatcher` (Go) +
  Rust's equivalent enforce per-message-group ordering. Same group →
  serial dispatch; different groups → concurrent under a global
  semaphore cap.

- **Headers.** `X-FC-Job-ID`, `X-FC-Subscription-ID`, `X-FC-Attempt`,
  `X-FC-Max-Attempts`, `X-FC-Message-Group` are emitted on every
  dispatch. **Don't rename these — consumers parse them.**

- **Retry strategy.** Exponential backoff with jitter (default).
  Specifically: `min(base * 2^(attempt-1), cap)` + ±15% jitter.
  Configurable per dispatch-pool. The values + jitter algorithm need
  to match Rust's; the current Go impl needs an audit pass to confirm.

- **HTTP transport parity (`internal/router/mediator.go`).** Audited
  against `crates/fc-router/src/mediator.rs`. Aligned on
  `MaxIdleConnsPerHost=10` ↔ `pool_max_idle_per_host(10)`,
  `IdleConnTimeout=90s` ↔ reqwest default, HMAC sign format
  (`%Y-%m-%dT%H:%M:%S%.3fZ`), retry policy (3 × [1s,2s,3s]), skip-retry
  rules (Success/ErrorConfig/RateLimited), HTTP/2 default + HTTP/1.1
  forcing (`TLSNextProto={}` + `ForceAttemptHTTP2=false` is
  functionally equivalent to reqwest's `http1_only()` for HTTPS — the
  only mode used in prod). **One bug caught + fixed during the audit:**
  `MediatorConfig.ConnectTimeout` was stored but never wired into the
  Transport's `DialContext`, so a slow TCP connect was bounded by
  `Client.Timeout` (15min prod) rather than `ConnectTimeout` (30s prod).
  Fix: explicit `net.Dialer{Timeout: cfg.ConnectTimeout, KeepAlive: 30s}`
  feeding `transport.DialContext`. Regression test
  `TestMediatorConnectTimeoutHonoured` points at RFC-5737 TEST-NET-1
  and asserts elapsed < 2s with a 250ms ConnectTimeout.

- **Intentional Go-only divergence: `StrictMaxConcurrentStreams`.**
  AWS ALBs advertise a per-H2-connection stream cap (~128) via SETTINGS
  frames. Without `StrictMaxConcurrentStreams`, Go's HTTP/2 client
  ignores the hint and opens streams until the server returns
  `REFUSED_STREAM` / `GOAWAY` — bad for tail latency and failure-mode
  observability. With it, the client *waits* for an in-flight stream
  to complete before starting a new one. Rust's reqwest doesn't expose
  this knob today, so Go is strictly safer against ALB's H2→H1
  translation cap. Set via
  `http2.ConfigureTransports(transport).StrictMaxConcurrentStreams = true`
  in `NewHTTPMediator` (production HTTP/2 path only; HTTP/1.1 dev
  path unaffected).

- **Intentional Go-only divergence: per-host concurrency cap.**
  `internal/router/host_limiter.go` wraps the `http.Transport` with a
  per-target-host semaphore. Default cap: 100 (prod), 50 (dev). Caps
  in-flight requests to any single host regardless of which dispatch
  pool issued them. Sits ABOVE the transport so the cap is on logical
  requests, not TCP connections — covers both H1 (multiple TCP
  connections) and H2 (multiplexed streams on one connection)
  uniformly. Defence-in-depth for the AWS-ALB case: even if a peer
  doesn't advertise an H2 stream limit (so `StrictMaxConcurrentStreams`
  has nothing to honour), the host limiter still bounds fanout.
  Cancellation while queued returns promptly without dispatching the
  request — verified by `TestHostLimiter_ContextCancelReleasesQueued`.
  Also covers the cross-pool case: if two dispatch pools target the
  same host, they share the per-host budget rather than each running
  their pool's `Concurrency` independently. Set
  `MediatorConfig.MaxConcurrentPerHost = 0` to disable.

- **Intentional Go-only knob: explicit `TLSHandshakeTimeout`.** Go's
  stdlib default is 10s but we set it explicitly via
  `MediatorConfig.TLSHandshakeTimeout` so a stdlib default change can't
  silently shift our handshake budget.

### Sub-systems that talk to consumers

- `/oauth/token` (client_credentials grant) — SDK consumers exchange
  the OAuth client_id + client_secret for a JWT, then call back into
  the platform's `/api/dispatch-jobs/batch` + `/api/events/batch` with
  the JWT in the Authorization header.

- `/api/dispatch-jobs/batch` — SDK outbox processors POST batched
  dispatch jobs. **This is the highest-traffic SDK endpoint.** Wired
  in `internal/platform/shared/sdk/dispatch_jobs_batch.go`.

- `/api/events/batch` — SDK consumers POST domain events for fan-out.
  Wired in `internal/platform/event/api/api.go`.

- **`/api/platform/cors/allowed`** — public; SDKs hit this pre-flight
  to learn which origins the platform accepts.

### Co-ordination

- **Same database, same queue.** During cutover, the Go scheduler can
  pick up where Rust left off because `msg_dispatch_jobs` is the
  source of truth — both implementations claim PENDING jobs via
  `FOR UPDATE SKIP LOCKED`. **It's safe to run one of each pointing at
  the same DB for the duration of the migration** (each will claim
  half the work).

- **Stale recovery.** Both implementations revert QUEUED→PENDING
  after a stale-after window. If running side-by-side, set Rust's
  window equal to Go's (or shorter on whichever you trust more).

### Tests we DON'T yet have

- **Contract tests** that hit the Rust binary + Go binary with the
  same input and diff the JSON / header output. This is the highest-
  leverage thing to build next for the router specifically.

- **Integration tests against a live Postgres.** All current tests are
  unit-level. A `docker-compose.test.yml` with PG + a parity harness
  would catch the byte-identical-API requirement automatically.

## 6. How to Verify Drop-in (proposed)

The drop-in claim is unverified until we actually run both binaries
against the same DB and confirm identical behaviour. Recommended steps:

1. **Run Rust binary against a fresh PG.** Apply migrations. Run
   the standard frontend smoke test. Confirm it works.

2. **Stop Rust. Start Go fc-server against the SAME PG.** Run the
   identical smoke test. Confirm identical responses.

3. ~~**Run a contract harness**~~ **Done (framework).**
   `tools/parityharness/` is a working binary. Usage:
   `go run ./tools/parityharness -rust=URL -go=URL -dir=tests/parity/requests`.
   YAML cases under `tests/parity/requests/` describe `name + request +
   expect (status, body_shape)`. `${VAR}` substitution in path/body/
   headers; missing vars cause clean SKIPs. Comparator does status +
   load-bearing-header diffs + placeholder-typed JSON shape matches
   (`tsid`, `uuid`, `iso8601-microsecond`, `any-*`). 15 unit tests on
   the comparator + placeholder matchers. Self-tested against fc-dev
   pointed at itself (both `-rust` and `-go` → same URL) — 2 PASS, 5
   SKIP (no `ANCHOR_TOKEN` env), exit 0; divergence path proved by
   pointing `-go` at a closed port (exit 1, attributed correctly).
   First self-test immediately caught 2 inaccurate YAML expectations
   (status 401 vs actual 403; error envelope `error/error_description`
   vs actual `code/message`) — exactly the kind of contract drift the
   harness exists to catch. Next: 6 starter YAMLs under smoke/,
   event-types/, dispatch-jobs/, principals/ — grow as new endpoints
   land.

4. **Frontend smoke** — copy the Vue app, set `VITE_API_BASE_URL` to
   the Go server, click through the main screens (clients, users,
   applications, event types, dispatch jobs).

5. **SDK consumer smoke** — point one of your real consumer apps at
   the Go server (change `FLOWCATALYST_URL`). Watch for outbox
   delivery, event ingestion, dispatch retries.

## 7. Suggested Sequence for the Next Session

If picking this up cold, I'd tackle in this order:

1. ~~**Subsystem wiring** (`internal/server/subsystems.go`)~~ — **Done.**
   All three launchers now host the real loops. New env knobs:
   `FC_STREAM_{EVENTS,DISPATCH_JOBS,FAN_OUT,PARTITIONS}_ENABLED` (default true),
   `FC_STREAM_BATCH_SIZE`, `FC_OUTBOX_PLATFORM_URL`,
   `FC_OUTBOX_PLATFORM_AUTH_TOKEN`, `FC_OUTBOX_{BATCH_SIZE,MAX_IN_FLIGHT,POLL_INTERVAL_MS}`,
   `FLOWCATALYST_CONFIG_URL`, `FLOWCATALYST_DEV_MODE`,
   `FC_NOTIFY_WEBHOOK_URL`, `FC_DRAIN_TIMEOUT_SECONDS`.

2. ~~**Auth middleware**~~ — **Done.** Bearer-token + `fc_session`
   cookie resolution via fosite introspection, mounted globally in
   `WirePlatform`. `FC_AUTH_ALLOW_TEST_HEADERS` gates the dev
   `X-FC-Test-Principal` path.

3. ~~**Run `fc-dev start` end-to-end.**~~ **Done.** Embedded PG boots,
   migrations apply, seed runs (1 application + 12 roles + 72 event
   types), `/health` returns 200, `/api/event-types` returns 403 with
   no token and 200 with `X-FC-Test-*` headers (dev mode). The boot
   path surfaced two bug classes:

   1. **Table-name mismatches** — the Rust→Go transcription picked the
      wrong "current" table name in several places along the migration
      history. Fixed: `iam_applications`/`msg_applications` →
      `app_applications`, `iam_clients` → `tnt_clients`,
      `iam_audit_logs` → `aud_logs`, `iam_webauthn_credentials` →
      `webauthn_credentials`, `iam_oauth_clients` → `oauth_clients`,
      `iam_anchor_domains` → `tnt_anchor_domains`,
      `iam_identity_providers` → `oauth_identity_providers`,
      `iam_idp_role_mappings` → `oauth_idp_role_mappings`,
      `iam_platform_configs` → `app_platform_configs`,
      `iam_platform_config_access` → `app_platform_config_access`,
      `iam_client_auth_configs` → `tnt_client_auth_configs`,
      `msg_dispatch_attempts` → `msg_dispatch_job_attempts`. Also
      cleaned the bogus join tables `iam_client_auth_config_*_clients`
      (those columns are JSONB on `tnt_client_auth_configs` per the
      schema) — `auth/repository.go` ClientAuthConfigRepo rewritten to
      read/write the two `JSONB` arrays directly.

   2. **chi middleware-after-routes** — `WirePlatform`'s `r.Use(...)`
      panicked when the caller had already registered `/health`. Fixed
      by wrapping the platform's route block in `r.Group(...)` so the
      Authenticator + CorrelationID middleware is scoped locally to
      the platform routes without ordering coupling to the caller.

   **sqlc adoption (in progress).** sqlc has been wired in:
   `sqlc.yaml` at repo root, queries live in `internal/sqlc/queries/`,
   generated code in `internal/sqlc/dbq/`. Schema source is the
   embedded migration set (`internal/migrate/sql/`) so sqlc's view of
   the DB matches what fc-dev/fc-server apply at boot. `make sqlc`
   regenerates; `make sqlc-verify` is wired into `make ci`.

   The **client** repository (`internal/platform/client/repository.go`)
   is migrated as the pattern: all 5 ops (FindByID, FindByIdentifier,
   Search, FindAll, Persist, Delete) go through `*dbq.Queries`. End-to-end
   smoke test passes (create → list → get → search). Remaining ~20
   repositories follow the same pattern.

   Migrating the first repo surfaced **three more bugs in the
   platformsink** (which writes to `msg_events` + `aud_logs`):

   1. **Event IDs were UUIDs but `msg_events.id` is `VARCHAR(13)`**. The
      Rust source uses an untyped 13-char TSID. The SDK's `pkg/fcsdk/usecase`
      now generates IDs via `pkg/fcsdk/tsid.GenerateUntyped()` — and to
      avoid duplication, the TSID primitives (`GenerateRaw`,
      `GenerateUntyped`, `GenerateWithPrefix`, `ToLong`, `FromLong`,
      Crockford encode/decode) have been moved to **`pkg/fcsdk/tsid`**;
      `internal/tsid` keeps the FlowCatalyst-specific `EntityType`
      catalog and forwards to the SDK primitives.

   2. **`msg_events` INSERT had `context` (wrong column), missing
      `time`/`correlation_id`/`causation_id`/`message_group`/`client_id`,
      and `ON CONFLICT (deduplication_id)` against a composite unique
      index** that Postgres can't infer. Now matches the Rust 14-column
      INSERT, no ON CONFLICT (matches Rust — dedup duplicates bubble up
      as tx failures).

   3. **`aud_logs` INSERT used phantom columns** (`event_id`,
      `event_type`, `aggregate_type`, `aggregate_id`, `command`,
      `created_at`). The actual schema has `entity_type`, `entity_id`,
      `operation`, `operation_json`, `principal_id`, `application_id`,
      `client_id`, `performed_at`. Also, `aud_logs.id VARCHAR(17)` —
      the old `newAuditID()` produced 26-char UUIDs. Now uses
      `tsid.Generate(tsid.AuditLog)` → `"aud_<13>"`.

   These three were also masking each other: each only became visible
   after the previous one was fixed.

   **sqlc bulk migration (mostly complete).** Repositories migrated (19/20):
   `client`, `role`, `cors`, `dispatchpool`, `identityprovider`,
   `process`, `application`, `connection`, `eventtype`, `serviceaccount`,
   `platformconfig`, `subscription`, **`application/client_config`,
   `principal`, `audit`, `webauthn/credentials`, `webauthn/ceremonies`,
   `auth/payload`, `emaildomainmapping`, `auth/{OAuthClient,AnchorDomain,
   ClientAuthConfig,IdpRoleMapping}`**. Each migration surfaced its own
   schema-vs-entity bug; tally of latent bugs caught and fixed during
   this pass:

   - `identityprovider` — the repo SELECTed an `allowed_email_domains`
     column that doesn't exist; the schema uses a junction table
     (`oauth_identity_provider_allowed_domains`). Rewritten to read/write
     the junction.
   - `process` — repo wrote `created_by` to `msg_processes`; the column
     doesn't exist (matches Rust's `created_by: None`). Dropped from
     persistence; entity field retained for API-shape compat.
   - `serviceaccount` — repo wrote `webhook_credentials JSONB` and
     `scope`, neither of which exist. Schema has flat `wh_*` columns;
     repo now maps `WebhookCredentials` struct ↔ flat columns.
   - `subscription` — repo wrote `endpoint` (schema is `target`),
     `filter` on the event-types junction (no such column),
     `key`/`value` on configs junction (schema is `config_key`/
     `config_value`), and `created_by` (no column). All fixed.
   - `principal` — repo read/wrote `user_identity` and `external_identity`
     JSONB columns that don't exist. Schema has flat
     `email/email_domain/idp_type/external_idp_id/password_hash/last_login_at`.
     Repo now maps the entity's UserIdentity/ExternalIdentity structs
     to those flat columns (mirrors Rust). `email_domain` is now
     computed at write-time from the email. Delete now explicitly
     clears `iam_principal_application_access` and
     `iam_client_access_grants` (only `iam_principal_roles` has FK
     ON DELETE CASCADE).
   - `webauthn/credentials` — repo wrote to a non-existent `credential`
     column. Schema has `passkey_data` (JSONB). Fixed the column name.
   - `webauthn/ceremonies` — INSERT to `oauth_oidc_payloads` omitted
     the `type` NOT NULL column. Now sets `type` to
     `WebauthnRegistration` / `WebauthnAuthentication`. Adds
     `PurgeExpired` to match Rust's purge surface.
   - `auth/OAuthClient` — repo wrote 5 columns that don't exist
     (`secret_hash`, `redirect_uris`, `grant_types`, `scopes`,
     `principal_id`). Schema has `client_secret_ref`, `default_scopes`
     (comma-joined VARCHAR), `pkce_required`,
     `service_account_principal_id`, plus junction tables for redirect
     URIs + grant types. Repo now: wires the `oauth_client_redirect_uris`
     and `oauth_client_grant_types` junctions; writes the Argon2 hash
     to `client_secret_ref`; joins `Scopes` as comma-string into
     `default_scopes`; maps `PrincipalID` to
     `service_account_principal_id`; defaults `pkce_required=true`.
     **Three more junction tables exist** —
     `oauth_client_post_logout_redirect_uris`,
     `oauth_client_allowed_origins`, `oauth_client_application_ids` —
     and the entity is missing `PostLogoutRedirectURIs`,
     `AllowedOrigins`, `ApplicationIDs`, `PKCERequired` fields. These
     are a separate task once the API surface needs them (see §4
     follow-ups).
   - `auth/IdpRoleMapping` — repo wrote `idp_type` and
     `platform_role_name` columns. Schema has only `idp_role_name`
     and `internal_role_name`; there's no `idp_type` (matches Rust,
     where the column was dropped). The entity's `IdpType` field is
     now ignored on persist and reads back as `""` to keep the API
     shape stable. `PlatformRoleName` maps to `internal_role_name`.
   - `auth/ClientAuthConfig` — the JSONB-array columns already matched
     the schema (this part was previously fixed); pure sqlc port.
   - `auth/AnchorDomain` — trivial sqlc port; no schema bugs.
   - `audit` — repo referenced `application_id` + `client_id` columns
     on `aud_logs`; they exist (added in 009_p0_alignment.sql) — no bug,
     but flagged `nullable JSONB` columns generating `[]byte` instead of
     `json.RawMessage` (the `db_type: "jsonb"` override only catches the
     non-nullable case). Wrapped in a `jsonOf()` helper. `DistinctValues`
     stays hand-rolled (dynamic column name).
   - `application/client_config` — trivial sqlc port; no schema bugs.

   **All repos migrated.** `dispatchjob` was the last; done in §7 #6.

   - `event` → reconciled + sqlc-migrated (see §4 boot-smoke note).
   - `scheduledjob` → sqlc-migrated. Schema (migration 021) matched
     the entity 1:1 — no reconciliation needed; `FindWithFilters` kept
     hand-rolled for the optional-filter pattern (mirrors application
     repo), all other ops go through `*dbq.Queries`.
   - `dispatchjob` → **Done.** Entity reconciled against the post-019
     schema (composite PK `(id, created_at)`, partitioned), repo
     sqlc-migrated. Specifics: dropped phantom `last_status_code`,
     `next_retry_at`, `dispatched_at`; added `last_attempt_at`,
     `last_error`, `duration_millis`, `scheduled_for`, `expires_at`,
     `idempotency_key`. `Metadata` switched from `map[string]string` →
     `[]Metadata{Key,Value}` so the JSONB column is `[]`-array shaped
     per Rust drop-in parity (the SDK BatchItem follows). Attempt
     persistence: `RecordAttempt` now mints a row id (TSID), derives
     the `status` column from the entity's `Success` bool, and drops
     the phantom `success` column. `FindWithFilters` + `DistinctValues`
     + `InsertBatch` stay hand-rolled (dynamic SQL / pgx.Batch);
     everything else goes through `*dbq.Queries`.

   **Build state.** `go build ./...`, `go test ./...`, `go vet ./...`,
   and `go run ./tools/analyzer/uowseal ./internal/platform/...` all
   pass after the sweep.

   **Boot smoke (run).** `fc-dev start --embedded-db-reset` boots cleanly
   on a fresh PG. The following round-tripped end-to-end through the
   new sqlc-backed repos:
   - `POST /api/principals` → 201 + `GET /api/principals/{id}` → 200
     with `userIdentity.email` correctly read back from the flat
     `email` column. List endpoint hydrates roles/assignedClients/
     accessibleApplicationIds as empty slices (junctions not wired in
     Persist by design — Phase 3c deferral).
   - `POST /api/oauth-clients` (CONFIDENTIAL) → 201 + returns the
     plaintext secret. `GET /api/oauth-clients/{id}` hydrates the
     `redirect_uris` and `grant_types` junction rows correctly.
   - `POST /api/oauth-clients/{id}/rotate-secret` → 200 + new
     plaintext secret. The hash lands in the `client_secret_ref`
     column via the sqlc `OAuthClientUpsert`.
   - Subsequent `POST /oauth/token` against the rotated client reads
     the row through the fosite Storage adapter (proves the read path).
     The 401 it returned was a business-logic check ("Client has no
     owning principal") — fosite read the row fine.
   - `GET /api/audit-logs` → returns rows with `entityType`,
     `entityId`, `operationJson`, `principalId` populated; the
     `IS NULL OR ...` filter (`?entityType=Oauthclient`) narrows
     correctly; `/api/audit-logs/entity-types` distinct-values facet
     works.
   - `POST /api/email-domain-mappings` with `additionalClientIds`
     populated → 201; read back hydrates the JSONB junction. (One
     gotcha: `identity_provider_id` is `VARCHAR(17)`; longer test
     IDs fail with a generic `PERSIST` 500 because the
     `repository persist failed` envelope swallows the underlying
     pgx error. Worth surfacing the cause to the API layer in a
     future polish pass.)
   - `GET /api/anchor-domains`, `GET /api/idp-role-mappings` →
     return empty lists; query paths exercised.

   **Not exercised** (require external state or a passkey
   authenticator): webauthn register/authenticate,
   `application/client_config` enable/disable (no direct API route
   today — driven by `POST /api/applications/{id}/clients/{id}/*`).

   **Pre-existing runtime errors surfaced by boot — now fixed:**
   - ~~`projection_status` doesn't exist~~ — `event_projection` +
     `dispatch_job_projection` now use `projected_at IS NULL` as the
     unprojected predicate (mirrors Rust). Both projectors run
     cleanly on an embedded boot.
   - ~~`fanout_status` doesn't exist~~ — `event_fan_out` now uses
     `fanned_out_at IS NULL`. The fanout implementation is
     **scope-limited**: it currently just stamps `fanned_out_at` on
     unfanned events without producing dispatch jobs (the "no
     subscriptions" fast path). Full pattern-matching subscription
     lookup + dispatch-job production lands together with the
     dispatchjob entity reconciliation — see §7 #6.
   - ~~`dispatch_mode` doesn't exist on msg_dispatch_jobs~~ — the
     scheduler poller now reads `mode` (the actual column name).
   - ~~`next_retry_at` doesn't exist~~ — the embedded schema has
     `scheduled_for` (from migration 004) but not `next_retry_at`
     (added in 011's `CREATE TABLE IF NOT EXISTS` which is a no-op
     once 004 has created the table). Poller now matches Rust:
     `WHERE status = 'PENDING'` only, ordered by
     `message_group ASC NULLS LAST, sequence ASC, created_at ASC`.
     Retry timing is owned by the dispatcher's backoff loop.
   - ~~ON CONFLICT mismatch~~ — `msg_events_read` and
     `msg_dispatch_jobs_read` PKs became `(id, created_at)`
     composites in migration 018. Both projectors now use
     `ON CONFLICT (id, created_at)`.
   - **Event aggregate reconciled.** `internal/platform/event/`
     entity + repo rewritten to match the actual schema:
     - Added `Time` field to the entity (CloudEvents `time`,
       distinct from `CreatedAt` which is DB insertion time).
     - `InsertBatch` now writes to `context_data` (was phantom
       `context`), includes `time` (was missing, NOT NULL
       constraint), drops `ON CONFLICT (deduplication_id)` (composite
       unique index can't always be inferred).
     - `FindByID` + `FindWithFilters` + `DistinctValues` drop the
       phantom `principal_id` column (no backing column on
       msg_events_read; the PrincipalID() helper still pulls from
       Context but reads come back with Context=[]).
   - **Event projection moved to Rust's CTE shape** — splits
     `application`/`subdomain`/`aggregate` from the `type` string
     via `split_part(type, ':', N)`; reads `e.data::text`, picks up
     `correlation_id`/`causation_id`/`message_group`/`client_id`.

   **End-to-end event verification (post-fix).** Create a principal
   → audit log row appears → event_projection picks up the
   `msg_events` row → `/api/events` returns 1 row with split
   `subject`/`type`/etc; `/api/events/{id}` and the filter +
   distinct-values endpoints all work.

   **Remaining stream gap (dispatchjob/scheduledjob):**
   the fanout fast path no longer crashes the projector loop, but
   dispatch-job production from subscription matches is stubbed.
   Lands with §7 #6 (dispatchjob entity reconciliation).

4. ~~**Boot smoke against the sqlc sweep.**~~ **Done.** All migrated
   repos round-trip end-to-end on a fresh PG. See §3 above for the
   exact endpoints exercised. The pre-existing
   `projection_status`/`fanout_status`/`dispatch_mode` column-mismatch
   errors on the stream + dispatch loops are now confirmed
   stream-processor blockers (not just CRUD-side) — the
   dispatchjob/scheduledjob/event schema reconciliation is the next
   bottleneck.

5. ~~**Init bootstrap depth**~~ **Done.** `fc-dev init` now mirrors the
   Rust `bin/fc-dev/src/init.rs` flow:
   - Runs migrations + the built-in seeds (idempotent).
   - Creates the anchor admin if no anchor USER exists — wires the
     internal IDP row, an anchor EDM for the admin's domain, the
     Principal with hashed password, and the `platform:super-admin`
     role assignment.
   - Resolves or creates the Default Client.
   - Errors if the supplied `--code` already exists; else creates the
     Application.
   - Mints the SA: a SERVICE Principal + a ServiceAccount row +
     attach back to the application + a CONFIDENTIAL OAuth client
     with `client_credentials` grant pointing at the SA principal.
   - Writes `.env` with `FLOWCATALYST_BASE_URL/APP_CODE/CLIENT_ID/
     CLIENT_SECRET` — in-place update for existing keys, appended
     under a `# FlowCatalyst (added by fc-dev init)` header
     otherwise. Idempotent: re-running with the same flags overwrites
     only the changed keys.
   - Flag set: `--admin-email`, `--admin-password`, `--code`,
     `--name`, `--app-type`, `--description`, `--default-base-url`,
     `--client-identifier`, `--client-name`, `--api-base-url`,
     `--root`, `--yes`, `--database-url`. `--yes` requires the
     required fields (`code`, `name`, `admin-email/password` on first
     run) to be provided as flags.

   **Two latent bugs caught + fixed during init port (both since
   superseded):**
   1. ~~`hashPassword`/`hashSecret` base64 stopgap~~ — replaced by the
      shared `passwordhash` PHC envelope (§4 #4 above).
   2. ~~`attach_service_account.go` FK bug~~ — fixed (§4 #20 above).
      The use-case now resolves the SA's principal id before writing
      `app.ServiceAccountID`.

   **WrapTxForBootstrap.** Added
   `pkg/fcsdk/usecasepgx.WrapTxForBootstrap(pgx.Tx) *DbTx` for
   infrastructure-bootstrap callers (init, seeders, admin tools) to
   reuse the sqlc-backed repos without going through the use-case
   envelope. Documented as bootstrap-only — production paths must
   still use `Commit/CommitDelete/CommitAll/EmitEvent`.

   **OAuthClient entity follow-up** (newly surfaced by the sqlc sweep):
   extend the entity with `PostLogoutRedirectURIs`, `AllowedOrigins`,
   `ApplicationIDs`, `PKCERequired`; wire the three corresponding
   junction tables. The repo currently hardcodes `pkce_required=true`
   and silently drops the other three concepts. Once the entity gains
   the fields, also extend create/update commands + API DTOs.
   Init currently uses a direct INSERT to `oauth_client_application_ids`
   as a workaround — the bootstrap is the one place that needs this
   linkage today.

6. ~~**Schema reconciliation for `dispatchjob`**~~ **Done.** Entity +
   repo reconciled against the post-019 partitioned schema, repo
   migrated to sqlc, and Rust's fanout pattern-matching ported into
   `internal/stream/fan_out.go` (`CachedSubscription` + wildcard
   matcher + dispatch-job assembly + per-cycle insert in the same tx
   as the `fanned_out_at` stamp).

   **End-to-end smoke verified on fresh embedded PG.** Created a
   subscription with pattern `test:demo:order:created`, posted a
   matching event via `/api/events/batch`, watched the fanout
   projector produce a `msg_dispatch_jobs` row with the correct
   payload (raw event data, `dataOnly=true`), `idempotencyKey`
   (`{eventId}:{subscriptionId}`), and inherited `mode=IMMEDIATE`
   from the subscription. The scheduler poller then claimed it
   (status PENDING → QUEUED).

   **Two latent bugs caught + fixed during the port:**
   1. **`msg_dispatch_jobs.id` is `VARCHAR(13)`** — Rust mints typed
      IDs (`djb_<13>` = 17 chars) which overflow the column. The Go
      port now uses `tsid.GenerateUntyped()` (13 chars) in both fanout
      (`internal/stream/fan_out.go`) and the SDK batch path
      (`internal/platform/shared/sdk/dispatch_jobs_batch.go`). Same
      latent bug exists upstream in Rust.
   2. **`metadata` JSONB column shape** — Rust stores `[{key,value}]`
      arrays; Go was marshalling `map[string]string` as `{k:v}`
      objects, breaking JSONB drop-in parity with consumer SDKs.
      Entity Metadata is now `[]Metadata{Key,Value}` end-to-end.

   **Known residual: subscription cache TTL race window.** The fanout
   subscription cache refreshes every 5s (matches Rust). Events
   ingested in the gap between a subscription being created and the
   next cache refresh will be claimed by the no-subs fast path and
   stamped `fanned_out_at` without producing jobs. Mitigation:
   producer apps shouldn't immediately follow `POST /api/subscriptions`
   with the first event. For tests, wait >5s before publishing. To
   tighten further: drop the TTL or refresh on every cycle.

7. ~~**Router gaps — loop-correctness pieces**~~ **Done.** RouterError,
   HealthService, WarningService, LifecycleManager all ported and
   wired into `internal/router/server.go`. 13 unit tests pass. See
   §4 #7 for the field-level summary. Manager-coupled lifecycle
   tasks (memory health, consumer restart, reaper) defer until the
   Go Manager grows the supporting surface. Next router work is the
   HTTP route surface (40+ endpoints under `/monitoring/*`,
   `/warnings/*`, `/config/*`) + Prometheus metrics.

8. ~~**Contract harness**~~ **Done (framework).** See §6 #3 for
   detail. To use it for the actual drop-in proof: bring up the Rust
   `fc-platform-server` (or whichever binary serves the API) on one
   port, bring up Go `fc-dev` on another, point the harness at both.
   Cases turn from SKIP → PASS as you supply auth (`ANCHOR_TOKEN` env)
   and as the YAML library grows. Wire into CI once both binaries can
   be brought up in the GitHub Actions runner (currently only Postgres
   is — Rust binary needs publishing).

9. ~~**Argon2id PHC salt**~~ **Done.** See §4 #4.

10. ~~**Frontend port**~~ **Done.** See §4 #14.

## 8. Conventions Cheat Sheet

- **Event types** follow `application:subdomain:aggregate:event` with
  hyphens (not underscores) inside segments. **Don't use underscores.**
  `platform:iam:user:roles-assigned` ✅
  `platform:iam:user:roles_assigned` ❌

- **Aggregate names with no hyphens** in the event-type catalog —
  e.g. `serviceaccount` (not `service-account`), `eventtype` (not
  `event-type`). HTTP routes use the hyphenated form
  (`/api/service-accounts`) but event-type codes don't.

- **Field naming.** `Type` (Go field) → `type` (JSON) is fine.
  But aggregate IDs use the aggregate's own field name in the payload
  — e.g. `principalId` (not `userId`) for user events, except where
  Rust uses `userId` (application_access_assigned) — see
  `principal/operations/events.go` for the deliberate divergence.

- **Source field on event metadata** always equals
  `application:subdomain` (e.g. `platform:iam`). Confusingly the
  earlier Go impl used `platform:admin` for IAM events — that was a
  bug, fixed during this port.

- **Per-aggregate code structure**:
  ```
  internal/platform/<aggregate>/
    entity.go              — aggregate root + sub-entities
    repository.go          — pgx-backed repo, implements Persist[T]
    operations/
      events.go            — every DomainEvent the subdomain emits
      create.go / update.go / etc. — one file per verb
    api/
      api.go               — chi routes, State struct, handlers
  ```

- **TSID prefixes** are pinned in `internal/tsid/tsid.go`. Don't
  reuse prefixes across entity types. Adding a new entity type means
  adding a new EntityType const + a new prefix in `Prefix()`.

## 9. Where the rough edges are

- The `usecasepgx.UnitOfWork.WithTx` shape doesn't quite match what
  the sync ops want — see the comment in
  `eventtype/operations/sync.go` about "TODO(sync-runtime): batch into
  one tx." A small refactor on the UoW would close that.

- `Scheduler.Run(ctx)` is naive — it doesn't surface the dispatcher's
  health, doesn't expose metrics, doesn't co-ordinate shutdown with
  in-flight messages. Adequate for now, not adequate for prod.

- ~~`cmd/fc-router/main.go` is monolithic — can't be imported by
  fc-server today.~~ Done — wiring lives in `internal/router/server.go`
  (`Server.Run(ctx)`). The cmd binary now only contributes signal
  handling + the `/health` `/ready` `/metrics` HTTP surface, and
  fc-server's `StartRouter` calls the same `Run`.

- The Go side has tests for the seed catalog + auth provider helpers,
  but **no integration tests against a real PG**. Every wired endpoint
  is technically untested until an integration harness lands.

## 10. Asking the Right Questions

If you're stuck and need to ask the user something, these are the
already-decided answers (don't re-ask):

- **Existing JWT compatibility:** not required. New tokens issued post-cutover.
- **Library choices:** fosite (OAuth), coreos/go-oidc (IDP bridge),
  go-webauthn (passkeys), pgx/v5 (Postgres), chi (router),
  robfig/cron/v3 (cron), fergusstrange/embedded-postgres (dev PG).
  All locked in.
- **Repo layout:** single Go module at github.com/flowcatalyst/flowcatalyst-go.
- **API parity:** byte-identical. No URL changes, no JSON shape changes.
- **Database:** same Postgres, same migrations, no schema rewrites.

If something doesn't match this contract, **fix the Go side, not the
contract.**
