# Ingestion in the platform: design

Status: design, 2026-10-24. Nothing is built. Source of the behaviour described here: the ingest control plane in
`inhance/mono-repo` (`apps/ingest-control-plane`, `libs/java/ingest-contract`, `docs/ingest`), read on 2026-10-24.
Owner direction (2026-10-24): ingestion is a platform capability for "building business event driven business
services and applications", not a feature of one organisation's mono-repo.

## 1. What it is and what moves

An **ingestion** takes data from wherever a system keeps it (HTTP push, SFTP, S3, SQS, RabbitMQ, a polled table or
API, or a source an extension adds), splits each payload into records, and turns each record into a **dispatch job**.
It is the inbound twin of subscriptions.

| Piece | Where it lives after this work |
|---|---|
| Definitions, versions, sets, leases, state, failure and metric records, operator API, permissions, audit | **The Go platform** (this document) |
| Operator pages | The platform SPA (section 9) |
| Definition files (`ingest-cli apply/export`) | `fcdev ingest ...` and the SDKs, over the platform API |
| The runtime that actually reads sources (Camel today) | **Outside the platform**: its own repository and release, talking to the platform's runtime API (section 7). Optional; an installation without it simply has no ingestions running |
| Inhance's own extensions (MiX, WebIQ) and its definition files | Stay in `mono-repo`, built on the released runtime |

Out of scope: any engine inside the platform process; the Java platform mirror (a hand-off document follows once the Go side exists; the Java control plane
is the reference for it).

## 2. The model

```
ingestion = (application, client, code) + source + split + target + flags
```

