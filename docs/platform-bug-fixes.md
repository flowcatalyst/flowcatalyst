# Platform bug fixes (2026-10-10)

The platform bugs found while porting the SPA to `/ui` (`docs/ui/README.md`, "Platform bugs
found", 1 to 25), the dispatch pool/connection items recorded earlier, and the same flaws found
next to them while fixing. One entry per bug: what was wrong, the fix, the commit, the wire impact,
the tests, and what the Java and Rust platforms must port to match (both are paused; nothing was
ported). Owner rulings are marked **ruling**; choices made here because the owner left them open
are marked **chosen** (conservative and wire-compatible, see the end).

Tests named `*_pg_test.go` run with `-tags=integration` on the embedded Postgres of
`internal/testpg`. Every test listed failed before its fix (checked by running it against the
previous code, or, for a test of a new function, because the behaviour it pins did not exist).

## Permission gates: the permission for the operation, never any of three

**Ruling**: a write checks the permission for its operation, as dispatch pools do since `c59e246`.

| # | Module | Commit | Now |
|---|---|---|---|
| 8 | subscriptions | `3d536a7` | create `subscription:create`; update, pause, resume `subscription:update`; delete `subscription:delete` |
| 13 | applications | `6be78ee` | create `application:create`; update, activate, deactivate `application:update`; delete `application:delete` |
| 17 | scheduled jobs | `210e4b5` | create `scheduled-job:create`; update, pause, resume, archive `scheduled-job:update`; fire and delete unchanged |
| - | event types | `1fa33ef` | create `event-type:create`; update `event-type:update`; add schema version `event-type:update` (the SDK's application-service create/update grant kept); delete unchanged |
| - | processes | `1f83887` | create `process:create`; update, archive `process:update`; delete unchanged |
| - | roles | `e03e54a` | create `role:create`; update and granting/revoking a permission `role:update`; delete unchanged |
| - | service accounts | `b49b73a` | create `service-account:create`; update, deactivate `service-account:update`; delete unchanged (anchor scope on create/update/delete unchanged, owner decision #19) |
| - | principals (users) | `4c87621` | creating users (create, createUser, bulk import) `user:create`; every change to an existing user (edit, activate, deactivate, passwords, 2FA reset, roles, application access, someone else's developer credential) and the reset-approval queue (list, approve, deny) `user:update`; delete unchanged. `auth.RequireUserAdmin` now takes the operation's check |
| - | comments | `54dcaa0` | role and application use-case comments name the per-operation gates |

- **Wire impact**: no shape change. A caller holding only some of create/update/delete is now
  refused (403 `PERMISSION_REQUIRED`) for the operations it lacks. The seeded admin roles hold
  create, update and delete together, so their principals are unaffected. The SDK's event-type schema push keeps its
  application-service grant.
- **Chosen**: lifecycle operations (pause, resume, activate, deactivate, archive) need **update**
  where there is no dedicated check today. The seed defines `user:activate`, `user:deactivate`,
  `user:assign-roles`, `scheduled-job:pause`, `event-type:archive` and `process:archive`, but no
  check reads them; switching to them would lock out custom roles that hold update only. Left as a
  decision (below).
- **Not changed**: `POST /api/scheduled-jobs/instances/{id}/log` and `/complete` keep the any-of
  gate (they are the runner's reports, not job writes; see new findings).
- **Tests**: `gate_test.go` in each `*/api` package (subscription, application, scheduledjob,
  eventtype, process, role, serviceaccount, principal): every write with each single permission,
  refused unless it is the operation's; `auth_test.go`
  `TestRequireUserAdmin_ChecksTheOperationsPermission`.
- **Java/Rust mirror**: in each controller replace the "any of create/update/delete" check with the
  table above; for event-type add-schema admit `event-type:update` or the application-service
  `event-type:create`/`:update`, and confine to the caller's applications unless it holds the
  messaging `event-type:update`; give the user-admin check a per-operation permission argument.

## 1, 2. `/bff/event-types` reads: no authorization; multi-value facets read the first value

- **Fix** (`308589b`): `GET /bff/event-types`, `/bff/event-types/{id}` and the applications,
  subdomains and aggregates filter endpoints make `GET /api/event-types`'s checks: the read
  permission (`CanReadEventTypes`), client scoping, and for an application service account only its
  own applications' event types (`eventtype.VisibleTo`, `eventtype.CheckReadable`, new in
  `internal/platform/eventtype/visibility.go`). Repeated `application`/`subdomain`/`aggregate`
  parameters match any of their values (`eventtype.MatchAny`), in the list and the cascading
  options.
- **Wire impact**: BFF only (not in the OpenAPI lock). A caller without the event-type view is
  refused (403) where it got every event type; an application service account sees its own. Single
  values behave as before.
- **Tests**: `internal/platform/shared/bff/event_types_pg_test.go`
  `TestBffEventTypeReads_AuthorizedScopedAndMultiValued`.
- **Java/Rust mirror**: the BFF event-type read and filter routes: read permission, client scoping,
  application confinement; treat each facet parameter as a list (repeated keys), OR within a facet.

## 3. Add schema stored double-encoded; MIME and schema type ignored

- **Ruling**: store correctly, honour MIME and schema type.
- **Fix** (`7b5453a`): `AddSchemaCommand` gains optional `MimeType` and `SchemaType`. A JSON Schema
  sent as a JSON string holding a document (the SPA's textarea) is stored as the document; a JSON
  Schema string that is not a document is refused (`INVALID_SCHEMA`); XSD and PROTO are text,
  stored as a JSON string (sent otherwise: `INVALID_SCHEMA`); an unknown type is
  `INVALID_SCHEMA_TYPE`; MIME defaults by type (`application/schema+json`, `application/xml`,
  `application/x-protobuf`), longer than 100 characters is `INVALID_MIME_TYPE`. The BFF passes the
  SPA's fields. `/ui` add-schema gains the SPA's MIME Type and Schema Type fields.
- **Wire impact**: `POST /api/event-types/{id}/schemas` (and `/versions`) keeps its body (no new
  fields) but gets the same normalisation: a string holding a document is stored as the document; a
  string that is not JSON is now 400. The BFF body is unchanged (its fields are now honoured).
- **Tests**: `eventtype/operations/add_schema_storage_pg_test.go` (document, MIME, XSD text,
  refusals); `bff/event_types_pg_test.go` `TestBffAddSchema_StoresTheSPAsSchemaAsTheDocument` (the
  SPA's exact body); `ui/pages/event_types_test.go` `TestEventTypeAddSchema`.
- **Java/Rust mirror**: add the optional `mimeType`/`schemaType` to the add-schema command; the
  normalisation rules and error codes above; the BFF passes them.

## 4. Stored XSS in the SPA's schema viewer

- **Ruling**: escape in `highlightJson` with a test; fix the Go port if it has the flaw.
- **Fix** (`17b79ef`): `highlightJson` escapes `& < >` before tokenising (quotes stay: element
  content, never an attribute), as `internal/schemacodegen` already did; output for JSON without
  those characters is unchanged, so the 47 goldens stand. The Go port did not have the flaw
  (`TestHighlightHostile`); its comment is updated.
- **Wire impact**: none.
- **Tests**: `frontend/tests/schema-highlight.test.ts` (hostile keys, strings and text; entities;
  unchanged output for plain JSON; the four code highlighters).
- **Java/Rust mirror**: none (SPA code). Any other UI that highlights stored schemas must escape.

## 5. Archive/delete rules only in the SPA

- **Ruling**: move them into the use cases, business-rule errors with clear codes.
- **Fix** (`62b72e3`): `ArchiveEventType` refuses unless every version is DEPRECATED
  (`SCHEMAS_NOT_DEPRECATED`, "Deprecate every schema version first."); `DeleteEventType` refuses
  unless the type is ARCHIVED or every version is FINALISING (`DELETE_NOT_ALLOWED`, the drawer's
  message). Rules on the entity (`EventType.CanArchive`/`CanDelete`), used by the `/ui` drawer.
- **Wire impact**: 409 for the refused cases on BFF archive/delete, `/ui`, and **`DELETE
  /api/event-types/{id}`**, which the TS SDK calls as `archive()` (it hard-deletes; see new
  findings).
- **Tests**: `eventtype/operations/lifecycle_guards_pg_test.go`; SPA rules in
  `frontend/tests/event-type-rules.test.ts`.
- **Java/Rust mirror**: the same two checks and codes in archive and delete.

## 6. Sync reports unchanged types as updated

- **Ruling**: report unchanged as unchanged.
- **Fix** (`b7aaaa4`): `SyncEventTypes` leaves a listed type whose name, description and schema are
  unchanged alone: not saved, no `EventTypeUpdated`, not counted.
- **Wire impact**: `updated` in the BFF sync-platform answer and the SDK sync response counts real
  changes only; fewer events.
- **Tests**: `eventtype/operations/sync_unchanged_pg_test.go`.
- **Java/Rust mirror**: compare name, description (nil equals nil only) and the 1.0 schema by
  value before saving.

## 7. SPA row pencil opens an archived event type in edit mode

- **Fix** (`b7b9a06`): the pencil is disabled for archived types (with the reason) and the drawer
  ignores `?edit=true` for them; the drawer's rules move to `utils/event-type-rules.ts`.
- **Wire impact**: none. **Tests**: `frontend/tests/event-type-rules.test.ts`.
- **Java/Rust mirror**: none (SPA).

## 9. Subscription create drops `clientScoped` and `sequence`

- **Chosen**: accept both on create (additive), keep the update body as it was.
- **Fix** (`eb537a7`): `CreateSubscriptionRequest`/`CreateCommand` gain optional `clientScoped`
  (default false) and `sequence` (default 99; negative is `INVALID_SEQUENCE`). `/ui` create sends
  its Client Scoped switch and gains the SPA's Sequence field.
- **Wire impact**: additive in `api/openapi.lock.json` (two optional request fields);
  `frontend/src/api/generated` regenerated. The SDK specs (`clients/*/openapi`) were not refreshed
  (`make sdk-spec` when the SDKs are next released).
- **Tests**: `subscription/api/create_body_pg_test.go` (the SPA's body); `ui/pages/subscriptions_test.go`.
- **Java/Rust mirror**: the two optional create fields and the validation.

## 10. SPA Queue is free text; the API accepts two values

- **Chosen**: fix the SPA, not the API.
- **Fix** (`8b98b1f`): create and edit offer Default / High priority (`utils/subscription-queue.ts`);
  edit shows an unset queue as Default.
- **Wire impact**: none. **Tests**: `frontend/tests/subscription-queue.test.ts`.
- **Java/Rust mirror**: none.

## 11. `dispatchPoolCode` stored only by the SDK sync

- **Chosen**: store it on create and update (no new error for an unknown pool id).
- **Fix** (`e6d1dc0`): create and update read the pool's code by id
  (`subscription.Repository.DispatchPoolCode`) and store it; an id naming no pool stores none. The
  `/ui` fallback lookup stays for older rows.
- **Wire impact**: `dispatchPoolCode` is now present in responses for such subscriptions.
- **Tests**: `subscription/operations/pool_code_pg_test.go`.
- **Java/Rust mirror**: look up and store the pool code in create and update.

## 12. Blank `connectionId` on update stored ""

- **Chosen**: a blank value is "not provided" (the use case's documented set-if-provided rule:
  re-point, never clear); values are trimmed.
- **Fix** (`c3768c7`). **Wire impact**: `connectionId: ""` no longer stores an empty string.
- **Tests**: `subscription/operations/blank_connection_pg_test.go`.
- **Java/Rust mirror**: ignore a blank `connectionId` in update.

## 14. Deleting an active application allowed by the API

- **Ruling**: SPA-only guards move server-side.
- **Fix** (`61dbcb9`): `DeleteApplication` refuses an active application with 409
  `APPLICATION_ACTIVE` ("Deactivate the application first.", the `/ui` reason), before the
  reference check.
- **Wire impact**: `DELETE /api/applications/{id}` on an active application is 409 (it deleted).
- **Tests**: `application/operations/ops_pg_test.go` `TestDeleteApplication_RefusedWhileActive`
  (the reference tests deactivate first).
- **Java/Rust mirror**: the same check and code, before the reference count.

## 15. Client "Deactivate" deleted the client

- **Ruling**: deactivate, not delete; use the existing state.
- **Fix** (`ff65ec4`): `INACTIVE` already existed and nothing set it. New use case
  `DeactivateClient`: sets `INACTIVE` with the request's reason (row and configuration kept;
  `ActivateClient` reactivates) and emits `platform:admin:client:deactivated` (`clientId`,
  `reason`). `POST /api/clients/{id}/deactivate` runs it; `DELETE /api/clients/{id}` still deletes.
  `/ui` runs it with the SPA's reason ("Manual deactivation"), keeps the drawer open, and uses the
  SPA's texts again. Inactive clients are already treated as not active (client selection, filter
  options).
- **Wire impact**: same body and answer; the client is kept (it was removed) and the event is
  `...:deactivated` (was `...:deleted`).
- **Tests**: `client/api/deactivate_pg_test.go`; `ui/pages/clients_page_pg_test.go`.
- **Java/Rust mirror**: the new use case and event; route deactivate to it.

## 16. Client list paging ignored by the API

- **Chosen**: fix the SPA (the API returns every client and stays as is).
- **Fix** (`db4e776`): the client list loads all clients once and the table pages and searches them
  (100 rows; 50/100/250/500). Found here too: the user drawer's client loader paged until a page
  came back short, which never happens from 100 clients: an endless request loop; it now loads the
  list once. `clientsApi.list` no longer takes page parameters.
- **Wire impact**: none. **Tests**: `frontend/tests/clients-list.test.ts`.
- **Java/Rust mirror**: none.

## 18. Filter options not tenant-scoped

- **Ruling**: scope to the caller's clients; same check for other endpoints with the flaw.
- **Fix** (`563102a`): `DistinctValues` (dispatch jobs and events) takes the list's tenant scope
  (platform rows plus the caller's clients; nil for an anchor); `GET
  /api/dispatch-jobs/filter-options` and `GET /api/events/filter-options` (same flaw) pass it.
  The `/ui` facet cache is kept per scope. Checked and fine: `/bff/filter-options/clients`,
  `/bff/scheduled-jobs/filter-options`; the audit facets follow an unscoped admin list.
- **Wire impact**: a client user sees fewer values (its own); anchors unchanged.
- **Tests**: `dispatchjob/api/filter_options_pg_test.go`, `event/api/filter_options_pg_test.go`;
  `ui/pages/dispatch_jobs_test.go`, `events_test.go` (the scope passed).
- **Java/Rust mirror**: add `(client_id IS NULL OR client_id = ANY(:clients))` for non-anchors to
  the distinct queries of both filter-options endpoints.

## 19. SPA dispatch-job application/subdomain/aggregate filters never have options

- **Chosen**: derive them in the SPA from the codes (no API change).
- **Fix** (`069a65d`): `utils/code-facets.ts`, each level narrowed by the levels above (as `/ui`).
- **Wire impact**: none. **Tests**: `frontend/tests/code-facets.test.ts`.
- **Java/Rust mirror**: none.

## 20. SPA shows Sign to every viewer

- **Fix** (`6116eee`): shown only with `dispatch-job:view-raw`. **Wire impact**: none.
- **Tests**: `frontend/tests/dispatch-job-sign.test.ts`. **Java/Rust mirror**: none.

## 21. `size` above 1,000 fell back to 100

- **Ruling**: clamp to 1,000.
- **Fix** (`858f02c`): dispatch jobs and events `FindWithFilters`, both `FindRecentRaw`, both
  `DistinctValues` (default 200 kept) clamp.
- **Wire impact**: `size`/`limit` above 1,000 return up to 1,000 rows (was 100).
- **Tests**: `dispatchjob/limit_clamp_pg_test.go`, `event/limit_clamp_pg_test.go`.
- **Java/Rust mirror**: `min(limit, 1000)`, default when absent.

## 22. Event drawer never shows Context Data

- **Fix** (`b3c28a4`): `GET /api/events/{id}` fills the context from `msg_events.context_data`
  (`event.Repository.ContextByID`; empty when the row is gone); `/ui`'s drawer does the same. The
  README's claim that the API sends `context` was wrong: the wire field was already `contextData`,
  the name the SPA reads; only the data was missing.
- **Wire impact**: `contextData` is populated.
- **Tests**: `event/api/context_data_pg_test.go`; `ui/pages/events_test.go` `TestEventDrawer`.
- **Java/Rust mirror**: read the context from the write table in the event detail.

## 23, 25. Process edit/delete rules only in the SPA; blank description stored ""

- **Ruling** (coordinator, owner-approved scope): guards in the use cases with clear codes.
- **Fix** (`ccf2dd5`): `UpdateProcess` refuses an archived process (409 `PROCESS_ARCHIVED`,
  "Archived processes cannot be edited."); `DeleteProcess` refuses a current one (409
  `PROCESS_NOT_ARCHIVED`, "Archive the process before deleting it."). A blank description is stored
  as none in create and update. The process sync writes through the repository and is unaffected.
- **Wire impact**: 409 for the refused cases (API and SDK); `description` absent instead of "".
- **Tests**: `process/operations/lifecycle_guards_pg_test.go`; `ui/pages/processes_pg_test.go`.
- **Java/Rust mirror**: the two checks and codes; normalise a blank description.

## 24. SPA row pencil opens the editor of an archived process

- **Fix** (`10a5ac1`): the pencil is disabled for archived processes; the editor sends a stale link
  back to the drawer (`utils/process-rules.ts`). **Wire impact**: none.
- **Tests**: `frontend/tests/process-rules.test.ts`. **Java/Rust mirror**: none.

## Dispatch pools, connections: client identifier, clearing fields, picking a client

- **Client identifier** (`60dc063`): create of a pool, connection or subscription stores the
  client's identifier (`repocommon.ClientIdentifier` over `tnt_clients`); it never did, so every
  client-scoped one showed as "Anchor-level". No client, or an id naming no client: none (no new
  error). **Routing**: the router document and the scheduler name a client pool's processing pool
  `{clientIdentifier}-{code}` from this column (`docs/router-architecture.md`); pools created from
  now on get that name, existing ones are **not backfilled** (a backfill would rename live router
  pools). Tests: `client_identifier_pg_test.go` in dispatchpool, connection and subscription
  operations. Mirror: look up and store the identifier on create.
- **Clearing a pool's rate limit and description** (`d1158fa`): a provided blank description or a
  rate limit of 0 clears the value (NULL; the router treats 0 and NULL as no limit). The SPA drawer
  and `/ui` send blanks as "" and 0 (they sent `value || undefined`, read as "leave it"). Wire: body
  unchanged; field docs in the lock say what a blank does. Tests:
  `dispatchpool/operations/clear_fields_pg_test.go`, `ui/pages/dispatch_pools_test.go`,
  `dispatch_pools_pg_test.go`, `frontend/tests/dispatch-pool-edit.test.ts`. Mirror: the same
  normalisation in update.
- **A client-scoped user can pick its client** (`108894d`): the client field searched clients
  through the anchor-only client permission, so a client user could create no pool, connection or
  subscription (no client means platform-wide, which needs anchor). The choices are now the active
  clients the caller can access: every client with `CanReadClients`, its own otherwise (as
  `/bff/filter-options/clients`); `/ui` refuses another tenant's identifier as "No client with this
  identifier."; the use cases' scope check is unchanged. Tests: `ui/pages/client_pick_test.go`,
  `frontend/tests/client-options.test.ts`. Mirror: none (UI), the BFF client options already exist.

## Found while fixing, fixed

- **Sync Platform Events** (`a3414cc`, "who may sync"): `POST /bff/event-types/sync-platform`
  checked anchor scope only. **Chosen**: anchor scope and `event-type:sync` or `:manage`
  (`auth.CanSyncPlatformEventTypes`); `/ui` follows. Test: `bff/sync_gate_test.go`. Mirror: same
  gate.
- **`/bff/roles`** (`5b75de5`): reads checked nothing; writes checked anchor scope only, so any
  anchor principal could create roles and grant any permission. Reads need `role:view`; create and
  defining a permission `role:create`; update `role:update`; delete `role:delete`; the platform
  role sync all three; anchor checks kept. Test: `bff/roles_gate_test.go`. Mirror: same gates.
- **Dialog slot morph** (`8b8322e`, coordinator scope): every page puts its dialogs in their slot
  with `patchDialogSlot` (replace), not a morph, so a second dialog opens modal. Tests: each page's
  dialog test asserts the replace patch; `ui/pages/dialog_slot_test.go`.

## Follow-up: the findings fixed (second batch, 2026-10-10)

The items of "New findings, not fixed" below that the owner asked to fix. Same format; each test
failed before its fix.

### F1. Scheduled-job runner reports refused to the application's own service account

- **Found**: `POST /api/scheduled-jobs/instances/{instanceId}/log` and `/complete` checked any of
  `messaging:scheduled-job:create/update/delete`. The caller that legitimately reports on a firing
  is the application's SDK, with the service account provisioned for the application
  (`ProvisionServiceAccount`: role `platform:application-service`, `all_applications` false, one
  application-access row). That role holds `application-service:scheduled-job-instance:write`
  (seeded, never checked) and none of the messaging grants, so the SDKs' `log`/`complete` calls
  (TS `scheduled-jobs.ts`, Laravel `ScheduledJobs.php`, Java `ScheduledJobsResource`) were 403.
- **Fix** (`5d63669`): both endpoints admit the messaging create/update/delete grants (as before)
  or `application-service:scheduled-job-instance:write` (`auth.CanReportScheduledJobInstances`).
  A caller admitted only by the latter reports only on firings of jobs of an application it can
  access (`auth.CanReportScheduledJobInstance`, the rule of sdksync's `requireAppAccess`): another
  application's job, or a job of no application, is 403 `APPLICATION_ACCESS_REQUIRED`. The
  per-instance client scope applies to both, as before.
- **Chosen**: the messaging grants keep the any-of gate on these two endpoints (they are the
  runner's reports, not job writes; no per-operation code exists for them).
- **Wire impact**: no shape change; an application's service account is now admitted for its own
  jobs' firings.
- **Tests**: `scheduledjob/api/instance_report_pg_test.go`
  `TestInstanceReports_ApplicationServiceAccountConfinedToItsJobs`.
- **Java/Rust mirror**: in the log and complete handlers admit
  `platform:application-service:scheduled-job-instance:write` besides the messaging grants; for a
  caller without a messaging grant, load the instance's job and require
  `canAccessApplication(job.applicationId)` (no application: refuse) before the client-scope check.

### F2. Event-type `clientId` on create silently dropped (and a scope bypass)

- **Found**: `msg_event_types` has no client column; `clientId` on `POST /api/event-types` (and
  `/bff/event-types`) was set on the entity and dropped. `CreateEventType`'s scope check ran on
  that `clientId`, so a client-scoped user with `event-type:create` naming its own client passed
  and created a **platform-wide** event type; the created event named a client the type did not
  have.
- **Chosen**: accept and ignore (no 400). The SDKs expose `clientId` on create (Laravel
  `CreateEventTypeRequest` DTO, the generated TS and Java request types), so a 400 could break
  their callers.
- **Fix** (`0a8df19`): `CreateCommand` no longer carries a client; the wire fields stay, ignored,
  and the OpenAPI description says so ("Ignored. Event types are platform-wide and have no
  client..."). The create is authorized as what it is, a platform-wide create:
  `CheckScopeAccess(ac, nil)` (anchor or super-admin), whatever `clientId` says. The created event
  names no client.
- **Wire impact**: `api/openapi.lock.json` changes one field description (no shape change);
  `frontend/src/api/generated` regenerated. A non-anchor caller that sent its own `clientId` is now
  403 `SCOPE_FORBIDDEN` (it got a platform-wide type it could not then edit). `make sdk-spec` not
  run (deferred to the SDK release).
- **Tests**: `eventtype/api/client_ignored_pg_test.go` `TestCreateEventType_ClientIDIsIgnored`;
  `eventtype/operations/ops_pg_test.go` `TestCreateEventType_ResourceScope` now expects the refusal.
- **Java/Rust mirror**: drop the client from the create command; authorize the create on no
  client (anchor); keep the wire field, ignored, with the same description; the created event
  carries no `clientId`.
- **Still open**: the event-type read paths' client scoping (`eventtype.VisibleTo`, `/ui`) reads a
  client that is never stored: dead code, left as is.

### F3. Client events catalogued under codes nothing emits

- **Found**: the seed catalogue listed `platform:iam:client:{created,updated,activated,suspended,
  deleted,note-added}`; the client use cases emit `platform:admin:client:*` (source
  `platform:admin`; the Java spec `docs/spec/client.md` and the Rust port name the same codes),
  nothing emits the iam codes, so a subscription made to "Client Created" from the catalogue never
  received an event. `platform:admin:client:deactivated` (bug 15) and
  `platform:iam:client:applications-updated` (emitted by the application use case) were not
  catalogued. The seeded schemas also described data that is not sent.
- **Ruling basis**: the emitted codes are right (consumers match on what is emitted; the other
  platforms agree); the catalogue follows them.
- **Fix** (`25c3ed6`): the seed catalogues the seven emitted `platform:admin:client:*` codes and
  `platform:iam:client:applications-updated`, each with a schema of the data it emits. Migration
  **070** (`070_client_event_type_codes.sql`) renames the six rows an older seed wrote (id,
  status, schema versions kept; subdomain `admin`), re-points subscription bindings to them, and
  replaces a 1.0 schema only while it is exactly what the seed wrote. Where the emitted code is
  already an event type it leaves both rows alone. Idempotent; Down renames back (schemas kept).
  The seeder (after migrations, every boot) then adds `deactivated` and `applications-updated`.
- **Chosen**: subscription bindings follow the rename, so a subscription made to a catalogued
  client event (which never fired) now receives client events.
- **Wire impact**: the catalogue (`GET /api/event-types`, Sync Platform Events) shows the admin
  codes; existing iam rows are renamed in place.
- **Tests**: `seed/emitted_catalogue_test.go` `TestEveryEmittedEventTypeIsCatalogued` (scans every
  `events.go` under `internal/platform` for event-type constants and fails on one the catalogue
  lacks; there was no such test); `seed/client_event_codes_pg_test.go` (the migration on a scratch
  embedded database: rename, ids kept, binding, schemas, edited schema kept, re-seed adds the rest
  once, re-run no-op, Down; and the clash case).
- **Found here, not fixed**: 79 other emitted codes are not catalogued either (the commit message
  says 78; the list in the test has 79). The test lists them as known gaps it only lets shrink (an
  entry catalogued or no longer emitted fails it). See new findings.
- **Java/Rust mirror**: catalogue `platform:admin:client:*` (seven, with these schemas) and
  `platform:iam:client:applications-updated`; the same rename migration (its SQL is portable);
  the consistency test over the emitted constants. The Rust seed already carries both sets.

### F4. Service-account writes outside the anchor-and-permission rule

- **Found**: deactivate checked `service-account:update` without anchor scope; assigning roles,
  regenerating the auth token or signing secret and minting a token checked anchor scope only, so
  any anchor principal (a read-only staff role, an application's own service account, which is
  anchor-tier) could re-role any service account, rotate its credentials or mint a bearer with its
  authority.
- **Fix** (`0ae0988`): every write to an existing service account makes one check
  (`authorizeWrite`): the permission (`service-account:update`; delete keeps `:delete`), anchor
  reach (owner decision #19), and, for an **application-scoped** caller (no all-applications
  access), only an account bound to an application it can access; a platform-wide or another
  application's account is 403 `APPLICATION_ACCESS_REQUIRED`. Create refuses such a caller a
  platform-wide account or another application's. Client-scoped users manage no service account
  (anchor, decision #19), so there is no client confinement to add.
- **Chosen**: assign roles, both regenerations and the mint need **update** (no dedicated code
  exists). Interactive users default to all-applications access, so the SPA's flows for
  administrators holding the permissions are unchanged; no seeded role but super-admin holds the
  service-account permissions, so in practice only super-admins manage service accounts.
- **Wire impact**: 403 for callers lacking the permission (`PERMISSION_REQUIRED`), anchor
  (`ANCHOR_REQUIRED`), or the application (`APPLICATION_ACCESS_REQUIRED`).
- **Tests**: `serviceaccount/api/gate_test.go` (each operation's permission, incl. the four new),
  `anchor_gate_test.go` (non-anchor refused for all), `application_reach_pg_test.go`
  (`TestServiceAccountWrites_ApplicationScopedCallerConfined`); the mint tests now hold the
  permission.
- **Java/Rust mirror**: one gate on deactivate, assign roles, regenerate token/secret, mint:
  `service-account:update` + anchor + application confinement (the account's `applicationId` in
  the caller's applications unless it has all-applications access); create checks the requested
  `applicationId` the same way.

### F5. Code-first sync did not store the client identifier

- **Fix** (`3f0dba4`): `SyncConnections` and `SyncSubscriptions` store the sync client's
  identifier on the rows they create (one `repocommon.ClientIdentifier` lookup per sync), as
  create does since `60dc063`; rows the sync updates are not backfilled (decision 10).
- **Wire impact**: `clientIdentifier` present on client-scoped code-first connections and
  subscriptions created from now on.
- **Tests**: `TestSyncConnections_StoresTheClientIdentifier`,
  `TestSyncSubscriptions_StoresTheClientIdentifier` (`client_identifier_pg_test.go` in both
  operations packages).
- **Java/Rust mirror**: look up and store the identifier on sync-created rows.

### F6. The SPA offered Sync Platform Events and the role editors to users then refused

- **Fix** (`fb36438`): `frontend/src/utils/admin-permissions.ts` makes the BFF's checks on the
  session user (anchor via `userScope`, `userHasPermission` with wildcards):
  `canSyncPlatformEventTypes` (anchor and `event-type:sync`/`:manage`), `canCreateRoles`,
  `canUpdateRoles`, `canDeleteRoles` (anchor and the role permission), `canSyncPlatformRoles`
  (all three). Hidden for others: Sync Platform Events (event type list, dashboard card), the
  dashboard's role sync card, Create Role, the role row pencil and trash, the drawer's Edit, Create
  Permission; the role editor sends them back to the drawer with a notice; Sync All runs only the
  syncs offered (the OpenAPI sync unchanged). `/ui` already gated its sync button on
  `CanSyncPlatformEventTypes` (`a3414cc`) and has no role pages; its nav only links to the SPA's
  role pages under `role:view`, which the reads need. Nothing to change there.
- **Wire impact**: none. **Tests**: `frontend/tests/admin-permissions.test.ts`.
- **Java/Rust mirror**: none (SPA).

## Decisions taken conservatively (owner to confirm)

1. Lifecycle writes need **update**, not the dedicated seeded permissions that no check reads
   (`user:activate`, `user:deactivate`, `user:assign-roles`, `scheduled-job:pause`,
   `event-type:archive`, `process:archive`).
2. Subscription `clientScoped`/`sequence`: accepted on **create** only (update unchanged; the SPA's
   edit still sends `sequence`, which is dropped).
3. Queue: the SPA offers the two accepted values; the API is unchanged.
4. Blank `connectionId`: "not provided" (cannot clear the connection, as documented).
5. Pool code on subscriptions: stored on create/update; an unknown pool id stores none, no error.
6. Client list: SPA pages in the browser; the API stays unpaged.
7. Filter options for application/subdomain/aggregate: derived in the SPA; the API is unchanged.
8. Add schema via the API: same normalisation, no new body fields.
9. Who may run Sync Platform Events: anchor and `event-type:sync`/`:manage`.
10. Client identifiers are not backfilled for existing pools, connections and subscriptions (nor
    by the code-first syncs, F5).
11. Scheduled-job runner reports (F1): the messaging grants keep the any-of gate; the
    application-service grant is confined to the caller's applications' jobs.
12. Event-type `clientId` on create (F2): accepted and ignored, not a 400; the create needs anchor
    reach whatever it says.
13. Client event codes (F3): the emitted `platform:admin:client:*` are kept and catalogued;
    subscription bindings follow migration 070's rename.
14. Service accounts (F4): assign roles, regenerate and mint need `service-account:update`; an
    application-scoped caller is confined to accounts of its applications.

## New findings, not fixed

Fixed in the second batch (above): the scheduled-job runner reports (F1), event-type `clientId`
(F2), client event names (F3), service accounts (F4), the sync paths' client identifier (F5), the
SPA's sync and role controls (F6).

- **`DELETE /api/event-types/{id}`** is documented "Archive an event type" and the TS SDK calls it
  `archive()`, but it hard-deletes (now only an archived or never-published type).
- `GET /api/dispatch-jobs` documents a default of 50 rows; the repository's default is 100.
- **Event-type client scoping is dead code** (F2): the read paths filter on a client that is never
  stored.
- **The event-type catalogue lags what is emitted** (F3): 79 emitted codes are not catalogued
  (anchor domains, auth configs, dispatch-pool activate/suspend, email-domain mappings, functions,
  identity providers, IdP role mappings, OAuth clients, passkeys, platform config, processes, roles,
  scheduled jobs, the OpenAPI sync, service-account deactivate, developer credentials, dispatch-job
  cancel/complete/resend, portal apps and identities; the list is
  `knownUncataloguedEventTypes` in `seed/emitted_catalogue_test.go`), so no subscription can pick
  them; and 16 catalogued codes are never emitted (`platform:admin:edm:*`, `platform:admin:idp:*`,
  `platform:iam:anchor-domain:*`, `platform:iam:auth-config:*`, `platform:iam:role:*`,
  `platform:iam:roles:synced`, `platform:iam:user:password-reset-requested`): the same rename
  drift as the client events.
- **Route permissions not in the seed**: the SPA's `ROUTE_PERMISSIONS` and the `/ui` nav gate pages
  on codes no role grants (`platform:iam:identity-provider:view`, seeded `platform:iam:idp:view`;
  `platform:iam:oauth-client:*`, seeded `platform:auth:oauth-client:*`;
  `platform:admin:audit:view`, seeded `platform:admin:audit-log:view`; `platform:admin:cors:view`,
  seeded `platform:admin:cors-origin:view`; `platform:admin:settings:view`, not seeded), so only
  wildcard holders reach those pages.
- **The dashboard's OpenAPI sync** is shown to everyone; `POST /bff/developer/sync-platform-openapi`
  needs anchor and `developer:application-openapi:sync` (the same exposure as F6, not in scope).
- **Service-account role assignment is unbounded**: a holder of `service-account:update` can assign
  any role, super-admin included, to a service account and mint its token (the user path bounds
  client administrators; this path has no such bound).

## Not verified

- No browser run of the SPA or `/ui` changes (unit, vitest and integration tests only).
- The Java/Rust ports (paused); the SDKs were not regenerated or released (`make sdk-spec`
  deferred to the SDK release; the lock's only change in the second batch is F2's description).
- Second batch: migration 070 ran on embedded scratch databases only (Up, re-run, Down by
  executing its SQL; through goose on every test database), never on a deployed one; the gated
  `TestDropInSafety` was not run; no browser run of the SPA role and sync controls.
- Routing of a newly created client-scoped pool end to end through a running router.
