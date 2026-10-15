# SDK audit and the Python SDK design

Status: audit and design only (2026-10-15). No SDK code was written or changed. Numbers come from
`api/openapi.lock.json`, the SDK trees under `clients/` and `pkg/fcsdk`, and a read-only look at the
AgentPlanner repo. Where a figure is a judgement (the class of an operation) it is labelled as such;
everything else was counted.

Contents: Part A (audit) · Part B (Python SDK) · AgentPlanner today and migration sketch ·
milestones and risks · owner decisions · Part C (Laravel SDK in the Inhance apps today; corrects Part A).

---

# Part A. Audit of the existing SDKs

## A.1 What each SDK is made of

SDK versions at HEAD: typescript-sdk 0.11.33, laravel-sdk 0.10.31, java-sdk 0.0.14, go-sdk v0.0.3
(pkg/fcsdk). The fn SDKs (fn-go, fn-ts, fn-rust) are a different product (see A.7).

The platform spec (`api/openapi.lock.json`) has **289 operations on 212 paths, 287 schemas, 33 tags**.

| | TypeScript | Laravel (PHP) | Java | Go (`pkg/fcsdk`) |
|---|---|---|---|---|
| Generator | hey-api `openapi-ts` (fetch client) | Jane PHP (PSR-18), post-processed spec | openapi-generator, models only | none, fully hand-written |
| Generated ops | **289** (`sdk.gen.ts`, 2,186 lines) | **289** endpoint classes | 0 (models only) | 0 |
| Generated types | 1,949 exported types (`types.gen.ts`, 14,230 lines) | 287 models | 566 model classes (build output) | 0 |
| Generated size | 592 KB source; package `dist` 2.2 MB | **4.7 MB, 95,105 lines, 81% of all PHP lines** | n/a (compiled into jar) | 0 |
| Hand-written layer | 15 resource classes, about 121 public methods | 14 resource classes, 121 methods | 15 resource classes, about 125 methods | 20 resource files, about 127 exported methods (about 12 are plumbing) |
| Hand-written LOC | about 13.9k (30.3k total minus generated) | about 21.6k (116.7k minus generated) | 6,453 (44 files) | 10,979 non-test |
| Generated ops actually called by the hand layer | **70 of 289 (24%)** via `sdk.*`; the other resources (connections, dispatch pools, applications, scheduled jobs, sync of 5 more categories) bypass the generated client with literal URLs | **about 0%**: resources call `request('GET', '/api/...')`; the Jane client is reachable only through `$client->generated()` and 2 `fromGenerated` DTO converters | n/a (Jackson on the models, own `Transport`) | n/a |
| Tests (files) | 20 `*.test.ts` | 42 PHPUnit files | 19 | 35 |
| Release path | `make release-ts-sdk`, tag `typescript-sdk/v*`, split workflow | same with `laravel-sdk/v*` | `make release-java-sdk` (no split workflow yet) | consumed straight from this repo by tag |

Beyond resources, each SDK also ships non-HTTP-client features. These are the parts an application
actually adopts first:

| Feature | TS | Laravel | Java | Go |
|---|---|---|---|---|
| Outbox writer (events, dispatch jobs, audit) | yes (pg/mysql drivers) | yes | yes (JDBC) | yes (`outbox`, `outboxpgx`, `outboxsql`) |
| Use-case envelope (`usecaseop`, Plan, UoW) | **two trees**: `usecase/` and `effect/usecase/` | yes (`UseCase/`) | no | yes (`usecase`, `usecaseop`, `usecasepgx`) |
| Definition sync (declarative) | `sync/` (560 + 1,155 lines) | `Sync/` plus attributes and scanner | `sync/` plus annotations | `sync/` |
| Scheduled-job runner (lock, logs, completion) | yes | yes | no | yes |
| Webhook signature validation | `webhook/signature.ts` | `Webhook/WebhookValidator` + `BearerTokenCheck` | `WebhookSignature` | **two validators** (`Validator` and `Verifier`) |
| Token validation (JWKS, RS256) | in `fastify/` (jose) | `Auth/Support/*` | none | `auth` (jwx) |
| Client-credentials provider | `OidcTokenManager` | `OidcTokenManager` | `ClientCredentialsTokenManager` | `ClientCredentialsProvider` |
| Login / OIDC session | `fastify/` plugin: oidc, pg/redis/cookie session stores | guards, controllers, PKCE, refresh, `session-table` command | none | `OAuthClient` (PKCE, refresh, logout) only |
| Framework adapter | Fastify plugin (507 lines), RBAC, guards | whole Laravel integration | none | none |
| Cache / locks | memory, pg, redis | via Laravel cache | none | memory, pg, redis |
| Test helpers | none | `Testing/` (fake permission resolver, test principal) | none | none |

## A.2 Classification of the platform API (the generated bundle)

Class 1 belongs in an application SDK; class 2 is admin / platform-console surface; class 3 is
legacy, duplicate or compat alias. Judgement counts, from tags and paths of the 289 operations:

| Class | Ops | Share | What is in it |
|---|---|---|---|
| 1 belongs | **45** | 15.6% | `sdk-sync` (11 sync endpoints for roles, event types, subscriptions, connections, dispatch pools, processes, principals, scheduled jobs, docs, openapi), `POST /api/events/batch`, read of roles / permissions / event types / subscriptions / connections / processes / dispatch pools / applications / clients, scheduled-job instance get / complete / log / logs, scheduled-job get / by-code / list / fire, docs read |
| 2 admin | **233** | 80.6% | principals (29 ops), functions (23) and function-control (5), applications CRUD and provisioning, clients CRUD, roles / subscriptions / connections / event-type / pool / process writes, service-accounts (14), oauth-clients (11), audit-logs (9), identity-providers, email-domain-mappings, anchor-domains, auth-configs, cors-origins, login-attempts, reset-approvals, idp-role-mappings, portal-users / portal-apps, webauthn (6, passkeys), platform-config (7), dispatch-jobs admin (15, list / requeue / cancel / raw), events admin (list / filter-options), dispatch pools admin, router config |
| 3 legacy / alias | **11** | 3.8% | `/raw`, `/list-raw`, `by-event` vs `event`, `regenerate-secret` vs `rotate-secret`, `GET /clients/search` vs POST, raw variants added for the Laravel SDK (see `docs/laravel-sdk-compat.md`) |

Two things are missing from the lock that application SDKs rely on: `POST /api/dispatch-jobs/batch`
and `POST /api/audit-logs/batch` (and `POST /api/events`). The Java `DispatchJobsResource.createBatch`
and the outbox processor call the first; the lock is therefore not a complete description of the
app-facing contract. Fix: tag these routes into the spec before an allow-list is introduced.

## A.3 What each hand-written layer exposes, and its class

Per resource, with the operations classified (counts are for the Laravel layer, which is the most
complete; the TS, Java and Go layers differ by one to three methods per resource and follow the same
split). "Overlaps sync" means the same effect is reachable through the declarative sync call, which
is the recommended application path.

| Resource | Methods | Class 1 (keep) | Class 2 admin | Class 3 overlaps sync |
|---|---|---|---|---|
| applications | 15 | get, getByCode, listRoles (3) | list, create, update, delete, activate, deactivate, provisionServiceAccount, getServiceAccount, listClients, updateClientConfig, enable/disableForClient (12) | 0 |
| audit-logs | 2 | 0 | list, get (2) | 0 |
| clients | 14 | list, get, getByIdentifier (3) | search, create, update, activate, suspend, deactivate, addNote, applications x3 (11) | 0 |
| connections | 8 | list, get, sync (3) | pause, activate (2) | create, update, delete (3) |
| dispatch pools | 9 | list, get, sync (3) | archive, suspend, activate (3) | create, update, delete (3) |
| event types | 8 | list, get, getByCode, sync (4) | 0 | create, update, addSchemaVersion, archive (4) |
| me | 3 | 3 | 0 | 0 |
| permissions | 2 | 2 | 0 | 0 |
| principals | 17 | sync, syncUsers (2) plus "app-delegated user management": get, findByEmail, createUser, resetPassword, update (5; **used by all three Inhance apps**, see section C) | list, activate, deactivate, getRoles, add/remove/setRoles, client-access x3 (10) | 0 |
| processes | 8 | list, get, getByCode, sync (4) | 0 | create, update, archive, delete (4) |
| roles | 9 | list, get, getByCode, sync (4) | grantPermission, revokePermission (2) | create, update, delete (3) |
| router | 2 | 0 | in-pipeline checks (2): router internals on another host (**`areInPipeline` is called by Integral**, see section C) | 0 |
| scheduled jobs | 16 | list, get, getByCode, getInstance, logForInstance, completeInstance, sync (7) | pause, resume, fire, listInstances, listInstanceLogs (5) | create, update, archive, delete (4) |
| subscriptions | 8 | list, get, sync (3) | pause, resume (2) | create, update, delete (3) |
| **Total (Laravel)** | **121** | **41 (34%)** plus 5 optional (4%) | **51 (42%)** | **24 (20%)** |

So roughly **one hand-written method in three is something an application needs, and 42% is admin
console surface that an application's service account is not even permitted to call** (the
AgentPlanner team hit this: the admin route returns a bare 403 because an application service
account holds `platform:application-service:*`, not `platform:messaging:*`).

## A.4 Where the generated clients drag admin endpoints into app SDKs

1. **TS ships all 289 generated operations** in `dist` (2.2 MB package), exported through
   `sdk.gen`. 244 of them (84%) are class 2 or 3. The hand layer uses 70 and the rest are dead weight
   plus a tempting escape hatch (`createOAuthClient`, `resetPrincipalPassword`, `deleteWebauthnCredential`
   are one import away).
