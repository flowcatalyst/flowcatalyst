# Push gateway (A4): specification draft

Status: **accepted 2026-11-09 with the owner rulings of section 17** (ruling 3 brings instant
outbox wake-ups into scope). Build order: section 14. Steps 2a (the gateway core) and 2b (sync,
ingest, the outbox `PUSH` row, instant wake-ups, per-application settings) are built: see "Build
status".

The gateway builds on the framework push contract
(`flowcatalyst-framework/docs/contracts/push.md`, accepted 2026-11-08, amendments 17.1 to 17.15).
Section 12 of that contract sketches A4. This document turns the sketch into a buildable plan for
the platform. Everything marked **new** does not exist yet. File paths are this repo's unless they
name the framework.

## Build status

**Step 2a, the gateway core, is built** (branch `feat/push-gateway`; code in
`internal/platform/pushgw`, wiring in `internal/server/push.go`), **and so is step 2b**. Sections
2, 2.1(a), 2.2, 4, 5, 6, 8, 10 and 12.1 are done as written, except for the points marked
*differs*; the "Step 2b" list below says what was built and decided. Not built yet: console
pages 1 and 2 (11) and the stats rows behind page 2, the recent-pushes and test-send pages (11),
the SDK changes other than Go (13: Java, TypeScript, Laravel, Rust, Python), `push.driver=platform`
in the framework (13), and the load and isolation tests of nonprod (14).

What exists:

- **Component.** `FC_PUSH_ENABLED` (envcfg, `needsDB`, `startPushIfEnabled` in `run.go`; fcdev
  `--push`, default on, a random stream secret per start). `Run` starts the gateway before it binds
  the listeners, mounts it at `/api/push/*` and adds `GET /health/push`. At shutdown `Drain()` runs
  before the HTTP server's own shutdown and the module stops after it. Settings are the module's
  `push.*` as `FC_PUSH_*` (`docs/environment-variables.md`).
- **Hosting (2.1(a)).** `pushgw.Start` builds a small embedded `fw.App`: environment configuration
  only (no files), the platform database on a pool of its own, locks on that database, the module
  declared with `push.ExternalMigrations()` and `push.OptionalTicketKey()`, and
  `push.MultiApplication` with a `Store` (rules, ticket keys and limits read from
  `push_rules`, `push_ticket_keys`, `push_app_settings`; reloaded on `NOTIFY fw_push_config`, polled
  every 60 s, active applications only). A1 is the default; `FC_PUSH_DRIVER=redis` selects A2 with
  `FC_REDIS_URL`.
- **Migrations.** `104_push_tables.sql`: the framework's push tables and lock table, verbatim, then
  the gateway's five tables. `105_push_field_session_end.sql`: the revocation trigger (below).
- **Authenticator (5).** Bearer access tokens and the session cookie (with the console's CSRF
  rule, `middleware.SessionCSRFChecker`) for users and service accounts; field tokens with DPoP
  (proof signature, `htm`, `htu`, `ath`, key = `cnf.jkt`, then `TokenActive`); tickets alone pass
  through. Client rules are those of section 4. *Differs:* the permission is
  `platform:push:stream:all-clients`, four segments, because wildcard matching is segment-wise and
  the platform pins four segments (a three-segment name could never be held by `platform:*:*:*`).
  A stale session cookie counts as no credential, as for the platform's own authenticator. The
  proof's nonce is not required (the resource side has no nonce store); freshness is the proof's
  `iat` window plus the token's own expiry.
- **Revocation (5).** *Differs from the sketch:* instead of a call in each `fieldidentity`
  operation (there are about ten raw `UPDATE field_sessions` sites, several inside use-case
  transactions), a statement trigger on `field_sessions` notifies `fc_push_session_end` with the
  login family when a session is revoked for a reason other than `ROTATED` or deleted live. Every
  platform sign-out (`/auth/logout`, the OIDC session end, `/ui/logout`) notifies
  `fc_push_principal_end`. Each gateway task listens and calls `EndStreams`, which reaches every
  task through the log's end channel on either driver. It works when the gateway is a separate
  service, and no wire is duplicated.
- **CORS (4).** Only `/api/push/*`, from `tnt_cors_allowed_origins` (15 s cache), never
  `Allow-Credentials`; a preflight from an unlisted origin is 403.
- **Tests.** `internal/platform/pushgw/*_test.go` (the `*_pg_test.go` files need
  `-tags=integration`): the four conformance vector files over HTTP (pinned copies in
  `testdata/conformance/push/`; two ticket cases and three rule cases that cannot be expressed
  through a platform credential are listed in the test log), two tasks on one database, the
  authenticator matrix, field DPoP, revocation across tasks, drain, CORS, per-application limits.

### Step 2b (built 2026-11-09)

- **Sync (8).** `POST /api/applications/{appCode}/push/sync`, `pushgw.ConfigRepo` and
  `ValidateSync`. Checks: `auth.CanSyncPush` (`platform:application-service:push:sync`),
  `resolveApp`, `requireAppAccess`; rules whose pattern starts with another application's code,
  open rules (unless `public`), a public rule with `requires`, duplicate patterns, more than 200
  rules, more than 4 keys, keys that are not public Ed25519 JWKs, duplicate kids are refused (400,
  with a code) before anything is written. One transaction replaces the rules and the keys and
  sends `NOTIFY fw_push_config` (delivered at commit; every task reloads within a second, tested
  on two tasks). The result is `{applicationCode, rules{created,updated,deleted},
  keys{...}, syncedPatterns, syncedKids}`. Audited (`PUSH_CONFIG_SYNCED`, with the actor).
  Classified as an app operation in the SDK spec (`tools/dump-spec/sdkclass.go`).
