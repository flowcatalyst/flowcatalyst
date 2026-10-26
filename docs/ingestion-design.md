# Ingestion in the platform: design

Status: built, 2026-10-25. Sections 2 to 7 are built in the Go platform (`internal/platform/ingestion`: model,
validation, tables 088 to 090, operations and API, sets, the runtime API, retention), `fcdev ingest` and the pages
(server-rendered templ under `/ui/ingestions`, section 9). The hardening rulings of 2026-10-25 and the three later
rulings of that day (drafts, route authors, secret fetches; section 12) are in.
Not built: the cutover tooling (section 8) and the Java platform mirror. Source of the behaviour described here: the
ingest control plane in `inhance/mono-repo` (`apps/ingest-control-plane`, `libs/java/ingest-contract`, `docs/ingest`),
read on 2026-10-24. Owner direction (2026-10-24): ingestion is a platform capability for "building business event
driven business services and applications", not a feature of one organisation's mono-repo.

## 1. What it is and what moves

An **ingestion** takes data from wherever a system keeps it (HTTP push, SFTP, S3, SQS, RabbitMQ, a polled table or
API, or a source an extension adds), splits each payload into records, and turns each record into a **dispatch job**.
It is the inbound twin of subscriptions.

| Piece | Where it lives after this work |
|---|---|
| Definitions, versions, sets, leases, state, failure and metric records, operator API, permissions, audit | **The Go platform** (this document) |
| Operator pages | The platform's server-rendered pages (templ and Datastar under `/ui/ingestions`, section 9) |
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
  connection" and when it takes the lease, reporting `FAILING` with the reason. **The route permission counts only for
  an anchor-tier caller** (ruling R2): below anchor tier a `camel-route` save is `403 INGESTION_ROUTE_ANCHOR_REQUIRED`
  whatever it holds. **Routes are written by a pipeline or an ingestion administrator** (ruling R9): the permission
  counts only for a `SERVICE` principal (a pipeline's account holding the route-author role) or for a caller who also
  holds `platform:ingestion:ingestion:delete` (the administrator); a user holding only the route permission is `403
  INGESTION_ROUTE_PIPELINE_REQUIRED`. Role assignment has no hook to keep a role off users, so the rule is enforced at
  use (`ingestion.RequireKindPermission`). Operators hold no route and cannot write routes. The permission a runtime declares for any of its kinds is **remembered** (`ing_kind_requirements`,
  the last report wins) and enforced while no runtime that reports the kind is live (ruling R4f); `camel-route`'s is
  built in.
- **Where a source points is guarded** (ruling R4b). Every outbound address of a source (an `api-poll` URL, its
  login and token URLs, an `sftp` host, a RabbitMQ URI, a database's JDBC URL, an absolute URL in an extension's
  settings) passes an outbound guard at save (in `Definition.Validate`, so every save path and the test of an unsaved
  definition) and when a saved ingestion is tested. It is `netguard`'s address check with private networks allowed (an
  on-premises source is the normal case): loopback and `localhost`, link-local (169.254.0.0/16, fe80::/10, so the
  metadata services), unspecified and multicast addresses, the metadata addresses outside link-local (`fd00:ec2::254`,
  `100.100.100.200`), the metadata host names (`metadata.google.internal`, `metadata`, `instance-data`...) and hosts
  written as bare numbers (`2130706433`, `0x7f.1`) are refused. Host names are not resolved at save (the runtime
  checks the address it connects to). A JDBC URL must name its server (`//host:port`, several hosts, `[v6]`, SQL
  Server `\instance` and `serverName=`, MySQL `address=(host=)`, Oracle `@host:port`, `@//host` and `(HOST=)`); one
  with no host to read (an embedded database, a file, an Oracle TNS alias) is refused, and so are driver properties
  that load classes or touch files on the runtime (`socketFactory`, `sslfactory`, `allowLoadLocalInfile`,
  `autoDeserialize`, the `*Interceptors`, `loggerFile`, H2's `INIT` and the like).
- **The runtime's own identity and named secrets are read paths** and have their own permissions (rulings R4a and
  R4d, `Repository.CheckAuthor`): an `sqs` or `s3` source **without a credential** is read with the runtime fleet's
  own AWS identity, so saving one that is new or whose source differs from the live version needs
  `platform:ingestion:ingestion:runtime-identity`; and a definition that **names secrets** makes the runtime present
  them to the host it names, so saving one that is new, or that differs from the live version in an outbound address or
  in the set of secret names, needs `platform:ingestion:ingestion:bind-secrets`. For an extension kind every text setting
  counts as a secret name, so any change of its settings counts. Both are judged against the live version (a draft
  saved over nothing live is judged as a create); a change that touches neither (a directory, a name, the target)
  needs neither. Both hold for create, update, save version, set apply (per item) and the test of an unsaved
  definition.