2. **Laravel ships 289 endpoint classes and 287 models (4.7 MB, 81% of the package's PHP)** and the
   hand layer does not use the endpoints. It is only reachable through `FlowCatalystClient::generated()`.
   The generated client is dead code with a public accessor, and it enlarges composer installs,
   opcache and static analysis for every Laravel app.
3. **Java generates only models (566 classes)** and has its own `Transport`: the right shape, but the
   models for all 287 schemas (including WebAuthn, IdP, OAuth client, function control plane) are still
   compiled in.
4. **Go has no generated layer** but its hand layer carries 20 resource files, the same admin
   resources (clients, principals, service-accounts, applications CRUD, router).

## A.5 Duplication between the hand-written and generated layers

* TS: each resource wraps `sdk.fn(...)` for 70 ops and re-implements the others with literal URLs.
  Comments in `applications.ts` and `dispatch-pools.ts` claim "the OpenAPI spec does not include
  these routes", which is no longer true (the lock has them): stale, so the bypass is accidental
  drift rather than design.
* TS has **two use-case trees** (`usecase/` and `effect/usecase/`) and two ways to do the same
  error handling (neverthrow `ResultAsync` in the client, Effect in `effect/`).
* Go has **two webhook validators** (`Validator` and `Verifier`) that its own doc calls "functionally
  equivalent, differ only in API style", with different header conventions.
* Laravel has three representations for one concept: generated model, `DTOs/*`, and
  `DTOs/Requests|Responses/*` (the Principal / Client DTOs even convert from the generated model).
* Every SDK hand-copies the same sync DTOs, the same QualifiedCode / TSID / dispatch-job
  validation, the same permission matching. The conformance vectors only exist in the framework
  repo (outbox, receivers) and as a webhook golden in `tests/golden/webhook` and `pkg/fcsdk/webhook/testdata`;
  TS, Laravel and Java do not run the framework's outbox vector.

## A.6 Where the SDKs disagree (behaviour)

| Topic | TS | Laravel | Java | Go |
|---|---|---|---|---|
| Error model | neverthrow `ResultAsync<T, SdkError>` (a discriminated union: network / timeout / http_error / validation / not_found / forbidden / conflict / rate_limited / partial_failure) | exceptions (`FlowCatalystException`, `ValidationException`, ...) | `Result`-style sealed `SdkError` (records) plus `FlowCatalystException` | `*APIError` with `Retryable()`, sentinels per package |
| Retried statuses | 408, 429, 502, 503, 504 | 408, 429, 502, 503, 504 and transport failures; **500 never** | 408, 429, 502, 503, 504 | **408, 425, 429, 500, 502, 503, 504** |
| Backoff | `delay * 2^attempt`, no jitter, default 3 attempts / 100 ms | 3 / 100 ms | `delay << attempt` | exponential, 3 / 100 ms |
| Timeout | 30 s | 30 s | connect timeout = request timeout | per client |
| Retry-After honoured | `retryAfterMs` carried on the error, not used to wait | no | no | no |
| Pagination | resources accept `page/size` but several ignore it (`list(_pagination)` in roles) | query builders | `Filters` records | typed lists |
| Auth providers | client credentials or user token (static or callback) | token provider interface, config driven | token provider | `TokenProvider` interface, bearer |
| Metadata on dispatch jobs / events | `Record<string,string>` map | `array<string,string>` map | map | **array of `{key,value}` entries** (what the batch endpoint reads) |
| Event `data` / audit `operationData` in the outbox payload | JSON string holding encoded JSON | string | string | **the JSON value itself** (deliberate; platform unwraps strings on ingest) |
| Sync result shape | `Skipped` sentinel per category; partial failure error | `SyncResult` | sealed `Category` (`Synced`, `Skipped`) incl. `skippedRoles`, `passwordHashIgnored` | `Principals.SkippedRoles`; categories typed |
| `removeUnlisted` | option | option | option | option, explicit |
| Role-sync name rule | names go up unprefixed, platform adds `{app}:` | same | same | same, but each re-documents it |
| Webhook verification | ISO8601-ms timestamp, 300 s past / 60 s future | same, plus optional bearer gate | same | same; second validator also accepts Unix seconds |
| camelCase | wire is camelCase; TS passes through | DTOs map snake to camel by hand | Jackson | struct tags |
| Principal / permission matching | `matchesAny` in `fastify/` | `RbacCatalogue::matches` | none | `auth` package |

Disagreements that bite: (1) the 500-retry difference means the same transient platform fault is
retried by Go only; (2) the metadata array/map shape means Go cannot round trip what TS writes;
(3) two payload encodings for `data`; (4) `Retry-After` is ignored everywhere; (5) pagination is
inconsistent; (6) the AgentPlanner team found that the TS SDK compares the whole `"id:identifier"`
client string, and the PHP SDK compares an application code against an id list, so each SDK answers
"may this principal access client X" differently (see their notes in Part B).

## A.7 fn SDKs (brief)

`fn-go`, `fn-rust`, `fn-ts` are **guest-side SDKs for writing functions** that run inside the
function runner (Wasm for Go / Rust, QuickJS for TS): routes (`fn.open`, `fn.platform`,
`fn.webhook`), config, db, `fn.emit`, caller identity. They are a separate product with their own
release flow, not application-platform clients, and contain no generated client. Keep them out of
the audit's trim plan. The corresponding admin surface (`functions`, `function-control`: 28
operations) is the function-runner control plane and must not be in any application SDK. There is
no platform "invoke function" endpoint in the lock; invoking a function from an application today
means calling its webhook / route URL, which the platform authenticates. If "invoke functions" is
wanted in app SDKs, the platform needs to define a stable invoke operation first.

## A.8 Trimming plan

### Principle

An application SDK is for **consuming** FlowCatalyst. It owns: definitions and sync, outbox and
ingest, dispatch-job creation, token acquisition and validation, receivers and their signature
check, scheduled-job completion, and a small set of reads. Everything that manages tenants, users,
IdPs, OAuth clients, service accounts, passkeys, pools, router or functions belongs to the console
(the SPA) and to an optional, separately versioned **admin client**.

### Mechanism (recommended)

1. **Tag the contract once, in the platform.** Add an `x-sdk` vendor extension (values `app` or
   `admin`; absent means internal) on each huma operation, driven by an allow-list in `tools/dump-spec`
   (it already keeps its own Register list and a drift guard test). Also add the missing app-facing
   routes (`dispatch-jobs/batch`, `audit-logs/batch`, `events` single) into the dumped spec with
   `x-sdk: app`.
2. **`make sdk-spec` writes two documents**: `openapi.app.json` (about 45 to 60 ops) and
   `openapi.admin.json` (the rest). Generators take the app document only.
3. **Generated layer shrinks to types plus an internal transport**: TS keeps hey-api but emits only
   the app document (about 60 ops, a few hundred types); Laravel drops Jane completely (the hand
   layer already does not use it) and keeps hand-written DTOs; Java keeps model generation from the
   app document; Go stays hand-written.
4. **Hand-written facades become the only public surface** (see the table below). The generated
   code is `internal` (TS: not exported from `index.ts`; PHP: namespace marked `@internal`; Java:
   package-private or `internal` module export).
5. **Admin client = separate package** (`@flowcatalyst/admin-sdk`, `flowcatalyst/admin-sdk`) generated
   from `openapi.admin.json`, only if someone asks for it. Today the callers are the three Inhance Laravel apps' user
   management (HR, RFP, Integral) and ops scripts (corrected: the first draft named Integral only; see
   section C). Principals user management goes into the admin package under an explicit name; until then
   it stays in the app SDK as `principals()` with a deprecation notice.

### Curated public surface per SDK

| Area | Keep in app SDK | Move to admin package | Remove / deprecate |
|---|---|---|---|
| Auth | client-credentials provider, token cache, JWKS verifier, `Principal`, permission match, webhook verifier, OIDC login helpers where the framework has them | OAuth-client and service-account management | Go's second validator; TS `UserTokenConfig` stays |
| Sync | all `sync` calls, `DefinitionSet`, dry-run result, skipped categories | none | CRUD on event types / subscriptions / connections / pools / processes / roles / scheduled jobs that duplicate sync (24 methods in Laravel) |
| Ingest | outbox writers, batch ingest for events, dispatch jobs, audit | audit-log read | none |
| Scheduled jobs | `runner`, completion, log, get / list / by-code | create, pause, resume, archive, fire, delete | create / update / delete (dup of sync) |
| Reads | event types, roles, permissions, subscriptions, connections, processes, applications (get, roles), clients (get, list, by-identifier), `me` | applications CRUD, client CRUD, principals admin | `search`, `/raw`, aliases |
| Router / pools | none | dispatch-pool admin, router in-pipeline | `RouterResource` (it needs the router host, not the platform host) |

Result: **Laravel 121 methods to about 46 (and zero generated endpoint classes)**; **TS 121 to about
46 and 289 generated ops to about 50**; **Java about 125 to about 46**; **Go about 127 to about 46**.
The trimmed PHP package drops roughly 4.7 MB of generated source.

### Deprecate vs remove, and breaking-change cost

Rule: **deprecate in the next minor, remove at the next breaking line.** Pre-1.0 versions allow it, but
the Inhance Laravel apps (HR, RFP, Integral plus the shared `packages_root`; section C) and in future AgentPlanner on Python
depend on the surface; nothing else is known to. (The first draft said two apps; the Laravel SDK has three direct app consumers plus a package monorepo.) Deprecation first, with a runtime log once per process (`E_USER_DEPRECATED` / `console.warn`
/ `@Deprecated(forRemoval=true)` / `// Deprecated:`).