- **Ingest (6.2).** `POST /api/push/batch` is served by the push component (a small huma API of
  its own, `Gateway.IngestHandler`, mounted at `/api/push/batch`), not by the platform's API,
  so a gateway without the platform API serves it and a load balancer that sends `/api/push/*`
  to the gateway sends it there. It is therefore not in `api/openapi.lock.json`. Bearer tokens
  only (cookies are dropped). Permission `platform:messaging:batch:push-write`; the built-in
  `platform:application-service` role carries it (migration 107 grants it to existing
  deployments). Per item, in this order: topic syntax, ownership of the application
  (`TOPIC_NOT_OWNED`, FORBIDDEN, for an application-scoped caller; an unknown application is
  `APPLICATION_UNKNOWN`, BAD_REQUEST, for an unscoped one), type and data (the framework's codes,
  BAD_REQUEST, with the application's data limit), `pushId` (`PUSH_ID_INVALID`), the client
  (`sdk.ResolveEventClient`; a refusal is `CLIENT_FORBIDDEN`, FORBIDDEN; the platform client is
  NULL). Duplicate `pushId`s (within the request, or from an earlier one) are answered SUCCESS
  with code `DUPLICATE` and written once; the ids are recorded in `push_ingest_ids` in the same
  transaction as the write (`PublishBatch`, one statement, one NOTIFY), so a failed write records
  nothing. The answer is positional `{results:[{id,status,error,code}]}` (`id` = the pushId).
  `push_ingest_ids` is pruned past `push.retention` (every 5 minutes, by every task).
- **Pause and rate (10).** An application that is paused, or over its publish rate, answers the
  whole batch `429` with `Retry-After` and writes nothing (rate: a token bucket per application
  **per gateway task**, `push_app_settings` rate and burst, default 500/s and 2,000; a batch
  larger than the burst takes the whole bucket; paused: `Retry-After: 5`). The platform's rate
  limiter (`shared/ratelimit`) is a login-attempt window store and does not fit, so
  `golang.org/x/time/rate` is used. *Limit:* the rate is per task, so N tasks admit N times it.
- **Outbox (6.1, 13).** `common.OutboxItemPush = "PUSH"` (`APIPath` `/api/push/batch`). The
  Go SDK has `outbox.TypePush`, `Push` / `NewPush`, `PushRow`, `PushPayload`, the refusal codes
  (`PushError`), `Sink.WritePush` (outboxpgx) and `Writer.WritePush` (outboxsql). The payload is
  `{"topic","type","data","clientCode","pushId"}` in that order, `data` the object itself, the
  group is the topic unless `WithMessageGroup` / `WithUngrouped`, the client rule is the dispatch
  job's, `pushId` is the row id (at most 26 characters, the column's width). The conformance
  vector has two `pushes` cases and `invalidPushes`.
- **429 does not spend attempts.** The platform's processor used to retry a 429 like any other
  failure, three times in about three seconds, and then leave the row failed (a pause would have
  lost the group). It now holds the message group (or the type, for ungrouped rows) for the
  `Retry-After` and puts the rows back to PENDING with no attempt, so a pause or a burst waits in
  the application's own outbox however long it lasts, and order within a group is kept. This is
  the processor's rule for every row type (`DispatchOutcome.RetryAfter`).
- **Instant wake-ups (ruling 3).** The writer NOTIFYs `fc_outbox_push` in the statement that
  inserts a `PUSH` row (`WITH woken AS (SELECT pg_notify(...)) INSERT ... SELECT ... FROM woken`,
  the framework's own pattern, so the notification is delivered at the commit and a rollback
  notifies nobody). A Postgres outbox repository implements `outbox.Waker` (`LISTEN
  fc_outbox_push` on a connection of its own, reconnecting, and a catch-up wake after every
  connect); the processor polls at once on a wake (leader-gated and back-pressured like a tick). The
  poll stays as the fallback (writers that cannot notify: MySQL, SQLite, the other SDKs until they
  learn the statement; a lost connection). *Why in the statement, not a trigger:* it needs no DDL on
  the application's outbox table (the SDK migration is the application's) and works for any
  application whose processor polls Postgres; a trigger remains an option for a writer that cannot
  run the statement. Measured on one machine with embedded Postgres, the processor polling every
  30 s: insert to frame on a listening stream, 40 pushes after 5 warm-up, p50 3 to 6 ms, p95 about
  9 ms, max 10 ms.
- **Settings (10).** `GET` / `PUT /api/applications/{appCode}/push/settings`: anchor scope and
  `platform:admin:push:manage` (in super-admin), audited (`PUSH_SETTINGS_CHANGED`), NOTIFY on
  change. A PUT is the whole set of overrides (what it leaves out returns to the default); the
  answer also gives the effective values. Tag `push-gateway` (admin class).
- **Field workers are tightened (5, owner ruling 2026-11-09).** A field token's `aud` is its
  application. The module decides rules and tickets without knowing the credential's
  application, so the gateway narrows from outside: (1) the store adds the synthetic requirement
  `fc:push:app:<code>` to every non-public rule; a platform credential holds `fc:push:app:*`, a
  field worker holds `fc:push:app:<its aud>` only; (2) `fieldScope` removes from a field
  worker's subscribe body the tickets whose `iss` is another application (read unverified; it
  only removes, and the module still verifies the token). Their topics are denied
  `TOPIC_FORBIDDEN`. *Public rules stay open to everyone* (a public topic of another application
  is heard by a worker of any application, by definition). A user's or service account's
  credential is not narrowed.
- **DPoP replay (5, owner ruling 2026-11-09).** The field plane accepts a proof whose `iat` is
  within 24 hours (devices keep bad clocks) and defends its own endpoints with single-use nonces;
  the gateway's subscribe has no nonce, so the proof's `jti` is the one-time value:
  `PGReplayGuard` records `sha256(jkt, jti)` in `push_dpop_proofs` (migration 106) with the
  proof's window (`iat` + 24 h + 1 min), after the token, the proof and the live session have
  been checked, so a failed request takes no place. A second use is `401 invalid_dpop_proof`. The
  table is shared by every task and pruned every 5 minutes; it holds one row (about 100 bytes)
  per subscribe of the last day.
- **Standalone gateway (12.1, owner ruling 2026-11-09).** A gateway running with
  `FC_PLATFORM_ENABLED=false` validates tokens the platform minted, so it must be configured with
  the platform's JWT signing key (`FLOWCATALYST_JWT_PRIVATE_KEY`, `FC_JWT_SIGNING_KEY_PEM` or
  `FC_JWT_SIGNING_KEY_PATH`; the same `FC_JWT_ISSUER`; `FC_JWT_PREVIOUS_PUBLIC_KEY` during a
  rotation). With no key configured it **refuses to start** with a message that says so, unless
  it is a development process (`FLOWCATALYST_DEV_MODE=true`, fcdev).