- **An sftp source checks the server's host key** (2026-10-25, with the runtime): `knownHosts` (the server's
  known_hosts line) or `acceptAnyHostKey: true` (default false; not both). The runtime refuses to start an sftp route
  with neither and reports it `FAILED`; the platform still saves one (a definition from before saves unchanged) and
  answers it with a `warnings` entry, shown in the drawer. Accepting any key sends the credential to whoever answers at
  the address, so turning it on (new, or off in the live version) needs `bind-secrets`, as pointing a secret at a new
  host does.
- **The target says where the job goes, and what signs it, in platform terms.** A dispatch job carries its own delivery URL, pool, client and signing service account; a connection has **no URL** (it holds a service account that signs, and pausing it holds its jobs). So the target is
  `{code, targetUrl, pool, connection?, serviceAccount?, mode, batchSize}`:
  - `targetUrl`: the delivery URL, checked at save with the platform's outbound-URL guard (no private or metadata addresses);
  - `pool`: a dispatch pool **code**, resolved and checked at save (it must exist and be usable by the ingestion's client);
  - `connection`: an **optional** connection code. When set, the connection's service account signs each delivery and pausing the connection holds this ingestion's jobs, as for subscriptions;
  - `serviceAccount`: an **optional** service account code, for signing without a connection; it may not be set together with `connection` (the connection's account is the signer). It must be **one of the ingestion's application's own accounts** (`applicationId` is the ingestion's application) and reach the ingestion's client; one linked to no client reaches all of its application's clients, which is acceptable only because it is bound to this application. A platform account (bound to no application) or another application's is refused (ruling R4c);
  - neither set: unsigned delivery;
  - `code`: the job's code, whose first segment must be the **ingestion's own application** (matched without regard to case), the batch dispatch API's `checkJobApplication` rule 1 (ruling R4e);
  - `mode` (`IMMEDIATE`, `NEXT_ON_ERROR`, `BLOCK_ON_ERROR`) and `batchSize` as today.
  The target is resolved again when a runtime creates jobs, so an ingestion saved before these rules is `409 INGESTION_TARGET_INVALID` there.
  The job's **client is the ingestion's client**; it is not a target field. Anything that does not resolve is a `400` at save, not a failure in the runtime later. A runtime creates the jobs through its own runtime call (section 7), which builds them from this target.
- Ids are platform TSIDs (`ing_...`), not ULIDs.

## 3. Tables (Go migrations, from 088)

All tables are created whether or not the feature is enabled (section 6). Prefix `ing_`. Money-for-nothing rule: no
table holds a secret value.

| Table | Replaces | Notes |
|---|---|---|
| `ing_ingestions` | `ingestions` | `id`, `application_id` (FK), `client_id` (**not null**), `code`, `name`, `active`, `version`, `latest_version`, `source/split/target` JSONB, `keep_failed_payloads`, `managed_by` (the owning set), timestamps. Unique `(application_id, client_id, code)` |
| `ing_versions` | `ingestion_versions` | one row per saved version: definition, `definition_digest` (093: sha256 hex of the definition's canonical form, computed by the database from the jsonb text, indexed with the ingestion id; a save that matches a version reuses it), `note`, `applied_from` (set apply), who and when. `live` is the version runtimes receive (see section 5) |
| `ing_runtimes` | `runtimes` | `runtime_id`, `last_seen_at`, `kinds` JSONB (extension kinds), `builtin_kinds` JSONB (090; NULL is the v2 baseline), `contract_version`, `url` (informational); pruned 7 days unseen |
| `ing_kind_requirements` | (new, 090) | per runtime-reported kind, the permission the last report declared; enforced while no runtime that reports the kind is live |
| `ing_leases` | `ingestion_leases` | polled sources run on one runtime at a time; `expires_at`, `released`; timed by the database's clock |
| `ing_job_keys` | (new) | the idempotency key (`externalId`) of each job a runtime created, per ingestion; written with the job, **pruned** after 7 days |
| `ing_state` | `ingestion_state` | small values a polled source keeps (watermark, cursor); only the lease holder writes |
| `ing_route_states` | `ingestion_route_states` | per runtime: version, status, error; a runtime's rows are pruned once it is unseen for 7 days (or forgotten) |
| `ing_secret_fetches` | (new, 093) | per (ingestion, secret name, runtime): `last_fetched_at` (database clock), `fetch_count`, the live `version` and the `principal_id` of the last fetch; upserted on each successful secret fetch, pruned with a runtime's route states, cascade-deleted with the ingestion. No value |
| `ing_exchange_metrics` | `ingestion_exchange_metrics` | per window: completed, failed, records, jobs, `created_at` (090, server-written); **pruned** after 30 days |
| `ing_failures` | `ingestion_failures` | kind, `category` (090: `payload` or `rejected`), message, `after_records`, optional `excerpt` (only with `keepFailedPayloads`), `dismissed_at`/`dismissed_by` (090); pruned after 30 days |

**Retention** (`retention.go`, hourly on every replica, idempotent) prunes failures, metrics, job keys and test
requests by `created_at`, the time the platform wrote the row, never by a time the runtime sent (ruling R5). Each
high-volume table gets up to `FC_INGESTION_RETENTION_MAX_CHUNKS` (default 500) chunks of 10,000 rows a sweep, 5 million
rows a table an hour: steady state for about 1,400 records a second carrying an `externalId`, where the first budget
(100,000 rows an hour) fell behind at 28 a second and never caught up. A runtime-supplied time more than five minutes
in the future (a failure's `at`, a window's `from`) is recorded as the platform's now.
| `ing_sets`, `ing_set_applies` | `ingestion_set_applies` | named sets of definition files and their apply history; the archive of applied files goes in the platform's artifact store (`file://` or `s3://`, as function artifacts) |
| `ing_test_requests` | (new) | pending "test connection" requests and their results (section 7) |

**Audit.** `ingestion_audit` is not carried over: changes go through the use-case envelope and so into the platform
audit log (`aud_logs`), which already records who, what, and when; the History tab reads it.

## 4. Permissions and roles

Subdomain `ingestion` (four segments, like every platform permission):

| Permission | Grants |
|---|---|
| `platform:ingestion:ingestion:view` | list, read, versions, history, failures, definition export, source kinds, the secrets an ingestion binds and their fetches (`GET /api/ingestions/{id}/secrets`) |
| `platform:ingestion:ingestion:create` / `update` / `delete` | change definitions; `update` includes pause and resume |
| `platform:ingestion:ingestion:promote` | make a version live (section 5): a create that is live, an update that is live, a version saved live, make live. Without it a create or update is saved as a draft |
| `platform:ingestion:ingestion:test` | run "test connection" (it makes a real outbound call from a runtime, so it is its own grant) |
| `platform:ingestion:ingestion:route` | create or change an ingestion of a kind that declares it needs this (`camel-route`), only at anchor tier and only for a `SERVICE` principal or an ingestion administrator; removing one needs only `delete` |
| `platform:ingestion:ingestion:runtime-identity` | save an `sqs` or `s3` source without a credential (the fleet's own AWS identity reads it), section 2 |
| `platform:ingestion:ingestion:bind-secrets` | save a definition that names secrets and points them somewhere new, section 2 |
| `platform:ingestion:set:apply` | apply or release a set |
| `platform:ingestion:runtime:operate` | the runtime API: poll, report, state, jobs, secrets |

Seeded roles: `platform:ingestion-admin` (all but the runtime's, route included, so an administrator pushes routes
with `fcdev ingest apply`; the only role with `runtime-identity`), `platform:ingestion-operator` (view, update, test;
no route), `platform:ingestion-viewer`, `platform:ingestion-route-author`, **the pipeline role** (view, create, update,
route, bind-secrets: held by a pipeline's service account, which pushes routes as drafts an administrator makes live;
`bind-secrets` because every text setting of a route counts as a secret name; given to a user it writes no route,
section 2) and `platform:ingestion-runtime` for a runtime's service account. Super-admin's wildcard covers everything. The role
ceiling rules (a caller cannot grant what it does not hold) apply as elsewhere.

**Promote and drafts** (rulings R3 and R8). Making a version live needs `promote` whichever call does it, but a save
without it is **a draft, never a refusal** (R8 replaced R3's refusal): a create without promote makes an ingestion
whose first version is not live, whatever `live` says (the API passes `operations.LiveIfPermitted`, so the audited
command records `live: false`); `PUT /api/ingestions/{id}` without promote saves a version that is not live. A version
saved after the live one (or while none is live) and not live is a **draft**. Every save is **deduplicated by
digest** (`ing_versions.definition_digest`, the sha256 of Postgres's jsonb text of the definition: keys sorted,
duplicates dropped, no insignificant whitespace; the database computes it at insert, at lookup and in the 093
backfill, so there is one canonical form):

| The definition | Without promote | With promote |
|---|---|---|
| is the live version's | nothing written (`UNCHANGED`) | the same |
| is a saved version's that is not live | nothing written, that draft answered (`DRAFT_EXISTS`) | that version made live, nothing saved (`MADE_LIVE`, audited as `MakeVersionLiveCommand`) |
| is new | saved as a draft (`DRAFT`, audited as `SaveVersionCommand` with `live: false`) | saved and made live (`SAVED`) |

`POST .../versions` follows the same table with `live` deciding (live needs promote, else `403`). A create or update
answer carries `save: {outcome, version, awaitingPromote, message}`; a version carries `draft` and `digest`. The pages
offer the "make live" switch only to a holder of promote; without it the forms say "Saved as a draft; someone with
promote makes it live", the Versions tab badges drafts, and the History tab reads drafts as drafts. A set apply makes
its versions live under `set:apply`, which only the admin role holds, unchanged (not deduplicated against drafts).

**The runtime's account** (ruling R1). The runtime API (`auth.RequireIngestionRuntime`) needs
`platform:ingestion:runtime:operate` **and** a principal that is a runtime's: a service account
(`INGESTION_RUNTIME_SERVICE_ACCOUNT_REQUIRED` otherwise), at anchor tier (`INGESTION_RUNTIME_ANCHOR_REQUIRED`: a
client-tier token is forged or mis-provisioned) and bound to no application (`INGESTION_RUNTIME_APPLICATION_BOUND`: an
application's own account, or one confined to a list of applications, is not a runtime's), all `403` on every runtime
route. An account with access to every application is not bound to one. The iac runbook's account (made without
`applicationId`, linked to no client) and fcdev's `fcdev-ingestor` (an anchor `SERVICE` principal with every
application) both pass.

**Reach.** A caller below anchor tier sees and changes only ingestions of its own clients. An application-scoped service
account reaches only its own application's ingestions (the existing confinement). Per-application and per-client scoping
is therefore the same axes as the rest of the platform.

**Secrets.** An ingestion names secrets (`sftp-password`). A runtime reads them through the runtime API's secrets call
(section 7), not through the config reveal: the call answers only the secrets the ingestion's live definition binds, at
the ingestion's client, only to a runtime that carries the kind and (for a polled source) holds the lease. It is security
telemetry, not an audited reveal (ruling R10, section 7). The runtime's service account therefore holds no
`platform:application-service:config:reveal`, and `platform:ingestion-runtime` gains nothing beyond
`platform:ingestion:runtime:operate`: the jobs and secrets calls are authorised by it plus those checks. The platform may
warn at save when a named secret has no value (a masked config listing is enough to know).

## 5. API (OpenAPI, then SDKs)

Everything below is huma, in `api/openapi.lock.json`, so the SDKs and the SPA types regenerate from it.

| Operation id | Route | Notes |
|---|---|---|
| `listIngestions` | `GET /api/ingestions` | filters: `applicationCode`, `clientId` (id or identifier), `q`, `status`, `kind`; cursor paging (`limit` 1 to 200) |
| `createIngestion`, `getIngestion`, `updateIngestion`, `deleteIngestion` | `/api/ingestions[/{id}]` | `updateIngestion` takes `expectedVersion`; a no-op update answers 200 and writes nothing (platform rule). Create takes `live: true` default. Without promote both save a draft; both are deduplicated by digest and answer `save` (section 4). A response carries `notices`, `latestNotice` and `latestNoticeAt`: the undismissed `rejected` failures |
| `listIngestionVersions`, `saveIngestionVersion`, `makeIngestionVersionLive` | `/api/ingestions/{id}/versions[/{v}/live]` | a saved version is not run until it is live; `live` moves the pointer |
| `getIngestionDefinition` | `GET /api/ingestions/{id}/definition` | the file form (YAML) that `fcdev ingest apply` reads back |
| `listIngestionSecrets` | `GET /api/ingestions/{id}/secrets` | `view` and the ingestion's reach: each secret name the live definition binds (and names fetched under an earlier live version, `bound: false`), with per runtime `lastFetchedAt`, `fetchCount`, `version`, `principalId`. Never a value |
| `listIngestionHistory`, `listIngestionFailures` | `/api/ingestions/{id}/history\|failures` | history from `aud_logs`; failures paged by id, filtered by `category` and `undismissed`, each with `category`, `createdAt`, `dismissedAt`, `dismissedBy` |
| `dismissIngestionFailure` | `POST /api/ingestions/{id}/failures/{failureId}/dismiss` | `update` and the ingestion's reach; once (again answers it as it is and writes nothing); emits `platform:ingestion:ingestion:failure-dismissed` on the ingestion's subject, so it reads in its history. Any failure can be dismissed; a `rejected` one is a notice the list and the drawer show until it is |
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
as the runtime's service account (`platform:ingestion:runtime:operate`, and an anchor-tier service account bound to no
application: section 4).

**Additions within contract version 2 (2026-10-25).** All additive: the version stays 2, and a runtime that sends none
of them (N-1, or a v2 runtime from before) is served as before.

| | Field | Meaning |
|---|---|---|
| C1 | poll `builtinKinds: [string]` | The built-in kinds the runtime carries. Absent: the v2 baseline (`http`, `sftp`, `s3`, `sqs`, `rabbitmq`, `database-poll`, `api-poll`), or, as before, exactly the built-in kinds it names among `kinds`. A runtime is listed only ingestions of a kind it carries (`builtinKinds`, its extension kinds, `camel-route` when it reports it), and the notice and the per-kind fair share use the same set. A name the platform does not know is ignored |
| C2 | (behaviour) | Fair share per kind; the lease section serialised; leases timed by the database's clock (below) |
| C3 | report failure `category` | `payload` (the default: a record or payload it could not take) or `rejected` (the platform refused its jobs request permanently: a `409 INGESTION_TARGET_INVALID`, `CLIENT_NOT_ACTIVE` and the like). The Java runtime sends it, lower case, on `REJECTED`-kind failures only; a failure without one is `payload`. Stored; a `rejected` failure is a notice until someone dismisses it (section 5) |
| C4 | (limits) | The jobs call takes bodies up to 10 MiB (`FC_INGESTION_JOBS_MAX_BYTES`), the report up to 4 MiB (`FC_INGESTION_REPORT_MAX_BYTES`), the rest huma's 1 MiB. NUL characters in report text (failure messages and excerpts, route errors, test steps and excerpts) are dropped before storing: Postgres refused the whole report over one |
| C5 | jobs `externalId` | The runtime sends the sha256 hex (64 characters, within the 100 allowed) of each record's source identity when it has one. Of two records of one request with the same key the earlier makes the job and the later answers it, every time (the keys are claimed in a stable order) |

- **Poll** (every minute): the runtime sends `{runtimeId, contractVersion, kinds, builtinKinds?, url?}`; the answer is the
  ingestions it should run (live version only, of an **active** client: a suspended or deactivated client's run
  nowhere), the lease length, and pending test requests. Leased sources (`sftp`, `s3`, `database-poll`, `api-poll`,
  `custom`) are given to one runtime holding a lease in `ing_leases` (a runtime that cannot renew stops before the lease
  ends); `http`, `sqs` and `rabbitmq` go to every runtime that carries them. An ingestion of a kind no live runtime
  reports shows as **"no runtime carries this kind"**, not as a failure.
  **Fair share, per kind (C2, 2026-10-25):** for each kind, a runtime may hold ceil(polled ingestions of the kind / live
  runtimes that carry the kind), the live runtimes being those that polled within one lease window, the asker included.
  It renews what it already holds of the kind up to that share, claims unleased ones to fill it, and stops listing the
  surplus, marking those leases released. A lease is never handed over directly: the surplus is claimed by another
  runtime only after it has expired, by which time the first runtime has stopped it. A runtime that goes silent drops
  out of the count after a window and the survivors take its share. (The first version, ported from the Java control
  plane, counted every polled ingestion over every live runtime, so the only carrier of a kind in a mixed fleet was
  capped below what the kind needed and the rest never ran.) The lease section of a poll runs under
  `pg_advisory_xact_lock`: each poll renews its own leases and then claims in id order, and two polls interleaved
  locked rows in different orders and deadlocked. Leases are taken, renewed, checked and expired by the **database's
  clock** (`now()` read once per transaction), so replicas whose clocks disagree agree on who holds what; `asOf` in the
  answer is that time.
- **The runtime's account.** A runtime holds the role `platform:ingestion-runtime`, and that role is **platform-wide**
  (owner 2026-10-24: one fleet serves every application's ingestions, no fleet per application): the runtime routes
  (poll, report, state, jobs, secrets) reach every client and every application. Because of that the account must be
  a runtime's and nothing narrower (ruling R1, section 4): a client-tier or application-bound account is refused, never
  widened. The fair share spreads the ingestions over the instances of the fleet.
- **Report:** route states, exchange windows, failures (with the lease checked; `category`, C3), idempotent per window.
  Up to 4 MiB. Times more than five minutes ahead of the platform's are recorded as its now; rows are kept by the time
  the platform wrote them.
- **State:** `GET/PUT /state/{ingestionId}/{key}`; a write from a runtime without the lease is `409 LEASE_NOT_HELD`.
- **Test connection** is pull-based: the pages queue a request (`ing_test_requests`); a runtime that carries the kind
  receives it in its next poll, runs it without dispatching anything, and posts the result. This replaces the control
  plane calling the runtime's address, which does not work from a platform behind a firewall.
- **Jobs:** `POST /api/ingestion-runtime/v1/ingestions/{id}/jobs?runtimeId=` takes `{records: [{payload, payloadContentType?,
  messageGroup?, externalId?, correlationId?, subject?}]}`, at most 1000 per request and 10 MiB, and answers
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
  (`409 RUNTIME_KIND_NOT_CARRIED`) and, for a polled source, hold its lease (`409 LEASE_NOT_HELD`), checked **in the
  transaction that makes the jobs** with the lease row held (`FOR SHARE`) until it commits (ruling R6), so the lease
  cannot pass to another runtime in between; the client must be active (`409 CLIENT_NOT_ACTIVE`, checked there too).
  A record that cannot be a job (no payload, a field over its length) is answered with its `error`; the rest are
  made. An `externalId` is the record's idempotency key **within the ingestion**: sending it again answers the job it
  made first (`existing: true`), also within one request, where the earlier record wins (C5). Keys live in
  `ing_job_keys`, written in the transaction that creates the job and pruned after 7 days.
- **Secrets:** `GET /api/ingestion-runtime/v1/ingestions/{id}/secrets/{name}?runtimeId=` answers `{value, updatedAt}` for a
  secret the ingestion's **live definition names** (found by walking the typed source model: source credentials, http auth
  tokens, api-poll auth, database and queue passwords; for an extension kind, its text settings), read from the
  ingestion's application, section `secrets`, at the ingestion's client with no global fallback. A name the definition does
  not reference, or that has no value, is `404 SECRET_NOT_FOUND`. The same kind, lease, liveness and client-status checks
  as the jobs call apply. `Cache-Control: no-store`. The runtime's service account holds no config-reveal permission and
  needs none. **It is not a config reveal** (ruling R10): the value is read with
  `platformconfig.Service.FetchBoundSecretForIngestionRuntime` (one caller by convention: this call; no reveal
  permission, no audit row) and every successful fetch is **security telemetry**: an INFO log line
  `security.ingestion_secret_fetch` (fields `event`, `outcome`, `ingestionId`, `applicationCode`, `clientId`,
  `secretName`, `runtimeId`, `principalId`, `version`; never the value), the counter
  `fc_ingestion_secret_fetch_total{outcome=success|refused|error}` (`ingestion.MetricsRegistry`, on the metrics
  listener), and an upsert of `ing_secret_fetches` (shown in the drawer's Secrets section and by
  `listIngestionSecrets`). A **refused** fetch (a name the live definition does not bind, no value at the client, no
  lease, the kind not carried, a paused ingestion, an inactive client, a caller that is not a runtime) writes an
  `aud_logs` row `SECRET_FETCH_REFUSED` on the ingestion (`entity_type` `Ingestion`, so the history shows it; detail:
  secret name, runtime id, reason code) and the same log line at WARN with `reason`. The Java runtime re-reads each
  secret every `INGEST_SECRETS_CACHE_TTL` per non-HTTP route; that volume now lands in logs and one row per
  (ingestion, secret, runtime), not in `aud_logs`.
- **Versioning:** the contract is a public API. Poll carries `contractVersion`; the platform serves the current version
  and the one before (N-1) and answers `426` with the supported range otherwise. Additive changes do not bump it.

## 8. Cutover for Inhance

Not built yet (the state-copy script is work package 9). The order matters: two runtimes reading the same source, or a
runtime starting without the old watermark, dispatch the same records twice, and the platform cannot tell (below).

1. **Prepare, with nothing running against the platform.** Create the integration application(s), the client(s), the
   dispatch pools and connections the targets name, and the runtime's service account: anchor tier, bound to no
   application, holding `platform:ingestion-runtime` (section 4). Set every secret the definitions name at the
   ingestion's client (`secrets` section).
2. **Export and apply as drafts.** Export every ingestion (`ingest-cli export`), add the `application` each belongs to,
   rewrite each target's `code` to begin with that application (section 2) and its `serviceAccount` to one of that
   application's own, give every sftp source its server's `knownHosts` line (or, deliberately, `acceptAnyHostKey:
   true`: the new runtime refuses an sftp source with neither and reports it `FAILED`; the old one connected without
   checking), and apply the files as a set with every ingestion **paused** (`active: false`), dry run first.
   Ids change (`ing_...` TSIDs); nothing runs yet. The apply is the check that every reference resolves.
3. **Freeze the old side.** Stop the old runtime (every instance) and wait for its in-flight batches to finish, then put
   the old control plane in read-only mode (no edits, no new leases). From here until step 6 nothing ingests: keep the
   window short, and do it when the sources can wait (a polled source loses nothing; an HTTP push source returns errors
   its sender must retry; a queue keeps its messages).
4. **Copy the state** (watermarks and cursors) from `ingestion_state` into `ing_state`, mapping each row by
   **`(application, client, code)`** to the new ingestion id (the old key is `(client, code)`; the application is the one
   step 2 assigned). The script must be idempotent and refuse to run while an old runtime is polling or a new one holds
   a lease. Without this every polled source reads from its beginning and dispatches everything again.
5. **Idempotency keys do not carry over.** The old control plane's dedup (whatever it kept per record) is not migrated:
   `ing_job_keys` starts empty, and a key names a job in the new platform, which an old job is not. A record the old
   runtime already dispatched and the new one reads again is a **new job**. The only protection is order: the old
   runtime stopped in step 3, after its last batch, and the copied watermark tells the new one where that batch ended.
   Never run both runtimes at once against the same sources, not even briefly.
6. **Start.** Point the runtime at the platform (base URL and the account's credentials; the contract is the same apart
   from sections 2 and 7), let it poll once, then resume the ingestions (one set apply with `active: true`). Watch the
   Activity and Failures tabs; a `rejected` notice means a target does not resolve.
7. **Rollback** (keep the old control plane read-only, not deleted, for 14 days). To go back: pause the ingestions on the
   platform and wait until no lease is held, stop the new runtime, copy `ing_state` back into `ingestion_state` by the
   same `(application, client, code)` mapping (the same script, reversed: the platform's watermarks are newer), take the
   old control plane out of read-only and start the old runtime. Records dispatched by the new platform are not undone;
   the copied watermark stops them being read again.
8. **Retire** the old control plane once the 14 days pass with no rollback.

**What is not migrated:** versions and their notes (the platform's history starts at the apply; the old version list
stays readable in the old control plane while it is kept), the old audit log, route states, exchange counts, failures,
test results and leases. **The job `source` changes:** jobs the platform makes carry `ingestion:<application>:<code>`
(section 7) where the old ones carried the old control plane's; a dashboard, a dispatch-job filter or a consumer that
reads `source` must accept both until the old jobs age out.

## 9. Pages

Built: server-rendered templ pages with Datastar under `/ui/ingestions` and `/ui/ingestion-sets`
(`internal/ui/pages/ingestion*`), on the same use cases as the API. The pages of the old control plane
(`apps/ingest-control-plane/web`, about 7,400 lines, Nuxt UI on Vue 3 and Vite) were the specification: list with
filters, a form drawn from the source kinds, drawer tabs (activity, failures, history, versions, definition), "Export
YAML", test connection, sets and their applies, pause and resume, and read-only state for ingestions a set owns. Since
2026-10-25 the list rows and the drawer show undismissed notices (count and newest message), the Failures tab tags
notices, says who dismissed what and offers Dismiss to a holder of `update`, the create form offers "make live" only to a
holder of `promote`, a `camel-route` is offered only to a pipeline or an administrator at anchor tier (R9), the change form says a save without promote is a draft, the Versions tab badges drafts, and the drawer has a Secrets section (names, which runtime last fetched each and when; R10). The paragraph below is the history of the
decision. **Which UI stack the platform uses is decided separately** (`flowcatalyst-framework/docs/go/ui.md` section 16 and the
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

Decided 2026-10-25 (owner), with the runtime contract additions C1 to C5 (section 7):

- **R1, the runtime's identity:** the runtime API needs `runtime:operate` and an anchor-tier service account bound to
  no application; anything narrower is a `403` with a code that says which (section 4).
- **R2, route authors:** the route permission counts only at anchor tier (section 2).
- **R3, promote:** a save whose result is live needs promote: a create without it must say `live: false`, an update
  without it is refused (section 4); superseded by R8: it is now saved as a draft. The route-author role writes drafts.
- **R4, authoring reach:** (a) `runtime-identity` for an SQS or S3 source without a credential; (b) the outbound guard
  on source addresses, private networks allowed; (c) `target.serviceAccount` confined to the ingestion's application
  and client; (d) `bind-secrets` for a definition that names secrets and points them somewhere new; (e) the target's
  job code belongs to the ingestion's application; (f) a kind's declared permission is remembered and enforced while
  no runtime reports it (sections 2 and 3). `runtime-identity` and `bind-secrets` are seeded to the admin role only (R9 later gave `bind-secrets` to the route-author pipeline role).
- **R5, retention:** by the server-written `created_at`, a budget that converges (configurable), runtime times clamped,
  route states of gone runtimes pruned (section 3).
- **R6, lease checks and client status:** the jobs call holds the lease row in the transaction that makes the jobs; a
  suspended or deactivated client's ingestions are listed to no runtime and take no jobs (section 7).
- **R7:** this document and the published guide brought up to date, the cutover plan rewritten (section 8).

Choices made where a ruling left one (2026-10-25): (superseded by R8) an update without promote is refused rather than saved as a
draft (the call says it makes the version live; doing something else quietly would surprise), the create form
without promote saves a draft; `runtime-identity` and `bind-secrets` are judged against the **live** version, so
renaming or retargeting an ingestion that already reads with the fleet's identity or already sends its secrets to the
same host needs neither; a set apply checks both per item against what exists; the test of an unsaved definition
needs what creating it would; the JDBC check also refuses a URL with no host to read and driver properties that load
code or touch files; the job-code check ignores case (an application code may hold capitals, a job code may not);
`builtinKinds` names the platform does not know are dropped, not stored; turning on an sftp source's
`acceptAnyHostKey` needs `bind-secrets` (not anchor tier: it is a secret-steering switch, and the admin role holds it).

Decided 2026-10-25 (owner), later the same day:

- **R8, drafts:** an update (and a create) without promote becomes a draft instead of a refusal, and every save is
  deduplicated by the digest of the definition (section 4). Set apply is unchanged.
- **R9, route authors:** camel routes are written by a pipeline (a `SERVICE` principal holding the route-author role,
  which gains `bind-secrets`) or by an ingestion administrator (whose role gains `route`, for `fcdev ingest apply`);
  operators cannot write routes (section 2).
- **R10, secret fetches:** the runtime fetching its ingestion's bound secrets is not a config reveal and is not written
  to `aud_logs` per read; it is security telemetry (log line, counter, last-fetched record), and a refused fetch is
  audited (section 7). This closes the open item on the secret-reveal audit volume.

Choices made where these rulings left one: the digest is computed by the database from the jsonb text (one
canonicalisation shared by the backfill, the insert and the lookup) rather than in Go; `POST .../versions` is
deduplicated by the same table as the update; a create's explicit `live: true` without promote is a draft too; the
audit row of a deduplicated or draft update names what was written (`SaveVersionCommand`, `MakeVersionLiveCommand`), so
the history reads it as it happened; refusals are audited on the ingestion (not the config row, which may not exist);
a failure to write the last-fetched record or a refusal's audit row is logged and does not change the answer; the
secret fetch record is per runtime id and goes with the runtime's route states after 7 days unseen.

## 13. Per-environment settings in platform config (2026-10-26)

A definition file is the same in every environment (it lives in git and every code in it is shared across
environments); what differs per environment or per client (a vendor's login, an organisation id, the delivery URL)
lives in the platform's config of that environment, never in environment variables of whoever applies the file. The
`${env:NAME}` placeholders of `fcdev ingest apply` are removed.

### The reference

A source either carries its settings inline (as before) **or** names a config entry, never both:

```yaml
source:
  kind: mix                 # a flat extension kind, or a built-in (kind: api-poll, kind: sftp, ...)
  config: mix-positions     # or `config: true`: the ingestion's own code
```

In JSON it is `{kind, config}` for a built-in and `{kind: custom, type, config}` for an extension kind. The key
follows the code's rules. A source with `config` holds nothing else.

The entry is platform config **application** = the ingestion's application, **section** `ingest`, **property** = the
key, **scope** `CLIENT` = the ingestion's client, type `PLAIN`. Its value is a JSON object: the kind's settings in
the same shape as inline (a built-in kind's fields without `kind`; an extension kind's `settings`) plus an optional
`targetUrl`. Secrets stay where they are, in section `secrets` at the client; a secret setting in the value holds the
secret's **name**. The delivery URL is in the file's `target.targetUrl` **or** in the value's `targetUrl`, exactly one.
Several ingestions of the same application and client may name one key (one login for two feeds); a value must then
suit each of them.

### The effective definition

The platform resolves the entry into the **effective definition** wherever a definition is used to do something:

| Where | Uses |
|---|---|
| The poll (section 7) | the effective source and target. A runtime never sees `config` and needs no change: the record it is handed changes when the value does, and it rebuilds the route as for any change |
| The secrets call | the secret names of the effective live source |
| The jobs call | the effective target (its `targetUrl`) |
| Tests | a saved ingestion's effective definition; an unsaved file's entry is read when the test is queued |
| The drawer (where it points, its secrets) | the effective definition; the export keeps the reference |

An ingestion whose entry is **missing** or **invalid** is not listed to any runtime and takes no jobs; it shows as
needing configuration, with the reason, in the list and the drawer (a computed notice, like "no runtime carries this
kind"), and starts by itself once the entry is valid.

### Saving

- **A definition** that names an entry is saved even when the entry does not exist yet in this environment, with a
  `CONFIG_MISSING` warning (owner ruling), so a pipeline applying a set never depends on someone having entered the
  environment's values first. When the entry exists, the effective definition is validated and judged by
  `CheckAuthor` against the effective live one, as an inline definition is.
- **An entry** is written only through the ingestion (owner ruling): `PUT /api/ingestions/{id}/config` (and
  `DELETE`), the drawer's "Settings (this environment)" form drawn from the kind's schema, and `fcdev ingest config`.
  It needs the permission to update the ingestion (also for a set-managed one: the entry is the environment's part,
  not the set's) and passes, for **every** ingestion of that application and client naming the key, `Validate` of the
  effective definition, the kind's schema, and `CheckAuthor` with the effective definition under the old value as
  live and under the new value as next: the outbound guard, `bind-secrets` when it points secrets somewhere new,
  `runtime-identity`, and the kind's permission, exactly as a save. The generic config API and page refuse writes and
  deletes in section `ingest` (`400 CONFIG_SECTION_RESERVED`) and show its entries read-only with a link to the
  ingestion. A write is audited on the ingestion and emits `platform:ingestion:ingestion:config-set`; it is not a
  version of the ingestion.
- **The kind's schema.** A built-in kind's is its definition (strict decoding and `Validate`). An extension kind's is
  the field list a live runtime reports for it (type, required, choices, pattern, bounds, list counts), checked by the
  platform; with no live runtime reporting the kind the value is stored with a warning that it was not checked, and
  the runtime still checks it before it builds the route. The UI draws the form from the same schema.

### Fixed with this (gaps found on the way)

- Making a draft live runs `CheckAuthor` against the version live at that moment (it was judged only against the one
  live when the draft was saved).
- The UI's test of an unsaved definition runs `CheckAuthor` in full, and its test of a saved ingestion runs the
  outbound guard, as the API's tests do.

### Rulings (owner, 2026-10-26)

1. No environment variables: per-environment and per-client values are platform config, section `ingest`, per
   client, one JSON value per key whose schema is the kind's, reflected in the UI.
2. A definition naming a missing entry is saved with a warning and does not run until the entry exists.
3. An entry is edited only through its ingestion; the generic config surfaces show it read-only.
4. A source is inline or config, never both.
5. The key is explicit; `config: true` is the ingestion's code; ingestions of one application and client may share one.