| SDK | Version line | Breaking cost of removing class 2 and 3 | Notes |
|---|---|---|---|
| TypeScript | 0.11.x (0.11.33) | **Low to medium.** Class 2 removal touches `clients()`, `principals()`, `applications()` CRUD, `dispatchPools()` CRUD, `auditLogs()`, `router()`. Any consumer using them breaks at 0.12.0. Hiding `sdk.gen` exports is a pure removal of an unadvertised path. Use a `/admin` sub-path export first (no break) and flip default later. | Pre-1.0: minor bump is allowed to break. |
| Laravel | 0.10.x (0.10.31) | **Medium to high (revised up from Medium; section C).** Verified by grep: nothing in HR, RFP, Integral or `packages_root` calls `generated()` or imports `Generated\*`, so dropping Jane is safe (zero break). But 12 `principals()` methods, `clients()->list/create`, `applications()->create/listClients`, `router()->areInPipeline` and the public `request()` escape hatch (5 distinct admin/uncovered routes) are in live use across all three apps and 13 shared-package files, three of them classed admin or remove here. Keep these behind an `Admin` facade first; do not remove `request()`. | Drop Jane in 0.11.0 (safe). HR is pinned `^0.8` (locked 0.8.19) and cannot receive 0.10.x or later without a composer constraint change. |
| Java | 0.0.14 | **Low.** Very early line, no known production consumer other than the framework / Camel work. | Remove directly, document in `CHANGES-0.0.x.md`. |
| Go | v0.0.3 | **Low to medium.** Consumers: the framework's FlowCatalyst module (uses sync, outbox, scheduled jobs, client credentials; not admin). Verify with the framework module before pruning `client.Clients()` / `Principals()`. | Go cannot deprecate-and-hide gracefully; use `// Deprecated:` then delete at v0.1.0. |

Sequencing: (1) platform: `x-sdk` tagging + app spec; (2) Java and Go (cheapest) to prove the cut;
(3) TS; (4) Laravel last (largest consumer and largest generated tree); (5) a shared cross-SDK
behaviour contract (A.9), which is a prerequisite for Python.

## A.9 Behaviour contract all SDKs should converge on (and Python starts with)

| Topic | Rule |
|---|---|
| Retry | statuses 408, 425, 429, 502, 503, 504 and transport errors; **500 is not retried by default** (a 500 may have done the work; make it opt-in for idempotent GET); 3 attempts; exponential backoff with full jitter (base 100 ms, cap 5 s); honour `Retry-After` (seconds or HTTP date) up to a cap. Only idempotent methods (GET, PUT, DELETE) and POST with an idempotency key retry on a transport error after the request was sent. |
| Timeouts | connect 5 s, read 30 s, overall 60 s; per-call override. |
| Errors | one taxonomy: `Network`, `Timeout`, `Auth` (token fetch failed or 401 after refresh), `Forbidden` (403), `NotFound`, `Conflict`, `Validation` (400/422 with field map), `RateLimited` (429 with retry-after), `Server` (5xx), `Partial` (sync / batch with per-item failures). Each carries status, platform `code` and `message`, and `request_id` if present. |
| 401 | one refresh-and-retry, then `Auth`. |
| Pagination | page / size params everywhere a list accepts them; async iterator / generator helper where the platform paginates. |
| Metadata | array of `{key, value}` on the wire; SDK accepts a map and converts. |
| Outbox payload | camelCase; `data` as the JSON value (Go form) is accepted by the platform; vectors compare parsed values so both pass; new SDKs should write the value form. |
| Sync | `removeUnlisted` default false; role names unprefixed; result includes `created / updated / deleted`, `skippedRoles`, `passwordHashIgnored`; fail open on boot is the caller's choice, SDK returns a result, never swallows. |
| Webhook verify | HMAC-SHA256 over `timestamp + rawBody`, hex, constant time; headers `X-FlowCatalyst-Signature` / `X-FlowCatalyst-Timestamp`; ISO8601 (ms) or Unix seconds; 300 s past / 60 s future; empty secret fails closed. |

---

# Part B. Python SDK design

## B.1 Goals and non-goals

Goals: async-first (asyncio, httpx), pydantic v2, strict typing with `py.typed`, Python 3.11+,
small curated surface (the class 1 set in A.3), FastAPI helpers as the first-class integration,
conformance with the shared vectors, and first adopter AgentPlanner.

Non-goals for v1: admin client (users, clients, IdPs, OAuth clients, service accounts, passkeys,
pools, router, functions), generated 289-operation client, Django / Flask adapters, a use-case
envelope clone (see B.5), PyPI publishing.

AgentPlanner (below) is the primary requirements source: it is an async-FastAPI plus sync-psycopg
application whose `central_agent/flowcatalyst/` package is already "the seed of a Python FlowCatalyst
SDK". Its real needs set the scope of milestones M1 to M4; outbox and ingest (which it does not use
yet) come after.

## B.2 Packages

Two distributions in one repo (`clients/python-sdk`, a uv workspace):

| Distribution | Import | Depends on | Contents |
|---|---|---|---|
| `flowcatalyst` | `flowcatalyst` | httpx, pydantic>=2.7, PyJWT[crypto] (or `joserfc`) | core client, auth, sync, ingest, outbox, webhook, receivers (framework-neutral), errors, testing |
| `flowcatalyst-fastapi` | `flowcatalyst_fastapi` | `flowcatalyst`, fastapi>=0.115, starlette | dependencies, router factories, lifespan, OpenAPI metadata, session/OIDC (optional extra `[session]`), test helpers |

Optional extras on `flowcatalyst`: `[postgres]` (asyncpg / psycopg 3 outbox + lock + cache stores),
`[sqlalchemy]`, `[redis]`. Nothing imports a database driver at top level.

```
flowcatalyst/
  __init__.py            # curated public API; __all__ is the contract
  client.py              # FlowCatalyst (async), FlowCatalystSync (blocking facade)
  config.py              # Settings (pydantic-settings optional), from_env()
  auth/
    tokens.py            # ClientCredentialsTokenProvider (cache, single-flight refresh, early expiry)
    jwks.py              # JwksCache (TTL 10 min, kid-miss throttle), RS256-only verifier
    principal.py         # Principal, ClientAccess/ApplicationAccess ("*" = ALL), tier, kind
    permissions.py       # matches(), Catalogue (role -> permissions), HUMAN_ONLY helper
  http.py                # transport: retry/backoff/timeouts/Retry-After, request-id, error mapping
  errors.py              # FlowCatalystError tree (A.9)
  models/                # hand-typed pydantic models for the ~45 app ops (camelCase aliases)
  resources/             # event_types, roles, permissions, subscriptions, connections, processes,
                         # scheduled_jobs, applications (read), clients (read), docs, me
  sync/                  # DefinitionSet, Synchronizer, SyncResult, dry_run
  ingest/                # events.batch(), dispatch_jobs.batch(), audit.batch(); builders + validation
  outbox/                # row builders, qualified codes, TSID, writers (asyncpg / psycopg / sqlalchemy)
  receivers/             # verify_signature(), parse dispatch / scheduled-job envelopes, results
  tsid.py
  testing/               # FakePlatform (respx), signed_delivery(), fake_token(), FakeClient
  py.typed
```

## B.3 Generated vs handwritten: recommendation

**Hand-written facade over hand-typed pydantic models for the 45 app operations. No generated
client.** Reasons:

* Maintenance: four SDKs already pay for a generator pipeline each (hey-api, Jane, openapi-generator,
  none) and the evidence is that the generated layer is mostly dead (0% used in Laravel, 24% in TS).
  A fifth pipeline would repeat the mistake.
* The wire contract the facade needs is small (about 45 ops) and stable; wire quirks (metadata
  arrays, sync shapes, `skippedRoles`) live in the hand layer anyway.
* Drift protection without generating: a **contract test** loads `api/openapi.lock.json` (vendored
  copy, refreshed by the same `sdk-spec` step) and asserts, for every operation the SDK calls, that
  method + path exist and that the pydantic model's fields are a superset of the required response
  fields and a subset of the request schema's properties (`additionalProperties:false` is the
  platform's rule, see the huma strictness note). Cheap, catches drift in CI.
* If the surface grows beyond about 100 operations, revisit with `datamodel-code-generator` for models
  only (no client), driven by the app spec produced in A.8.

Models: `model_config = ConfigDict(populate_by_name=True, alias_generator=to_camel, extra="ignore")`
for responses (forward compatible), `extra="forbid"` for requests (the platform rejects unknown
fields). Datetimes parse RFC3339 with 6-digit microseconds (wire contract).

## B.4 Public API sketch

```python
from flowcatalyst import FlowCatalyst, ClientCredentials

async with FlowCatalyst(
    base_url="https://platform.example.com",
    credentials=ClientCredentials(client_id="...", client_secret="..."),   # service account pair
    app_code="agent-planner",
    timeout=Timeouts(connect=5, read=30), retry=RetryPolicy(max_attempts=3),
) as fc:
    result = await fc.sync.roles(app_code, roles, remove_unlisted=False)
    result = await fc.sync.apply(definition_set, dry_run=True)       # all categories in one go
    await fc.ingest.events([...]); await fc.ingest.dispatch_jobs([...])
    ev = await fc.event_types.list(); role = await fc.roles.get_by_code("app:role")
    await fc.scheduled_jobs.complete(instance_id, status="SUCCESS", result={...})
    token = await fc.tokens.get()                                     # cached bearer
    principal = await fc.verify_bearer(token)                         # RS256 only, token_use=api
```