- **Metrics (11).** The framework registry (`fw_push_*`) and the gateway's counters
  `fc_push_ingest_items_total{application,result}` and
  `fc_push_ingest_rejected_total{application,code}` are served on the platform's Prometheus
  listener beside the process metrics. The module's series are not yet labelled by application
  (that needs a framework change).
- **Second database pool.** The module opens its own pool (`database.Open`); the framework's
  `DB` cannot wrap an existing pool, so the gateway cannot share the platform's without a
  framework change. It is bounded instead: `FC_PUSH_DB_POOL_MAX` (default 10, pgx's
  `pool_max_conns`). Ingest and sync use the platform's pool.

## 1. Goals and non-goals

**Goals**

1. Applications that cannot hold streams themselves can push to their clients. These include
   Laravel/PHP-FPM, TypeScript, functions, and any app without a long-lived server. They publish
   through the outbox they already write; the platform holds the streams.
2. A client listens to several applications on one stream, for example a field device running
   two apps.
3. The wire is the framework contract's, unchanged:
   - `POST /push/subscriptions`, `GET /push/stream`, `GET /push/keys`;
   - the `subscribed`, `reset` and `auth-expired` events;
   - the vectors in `conformance/push/`.

   The existing readers work against the gateway by changing `baseUrl`. These are field-js
   `PushStream` and field-react-native `PushStream`.
4. Applications are isolated from each other:
   - Per-application limits cover publish rate, data size and open streams, and each app has a
     pause.
   - One application's burst never slows another's streams.
5. Operators get one view: the console, metrics and a gated "recent pushes" trace.

**Non-goals**

- Re-implementing push. The gateway is the framework's Go module `go/push` (hub, relay, logs,
  tickets, tokens, the wire), configured for many applications (section 3).
- Replacing app-hosted push. A framework app with its own servers (fulfil-go) keeps A1/A2 in its
  own database; the gateway is for the rest. An app may move between the two by configuration
  (`push.driver`, section 6.4).
- Delivery guarantees beyond the contract. Pushes are signals; durable work is events and dispatch
  jobs.
- Ordering across applications, or across message groups of one application (section 6.1).
- A dependency on any one cloud provider's managed services (section 12).

## 2. Architecture

```
app tx ──► outbox_messages (PUSH row) ──► outbox processor ──► POST /api/push/batch ──┐
                                                                                     ▼
                         gateway task(s): ingest ─► fw_push_outbox ─► relay (one, locked)
                                                                         │
                                                                         ▼
                                                      log: A1 fw_push_log | A2 Redis stream
                                                                         │ follow
         browsers / apps / field devices ◄── SSE ◄── hub (each task) ◄────┘
```

- **One component, `push`, in the `fc-server` binary** (section 12.1), running the framework's
  `go/push` module. The ingest endpoint writes pushes with the module's `PublishAlone`, one
  transaction per batch. The relay, log, hub, stream tokens and wire are the module's, unmodified.
- **The ingest path is the platform's.** The SDK outbox row, the outbox processor and a batch
  endpoint follow the pattern of `/api/events/batch` (`internal/platform/event/api/api.go`) and
  `/api/dispatch-jobs/batch` (`internal/platform/shared/sdk/dispatch_jobs_batch.go`).
- **Subscriber authentication is the platform's** (section 7). It is adapted to the module's
  `Authenticator` (framework `go/push/auth.go`: `Credentials{Context, ExpiresAt, ProofKey, Device,
  Session}`). The platform identity becomes an `fw.Context` through `fw.ContextFor(client)
  .WithPrincipal(...).WithPermissions(...)` (framework `go/fw/context.go`). No new identity model is
  needed.

### 2.1 What the framework module needs for A4 (framework contract amendment, new)

`go/push` is built for one application today, and four things must change. These are framework
changes, made in Go first, then Java where it applies. They go into the framework contract as a
section 17 amendment.

1. **Many applications in one host.**
   - The `granter` holds one `application`, one rule list and one `ticketKeys`
     (framework `go/push/grant.go:27`). Tickets are checked against `g.application` and the host's
     own keys (`go/push/tickets.go`).
   - New: a multi-application mode. Rules come from a provider keyed by application code. Ticket
     keys come from a resolver keyed by `iss` and `kid` (section 9). The check "the topic's first
     segment is the application" applies per application, not per host.
   - A single-application host behaves exactly as today.
2. **Limits per application.**
   - The hub's limits are host-wide and per principal (`push.max-streams`,
     `push.max-streams-per-principal`; `admitLocked` in `go/push/hub.go`).
   - New: per-application stream limits, plus a hook for publish rate and data size per
     application (section 10). The hub counts streams per topic application. A stream that spans
     several applications counts against each of them.
3. **Mounting without the framework HTTP server.**
   - `Pushes.Routes()` returns `*httpfw.Routes` (framework `go/push/module.go:338`). These mount
     only in an `httpfw` server, and `Build` takes the framework's `*fw.Boot`, `database.DB` and
     `locks.Locks`.
   - fc-server is not a framework app: chi/huma routes, a `pgxpool.Pool`, its own `server.Run`.
   - New: a way to host the module inside fc-server, either:
     - **(a)** a small embedded `fw.App` (env config, the platform's database URL, locks on that
       database), its routes exposed as an `http.Handler` mounted at `/api/push`; or
     - **(b)** a constructor that takes a pool, a lock and a logger without `fw.Boot`.

     Recommended: (a). It keeps the module's config, loops, health and drain wiring as tested. It
     needs `(*Pushes).Handler() http.Handler` (new) and the drain signal fed from fc-server's
     shutdown.