| Field | Meaning |
|---|---|
| `application` | The owning application, **required, of type `INTEGRATION`** (owner ruling 2026-10-24). It carries the permissions, the audit scope and the "who owns this" answer, as for connections and event types. A platform installation creates integration applications the usual way |
| `client` | **Required** (owner ruling 2026-10-24: it makes the wrong thing hard to do, and it is today's schema). A client id or identifier, resolved to the id as in platform config. Data that is not a particular client's goes under a client the business keeps for that purpose (its own home client); there is no client-less ingestion |
| `code` | Stable, lowercase, unique within `(application, client)` |
| `name`, `active`, `keepFailedPayloads` | As today. `keepFailedPayloads` stays off unless the client has agreed (a failure may then carry the start of a payload) |
| `source` | One of: `http`, `sftp`, `s3`, `sqs`, `rabbitmq`, `database-poll`, `api-poll`, `custom` (kind added by a runtime extension). Settings as in the current contract. Secrets are **names**, never values |
| `split` | `none`, `csv {header}`, `json {arrayPath}`, `xml {element}` |
| `target` | The dispatch job each record becomes (below) |

**Changed from the current contract:**

- `mix` and `webiq` are no longer built-in kinds: they are kinds supplied by a runtime extension (owner: moving to
  extensions), so the platform's own list is nine kinds, not eleven.
- **`camel-route` stays, but as a kind the Camel runtime reports, not as part of the platform** (owner 2026-10-24: keep it
  if it earns its place). Today nothing in `client-code` uses it, only the guide's examples; it is the escape hatch for
  a component or a reshaping step that does not justify an extension. The platform stores it as any runtime-reported
  kind (`{kind: "camel-route", settings: {yaml}}`), knows nothing about Camel, and only leases it to a runtime that carries
  it, so a non-Camel runtime later simply does not report it. It is dangerous (the route runs with the runtime's network,
  identity and environment), so a kind may declare a **required permission** in what the runtime reports: the platform
  refuses to create or change an ingestion of that kind unless the caller holds it (`platform:ingestion:ingestion:route`,
  role `platform:ingestion-route-author`), and the Camel runtime reports the kind only when
  `INGEST_CAMEL_ROUTES_ENABLED` is true (default false). The runtime, not the platform, validates the YAML: at "test
  connection" and when it takes the lease, reporting `FAILING` with the reason.
- **The target says where the job goes, and what signs it, in platform terms.** A dispatch job carries its own delivery URL, pool, client and signing service account; a connection has **no URL** (it holds a service account that signs, and pausing it holds its jobs). So the target is
  `{code, targetUrl, pool, connection?, serviceAccount?, mode, batchSize}`:
  - `targetUrl`: the delivery URL, checked at save with the platform's outbound-URL guard (no private or metadata addresses);
  - `pool`: a dispatch pool **code**, resolved and checked at save (it must exist and be usable by the ingestion's client);
  - `connection`: an **optional** connection code. When set, the connection's service account signs each delivery and pausing the connection holds this ingestion's jobs, as for subscriptions;
  - `serviceAccount`: an **optional** service account code, for signing without a connection; it may not be set together with `connection` (the connection's account is the signer);
  - neither set: unsigned delivery;
  - `code`, `mode` (`IMMEDIATE`, `NEXT_ON_ERROR`, `BLOCK_ON_ERROR`) and `batchSize` as today.
  The job's **client is the ingestion's client**; it is not a target field. Anything that does not resolve is a `400` at save, not a failure in the runtime later. A runtime creates the jobs through its own runtime call (section 7), which builds them from this target.
- Ids are platform TSIDs (`ing_...`), not ULIDs.

## 3. Tables (Go migrations, from 088)

All tables are created whether or not the feature is enabled (section 6). Prefix `ing_`. Money-for-nothing rule: no
table holds a secret value.

| Table | Replaces | Notes |
|---|---|---|
| `ing_ingestions` | `ingestions` | `id`, `application_id` (FK), `client_id` (**not null**), `code`, `name`, `active`, `version`, `latest_version`, `source/split/target` JSONB, `keep_failed_payloads`, `managed_by` (the owning set), timestamps. Unique `(application_id, client_id, code)` |
| `ing_versions` | `ingestion_versions` | one row per saved version: definition, `note`, `applied_from` (set apply), who and when. `live` is the version runtimes receive (see section 5) |
| `ing_runtimes` | `runtimes` | `runtime_id`, `last_seen_at`, `kinds` JSONB, `contract_version`, `url` (informational) |
| `ing_leases` | `ingestion_leases` | polled sources run on one runtime at a time; `expires_at`, `released` |
| `ing_job_keys` | (new) | the idempotency key (`externalId`) of each job a runtime created, per ingestion; written with the job, **pruned** after 7 days |
| `ing_state` | `ingestion_state` | small values a polled source keeps (watermark, cursor); only the lease holder writes |
| `ing_route_states` | `ingestion_route_states` | per runtime: version, status, error |
| `ing_exchange_metrics` | `ingestion_exchange_metrics` | per window: completed, failed, records, jobs; **pruned** after 30 days (a partition-or-delete job like the other high-volume tables) |
| `ing_failures` | `ingestion_failures` | kind, message, `after_records`, optional `excerpt` (only with `keepFailedPayloads`); pruned |
| `ing_sets`, `ing_set_applies` | `ingestion_set_applies` | named sets of definition files and their apply history; the archive of applied files goes in the platform's artifact store (`file://` or `s3://`, as function artifacts) |
| `ing_test_requests` | (new) | pending "test connection" requests and their results (section 7) |

**Audit.** `ingestion_audit` is not carried over: changes go through the use-case envelope and so into the platform
audit log (`aud_logs`), which already records who, what, and when; the History tab reads it.

## 4. Permissions and roles

Subdomain `ingestion` (four segments, like every platform permission):

| Permission | Grants |
|---|---|
| `platform:ingestion:ingestion:view` | list, read, versions, history, failures, definition export, source kinds |
| `platform:ingestion:ingestion:create` / `update` / `delete` | change definitions; `update` includes pause and resume |
| `platform:ingestion:ingestion:promote` | make a version live (section 5) |
| `platform:ingestion:ingestion:test` | run "test connection" (it makes a real outbound call from a runtime, so it is its own grant) |
| `platform:ingestion:ingestion:route` | create or change an ingestion of a kind that declares it needs this (`camel-route`); removing one needs only `delete` |
| `platform:ingestion:set:apply` | apply or release a set |
| `platform:ingestion:runtime:operate` | the runtime API: poll, report, state, jobs, secrets |

Seeded roles: `platform:ingestion-admin` (all but runtime), `platform:ingestion-operator` (view, update, test),
`platform:ingestion-viewer`, and `platform:ingestion-runtime` for a runtime's service account. Super-admin's wildcard
covers everything. The role ceiling rules (a caller cannot grant what it does not hold) apply as elsewhere.

**Reach.** A caller below anchor tier sees and changes only ingestions of its own clients. An application-scoped service
account reaches only its own application's ingestions (the existing confinement). Per-application and per-client scoping
is therefore the same axes as the rest of the platform.

**Secrets.** An ingestion names secrets (`sftp-password`). A runtime reads them through the runtime API's secrets call
(section 7), not through the config reveal: the call reveals only the secrets the ingestion's live definition names, at the
ingestion's client, only to a runtime that carries the kind and (for a polled source) holds the lease, and audits each
reveal under the runtime's principal and the ingestion. The runtime's service account therefore holds no
`platform:application-service:config:reveal`, and `platform:ingestion-runtime` gains nothing beyond
`platform:ingestion:runtime:operate`: the jobs and secrets calls are authorised by it plus those checks. The platform may
warn at save when a named secret has no value (a masked config listing is enough to know).

## 5. API (OpenAPI, then SDKs)

Everything below is huma, in `api/openapi.lock.json`, so the SDKs and the SPA types regenerate from it.

| Operation id | Route | Notes |
|---|---|---|
| `listIngestions` | `GET /api/ingestions` | filters: `applicationCode`, `clientId` (id or identifier), `q`, `status`, `kind`; cursor paging (`limit` 1 to 200) |
| `createIngestion`, `getIngestion`, `updateIngestion`, `deleteIngestion` | `/api/ingestions[/{id}]` | `updateIngestion` takes `expectedVersion`; a no-op update answers 200 and writes nothing (platform rule). Create takes `live: true` default |
| `listIngestionVersions`, `saveIngestionVersion`, `makeIngestionVersionLive` | `/api/ingestions/{id}/versions[/{v}/live]` | a saved version is not run until it is live; `live` moves the pointer |
| `getIngestionDefinition` | `GET /api/ingestions/{id}/definition` | the file form (YAML) that `fcdev ingest apply` reads back |
| `listIngestionHistory`, `listIngestionFailures` | `/api/ingestions/{id}/history\|failures` | history from `aud_logs`; failures paged by id |
| `listIngestionSourceKinds` | `GET /api/ingestion-source-kinds` | built-in kinds plus the kinds the live runtimes report; the form is drawn from this |
| `testIngestionConnection` | `POST /api/ingestion-tests` | queues a test (section 7); `GET /api/ingestion-tests/{id}` for the result |
| `applyIngestionSet`, `listIngestionSets`, `listIngestionSetApplies`, `reapplyIngestionSet`, `releaseIngestionSet` | `/api/ingestion-sets...` | as today: a set owns its ingestions (read-only in the pages) until released |
| `ingestionRuntimePoll`, `ingestionRuntimeReport`, `ingestionRuntimeGetState`, `ingestionRuntimePutState`, `ingestionRuntimeCreateJobs`, `ingestionRuntimeGetSecret` | `/api/ingestion-runtime/v1/...` | the runtime contract (section 7) |

Errors use the platform envelope with codes in the platform's style (`INGESTION_NOT_FOUND`,
`INGESTION_CODE_EXISTS`, `INGESTION_TARGET_INVALID`, `INGESTION_SET_OWNED`, `LEASE_NOT_HELD`).

**Sets (built).** `PUT /api/ingestion-sets/{name}` takes `{items, files?}`: each item is a definition file with its
`application`, `client` (id or identifier) and `code`. In one transaction, serialised with every other set apply, it
resolves every item (application, client, target, reach, kind permission) and then, per item: **CREATE** when nothing
has that (application, client, code); **UNCHANGED** when the set owns it and it says the same; **CHANGE** when the set
owns it and it differs (the definition is saved as the next live version, `applied_from` naming the apply; `active` is
set from the file, so a pause made since is undone); **ADOPT** when it was made by hand (taken into the set, a new
version only if it differs). An ingestion owned by another set is a `409 INGESTION_SET_OWNED` and nothing is written.
**An ingestion the set owns but no longer lists is deleted**, as `DELETE /api/ingestions/{id}` would (versions, leases and
state go; failure and metric rows are left to retention); it is not deactivated. Any unresolvable item fails the whole
apply. `dryRun=true` does the checks and reports the changes, writing nothing. An apply that changes something is recorded
in `ing_set_applies` (changes and counts in `summary`, `applied_by`), its items and files are archived in the artifact
store (the function artifact store, `file://` or `s3://`; the digest is in `archive_uri`), and the newest 10 applies are
kept. `POST .../applies/{applyId}/reapply` applies an archived apply's items again as a new apply (`goingBackTo`).
`POST .../release` ends ownership and changes nothing else. A set's ingestions are read-only (update, save version,
make live, delete); pause and resume still work. Applying and releasing need `platform:ingestion:set:apply`; listing sets
and applies needs `view` and a caller that reaches all clients and applications.

**SDKs.** The TypeScript, Java, Go, Python, Rust and Laravel SDKs get an `ingestions` resource (CRUD, versions,
definition, sets, source kinds, test) the same way every other resource arrives: from the lock, with a conformance
case per operation. The runtime operations are **not** in the application SDKs (they are the runtime's own client, in
the runtime repository). **CLI.** `fcdev ingest list|get|apply|export|test|pause|resume`, using fcdev profiles; `apply`
takes a folder of YAML as a set (the `ingest-cli` behaviour, including `--dry-run`), `export` writes the file form.
`ingest-cli` is retired when `fcdev ingest` has parity.

## 6. Feature flag

`FC_INGESTION_ENABLED` (default `false`; `fcdev` default `true`). Off: the ingestion routes are not mounted (`404`),
the navigation entry is absent (the public platform config the SPA reads carries `features.ingestion`), the roles and
permissions are still seeded (harmless), and the tables exist but stay empty. The runtime API is mounted only when
the flag is on.

## 7. The runtime contract

The JSON of `ingest-contract` carries over with the changes in section 2, under `/api/ingestion-runtime/v1`, authenticated
as the runtime's service account (`platform:ingestion:runtime:operate`).

- **Poll** (every minute): the runtime sends `{runtimeId, contractVersion, kinds, url?}`; the answer is the ingestions it
  should run (live version only), the lease length, and pending test requests. Leased sources (`sftp`, `s3`,
  `database-poll`, `api-poll`, `custom`) are given to one runtime holding a lease in `ing_leases` (a runtime that cannot
  renew stops before the lease ends); `http`, `sqs` and `rabbitmq` go to every runtime that carries them. An ingestion
  of a kind no live runtime reports shows as **"no runtime carries this kind"**, not as a failure.
  **Fair share (built 2026-10-24, ported from the Java control plane):** the leased ingestions spread evenly over the live
  runtimes (those that polled within one lease window, counting the asker): a runtime may hold ceil(leased / live). It renews
  what it already holds up to its share, claims unleased ones to fill it, and stops listing the surplus, marking those
  leases released. A lease is never handed over directly: the surplus is claimed by another runtime only after it has expired,
  by which time the first runtime has stopped it. A runtime that goes silent drops out of the count after a window and the
  survivors take its share. The share counts every leased ingestion whichever runtime can run it, so a runtime that
  carries fewer kinds simply holds what it can.
- **The runtime's account.** A runtime holds the role `platform:ingestion-runtime`, and that role is **platform-wide**
  (owner 2026-10-24: one fleet serves every application's ingestions, no fleet per application): the runtime routes
  (poll, report, state, jobs, secrets) reach every client and every application whatever way the account was provisioned,
  and the permission is the gate. The fair share spreads the ingestions over the instances of the fleet.
- **Report:** route states, exchange windows, failures (with the lease checked), idempotent per window.
- **State:** `GET/PUT /state/{ingestionId}/{key}`; a write from a runtime without the lease is `409 LEASE_NOT_HELD`.
- **Test connection** is pull-based: the pages queue a request (`ing_test_requests`); a runtime that carries the kind
  receives it in its next poll, runs it without dispatching anything, and posts the result. This replaces the control
  plane calling the runtime's address, which does not work from a platform behind a firewall.
- **Jobs:** `POST /api/ingestion-runtime/v1/ingestions/{id}/jobs?runtimeId=` takes `{records: [{payload, payloadContentType?,
  messageGroup?, externalId?, correlationId?, subject?}]}`, at most 1000 per request, and answers
  `{created, results: [{index, id} | {index, error}]}`. A runtime does **not** use the batch dispatch-job API: that takes ids,
  has no connection field, and an application-confined service account cannot create jobs for another application's
  ingestion. The platform builds every job from the **ingestion's own target**, resolved as at save: the target's `code` and
  `targetUrl`, the pool by code, the connection by code (the job carries `connection_id`, so pausing the connection holds it),
  the signer (the connection's service account, or the target's explicit `serviceAccount`, or none), the `mode`, the
  ingestion's client, source `ingestion:<application>:<code>`, and metadata naming the ingestion and the runtime. A record
  carries only the record: a field naming any of those is refused, so the runtime can neither choose nor alter them.
  Job creation is the batch API's own (`sdk.NewJobFromItem`), so a job persists identically whichever way it was made. Only a
  **live, active** ingestion takes jobs (`404` unknown, `409 INGESTION_NOT_LIVE` paused or no live version, `409
  INGESTION_TARGET_INVALID` when the target no longer resolves); the runtime must have polled and carry the ingestion's kind
  (`409 RUNTIME_KIND_NOT_CARRIED`) and, for a polled source, hold its lease (`409 LEASE_NOT_HELD`). A record that cannot be a
  job (no payload, a field over its length) is answered with its `error`; the rest are made. An `externalId` is the
  record's idempotency key **within the ingestion**: sending it again answers the job it made first (`existing: true`), also
  within one request. Keys live in `ing_job_keys`, written in the transaction that creates the job and pruned after 7 days.
- **Secrets:** `GET /api/ingestion-runtime/v1/ingestions/{id}/secrets/{name}?runtimeId=` answers `{value, updatedAt}` for a
  secret the ingestion's **live definition names** (found by walking the typed source model: source credentials, http auth
  tokens, api-poll auth, database and queue passwords; for an extension kind, its text settings), read from the
  ingestion's application, section `secrets`, at the ingestion's client with no global fallback. A name the definition does
  not reference, or that has no value, is `404 SECRET_NOT_FOUND`. The same kind, lease and liveness checks as the jobs call
  apply. It is the audited reveal of platform config (`platformconfig.Service`): refused if the audit row cannot be written,
  `Cache-Control: no-store`, and the audit row names the runtime's principal, the ingestion and the runtime id. The runtime's
  service account holds no config-reveal permission and needs none.
- **Versioning:** the contract is a public API. Poll carries `contractVersion`; the platform serves the current version
  and the one before (N-1) and answers `426` with the supported range otherwise. Additive changes do not bump it.

## 8. Cutover for Inhance

1. Create the integration application(s) and the runtime's service account with the `platform:ingestion-runtime` role.
2. Export every ingestion (`ingest-cli export`) and apply the files as a set against the platform. Ids change.
3. **Copy `ingestion_state`** (watermarks and cursors) into `ing_state` keyed by `(client, code)`, **before** the runtime
   is pointed at the platform, or every polled source reads from its beginning and dispatches duplicates. A one-off script
   in the runtime repository does this and is idempotent.
4. Point the runtime at the platform (base URL and token); the contract is the same apart from section 2.
5. Retire the old control plane after one clean day.

## 9. Pages

Stack-neutral here. The current pages (`apps/ingest-control-plane/web`, about 7,400 lines, Nuxt UI on Vue 3 and Vite) are the
specification: list with filters, a form drawn from the source kinds, drawer tabs (activity, failures, history,
versions, definition), "Export YAML", test connection, sets and their applies, pause and resume, and read-only state for
ingestions a set owns. **Which UI stack the platform uses is decided separately** (`flowcatalyst-framework/docs/go/ui.md` section 16 and the
owner's 2026-10-24 question about moving to Nuxt UI); the work package for the pages waits on it.

## 10. The runtime repository: consumed, not embedded (separate document to follow)

A business should **consume** the runtime and keep its own extensions outside it; it should not carry the runtime's
source inside its mono-repo. Three tiers, from least to most involved:

| Tier | The business has | Upgrade |
|---|---|---|
| A. Image plus extensions directory | A Dockerfile `FROM` the released runtime image and a folder of extension jars copied to `INGEST_EXTENSIONS_DIR`, loaded at start | change the image tag |
| B. Maven assembly | A small module in its own mono-repo (`apps/ingest-runtime`) that depends on the released `ingest-runtime` artifact and on its own extension modules (which live anywhere in its repository), and builds its image | change one version |
| C. Source | A fork, or the runtime repository pulled in with `git subtree` (preferred over a submodule: no second checkout step in CI), only for a business that changes the runtime itself | `git subtree pull` |

The runtime repository is split from the monorepo by CI like the function SDKs, tagged, and publishes the image and the
artifacts (`ingest-contract`, `ingest-extension`, `ingest-runtime`) to the registry the framework already publishes to.
Compatibility is checked, not assumed: the runtime refuses an extension built against an incompatible extension API
major version, and the platform serves the current and previous runtime contract (section 7). `fcdev` installs the
runtime for local work (image or jar) and scaffolds an extension project. A business keeps its CI test of its own
extensions against each new runtime release (a Renovate or Dependabot bump of the image tag or version).

## 11. Work packages (each sized for one agent, with tests and a mutation check)

| WP | Scope | Depends on |
|---|---|---|
| 1 | Migrations, entities, repositories, definition model and validation (source, split, target resolution) | |
| 2 | Operations and API: CRUD, versions, live, definition, history, source kinds; permissions and seeded roles; OpenAPI lock | 1 |
| 3 | Runtime API: poll, leases, state, report, test requests, contract versioning; pruning jobs | 1, 2 |
| 4 | Sets: apply, reapply, release, archive store; the definition file format | 2 |
| 5 | Feature flag and public config; SDK resources (six SDKs) and conformance cases | 2, 4 |
| 6 | `fcdev ingest ...` | 4, 5 |
| 7 | Pages | 2, 4 and the UI decision |
| 8 | Runtime repository split, contract artifacts, extension directory | 3 |
| 9 | Cutover tooling (state copy) and the Java platform hand-off | 3 |

## 12. Rulings

Decided 2026-10-24 (owner):

1. **Ownership:** an application (type `INTEGRATION`) **and** a client, both required.
2. **Target:** `{code, targetUrl, pool, connection?, serviceAccount?, mode, batchSize}` as in section 2.
3. **Test connection:** pull-based (section 7).
4. **Contract:** current and previous version (N-1), section 7.
5. **`camel-route`:** kept as a runtime-reported kind needing its own permission (section 2).

Assumed unless the owner objects: the resource is "ingestion", the permission subdomain `ingestion`, the flag
`FC_INGESTION_ENABLED`.