* **Async first, sync second.** `FlowCatalyst` is async. `FlowCatalystSync` is a thin blocking facade
  that runs the async client on a private event-loop thread (`anyio.from_thread` portal), exposing the
  same methods. Required because AgentPlanner's boot-time sync (`sync_on_boot`, CLI `main()`), its
  `send_trigger.py`, and its sync psycopg code run outside a loop. One implementation, not two
  (no `unasync` duplication). Decision D2.
* **Token provider**: `ClientCredentialsTokenProvider` implements `async def get(force=False)`;
  `asyncio.Lock` single-flight; refresh `margin=60 s` before expiry; on 401 the transport calls
  `get(force=True)` once; exposes `service_principal()` (decodes `sub` and `type=SERVICE` from the
  token without verifying; same rule as the framework's 4b), required by receivers.
* **Retry/timeouts/errors**: exactly A.9. All httpx errors map into the error tree; no `httpx`
  exception escapes; `raise_for_status()` is never used by callers.
* **Verification**: `JwksCache` does discovery (`/.well-known/openid-configuration`), JWKS TTL 10 min,
  refetch once on unknown `kid`, then a negative cache for that `kid` (prevents the amplifier
  AgentPlanner already guards against), `alg` must be RS256, requires `exp iat sub`, `iss`, `aud`
  (issuer for access tokens, client id for id tokens), leeway 30 s, and for bearer tokens `token_use ==
  "api"`. Failure to reach the platform is `Unavailable` (503), not "bad credentials" (sessions
  contract 5.6).
* **Principal**: parses `clients` and `applications` entries (`"id:code"`, bare, or `*`); access checks
  use the **code half** (the bug AgentPlanner documents in both existing SDKs); `tier`, `type`
  (`USER` / `SERVICE`), `roles`, `scope` (granted permissions). `Principal.is_anchor` widens client
  visibility but grants no permission.
* **Permission matching**: `matches(granted, needed)` = exact, `*`, or `prefix:*` at a segment
  boundary; same number of segments rule from the sessions contract 7.3 (held `*` segment matches one
  segment). Note the AgentPlanner implementation (prefix wildcard covers deeper paths) and the
  framework (same segment count) differ; the contract is the framework's; decision D6.
* **Definitions**: Python has no annotation scanning culture like PHP / Java; use explicit
  declarative objects and a small decorator registry:

  ```python
  defs = DefinitionSet(app_code="agent-planner")
  defs.role("planner", display_name="Molly Planner", permissions=[...])
  defs.event_type("agent-planner:planning:run:started", schema=RunStarted)   # pydantic model -> JSON schema
  defs.scheduled_job("molly-tick", crons=["* * * * *"], target_url=..., tracks_completion=False)
  ```

  `sync.apply(defs, dry_run=True)` returns the per-category plan without calling write endpoints when
  the platform has no dry-run; client-side diff against the list endpoints for dry run. Sync is **one
  call per category** and always sends the whole category (archive semantics, AgentPlanner's lesson
  with scheduled jobs).
* **Receivers**: framework-neutral core in `flowcatalyst.receivers`:
  `verify_signature(raw, headers, secret, *, now, tolerance)`, `parse_dispatch(raw, headers)`,
  `parse_scheduled_job(raw)`, `DispatchResult.ack() | defer(seconds) | reject(reason)` mapping to
  `200 {"ack":true}`, `200 {"ack":false,"delaySeconds":n}`, `422` + `FlowCatalyst-Outcome: reject`;
  handler exception or timeout maps to 503 (transient), unknown type is a 422 reject, unknown job
  code is 404, scheduled-job receiver answers 202 immediately and runs the handler in a background
  task (platform timeout is 10 s). Also the receiver principal rule (framework contract 4b): the
  context carries the application's service principal from the token; if it cannot be resolved the
  receiver answers 503 and runs no handler.
* **Outbox**: see B.5.

## B.5 Outbox pattern support

Writer functions build rows identical to the framework conformance vector
(`/Users/andrewgraaff/Developer/flowcatalyst-framework/conformance/outbox/outbox.json`, also pinned
for Go in `pkg/fcsdk/outbox/testdata`):

* `outbox_messages` columns: `id` (TSID 13 chars, dispatch jobs allow caller id `[0-9A-Za-z_-]{1,17}`),
  `type` (`EVENT | AUDIT_LOG | DISPATCH_JOB`), `message_group`, `payload` TEXT, `status 0`,
  `retry_count 0`, `client_id` (client code), `payload_size`, `headers`, `created_at`, `updated_at`
  (UTC, microseconds).
* Event payload members (camelCase): `specVersion "1.0"`, `type`, `source`, `subject`, `data`,
  `correlationId`, `causationId`, `deduplicationId` (= event id), `messageGroup`, `clientCode`,
  `contextData` as an ordered array of `{key, value}` (`principalId`, `executionId`).
* Audit payload: `entityType, entityId, operation, operationData, principalId, performedAt, source,
  correlationId`; `principalId` is mandatory (platform rejects audit rows without it).
* Dispatch job payload with the validation rules from `invalidDispatchJobs` (code
  `{application}:{rest}`, target URL absolute http(s) without user info, id pattern, sequence >= 1,
  source 1 to 255, metadata keys non blank, at most 100).
* Writers: `OutboxWriter` protocol with `AsyncpgOutbox`, `PsycopgAsyncOutbox`, `SqlAlchemyAsyncOutbox`
  (all take **the caller's connection / session** so the insert shares the business transaction;
  the SDK never opens its own transaction). `InitSchema` DDL string for Postgres / MySQL for tests;
  real schema is the app's migration.
* Use-case envelope: **do not port `usecaseop` in v1.** It is the heaviest part of the Go / PHP /
  TS SDKs and the framework's Python story is parked. Provide a small
  `unit_of_work(conn, ctx)` helper that collects events, an optional audit entry and dispatch jobs and
  writes them at commit. A `usecaseop`-style Plan/Operation envelope can follow if AgentPlanner or
  a second app asks (decision D8).
* Context: `ExecutionContext(client, principal, application, correlation, causation, execution)`
  populated by the FastAPI dependency (from the token) or by the receiver (principal = service
  principal, causation = job id or instance id, per contract 4b).

## B.6 FastAPI integration (`flowcatalyst-fastapi`)

Everything is a plain dependency or router factory, no monkey patching, no global state; configured
through an `FcAuth` object created at app construction.

```python
from flowcatalyst_fastapi import FlowCatalystAuth, require_permission, Principal, signed_delivery_dep
fc_auth = FlowCatalystAuth(settings, catalogue=CATALOGUE, application="agent-planner")

app = FastAPI(lifespan=fc_auth.lifespan(sync=defs, sync_on_boot="dry-run|apply|off", fail_open=True))

@app.get("/api/runs", dependencies=[Depends(require_permission("agent-planner:planning:run:read"))])
async def runs(p: Principal = Depends(fc_auth.principal), client: str = Depends(fc_auth.client)): ...
```

| Piece | Behaviour |
|---|---|
| `fc_auth.principal` | reads `Authorization: Bearer` (verify via JwksCache; `token_use=api`; 401 `WWW-Authenticate: Bearer realm="flowcatalyst"`; 503 when keys unreadable). No fall back to cookie after an invalid bearer. Optional: a pluggable `SessionAuthenticator` (below). |
| `require_permission(*codes, any_of=False)` | resolves permissions: bearer `scope` list, or `Catalogue.resolve(roles)` for role-only tokens; applies `matches`; `human_only=` guard rejects `type != USER`. |
| `require_application()` / `fc_auth.client` | enforces application access and selects/validates the client (`X-Client` header or path parameter; `_platform` when none; 403 `CLIENT_NOT_PERMITTED`). |
| route-map mode | `fc_auth.route_permissions({(method, path): code})` + `PermissionCoverageTest` helper: the AgentPlanner pattern of one map plus a test that every `/api` route is in it. |
| `fc_auth.dispatch_receiver(path="/fc/dispatch")` | `APIRouter` that verifies the signature on the raw body before parsing, resolves the service principal, looks up the handler by event type (exact or `prefix:*`), maps `DispatchResult`, applies `once()` idempotency, returns the platform response codes (B.4). |
| `fc_auth.scheduled_job_receiver(path="/fc/scheduled-jobs")` | verifies, validates, answers 202, runs handler as a background task with timeout, completion callback (`tracksCompletion`), lock hook (`LockProvider`, default in-memory, Postgres optional), duplicate delivery guard on `instanceId`. |
| `signature_verified` dependency | for apps with their own endpoint (AgentPlanner's `/triggers/flowcatalyst`): returns the raw body + parsed envelope or raises 401 before any parsing; secret from `FLOWCATALYST_SIGNING_SECRET` (the name the platform and Go SDK use). |
| `Once` hook | `await delivery.once(store, handler_name)`: `IdempotencyStore` protocol with a SQL implementation (table `(handler, job_id)` unique) and in-memory for tests; matches framework 4a. AgentPlanner has its own `trigger_intake` table with `dedupe_key = job_code:instance_id` and a durable queue; the hook must allow "enqueue and ack" patterns (return Ack after your own durable write). |
| `fc_auth.lifespan(...)` | opens/closes the `FlowCatalyst` client (stored on `app.state.fc`), starts JWKS warm-up in background, optional sync on startup with `dry_run` or `apply`, `fail_open=True` logging WARNING and continuing (AgentPlanner's choice) or raising. Composable with the app's own lifespan (`async with`). |
| OpenAPI | `fc_auth.security_scheme()` registers `HTTPBearer` (`bearerFormat: JWT`) and optional `OAuth2` (client-credentials / auth-code) so `/docs` shows Authorize; `require_permission` adds `x-required-permissions` to the operation; receivers are tagged `flowcatalyst-receivers` and flagged `x-signature` HMAC (header API keys). |
| Errors | handlers translate `FlowCatalystError` and auth failures to the framework envelope `{error, message}` (UNAUTHENTICATED, FORBIDDEN, CLIENT_NOT_PERMITTED, UNAVAILABLE) with an opt-out for apps with their own envelope. |
| Testing | `flowcatalyst.testing`: `FakePlatform` (respx-based token endpoint, discovery, JWKS, sync endpoints with request counters; derived from AgentPlanner's `tests/central_agent/fake_platform.py`), `mint_token(claims)`, `signed_delivery(body, secret, now=...)` returning headers, `FakeClient` for app unit tests, `override_principal(app, Principal(...))` dependency override. |

### Sessions / OIDC login (optional extra `flowcatalyst-fastapi[session]`)

The framework sessions contract (`docs/contracts/sessions-and-login.md`) defines a relying party:
authorization code with PKCE S256, state and nonce, `/auth/login`, `/auth/callback`, `/auth/session`,
`/auth/logout`, opaque cookie, server-side store (drivers `database`, `redis`, `cookie`), CSRF via
Fetch Metadata, 401-JSON versus 302 for HTML navigation, refresh and rotation. AgentPlanner has
already built exactly this by hand (`oidc.py` 279 lines, `sessions.py` 245, `auth_routes.py` 158,
`auth.py` 208) with a Postgres session table, a row lock around refresh (the platform detects
refresh-token reuse and revokes the family), and `PlatformUnavailable` kept distinct from "session
dead". So the helper **is** sensible for FastAPI: it is the second-largest piece of boilerplate the
app owns. Recommendation: ship it as an optional extra, in milestone M5, after the bearer and
receiver paths, with a `SessionStore` protocol and a Postgres (asyncpg / psycopg async) and an
in-memory implementation, mirroring contract sections 4 to 7. Do not ship a cookie driver or redis
driver in v1. Honour the sessions contract rulings: no CORS, no framework-rendered login pages,
tokens never to the browser, roles come from the **id_token** (not the access token, which is
`token_use=identity` for a default login client).

## B.7 Packaging, tooling, versioning, location

* Location: `clients/python-sdk/` in this repo (uv workspace root with `packages/flowcatalyst` and
  `packages/flowcatalyst-fastapi`), same shape as the other `clients/*` SDKs, with a `VERSION` file
  and a `make release-python-sdk BUMP=...` target and a `split-python-sdk.yml` workflow to be added
  later following the pattern of `split-typescript-sdk.yml` (**do not create workflows now**; note the
  existing split workflows are disabled with `if: false` since 2026-09-24). Tag format
  `python-sdk/vX.Y.Z`.
* Tooling: `pyproject.toml` (PEP 621, hatchling), `uv` for env / lock / build, `ruff` (lint + format,
  `select = ["E","F","I","UP","B","SIM","ASYNC","S","RUF"]`), `mypy --strict` and `pyright` (strict) in
  CI (pick one blocking, run the other as a report), `pytest` + `pytest-asyncio` (auto mode) +
  `respx` + `pytest-cov`, `hypothesis` for header / timestamp parsing. Python 3.11, 3.12, 3.13 matrix.
* Versioning: `0.x` aligned with the others; the Python line starts at `0.1.0`; the two
  distributions release together with the same version (a single tag) to avoid compatibility
  matrices. Publishing to PyPI: **not done now**; first consumer installs from git
  (`pip install "flowcatalyst @ git+https://github.com/flowcatalyst/python-sdk@v0.1.0"`) or a path
  dependency during development.
* Public API: `__all__` per module; anything not re-exported from `flowcatalyst/__init__.py` is
  private; a test enumerates `__all__` against a snapshot so surface growth is deliberate (this is
  how the Python SDK avoids the "289 ops" problem the audit found).
* Dependencies kept minimal: httpx, pydantic, PyJWT[crypto]. Optional: asyncpg, psycopg, sqlalchemy,
  redis. No `requests`, no `aiohttp`, no `python-jose`.

## B.8 Conformance and test plan

| Vector | Source | Test |
|---|---|---|
| Outbox rows | `flowcatalyst-framework/conformance/outbox/outbox.json` (3 cases, `invalidDispatchJobs`) | copy to `tests/vectors/` (like `pkg/fcsdk/outbox/testdata`) with a refresh script; compare parsed values per `conformance/README.md` (key order free, a JSON document inside a string is parsed too) |
| Receivers | `conformance/receivers/context.json` (11 cases: 3 dispatch with/without client header and with a space in the correlation id, scheduled job, user token / missing sub / missing type / opaque / token endpoint 401 / unknown type) | table-driven test using the FastAPI receivers with `FakePlatform` |
| Webhook signature | `tests/golden/webhook` in this repo (also `pkg/fcsdk/webhook/testdata`) and the vector inside `context.json` (`257e2fe8...` for secret `whsec-test-only`, clock 2026-10-09T12:00:00Z) | exact hex equality, tolerance edges 300 / 60 s, Unix-seconds header, empty secret |
| TSID | `conformance/tsid/` | encode / decode, prefix |
| Auth / sessions | `conformance/auth/login-failure.json` (when M5 lands) | JSON envelope vs 302 |
| Wire contract | `api/openapi.lock.json` | the drift test in B.3 |
| Sync | httpx `MockTransport` per category plus the Go test fixtures (`pkg/fcsdk/sync/categories_test.go`) | response mapping incl. `skippedRoles`, `passwordHashIgnored` |

Integration against an isolated platform: `scripts/e2e-platform.sh` (`make e2e-platform`), with
`E2E_DB_MODE=own` so it starts its own Postgres cluster on `127.0.0.1:25432` and never touches
the developer's shared embedded Postgres on 15432 or fcdev on 8080. The platform listens on
`http://localhost:18080`. Pytest marker `@pytest.mark.e2e`, skipped unless `FC_E2E_BASE_URL` is set;
a session fixture shells out to `make e2e-platform CMD=up` / `env` when asked, reads the generated env
file, creates an application and service account via its seed. Round trip scenarios: client
credentials + `/api/me`-style token check, role sync then list, event type sync, outbox write +
platform-side processing (outbox processor is **off** in the e2e platform, so use `POST
/api/events/batch` directly for ingest and assert the dispatch round trip with `CMD=verify`'s
receiver), scheduled-job sync then a manual fire hitting a FastAPI receiver, signature rejection. Do
not run these during the audit phase; they are specified for M6.

## B.9 AgentPlanner today (primary requirements source)

Note on the path: the brief gave `../inhance/InhanceMono/apps/AgentPlanner`, which does not exist.
The app is at `/Users/andrewgraaff/Developer/inhance/AgentPlanner` (a standalone repo, product name
"Molly", package `central_agent`). That is what was read (read-only). No secret values were copied;
environment variable *names* are listed, values are redacted.

### Profile

Python 3.11+, FastAPI + uvicorn, sync `httpx.Client`, sync `psycopg`, `pyjwt[crypto]`, LangGraph agent.
FlowCatalyst code: `src/central_agent/flowcatalyst/` (5 modules, about 700 lines) plus
`web/auth.py`, `web/auth_routes.py`, `web/sessions.py`, `permissions.py`, `access.py`, and one route in
`web/server.py`. Tests: `tests/central_agent/flowcatalyst/` (webhook, sync, rbac, scheduled_jobs, oidc
flow, oidc verify, principal) and `fake_platform.py`. Its own docs call the package "the seed of a
Python FlowCatalyst SDK".

### What it uses

| Need | How it does it today | Platform contract used |
|---|---|---|
| Client-credentials service token | `ServiceTokens` (`flowcatalyst/sync.py`), sync httpx, cached in memory, expiry minus 60 s, force-refresh on 401 once | `POST /oauth/token` form `grant_type=client_credentials` with a **service account** pair (a plain OAuth client gets 500 "Client not properly configured") |
| Role/permission sync | `sync_roles` (`POST /api/applications/{app}/roles/sync`, body `{roles:[{name (unprefixed), displayName, description, permissions, clientManaged}]}`, `removeUnlisted` query) on boot (`CENTRAL_AGENT_FC_SYNC_ON_BOOT`, fail open) and as a CLI | sdk-sync roles |
| Scheduled-job sync | `sync_scheduled_jobs` (`POST /api/applications/{app}/scheduled-jobs/sync`, body `{jobs:[{code, name, crons, targetUrl, timezone, concurrent, tracksCompletion, timeoutSeconds, deliveryMaxAttempts, payload}]}`), whole set in one call because the platform archives omitted jobs; on boot, fail open | sdk-sync scheduled-jobs |
| Webhook signature verification | `flowcatalyst/webhook.py` `verify()`: hex HMAC-SHA256 of `timestamp + raw body`, headers `X-FlowCatalyst-Signature` / `-Timestamp`, ISO8601-ms or Unix seconds, 300 s / 60 s, fail closed on missing secret, constant time; `sign()` / `signed_headers()` for tests and `scripts/send_trigger.py`; secret `FLOWCATALYST_SIGNING_SECRET` (platform's own name) | wire-contract signatures; golden vectors computed with the Go SDK |
| Scheduled-job receiver | `POST /triggers/flowcatalyst` in `server.py`: verify raw bytes, parse `{jobCode, instanceId, payload}`, enqueue into a durable `trigger_intake` table with `dedupe_key = job_code:instance_id` (`ON CONFLICT DO NOTHING`), 202 (`duplicate` for repeats), worker pool drains the queue (skip-locked claim, retention, stale recovery) | framework-style receiver, `tracksCompletion=false` |
| OIDC relying party | `flowcatalyst/oidc.py`: discovery (failure not cached), hand fetched JWKS (so tests can inject an httpx transport), RS256 only, kid miss negative cache, `verify_id_token` (aud = client id, nonce), `verify_api_token` (aud = issuer, `token_use == "api"`), PKCE S256, auth URL, code exchange, refresh, end-session URL, `PlatformUnavailable` vs `TokenError` | `/.well-known/openid-configuration`, `/oauth/authorize`, `/oauth/token`, jwks |
| Sessions | `web/sessions.py` Postgres `auth_session` + `auth_flow` tables, hashed sid, idle TTL, `ensure_fresh` under `SELECT ... FOR UPDATE` so only one request rotates the refresh token (platform revokes the family on reuse), fail closed if no id_token / refresh token comes back | sessions contract (it predates it) |
| Principal and authorization | `Principal.from_claims` (clients / applications parsed to code halves, `*` = ALL sentinel, tier, type, roles), `rbac.matches`, `Catalogue` role-to-permission map, `authorize(principal, permission, client, registry)`, a `ROUTE_PERMISSIONS` map plus coverage test, `HUMAN_ONLY` permissions refused for `type != USER` | token claim shapes from the platform |
| Bearer for machine callers | `Authenticator._bearer` -> `verify_api_token` -> `Principal` | access token `token_use=api` |
| Browser vs API unauthenticated | redirect for HTML, 401 for fetch (auth_routes); origin check for cookie writes | sessions 7.2 / 7.4 |
| Test double | `fake_platform.py`: RSA key + JWKS + discovery + token endpoint, request counters, refresh-reuse detection, forced token status | n/a |

### What it hand-rolls that the SDK should own

1. Token provider + early-expiry cache + 401 retry (`ServiceTokens`), sync only.
2. HTTP retry/backoff: **none**. A boot-time sync that hits a 503 fails open and the roles are not
   registered until next boot. The SDK transport should absorb transient failures.
3. Error mapping: `SyncError` / `ScheduledJobError` with `f"... returned {status}"` strings and manual
   extraction of `message` from the body (needed because the 403 names the missing permission).
4. Signature verification and signing helpers (plus test vectors).
5. JWKS fetch, discovery, RS256 verification, claim requirements, negative kid cache, nonce / aud /
   `token_use` rules (it documents that "both shipped SDKs get one of these wrong").
6. Principal / client / application access parsing (with a documented bug list against the TS and PHP
   SDKs) and wildcard permission matching.
7. Role-sync payload shape and the role-name prefix rule; scheduled-job payload shape.
8. The whole OIDC login flow, session store and refresh coordination.
9. Boot orchestration: env flags, "fail open", logging of created / updated / deleted.
10. A fake platform for tests.

### What it does NOT use (so the SDK must not force it)

Outbox and event ingest (`/api/events/batch`, dispatch jobs, audit); event types, subscriptions,
connections, pools, processes sync; `GET /api/me`; the completion / log callbacks (jobs are
`tracksCompletion=false`); webhook bearer gate (the app notes it does not check
`Authorization: Bearer`); dispatch-job receivers (`/fc/dispatch`), `once()` idempotency (it uses its
own table); the use-case envelope; definitions via decorators; any admin API; sync-vs-async: it is
mixed. It runs sync psycopg and `asyncio.to_thread` inside async routes, and sync `httpx.Client`
at boot.

### Pain points / inconsistencies found

* Its docs carry stale guidance (`CLAUDE.md` says the signature "carries no timestamp"; the code and
  platform do). An SDK-owned verifier removes the second source of truth.
* Header name casing uses `X-FlowCatalyst-*` while one doc says `X-FC-Signature`.
* Two sync modules duplicate the same 401-retry-once and error-formatting block.
* The app maintains its own `Principal` after finding client-string comparison bugs in the TS and
  PHP SDKs (A.6, item 6).
* Role names: platform adds `{app}:` and rejects `:` in a name; easy to get wrong without a typed
  helper (the roles in tokens are prefixed, the sync payload is not).

### Migration sketch (AgentPlanner to the Python SDK)

| AgentPlanner today | Python SDK replacement | Notes |
|---|---|---|
| `flowcatalyst/sync.py: ServiceTokens` | `flowcatalyst.ClientCredentialsTokenProvider` (async) or `FlowCatalystSync(...).tokens` | same env names via `Settings.from_env(prefix="CENTRAL_AGENT_FC_")` |
| `sync_roles(...)` | `fc.sync.roles(app_code, DefinitionSet.roles, remove_unlisted=False)` | `Catalogue`/`ROLES` becomes `defs.role(...)`; `sync_payload()` disappears |
| `sync_scheduled_jobs(...)` and `job_definitions()` | `fc.sync.scheduled_jobs(app_code, defs.scheduled_jobs)` | typed `ScheduledJobDefinition`; whole-set semantic enforced by the SDK |
| `sync_on_boot()` / `register_on_boot()` | `fc_auth.lifespan(sync=defs, sync_on_boot="apply", fail_open=True)`; CLI `python -m flowcatalyst sync --defs central_agent.definitions:defs [--remove-unlisted] [--dry-run]` | the boot call is currently outside the loop in `main()`; use `FlowCatalystSync` there, or move to lifespan |
| `flowcatalyst/webhook.py` (`verify`, `sign`, `signed_headers`, `secret_from_env`) | `flowcatalyst.receivers.verify_signature`, `flowcatalyst.testing.signed_delivery` | golden vectors move to the SDK; AgentPlanner keeps a thin test |
| `POST /triggers/flowcatalyst` handler | `Depends(signature_verified)` returning raw + envelope, or `fc_auth.scheduled_job_receiver` with a handler that does the durable enqueue and returns `Ack`; keep `trigger_intake` as the app's own queue | the SDK's receiver answers 202 immediately; AgentPlanner already does (enqueue then 202) |
| `oidc.py: OidcClient` | `flowcatalyst.auth.JwksCache` + `OidcClient` (discovery, PKCE, exchange, refresh, end-session) | verbatim semantics preserved (RS256 only, nonce, `token_use`, negative kid cache) |
| `principal.py` and `rbac.py` | `flowcatalyst.auth.Principal`, `permissions.matches`, `Catalogue` | adopt the framework's segment-count wildcard rule or keep AgentPlanner's by option (D6) |
| `web/auth.py: authorize / require / ROUTE_PERMISSIONS` | `fc_auth.principal`, `require_permission`, `fc_auth.client`, `route_permissions` + coverage helper | the message wording stays app side (`on_denied` callback) |
| `web/sessions.py`, `auth_routes.py` | `flowcatalyst-fastapi[session]` with `PostgresSessionStore` (async) | big win but last; AgentPlanner uses sync psycopg, so the Postgres store needs a sync twin or an adapter via `asyncio.to_thread` (D2) |
| `tests/central_agent/fake_platform.py` | `flowcatalyst.testing.FakePlatform` | refresh-reuse detection and `omit_id_token_on_refresh` toggles carried over |
| `scripts/send_trigger.py` | `python -m flowcatalyst.testing send-delivery ...` | optional |

Migration order for AgentPlanner (each step independent, keeps tests green): (1) webhook verify and
test signer; (2) token provider + role sync + job sync (retries gain); (3) JWKS + `Principal` +
permissions; (4) route dependencies; (5) lifespan; (6) sessions. Steps 1 to 5 need no schema change.

---

# Milestones, sizes, risks

Sizes: S = 1 to 3 days, M = 1 week, L = 2 weeks (one engineer, including tests).

| # | Milestone | Content | Size | Risk |
|---|---|---|---|---|
| M0 | Platform prerequisites | `x-sdk` tagging in `tools/dump-spec`; add `dispatch-jobs/batch`, `audit-logs/batch`, `events` single to the dumped spec; shared behaviour contract doc (A.9); copy conformance vectors into this repo's `tests/golden` | M | spec drift guard test must be updated; touches platform code, needs owner approval |
| M1 | Core transport and auth | `FlowCatalyst` async client, `http.py` retry/timeouts/Retry-After, error tree, `ClientCredentialsTokenProvider`, `FlowCatalystSync` facade, settings/env | M | sync facade correctness (loop thread, cancellation); retry semantics for POST |
| M2 | Verification, principal, permissions | `JwksCache` / verifier, `Principal`, `matches`, `Catalogue`, webhook `verify_signature` + `signed_delivery`, `testing.FakePlatform` | M | contract differences between AgentPlanner and framework wildcard rule (D6); exact timestamp parsing |
| M3 | Sync | `DefinitionSet`, per-category calls, `SyncResult`, dry run, CLI, contract test against the lock | M | dry-run semantics (platform has none; client-side diff); archive semantics |
| M4 | FastAPI core | `FlowCatalystAuth`, `principal`, `require_permission`, client selection, route-map helper + coverage test, lifespan with sync, OpenAPI scheme, error envelopes, receivers (dispatch + scheduled job), `once()` | L | receiver contract details (4b principal, 202 immediacy, timeouts); DI ergonomics |
| M5 | Sessions / OIDC (extra) | relying party, `SessionStore` protocol + Postgres/in-memory, login/callback/session/logout routes, CSRF, refresh lock | L | highest security risk; sync-psycopg adopters; scope creep |
| M6 | Ingest + outbox | events / dispatch-jobs / audit builders and validation, asyncpg / psycopg / SQLAlchemy writers, unit-of-work helper, vector tests, reads (`event_types`, `roles`, ...) | L | transaction ownership with each driver; two `data` encodings |
| M7 | Integration | e2e against `make e2e-platform` with `E2E_DB_MODE=own`; AgentPlanner migration PR (their repo, steps 1 to 5) | M | needs the e2e platform seeded for a Python app; coordination with the Inhance team |
| M8 | Packaging | uv workspace, CI (outside this task), `release-python-sdk` Makefile target, README, versioning policy, git-installable tag | S | none |

Order for first release (0.1.0): M0 (partial), M1, M2, M3, M4, M7(partial), M8. M5 and M6 follow
(0.2.0). Estimated 6 to 8 engineer-weeks for 0.1.0, 10 to 12 for 0.2.0.

Top risks: (1) scope creep into admin or usecase envelope; (2) wire drift without a generator
(mitigated by contract test and the vendored lock); (3) platform gaps (missing dispatch-jobs batch
in the lock, no dry-run, no invoke-function endpoint); (4) async/sync duality for existing sync
adopters; (5) security surface of sessions.

---

# Owner decisions (with recommendation)

| # | Decision | Recommendation |
|---|---|---|
| D1 | Trim the other SDKs first or build Python first? | Build Python on the curated surface now; trim Java / Go (cheap) then TS, Laravel last. Python must not wait on the trims, but M0 should be agreed. |
| D2 | Sync vs async parity | Async is the implementation; ship a blocking `FlowCatalystSync` facade (thread-hosted loop) in 0.1.0 because AgentPlanner's boot, CLI and psycopg-sync code need it. No duplicated sync implementation. |
| D3 | Pydantic version | pydantic v2 only (>=2.7); no v1 shim. |
| D4 | Generated vs handwritten | Handwritten facade and models, contract test against the lock; revisit models-only generation if the surface exceeds about 100 ops. |
| D5 | Where does the admin surface go | Separate `admin` packages generated from `openapi.admin.json`, only on demand; no Python admin client in v1. The user-management and tenant-provisioning calls that HR, RFP and Integral make (section C.5) stay available in Laravel behind an `Admin` facade; revised from "Integral's" after the app survey. |
| D6 | Permission wildcard rule | Adopt the framework / platform rule (same segment count, `*` per segment) as default; offer `prefix_wildcards=True` for AgentPlanner's current prefix-covers-deeper behaviour during migration. |
| D7 | Sessions/OIDC login in the FastAPI package | Yes, as optional extra in M5 (AgentPlanner proves demand). Postgres + in-memory stores only; no cookie / redis drivers in v1. |
| D8 | Use-case envelope (`usecaseop`) for Python | Not in v1. Provide a minimal unit-of-work helper in M6; build the envelope only on a second adopter's request. |
| D9 | Outbox `data` encoding | Write the JSON value (Go form); vectors pass under the parsed-value rule and the platform unwraps strings anyway. Confirm with the framework owner that the Java reference is fine with the value form. |
| D10 | Retry policy across SDKs | Adopt A.9: do not retry 500 by default (opt-in), retry 425, honour `Retry-After`, add jitter. This changes Go (drop 500) and Java / TS / PHP (add 425 / jitter / `Retry-After`) as a follow-up, outside the Python work. |
| D11 | Django / Flask later | Keep the core framework-neutral (`flowcatalyst.receivers`, `auth`) so adapters are thin; do not build them until requested. Starlette (non-FastAPI) comes free. |
| D12 | Repo and release | `clients/python-sdk` in this repo, tag `python-sdk/vX.Y.Z`, one version for both distributions, git-installable first, PyPI later; split workflow only when the owner agrees (the existing split workflows are disabled). |
| D13 | Package names | `flowcatalyst` and `flowcatalyst-fastapi` on PyPI (check availability before publishing; the TS package is `@flowcatalyst/sdk`). |
| D14 | Missing platform operations | Add `x-sdk` tagging and the missing batch routes to the spec (M0); define a stable "invoke function" operation if the brief's "invoke functions" belongs in app SDKs. |
| D15 | AgentPlanner migration ownership | The Inhance team (or this repo's owner on request) migrates AgentPlanner in steps 1 to 5 after M4; AgentPlanner remains the acceptance test for 0.1.0. |

---

# Part C. Laravel SDK in the Inhance apps today

Surveyed read-only on 2026-10-15 in `InhanceMono` (`apps/hr`, `apps/rfp`, `apps/integral`, `packages_root`).
Counts are from grep over `*.php` excluding `vendor` and `node_modules`. No secrets were read; env var
names only. `apps/rfp` exists. Other `apps/*` (client-portal, in-motion, infra-admin, shortener, ...) have no
`flowcatalyst` entry in their `composer.json`; sibling repos under `inhance/` (AgentPlanner aside) were not
found requiring `flowcatalyst/laravel-sdk` either (`laravel-flowcatalyst-example` has no root composer.json).

## C.1 Version pins

| Consumer | composer.json constraint | Locked | Behind HEAD (0.10.31) |
|---|---|---|---|
| apps/hr | `^0.8` (VCS repo `github.com/flowcatalyst/laravel-sdk`) | **v0.8.19** | two minor lines; the constraint blocks 0.10 |
| apps/rfp | `^0.10` | v0.10.23 | 8 patches |
| apps/integral | `^0.10.12` | v0.10.26 | 5 patches |
| packages_root (monorepo root) | `^0.10.21` | v0.10.26 | 5 patches |
| packages/logistics-portal | `^0.10`; packages/router-client `*` | via root | n/a |

The SDK is a git/VCS dependency, not Packagist.

## C.2 Findings in numbers

| | HR | RFP | Integral (app) | packages_root/packages |
|---|---|---|---|---|
| PHP files referencing `FlowCatalyst\` | 113 (45 tests) | 89 (33 tests) | 12 (4 tests) | 37 (1 test) |
| Non-test files that hold a `FlowCatalystClient` | 7 | 5 | 2 | 13 |
| `generated()` / `Generated\*` / Jane | **0** | **0** | **0** | **0** |
| `#[AsPermission]` imports | 27 | 24 | 0 | 0 |
| `#[AsRole]` / `#[AsEventType]` / `#[AsScheduledJob]` / `#[AsSubscription]` | 14 / 10 / 1 / 0 | 9 / 0 / 2 / 0 | 0 / 0 / 2 / 0 | 0 / 0 / 0 / 1 |
| `FlowCatalystAuthenticatable`, `PermissionResolver`, `RbacCatalogue`/`RbacBuilder` | 23, 4, 5+1 | 9, 5, 3+1 | 0, 2 (Spatie resolver), 0 | 0, 1, 1 |
| OIDC (`OidcAuthController`, `OidcUserHandler`) | env-only (`FLOWCATALYST_OIDC_*`, `NATIVE_LOGIN`) | env-only | routes + handler + login controller | `OidcUserHandler` impl in integral-service-v2 |
| UseCase envelope (`Runner`, `Plan`, `Operation`, `Aggregate`, `Result`) | yes (Grading, 5+ files) | no | `Runner`/`Result` in 1 test | project-spar, integral-service-v2 |
| Outbox (`OutboxDriver`, migration `outbox_messages`) | yes (migration + `GradingEventService`) | no | no | via project-spar `SparUnitOfWork` |
| Scheduled-job callback (`ScheduledJobRunner` + `CacheLockProvider`) | yes | yes (`RoundSweepJob`) | yes (2 jobs) | yes (integral-service-v2) |
| Webhook validation | `ValidateWebhookSignature` via SDK wiring; `FLOWCATALYST_SIGNING_SECRET`, `FLOWCATALYST_WEBHOOK_AUTH_TOKEN` | same (1 import) | `FlowCatalystWebhookController` in package | same |
| Sync `DefinitionSynchronizer` / `SyncDefinitionSet` | no (attribute scan via `flowcatalyst:sync`) | no (same) | no | 2 files each: roles, event types, dispatch pools |

`flowcatalyst:sync` appears 75 times and `flowcatalyst:scan` 67 times in docs, deployment runbooks and
scripts: attribute-based sync is how HR and RFP register roles, permissions, event types and scheduled jobs.
Custom artisan commands in the apps: `hr:sync-test-principals`, `rfp:sync-test-principals`,
`flowcatalyst:create-user`, `flowcatalyst:sync-users`, `flowcatalyst:sync-clients`,
`flowcatalyst:sync-definitions`.

Env var names in use: `FLOWCATALYST_BASE_URL`, `_CLIENT_ID`, `_CLIENT_SECRET`, `_APP_CODE`, `_SIGNING_SECRET`,
`_WEBHOOK_AUTH_TOKEN`, `_OIDC_ENABLED`, `_NATIVE_LOGIN`, `_OIDC_CLIENT_ID`, `_OIDC_CLIENT_SECRET`,
`_OIDC_HANDLER`, `_OIDC_PERMISSION_RESOLVER`, `_OIDC_SINGLE_LOGOUT`, `_REDIRECT_AFTER_LOGIN`,
`_REDIRECT_AFTER_LOGOUT`, `_ERROR_REDIRECT`, `_SEED_SPATIE`, `_OUTBOX_ENABLED`, `_OUTBOX_AUDIT_ENABLED`,
`_OUTBOX_CLIENT_ID`, `_CLIENT_CODE`, `_TOKEN_URL`, `_TOKEN_CACHE_DRIVER`, `_TIMEOUT`, `_RETRY_ATTEMPTS`,
`_RETRY_DELAY`.

## C.3 Client methods actually called (non-test code)

| Resource call | Where | Plan class |
|---|---|---|
| `principals()->findByEmail` | HR (GoLiveAssignmentSeeder), RFP gateway, Integral (sync-users, 2), v2 (SetPasswordController, ResetUserPassword, UpdateUser) | optional keep |
| `principals()->createUser` | RFP gateway, Integral create-user + sync-users, v2 CreateUserUseCase | optional keep |
| `principals()->resetPassword` | v2 (3 sites) | optional keep |
| `principals()->update` | Integral sync-users | optional keep |
| `principals()->get` | RFP gateway | optional keep |
| `principals()->list(active:, type:)` | HR PrincipalDirectory, HR ImportRolesRegister, RFP PlatformPrincipalDirectory | **admin (removed)** |
| `principals()->addRole` / `removeRole` | RFP gateway (3 sites), Integral sync-users | **admin** |
| `principals()->getRoles` | Integral sync-users | **admin** |
| `principals()->grantClientAccess` | Integral sync-users | **admin** |
| `principals()->sync(app, entries, removeUnlisted)` | HR + RFP `*:sync-test-principals` | class 1 keep |
| `principals()->syncUsers` | Integral sync-users | class 1 keep |
| `clients()->list` | Integral sync-users, v2 SyncFlowCatalystClients | keep (class 1 reads) |
| `clients()->create` | v2 SyncFlowCatalystClients, RunTenantMigrationsJob (tenant provisioning) | **admin** |
| `applications()->getByCode` | HR FeedbackNotifyRecipients, v2 sync-definitions, v1 sync service | keep |
| `applications()->listClients` | HR FeedbackNotifyRecipients | **admin** |
| `applications()->create` | v2 sync-definitions, v1 FlowCatalystSyncService | **admin** |
| `roles()->list`, `roles()->sync` | Integral sync-users, v2 sync-definitions (own retry loop) | keep |
| `eventTypes()->list` | v2 EventTypeSchemaSync | keep |
| `scheduledJobs()` passed to `ScheduledJobRunner` | HR, RFP, Integral, v2 callbacks | keep |
| `router()->areInPipeline` | v2 ProcessRecordInProgressResetterJob | **removed (RouterResource)** |
| `DefinitionSynchronizer::sync` (event types, dispatch pools) | v1, v2 | keep |
| `client->request(...)` (public escape hatch) | RFP, Integral, v2 (see C.4) | not in plan |
| `new FlowCatalystClient(tokenProvider:, baseUrl:, timeout:, retryAttempts:, retryDelay:)` | RFP `PlatformWriteClientFactory` (single attempt on request paths) | constructor is public API |

## C.4 Raw HTTP through `request()` and other workarounds

All five routes below are in the current lock, so they are not "missing from the spec", they are missing
from the SDK's resources.

| Route | Consumer | Why raw |
|---|---|---|
| `GET/PUT/DELETE /api/config/{app}/{section}/{property}` and `GET /api/platform-config/{app}` | RFP `PlatformConfigStore` | no SDK resource for platform config |
| `GET /api/email-domain-mappings/by-domain/{domain}` | RFP `PlatformPrincipalGateway` | no SDK resource (admin tag) |
| `PUT /api/principals/{id}/client-association` | Integral `SyncUsersToFlowCatalystCommand` | comment: "SDK's Principals resource doesn't wrap this route ... request() escape hatch" |
| `POST /api/event-types/{id}/versions` | v2 `EventTypeSchemaSync` | comment: SDK `AddSchemaVersionRequest` omits `version` which the platform requires (huma 400) |
| (read of client by id or identifier) | RFP `PlatformWriteClientFactory` + resolveClient | SDK DTO defaults a missing `status` to ACTIVE, so raw answer is used |

Other pain points recorded in the apps' own comments:

* `DefinitionSynchronizer` reads `$row['code']` but `EventTypeDefinition::toArray()` omits it (every entry
  synced as ""): v2 passes plain arrays.
* Platform roles sync is atomic and returns only "sync save failed at index N": v2 wrote
  `syncRolesWithRetry()` (drop the named index, retry) and bypasses `->withRoles()`.
* The SDK client singleton uses 30 s x 3 retries: RFP builds its own one-attempt client for request paths.
* `SyncPrincipalEntry` has no `clientId`, so Integral does a second raw call per user.
* HR is held on 0.8.x (decisions doc: attribute / RBAC / Spatie-free design built on that line).
* Tests: RFP defines `ScriptedFlowCatalystClient extends FlowCatalystClient` and a fake `TokenProviderInterface`;
  the SDK's own `Testing/` helpers are not imported anywhere. `FlowCatalystClient`'s class shape and
  constructor are therefore part of the de-facto public API.

## C.5 What the trimming plan (A.8) would break

| Plan action | Breaks in the Inhance apps? | Where |
|---|---|---|
| Delete Jane / `generated()` / `Generated\*` | **No** (0 uses in 4 trees) | none; safest cut |
| Remove `principals()` list / addRole / removeRole / getRoles / grantClientAccess (class 2) | **Yes** | HR (2 files), RFP (2 files), Integral sync-users |
| Remove `clients()` create | **Yes** | v2 sync-clients and tenant provisioning (`RunTenantMigrationsJob`) |
| Remove `applications()` create / listClients | **Yes** | v1 + v2 sync-definitions (bootstrap app registration), HR FeedbackNotifyRecipients |
| Remove `RouterResource` | **Yes** | v2 `ProcessRecordInProgressResetterJob` (`areInPipeline`) |
| Remove event-type `addSchemaVersion` (class 3 dup of sync) | **No, but wrong rationale** | Integral already bypasses it because the SDK DTO is broken; sync does not carry JSON schemas, so removal is only correct once sync carries schema versions or a fixed method exists |
| Remove or hide `request()` | **Yes** (5 routes) | RFP, Integral, v2 |
| Remove CRUD that duplicates sync (create / update / delete on roles, subscriptions, ...) | No | no use found in code |
| Keep the 5 "optional" principals methods + syncs | covered | note `findByEmail`, `createUser`, `resetPassword`, `update`, `get` are used by **all three** apps |

## C.6 SDK methods in use that the plan classes as admin or removed

`principals()->list`, `addRole`, `removeRole`, `getRoles`, `grantClientAccess`; `clients()->create`;
`applications()->create`, `listClients`; `router()->areInPipeline`; public `request()` and the
`FlowCatalystClient` constructor options; `DefinitionSynchronizer` dispatch-pool sync (class 1 by
this audit, but pools are listed under admin in A.8's "Router / pools" row: the cut as written would
remove v2's dispatch-pool definitions, so pool *sync* must stay). That is 8 resource methods plus 2
structural features.

## C.7 What the first draft got wrong or left open

1. "Integral uses `principals()` ..., and probably `clients()`" and "two apps depend on it": there are three
   direct consumers (HR, RFP, Integral) plus 13 shared-package files. HR and RFP use `principals()` more than
   Integral does (directory listing, role assignment, invitations).
2. "Removing `generated()` is safe unless an app calls it (grep needed)": grep done, no caller anywhere.
   Closed, and in the apps' favour.
3. The principals "optional keep" set missed `list`, `addRole`, `removeRole`, `getRoles`, `grantClientAccess`
   (class 2 in the table but in live use); `clients()->create` and `applications()->create` / `listClients`
   likewise.
4. `RouterResource` was classed "needs the router host, not the platform host, remove": Integral calls it
   through the platform client.
5. `request()` was not considered; it is the apps' workaround for four SDK gaps and cannot be removed.
6. Version skew was not recorded: HR is two minor lines behind and its constraint blocks upgrades, so any
   Laravel breaking release is invisible to HR until it opts in.
7. A.1 row "Test helpers `Testing/`": unused by the apps; they subclass `FlowCatalystClient` instead.
8. Angles confirmed: hand layer uses `request()` literal paths, not Jane (consistent with row "about 0%").

## C.8 Revised breaking-change cost and migration order

Laravel cost: **Medium to high** if class 2 methods are removed from `FlowCatalystClient`; **Low** if the
cut is (a) drop Jane and `generated()` and (b) move class 2 resources behind an `Admin` facade with the
old accessors kept as deprecated aliases for one line. Recommended order for Laravel:

1. 0.10.x patch: add resources/methods for the four workarounds (platform config get/put/delete, email
   domain mapping by-domain, principal client-association, fixed `addSchemaVersion` with `version`, raw-answer
   client lookup) so the apps can delete their `request()` calls. Additive, no break. Fix
   `EventTypeDefinition::toArray()` code omission and the atomic role sync error message.
2. 0.10.x: add `Admin` facade exposing principals / clients / applications admin / router; old accessors
   emit a once-per-process deprecation.
3. 0.11.0: drop Jane and `Generated\*` (zero app impact) and the 24 sync-duplicating CRUD methods (zero use).
4. Apps move one at a time: Integral and `packages_root` (already on 0.10.26, owns most admin calls) first,
   RFP second, HR last (needs its `^0.8` constraint lifted and a 0.8 to 0.10 jump review).
5. 0.12.0 (or 1.0): remove the deprecated aliases only after all three apps and `packages_root` have
   switched to `Admin`, verified by grep for the accessor names.

Python SDK and the Java/Go/TS trims are unaffected by these findings; the Laravel step stays last in the
A.8 sequence, and D1 stands.