4. **A `platform` publish driver for framework apps** (the contract's section 12, "push.driver
   differs (platform)").
   - With `push.driver=platform`, `usecase.Push` / `andPush` and `Publish` write an outbox row of
     type `PUSH` to the app's `outbox_messages` in the same transaction, instead of
     `fw_push_outbox`. Section 6.1 gives the row format.
   - The app hosts no push routes. `Pushes.Ticket` still mints tickets with the app's
     `push.ticket.key`, for the gateway to verify.
   - Framework rules and keys are pushed to the platform by the framework's `flowcatalyst` sync
     (section 8). This is new in Go and Java.

### 2.2 Module and migrations in this repo

- The platform imports `github.com/flowcatalyst/framework/go`. It already imports
  `github.com/flowcatalyst/framework/go/ui` (`go.mod:21`). pgx (`v5.9.2`) and go-redis (`v9.19.0`)
  are the same versions in both `go.mod` files, so no version conflicts are expected.
- The module's tables (`fw_push_outbox`, `fw_push_log`) come from the framework migration
  `go/push/migrations/V0001__create_push_tables.sql`. They are copied byte for byte into a platform
  migration, `internal/migrate/sql/104_push_tables.sql` (new; the last migration today is
  `103_field_identity_phone.sql`).
- The platform runs goose migrations at start (`cmd/fc-server/main.go`, `migrate.Run`), not the
  framework's migration sets. So with option (a), the embedded app must skip its own `push`
  migration set; the platform migration owns the tables.
- The gateway's own tables are new: rules, keys, ingest ids and stats (sections 6.3, 8, 11). They
  go in the same or following migrations.

## 3. Log driver

Recommendation: **start with A1 on the platform database; switch to A2 by configuration when
measured load warrants it.** Both drivers pass the same contract tests in Go and Java (framework
amendments 17.10 and 17.15).

- **A1 (Postgres).**
  - Cost per push: one ingest insert into `fw_push_outbox`, then the relay's move into `fw_push_log`
    with a NOTIFY, then retention deletes.
  - It adds no service. The local load test held 5,000 pushes/s fan-out at a p99 of about 6 ms
    (framework `docs/push-load-test-results.md`).
  - Its risk is competition with event ingest on the same database. The platform's ingest
    throughput work targets 20k/s writes (memory: ingest throughput plan).
- **A2 (Redis Streams).**
  - It moves the log, its reads and its retention off Postgres. The outbox and the relay stay in
    Postgres.
  - The platform already reads `FC_REDIS_URL` for rate limiting
    (`internal/platform/shared/ratelimit/ratelimit.go:163`), so the gateway uses the same setting.
  - Redis 6.2+ or Valkey is required.
- **When to switch:** sustained publish rate above about 1,000/s, more than two gateway tasks, or
  when database CPU attributable to `fw_push_*` rises above about 10%. The switch is `push.driver`
  plus a restart. Streams reset once, because the log ids change from A1's 1, 2, 3 to A2's
  `ms<<20|seq`, and clients reload.

## 4. Wire seen by apps and clients

- **Base URL.** The gateway serves the module's routes under the platform's public origin at
  `{platform}/api/push` (new):
  - `POST /api/push/subscriptions`
  - `GET /api/push/stream?st=…`
  - `GET /api/push/keys`

  A reader's `baseUrl` is `{platform}/api`, so field-js / field-react-native `PushStream` work
  unchanged.
  - In a deployment where the gateway runs as its own service, the load balancer routes
    `/api/push/*` to it (section 12).
  - fcdev serves the routes in-process on `:8080`.
- **Bodies, events and errors** are the framework contract's sections 6 and 7, byte for byte. The
  shared vectors in `conformance/push/` run against the gateway in its contract tests (section 15).
- **One addition: an optional `client` member** in the subscribe body (new; a framework contract
  amendment).
  - A platform user may have access to several clients (anchor and partner tiers).
  - The framework derives a stream's client from the request's `Context` (6.1).
  - The gateway needs the user to name one: `{"topics": [...], "tickets": [...], "client":
    "<clientId>"}`. It must be one the credential can access (`ac.CanAccessClient`, as in
    `ResolveEventClient`, `internal/platform/shared/sdk/client_code.go:133`). Otherwise the answer
    is `403 CLIENT_FORBIDDEN`.
  - Without `client`, the credential's only client is used. A credential with several clients and
    no `client` is `400 VALIDATION`.
  - A platform-wide stream (one that hears every client) must ask for the platform client
    explicitly. It needs the new permission `platform:push:all-clients`. No stream becomes
    platform-wide by default.
- **CORS.** Web apps on their own origins call the gateway's origin with a bearer token or tickets
  only, never a cookie. The gateway answers CORS for `/api/push/*` only, from the platform's
  allowed-origins table (`internal/platform/cors/entity.go`; new: applying it to these routes). It
  never answers with `Allow-Credentials`, as the framework's CORS rule requires.

## 5. Who can subscribe (authentication)

The gateway's `Authenticator` (new) reads the request's credentials and returns `Credentials`.

| Subscriber | Credential | Maps to |
|---|---|---|
| Platform user or service account | Bearer access token, or the platform session cookie with the console's CSRF rule | Principal = `prn_…`. Client per section 4. Permissions = the token's granted app permissions (the `scope` of memory note "token scope + permissions"). `ExpiresAt` = token or session expiry. |
| Field worker | A field access token (`typ fc-field+jwt`) with its DPoP proof | Validation exists: `authservice.ValidateFieldToken` (`internal/platform/fieldauth/verifier.go:20`), `ParseProof` (`internal/platform/fieldauth/proof.go:98`). The session check also exists: `fieldidentity` `TokenActive`. Principal = the field identity, client = the token's client. `ProofKey` = the proof's thumbprint; `Device` and `Session` = the token's device and login family. |
| Anyone holding app-minted tickets | No credential, only tickets (section 9) | The client comes from the tickets (framework 6.4). |
| Portal users (`ptu_`) | Later; open question 6 | |

- A field worker is granted the topics of the application its token is for only, by rule or by
  ticket, and each DPoP proof is accepted once (see "Step 2b").
- Bad credentials answer `401` even when good tickets come with them, as in the framework.
- A field revocation (login family or device revoked, `fieldidentity` operations) calls
  `pushes.EndStreams(Holder{...})` in the platform process (new hook). The holder reaches every
  gateway task through the log's end channel (A1 `pg_notify('fw_push_end')`, A2 pub/sub).
- Signing a platform user out ends that principal's streams: `EndPrincipal`. The framework lists
  this as "not built yet" for sessions; the gateway hook is new.

## 6. Publishing

### 6.1 The outbox `PUSH` row (every SDK)

A new row type, `PUSH`, sits next to `EVENT`, `AUDIT_LOG` and `DISPATCH_JOB`
(`pkg/fcsdk/outbox/payload.go:34`; platform `internal/common/outbox.go:81`). It is written by the
app in the same transaction as its change. The columns are those of the shared `InsertSQL`
(`pkg/fcsdk/outbox/row.go`).

| column | value |
|---|---|
| `id` | a new TSID, as for other rows |
| `type` | `PUSH` |
| `message_group` | **the topic**, by default (see below). Overridable per push. |
| `payload` | `{"topic": "...", "type": "...", "data": {...}, "clientCode": "...", "pushId": "<id>"}`, camelCase in this order. `data` is the JSON object itself, never encoded JSON in a string (as `EVENT` data in the Go SDK). `clientCode` follows the dispatch-job rule (the writer's client, except `_platform`). `pushId` = the row id. |
| `client_id`, `payload_size`, `headers` | as for other rows |

- **The wake-up (ruling 3).** On Postgres the writer notifies in the statement that inserts the
  row: `WITH woken AS (SELECT pg_notify('fc_outbox_push', '')) INSERT INTO outbox_messages (...)
  SELECT ... FROM woken` (the Go SDK's `outbox.PushInsertStatement`), or a `SELECT
  pg_notify('fc_outbox_push', '')` in the same transaction. The notification is delivered at the
  commit. Other databases write the plain insert; the processor's poll delivers.
- **Why the topic is the message group.**
  - The outbox processor keeps FIFO order only within a message group. Ungrouped rows are batched
    by type into concurrent HTTP calls (`internal/outbox/processor.go:142-173`), so two ungrouped
    pushes on one topic can arrive in either order.
  - A push that carries state (allowed, framework 1) must not let an older state overtake a newer
    one.
  - The cost: grouped rows are sent one at a time per group (`p.distributor.Submit`), and
    block-on-error holds the group. Open question 4.
- **Validation at write:** the topic, type and data rules of framework section 1 (refusal codes of
  `conformance/push/topics.json`), and the data limit (default 16 KiB). The SDKs refuse invalid
  pushes when the row is built, as the framework does.

### 6.2 Ingest: `POST /api/push/batch` (new)

- **Shape** follows the other batch endpoints: `{"items": [payload, …]}` in, positional
  `{"results": [{"status": …, "code": …}]}` out. This keeps the processor's status mapping
  (`internal/outbox/http_dispatcher.go:33-49`).
- **Permission:** `platform:messaging:batch:push-write` (new), the counterpart of
  `platform:messaging:batch:events-write`.
- **Per item:**
  1. **App ownership.** The topic's first segment is the application. An application-scoped caller
     must have access to it (`AppScope.CanAccessCode`,
     `internal/platform/shared/sdk/app_scope.go:63`). Otherwise the result is `TOPIC_NOT_OWNED`
     (`FORBIDDEN`).
  2. **Client.** `clientId` or `clientCode` resolves through `sdk.ResolveEventClient` (the same
     writable-client rule as events). The platform client becomes NULL (framework 5). A refusal is
     `CLIENT_FORBIDDEN`.
  3. **Validation.** Topic, type and data limits, with the framework's codes (`TOPIC_INVALID`,
     `TYPE_INVALID`, `TYPE_RESERVED`, `DATA_INVALID`, `DATA_NOT_OBJECT`, `DATA_TOO_LARGE`) as
     `BAD_REQUEST`. A non-retryable item is marked failed by the processor, never retried.
  4. **Duplicates.** `pushId` is recorded in `push_ingest_ids(application_id, push_id, created_at)`
     (new, primary key on both ids) in the same transaction as the write. A duplicate answers
     `SUCCESS` and writes nothing. Rows older than `push.retention` are pruned.
  5. **Write.** The valid items of the batch are written with `PublishAlone` in one transaction
     (new: a batch form of `PublishAlone` in the framework, or one transaction around `Publish`
     calls).
- **Whole-batch answers:**
  - An application that is paused or over its publish rate answers HTTP `429` with `Retry-After`
    for the whole batch. The processor retries 429 and requeues (memory: "outbox 4xx-retry (429)
    fix").
  - So a pause or a burst waits in the app's own outbox instead of being lost.

### 6.3 Direct publish (apps without an outbox)

Functions and scripts can call `POST /api/push/batch` themselves with the same body. A service
account token scoped to the application is enough. Section 6.2's checks apply in full. There is no
transactional guarantee: the caller publishes after its own commit. It is offered for completeness;
the outbox path is recommended.

### 6.4 Framework apps (`push.driver=platform`)

Section 2.1 item 4. Application code is unchanged (`usecase.Push`, `Plan.andPush`). A framework app
can move between hosting push itself (A1/A2) and the gateway by configuration. Its clients'
`baseUrl` changes, and they reload once (their last ids mean nothing on the other host, so they get
`reset` `unknown-id`).

### 6.5 Ordering and latency through the gateway

- **Ordering.** The framework guarantee "log order = commit order" holds per host. Through the
  gateway it becomes:
  - pushes of one message group (default: one topic) of one application arrive in their outbox
    order;
  - otherwise in the order the gateway receives them.
- **Latency added** (ruling 3: instant wake-ups): the processor wakes on the commit of a `PUSH`
  row (`LISTEN fc_outbox_push`), so the poll interval no longer applies to a Postgres writer that
  notifies; then one HTTP hop and the relay (NOTIFY wake). Measured p50 3 to 6 ms locally (see
  "Step 2b"). A writer that cannot notify waits for the next poll (default 1 s).

## 7. Topics, namespaces and applications

- Every topic's first segment is an application code (framework section 1). The gateway accepts
  pushes, rules and tickets for an application only on that application's topics, so namespaces
  never collide.
- The application must exist in the platform. Unknown applications are refused at sync and at
  ingest.

## 8. Rules and ticket keys sync (new)

- **Endpoint.** `POST /api/applications/{appCode}/push/sync`, next to the other self-registration
  endpoints (`internal/platform/sdksync/api.go:106-119`). It uses the same checks: the sync
  permission (new `auth.CanSyncPush`), `resolveApp`, `requireAppAccess`
  (`internal/platform/sdksync/api.go:145`), and app-scoped service accounts (memory note "app-scoped
  service accounts").
- **Body:** `{"rules": [{"pattern": "...", "requires": ["..."], "public": false}], "keys":
  <JWKS>}`.
  - It is a declarative full replace, like the docs sync (`docs_sync.go`).
  - Rules are refused unless the pattern's first segment equals `{appCode}`. A rule with neither
    `{principal}` nor `requires` is refused unless `public` is set (framework 6.3: `ErrOpenRule`).
  - Keys are Ed25519 OKP JWKs with a `kid`, at most 4. The framework host serves the current and
    the previous key during a rotation (framework 6.4: `/push/keys`), and sync sends the same set.
- **Storage (new):**
  - `push_rules(application_id, pattern, requires text[], public, synced_at)`;
  - `push_ticket_keys(application_id, kid, jwk, synced_at)`.

  Both are audited like the other syncs.
- **Propagation.**
  - Each gateway task caches rules and keys per application.
  - A sync NOTIFYs `fw_push_config` (new) so the caches reload at once. A 60-second poll is the
    fallback.
- **Rotation.** The app adds the new key, syncs, waits one `exp` window (at most 5 minutes) after
  it starts signing with the new key, then drops the old key and syncs again. A ticket signed with
  a `kid` the gateway does not know grants nothing (`TICKET_INVALID`), as in the framework.
- **Who calls it:**
  - The framework's `flowcatalyst` module at start-up, next to its other syncs (new, Go and Java).
  - The SDKs' sync commands: Laravel `flowcatalyst:sync`, TS, Java, Go (new members).
  - The console shows the result read-only (section 11).

## 9. Tickets across applications

- **Verification.** The gateway reads each ticket's `iss`, finds that application's synced keys by
  `kid`, and checks it with the framework's ticket check (framework 6.4) for that issuer. A failure
  only denies the ticket's topics (`TICKET_INVALID`), never the whole subscribe.
- **One subscribe may carry tickets from several applications** (framework 6.4, "composable").
  Every valid ticket must name the same `client`, or the stream's client when there is a
  credential. Tickets naming another client grant nothing (`TICKET_INVALID`), because a stream
  belongs to one client.
- **Binding.** `cnf.jkt` binding works as in the framework: the subscribe must prove the key with
  DPoP. Today that means a field-token subscribe: the module takes the proof key from the
  credential (`Credentials.ProofKey`). A DPoP proof on a ticket-only subscribe would be new (a
  framework amendment), and is not needed for the first release.
- **The platform does not mint tickets for applications.** Apps mint their own with the framework
  (`Pushes.Ticket`) or the SDK helper (section 13). Platform topics such as `platform:*`, if any
  are added, would use a platform key. Not in this scope.

## 10. Isolation, quotas and limits

| Setting (per application; the default applies to all) | Default | Enforced at | Answer |
|---|---|---|---|
| `push.app.max-streams` | 20,000 | Subscribe and listen (hub, section 2.1 item 2) | `429 PUSH_LIMIT`, `{"scope": "application", "application": "<code>"}` |
| `push.app.publish-rate` | 500/s, burst 2,000 | Ingest | HTTP 429 with `Retry-After` (outbox requeues) |
| `push.app.max-data-bytes` | 16,384 (at most 65,536) | SDK at write, then ingest | Item `BAD_REQUEST` `DATA_TOO_LARGE` |
| `push.app.paused` | false | Ingest (subscribers stay connected) | HTTP 429 with `Retry-After`, as for rate |
| Host limits (`push.max-streams`, `push.max-streams-per-principal`, `push.max-topics`) | Framework defaults | Hub | Framework codes |

- Per-application overrides live in a new `push_app_settings` table, edited in the console. They
  are not synced by the app: limits are the operator's call.
- **Isolation in the hub.** Fan-out is per topic, and a slow stream is ended without blocking the
  hub (framework 8). One application's burst costs CPU on the shared tasks, but its publish-rate
  cap bounds that.
- **Measuring isolation.** The load test (section 15) runs two applications, one bursting at 5× its
  cap. The other's p99 delay must stay within 2× its idle value.
- **Retention is one value per host** (framework ruling 3, 10 minutes to 24 hours). It is
  gateway-wide, not per application.

## 11. Observability

**Metrics.** The module's names (framework 11):
- `fw_push_published_total`
- `fw_push_relay_lag_seconds`
- `fw_push_streams_open`
- `fw_push_delivered_total`
- `fw_push_streams_ended_total{reason}`
- `fw_push_resets_total{reason}`
- `fw_push_replay_source_total{source}`

New: an `application` label on published, open and delivered. Application codes are few, so the
cardinality is fine. Also new:
- `fc_push_ingest_items_total{application,result}`
- `fc_push_ingest_rejected_total{application,code}`

**Console, under `/ui` (templ + Datastar, like `internal/ui/pages`)** (new page set "Push"):

1. **Applications.** Per application: rules (read-only, from sync, with `synced_at`); keys (kid
   list, `synced_at`, the next-to-expire warning); limits and overrides (editable, audited); the
   pause toggle (audited).
2. **Live.** Open streams per application and per client; relay lag; delay p50/p99. Each task
   writes a stats row every 10 s to `push_gateway_stats(task_id, at, json)` (new), and the page
   sums the fresh rows.
3. **Recent pushes.**
   - Per topic, or per application and type, showing id, time, client and type. Data is shown only
     while a trace window is open.
   - Trace windows are gated and time-boxed like ingestion traces (memory "ingestion tracing": a
     per-item switch, a short window, the Activity page; `internal/platform/ingestion/
     repository_trace.go`).
   - The list reads the log (`ReadAfter` / `Newest`); no copy is kept.
4. **Test send.** Anchor only, audited. It publishes `{"test": true}` with type `fw-test.ping` to a
   chosen topic for a chosen client. Type names starting with `fw.` are reserved, so the test type
   uses `fw-test`.

## 12. Operations

### 12.1 The component (cloud-neutral)

- **Flag:** `FC_PUSH_ENABLED` (new) in `internal/server/envcfg.go`, next to `FC_ROUTER_ENABLED`,
  `FC_OUTBOX_ENABLED` and the others (`envcfg.go:372-378`). It also needs:
  - its `needsDB` term in `cmd/fc-server/main.go` (the push component needs the platform database
    for the outbox, the relay lock and A1);
  - `startPushIfEnabled` in `internal/server/run.go`.
- **fcdev** runs it in-process by default (`cmd/fcdev/start.go`, a `--push` flag defaulting on like
  `--router`), on the embedded Postgres with A1, at `:8080/api/push`.
- **The same image as the platform** (`inhance/flowcatalyst`). A gateway service runs with
  `FC_PUSH_ENABLED=true` and `FC_PLATFORM_ENABLED=false`. A small deployment can instead run the
  gateway inside the platform service's tasks with both flags on; the trade-offs are in section
  12.2.
- **A standalone gateway shares the platform's JWT key.** With `FC_PLATFORM_ENABLED=false` the
  gateway validates the platform's bearer tokens and field tokens itself, so it must be
  configured with the same signing key settings as the platform (`FLOWCATALYST_JWT_PRIVATE_KEY`,
  or `FC_JWT_SIGNING_KEY_PEM` / `FC_JWT_SIGNING_KEY_PATH`, and the same `FC_JWT_ISSUER`;
  `FC_JWT_PREVIOUS_PUBLIC_KEY` while a key rotates). It refuses to start without one (an ephemeral
  key would refuse every token), except in development. Inject the key from the same secret as the
  platform's. The gateway service also needs the ingest permission for the outbox processors that
  post to it: they authenticate with the application's own service account, as for events.
- **Requirements of any host:**
  - **Load balancer.** Its idle timeout must be greater than the 25 s heartbeat (60 s or more).
  - **Connection draining.** Short (10 to 15 s). On a drain the hub ends every stream at once and
    refuses new ones with `503` (framework amendment 17.11), and clients reconnect to another task
    with jitter.
  - **No sticky sessions.** Any task serves any stream.
  - **Readiness** is the module's health. A task is ready once its hub follows the log.
- **Secrets:**
  - `push.stream.secret` (32+ characters, the same on every gateway task) arrives as an environment
    value injected by the orchestrator from the deployment's secret store.
  - The gateway does not need `internal/secretref` (which resolves only `aws-sm://` and
    `aws-ps://` today; `gcp-sm://` is listed as unresolvable in `internal/secretref/write.go:13`).
  - Applications' ticket keys are public keys stored in the platform database, not secrets.
- **Redis (only for A2):** `FC_REDIS_URL`, any Redis 6.2+ or Valkey.
- **Sizing:**
  - Idle streams measured about 43 KiB each (framework `docs/push-load-test-results.md`, after the
    fixes), so 50,000 streams is about 2.1 GiB plus the process.
  - Plan 2 to 4 tasks of 2 vCPU / 4 to 8 GiB at 25k to 50k streams each for the 100k target.
  - Nonprod needs one small task.
  - Linux limits: raise `nofile` (100k+) on the task.
- **What the gateway does not use:** `internal/router/traffic.go` (the router's ELBv2 target
  registration for standby). The gateway relies on readiness and drain instead, so it has no
  load-balancer API dependency.

### 12.2 Inhance's deployment (AWS)

- A new ECS service per environment (nonprod and prod), using the platform image with the push
  flag.
- An ALB listener rule on the platform's host: path `/api/push/*` goes to the push target group.
  Settings:
  - idle timeout of 60 s or more;
  - deregistration delay of 15 s;
  - no stickiness;
  - health check on the readiness path.
- `push.stream.secret` comes from Secrets Manager or Parameter Store through the task definition's
  `secrets` (the orchestrator injects it; the code reads an environment variable).
- **A1 at first,** on the platform's RDS. **A2 later,** on the cache stack's Redis (`FC_REDIS_URL`;
  memory: the iac `cache` stack).
- **Deploy:**
  - A `deploy-push.yml` workflow in `inhance-supply-chain/flowcatalyst-deploy`, ticked in "Deploy
    FlowCatalyst (all)" after the platform (it shares the platform's migrations).
  - The service, target group and listener rule go in the iac `compute` stack.
  - Both repos are Inhance's (memory: prod deploy pipeline).
  - The ordering rule there applies: never apply prod iac between a migration-bearing deploy and
    its deploy run.
- **Separate service or inside the platform tasks.** Recommended: a separate service. It scales on
  connections and memory, not requests; platform deploys don't drop every stream; and a stream
  storm can't starve the API.

### 12.3 A future FlowCatalyst-hosted deployment (Google Cloud)

- **Compute.** GKE Autopilot rather than Cloud Run. Streams are held for up to
  `push.stream.max-lifetime` (1 h by default), and Cloud Run's request timeout and per-instance
  concurrency don't suit that.
- **Load balancer.** Cloud Load Balancing (the GKE Gateway/Ingress) with the backend service
  timeout at or above the stream lifetime, or at least 1 h, connection draining of 15 s, and no
  session affinity.
- **A2** on Memorystore (Redis or Valkey).
- **Secrets** from Secret Manager, injected as environment values (Secret Manager CSI or External
  Secrets).
- **Nothing gateway-specific is AWS-only.** The platform's other AWS-only code doesn't touch the
  gateway, though a full GCP deployment of the platform would need drivers for it:
  - `internal/queue/sqs` and `internal/platform/scheduler/publisher_sqs.go` (the router and
    scheduler; NATS and Postgres queue drivers exist in `internal/queue/`);
  - `internal/secretref` (`aws-sm`/`aws-ps`);
  - `internal/platform/function` (S3 artifacts);
  - `internal/router/traffic.go` (ELBv2).

## 13. SDK changes

| Package | Change (all new) |
|---|---|
| **Go SDK** (`pkg/fcsdk/outbox`, mirrored to go-sdk) | `TypePush = "PUSH"`; a `PushRow` builder (payload of 6.1, group = topic, `pushId` = row id, client rule as `DispatchJobRow`); validation per framework 1; a ticket helper (`Ed25519`, claims of framework 6.4); `client` gains `SyncPush` (section 8) and `PublishPushes` (6.3). |
| **Java SDK** (`clients/java-sdk`) | Same: `OutboxManager` PUSH rows, ticket helper, sync and publish methods. |
| **TypeScript SDK** | Same. Also re-export the push reader for web apps (field-js `PushStream` already works in browsers with `fetch: globalThis.fetch.bind(globalThis)`; a thin re-export avoids the field package dependency). |
| **Laravel SDK** | Same: a PUSH outbox row from Laravel's outbox, a ticket helper (sodium `crypto_sign_detached`), `push` in `flowcatalyst:sync`, and config for the ticket key. |
| **Rust SDK** | It sends outbox rows itself (`clients/rust-sdk/src/outbox/`), so it needs the PUSH row and its endpoint. |
| **Python SDK** | PUSH row and ticket helper if the outbox exists there; otherwise publish via 6.3. |
| **Framework (Go, Java)** | Section 2.1: multi-application granter, per-application limits, `Handler()`, `push.driver=platform`, and the rules/keys sync in the `flowcatalyst` module. |
| **Outbox drainers** | The platform's outbox processor (`internal/common/outbox.go` `APIPath`: `PUSH` → `/api/push/batch`) and any SDK that drains rows itself (Rust). **An old drainer posts an unknown type to the bare platform URL** (`APIPath` returns `""`, `http_dispatcher.go:144`) and fails the row. So drainers are upgraded before any app writes `PUSH` (section 14). |
| **Readers** | Unchanged: field-js `PushStream`, field-react-native `PushStream`, `baseUrl` = `{platform}/api`. |

**Conformance.**
- A `push` case is added to the shared outbox vector (framework `conformance/outbox/outbox.json`;
  pinned copy in `pkg/fcsdk/outbox/testdata/conformance/`). It pins the payload members, their
  order, the group and the client rule, and every SDK's conformance test reads it.
- A ticket vector exists already (`conformance/push/tickets.json`). SDK ticket helpers must produce
  tickets that verify against it.

## 14. Rollout

1. **Framework (Go)**, section 2.1: the multi-application granter, per-application limits,
   `Handler()`, and batch `PublishAlone`. Contract amendment first, for owner review.
2. **Platform gateway behind `FC_PUSH_ENABLED` (off by default in deployed envs, on in fcdev):**
   - the migration;
   - the authenticator (section 5) and the subscribe `client` member;
   - the sync endpoint;
   - ingest;
   - limits;
   - console pages 1 and 2;
   - the contract tests and vectors run against the gateway.
3. **Outbox drainers learn `PUSH`.** Release fcdev and the outbox processor image, and deploy them.
   Only after that do SDKs emit `PUSH` rows.
4. **SDKs:** Go, Java, TS, Laravel (PUSH rows, tickets, sync). Then the framework
   `push.driver=platform` (Go, then Java).
5. **Nonprod:** deploy the gateway service (section 12.2). Load test there: idle 50k, fan-out 5k/s,
   the two-application isolation test, and restart and drain.
6. **First consumer.** A Laravel or TypeScript app (no own push host). Field devices that use
   several apps follow.
7. **Prod**, then console pages 3 and 4. A2 once section 3's thresholds are met.

## 15. Tests

- **The framework contract suite of section 14**, run against the gateway (a test harness with two
  gateway tasks on one database): every case, on both drivers.
- **The four `conformance/push/` vector files**, over HTTP against the gateway.
- **Multi-application cases:**
  - topics and rules of two apps;
  - tickets from two issuers in one subscribe;
  - a ticket for app B's topic signed by app A, denied;
  - a ticket naming another client, denied;
  - an app's rules or keys replaced by sync and applied within one second on every task.
- **Ingest cases:**
  - ownership refusal;
  - client refusal;
  - a duplicate `pushId` accepted once;
  - pause and rate answered 429 and drained from the outbox afterwards;
  - order kept within a group across processor retries.
- **Authentication:**
  - a field token with DPoP, and a revocation ending its streams;
  - a session with CSRF, and cross-site refused;
  - a multi-client user without `client` gets `400`, and with another client's id gets `403`;
  - a platform-wide stream only with `platform:push:all-clients`.
- **Mutation checks** on the client filter, ticket issuer selection, app ownership at ingest, and
  the per-application stream count. Each is restored with `cp` and `touch`, as the house rule
  requires.

## 16. Open questions (each with a recommended default)

1. **Separate service, or inside the platform tasks** in Inhance's prod? Default: **separate
   service** (section 12.2).
2. **Log driver at launch.** Default: **A1 on the platform database**, switching to A2 past
   section 3's thresholds.
3. **Push latency through the outbox** (about 1 s from the processor's poll). Default: accept it
   for launch, and add LISTEN-on-insert wake-ups to the outbox processor as follow-up work if apps
   need sub-second delivery. (Ruling 3 brought the wake-ups into scope; built in step 2b.)
4. **Message group = topic by default** (order per topic, at one HTTP call per item for grouped
   rows). Default: **yes**. Apps that push only signals may opt out (ungrouped, batched, order not
   kept).
5. **Platform-wide streams** (anchor users hearing every client). Default: **only with an explicit
   platform client and `platform:push:all-clients`**, never implied.
6. **Portal users (`ptu_`) as subscribers.** Default: **not in the first release**. Apps mint
   tickets for their portal users instead.
7. **Default per-application limits** (20k streams, 500/s, 16 KiB). Default: as in section 10,
   adjustable per application in the console.

## 17. Owner rulings

Owner rulings, 2026-11-09 (on section 16):

1. **A separate service**, in nonprod and prod.
2. **A1 (Postgres) at launch**, switching to A2 past section 3's thresholds.
3. **Instant delivery is in A4's scope**, not follow-up work. The outbox processor wakes on a
   `PUSH` row's insert (LISTEN, plus a NOTIFY when the row is written), so the poll interval no
   longer adds latency to pushes. This amends sections 6.5 and 14 (step 3).
4. **Message group = topic by default** (order per topic), with the opt-out for signal-only apps.
5. **Platform-wide streams** only with an explicit platform client and
   `platform:push:all-clients`.
6. **Portal users are not subscribers in the first release**; apps mint tickets for them.
7. **The default per-application limits of section 10** (20k streams, 500/s burst 2,000,
   16 KiB), adjustable per application in the console.

Reviewer notes accepted with the rulings: the framework's Go module needs release tags before the
platform can import it (section 2.2, new); a busy grouped topic is bounded by one HTTP call per
push, so the opt-out (ruling 4) is documented where apps choose topics.
