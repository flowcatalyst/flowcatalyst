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
| - | principals (users) | `4c87621`, `e02159b` | creating users (create, createUser, bulk import) `user:create`; every change to an existing user (edit, activate, deactivate, passwords, 2FA reset, application access, someone else's developer credential) and the reset-approval queue (list, approve, deny) `user:update`; a change to the user's roles `user:assign-roles` (owner ruling 2026-10-11, F7); delete unchanged. `auth.RequireUserAdmin` now takes the operation's check |
| - | comments | `54dcaa0` | role and application use-case comments name the per-operation gates |

- **Wire impact**: no shape change. A caller holding only some of create/update/delete is now
  refused (403 `PERMISSION_REQUIRED`) for the operations it lacks. The seeded admin roles hold
  create, update and delete together, so their principals are unaffected. The SDK's event-type schema push keeps its
  application-service grant.
- **Chosen**: lifecycle operations (pause, resume, activate, deactivate, archive) need **update**
  where there is no dedicated check today. The seed defines `user:activate`, `user:deactivate`,
  `scheduled-job:pause`, `event-type:archive` and `process:archive`, but no check reads them;
  switching to them would lock out custom roles that hold update only. Left as a decision (below).
  `user:assign-roles` was in this list; the owner ruled it canonical for role changes (F7).
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
  entry catalogued or no longer emitted fails it). Catalogued in F9.
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

### F7. A user's roles changed under `user:update` (owner ruling 2026-10-11)

- **Found**: the first batch (`4c87621`) gated the user role routes on `user:update`, because no
  check read the seeded `platform:iam:user:assign-roles` and switching was thought to lock out
  custom roles holding update only (decision 1). The Java platform checks `USER_ASSIGN_ROLES`
  (`Access.requireRoleAssigner`) and Rust `user:assign-roles` (`authorization_service.rs`), so Go
  was the odd one out.
- **Ruling**: `user:assign-roles` is the permission for assigning or changing a user's roles.
- **Fix** (`e02159b`): `auth.CanAssignPrincipalRoles`. `PUT /api/principals/{id}/roles`,
  `POST /api/principals/{id}/roles` and `DELETE /api/principals/{id}/roles/{role}` check it before
  any load (in place of `user:update`); the `AssignRoles` use case checks it post-load after the
  scope rules (`requireRoleAssigner`), so every caller needs it. The bulk import creates users
  under `user:create` and assigns a row's roles through `AssignRoles`: without assign-roles the
  user is created and the row reports "created, but roles not applied" (Java does the same); a row
  without roles needs nothing more. There is no combined call: `PUT /api/principals/{id}` carries
  no roles, and no BFF or `/ui` path assigns user roles (`/ui` links to the SPA's user pages).
  SPA (`18c0c27`): `canAssignUserRoles` (`utils/admin-permissions.ts`, permission only, no anchor
  check: a client administrator holding it assigns its client's application roles). Without it
  the user drawer shows roles read-only (no Manage Roles / Assign Roles), Developer Users offers no
  Grant or Remove Developer Role (credential actions stay), and the CSV import dialog says the
  Roles column will not be applied.
- **Not changed**: the principal syncs (`POST /api/principals/sync`,
  `POST /api/applications/{appCode}/principals/sync`) keep their any-of gate (manage, create,
  update, delete or assign-roles), as on Java (`PrincipalApi.syncUsers`, `SdkSyncApi`); they write
  `SDK_SYNC` role assignments, not `ADMIN_ASSIGNED` ones. Service-account roles keep
  `service-account:update` (F4). The IdP role sync at login is the system's, not a caller's.
- **Seed**: `platform:iam-admin` and `platform:client-admin` hold update and assign-roles together
  (since `31fb230` and `9df6124`), `platform:super-admin` the wildcard; no seeded role holds update
  without assign-roles, so no migration. The seeder never rewrites an existing role, so a
  deployment that removed assign-roles from a built-in role by hand keeps that edit.
- **Wire impact**: no shape change. A caller holding `user:update` without `user:assign-roles` (a
  custom role) gets 403 `PERMISSION_REQUIRED` on the three role routes, and its import rows'
  roles are not applied; a caller holding assign-roles without update can now change roles.
- **Tests** (each failed before): `principal/api/gate_test.go` (each role route with each single
  permission), `principal/api/assign_roles_permission_pg_test.go`
  (`TestRoleChanges_RefusedWithUpdateOnly`, `TestRoleChanges_AllowedWithAssignRolesOnly`: role
  changes allowed, profile edit 403; `TestBulkImport_RowRolesNeedAssignRoles`),
  `shared/auth/auth_test.go` `TestCanAssignPrincipalRoles_NeedsAssignRoles`,
  `frontend/tests/admin-permissions.test.ts` ("user role assignment"). The role-route scope tests
  now hold assign-roles.
- **Java/Rust mirror**: none. Java already checks `USER_ASSIGN_ROLES` in `AssignRoles` and the
  add/remove role handlers; Rust already checks `user:assign-roles`. Both keep the syncs' any-of
  gate, as Go does.

### F8. Role assignment not bounded by the caller's own permissions (owner instruction 2026-10-11)

- **Found**: a holder of `service-account:update` could give any service account any role,
  `platform:super-admin` included, then mint its token (`POST /api/service-accounts/{id}/token`)
  and act as super-admin. The same gap on the user side: an anchor holder of
  `user:assign-roles` could give anyone, itself included, super-admin; the principal syncs
  (`POST /api/principals/sync`, `POST /api/applications/{appCode}/principals/sync`, gated on any
  user permission) wrote whatever role names the body carried onto any email; and
  `POST /api/applications/{id}/provision-service-account` checked anchor scope only, so any anchor
  principal, an application's own service account included, could provision an account holding
  `platform:application-service` for any application and take its client secret.
- **Rule** (`role.RequireRolesWithinCaller`, `internal/platform/role/ceiling.go`; ported from Java
  `RoleCeiling` and Rust `role::ceiling`, same code and wording): a caller may add or remove a role
  only when it holds every one of that role's **platform** permissions (first segment `platform`),
  matched with the enforcement rule (`auth.Grants`, segment wildcards). Holders of
  `platform:*:*:*` may assign anything. The rule applies to the change (`role.ChangedRoles`): roles
  the target keeps are not checked, removal counts (a lesser administrator cannot strip a
  super-admin). A role name that does not exist grants nothing and is never above anyone. A nil
  caller holds nothing; an unwired role repository refuses. Refusal: 403 `ROLE_ABOVE_CALLER`,
  "You may only assign or remove roles whose permissions you hold yourself: <roles>".
- **Chosen**: application permissions (`orders:order:view`) are outside the ceiling, as in Java and
  Rust. Nobody outside an application's roles holds them, not even a super-admin by matching, so
  a literal "subset of the caller's permissions" would stop every administrator, client
  administrators included, from assigning application roles. They stay bounded by the existing
  application rules (`assertAssignableRoles` for client administrators).
- **Fix**, every path that gives a principal roles, with how each is treated:

  | Path | Treatment |
  | --- | --- |
  | `PUT /api/service-accounts/{id}/roles` (`AssignRolesToServiceAccount`) | ceiling on the change, post-load (`09b33e8`) |
  | `POST /api/service-accounts/{id}/token` (mint) | refused when the account holds a role above the caller: the token carries the account's whole authority. Same code, message "You may only mint a token for a service account whose roles' permissions you hold yourself" (`09b33e8`) |
  | `PUT`/`POST /api/principals/{id}/roles`, `DELETE .../roles/{role}`, bulk-import rows (`AssignRoles`) | ceiling on the change, post-load, after the scope rules and role existence (`70c9ca7`); a refused import row reports "created, but roles not applied" |
  | `POST /api/principals/sync`, `POST /api/applications/{appCode}/principals/sync` (`SyncPrincipals`) | ceiling on every role added or removed across the sync, before any write; the whole sync is refused (`e08eee4`). An SDK syncing its own application's roles (application permissions only) passes for any caller |
  | `POST /api/applications/{id}/provision-service-account`, `/ui` provisioning | gate is now anchor + `service-account:create` + `application:update` (`auth.CanProvisionApplicationServiceAccount`, the Java/Rust gate), then the ceiling on `platform:application-service` (`appops.RequireProvisionWithinCaller`), in the handlers (`8b1cb0f`) |
  | `POST /api/service-accounts` (create) | carries no roles (Go has no create-with-roles), unchanged |
  | fcdev init `provisionAppCredentials` (`ProvisionServiceAccount` with no caller) | system path, not bounded: the ceiling is in the handlers, not the operation |
  | seed bootstrap admin (`seed/admin.go`), fcdev `createAdmin`, `router_bootstrap.go`, `mcp_bootstrap.go` | direct SQL inserts into `iam_principal_roles`, system paths, unchanged |
  | IdP role sync at login (`SyncIdpRoles`, `bridge/login_endpoint.go`) | the system's, from configured mappings; unchanged (the mappings themselves are a new finding below) |
  | email-domain `MoveMappingToProvider` | removes IdP-synced roles only, as the system; unchanged |
  | regenerate auth token / signing secret | webhook credentials (outbound to receivers), not platform authority; unchanged (F4 gate) |

  SPA (`90e4113`): `isRoleAboveUser` (`utils/admin-permissions.ts`, the same rule) locks such roles
  in the user and service-account role pickers (no add, no remove, a lock with the reason);
  the application drawer offers Provision Service Account only under
  `canProvisionApplicationServiceAccount`. `/ui` (in `8b1cb0f`) offers its button under the same
  gate; it has no role pickers.
- **Wire impact**: no shape change, OpenAPI lock unchanged. New 403 `ROLE_ABOVE_CALLER` on the
  role routes, the mint, the two principal syncs and provisioning; provisioning by an anchor
  lacking `service-account:create` or `application:update` is now 403 `PERMISSION_REQUIRED`
  (seeded: only `platform:super-admin` holds service-account create, so in practice only
  super-admins provision, as before for every non-system path). Super-admins and SDK syncs of an
  application's own roles see no change.
- **Tests** (each failed with its check removed or stubbed): `role/ceiling_test.go` (lacking,
  subset, wildcard incl. role wildcards, application permissions, unknown role, nil caller,
  lookup failure); `serviceaccount/api/role_ceiling_pg_test.go`
  `TestServiceAccountRoles_BoundedByCallersPermissions` (assign, removal, mint);
  `principal/api/role_ceiling_pg_test.go` `TestUserRoles_BoundedByCallersPermissions` (replace,
  add, remove, wildcard, bulk import); `principal/api/sync_role_ceiling_pg_test.go`,
  `sdksync/sync_principals_ceiling_pg_test.go` (both syncs);
  `application/api/provision_gate_pg_test.go` `TestProvisionServiceAccount_GateAndRoleCeiling`
  (gate, ceiling, super-admin, the no-caller system path still provisions);
  `shared/auth/auth_test.go`; `ui/pages/applications_test.go`;
  `frontend/tests/admin-permissions.test.ts` ("role ceiling (F8)"). Existing system-path tests
  (`TestProvisionServiceAccount_AssignsRoleAndScopesClient`, the seed and fcdev tests) unchanged
  and passing.
- **Java/Rust mirror**:
  - Already match: the ceiling itself and its use on service-account roles (Java
    `AssignRolesToServiceAccount` → `RoleCeiling.requireRoles`; Rust
    `service_account/operations/assign_roles.rs` → `ceiling::require_role_change`), user roles
    (Java `AssignRoles`; Rust `principal/operations/assign_roles.rs`) and the platform-route user
    sync (Java `SyncPrincipals.requireCeiling`; Rust `sync_users.rs`). The provisioning gate
    (Java `ApplicationApi.provisionServiceAccount`, Rust `application/api.rs`
    `provision_service_account`: anchor + `SERVICE_ACCOUNT_CREATE` + `APPLICATION_UPDATE`).
  - Go goes further, to mirror if the owner keeps it: (1) the **mint** is bounded (Java
    `MintServiceAccountToken.mint` and Rust `mint_service_account_token` flatten and mint with no
    ceiling, so a holder of `service-account:update` can still mint a token for an account a
    super-admin made stronger); (2) **provisioning** applies the ceiling to
    `platform:application-service` (Java/Rust: the gate only); (3) the **app-scoped** principal
    sync applies the ceiling (Java and Rust exempted it because they confine it to the
    application's own roles, `requireSyncableRoles` / `ROLE_APP_FORBIDDEN`; both apply the
    ceiling there too since their mirrors of F8, and Go has the confinement since F12).

### F9. Catalogue drift closed (owner ruling 2026-10-11: "catalogue please")

- **Found** (F3): 79 emitted event-type codes were not in the seed catalogue, so no subscription
  could pick them.
- **Fix** (`8ddaed4`): `seed.PlatformEventTypes()` catalogues all 79 (154 entries, was 75), each
  named in the existing style ("OAuth Client Secret Rotated", "Portal Identity App Granted") with a
  1.0 JSON schema of the data its use case emits (`ToDataJSON`; an omitempty field optional, a nil
  slice null). New subdomains in the catalogue: `platform:messaging`, `platform:developer`,
  `platform:portal`. The boot seeder upserts by code, so an existing database gains the rows on the
  next start; no migration (nothing renamed). The catalogue has no descriptions, as before.
- **Tests**: `knownUncataloguedEventTypes` is empty; `TestEveryEmittedEventTypeIsCatalogued` keeps
  failing on any new uncatalogued code. 70 of the 79 schemas were checked against real
  `ToDataJSON` output with `jsonschema-go` during the change (the other nine, the scheduled-job and
  dispatch-job events with unexported embedded data, were read off the code); that check was not
  committed.
- **Not changed then** (done in the follow-up below): the 16 catalogued codes nothing emits, and
  the older schemas, which were not audited.
- **Java/Rust mirror**: catalogue the same 79 codes and schemas.

#### F9 follow-up: unemitted codes pruned, older schemas fixed (owner ruling 2026-10-11)

- **Ruling**: delete every catalogue entry nothing emits; emitted codes keep their names (renaming
  one would break stored events and subscriptions, so the emitted `platform:admin:*` stay); fix
  the older schemas against what the use cases emit.
- **Pruned 16** (`45537af`): `platform:iam:role:{created,updated,deleted}`,
  `platform:iam:roles:synced`, `platform:iam:anchor-domain:{created,deleted}`,
  `platform:iam:auth-config:{created,updated,deleted}`, `platform:admin:idp:{created,updated,deleted}`,
  `platform:admin:edm:{created,updated,deleted}`, `platform:iam:user:password-reset-requested`.
  The set is catalogued minus emitted (the `events.go` scan), checked three ways: a repo-wide
  grep finds each code only in the seed and this ledger (no other emitter, no SPA, SDK or `/ui`
  reference; the `platform:iam:role:*` hits elsewhere are permissions); the schema audit below
  instantiates every event struct (136, covering all 138 emitted codes) and none returns one of
  them; `git log -S` shows no Go code ever emitted one. Emitted equivalents, for anyone re-binding:
  `platform:admin:role:*`, `platform:admin:roles:synced`, `platform:admin:anchor-domain:*`,
  `platform:admin:auth-config:*`, `platform:admin:identity-provider:*`,
  `platform:admin:email-domain-mapping:*`; password-reset-requested has none. The catalogue is
  now the 138 emitted codes.
- **Migration 071** (`071_prune_unemitted_event_types.sql`) deletes the 16 rows and their schema
  versions from existing databases (the seeder only upserts, so it would never remove them).
  What references an event type: subscription bindings (`msg_subscription_event_types`, a code
  pattern plus an optional row id, no foreign key), schema versions (deleted with the type),
  stored events and dispatch jobs (the code as text; nothing emitted these codes, so none exist,
  and stored events are not touched), the SPA and `/ui` (read the catalogue through the API; no
  hard-coded code). No other table names an event type.
- **Chosen for bindings**: a binding naming one of the 16 exactly, or pointing at one of the rows,
  never matched an event. It is **archived**, moved to the new
  `msg_subscription_event_types_archive` (original id, subscription, row id, code, spec version,
  reason `071_event_type_not_emitted`, time), not deleted outright (what the subscriber picked
  stays visible and Down restores it) and **not re-pointed** at the emitted equivalent (unlike
  070's rename: these rows were never the emitted event, the payloads differ, and re-pointing
  would start deliveries nobody asked for). Wildcard bindings (`platform:iam:role:*`) name no row
  and stay. A subscription left with no binding matches nothing, as it did before; its status is
  unchanged. Down puts the bindings back and drops the archive; it does not recreate the types
  (a build whose seed lists them adds them on boot). One statement with data-modifying CTEs, no
  temp table (sqlc reads migrations and would model one; the sqlc models regenerated here also
  pick up 070's temp table, which the committed models lacked). `fcdev fresh` truncates the
  archive.
- **Schemas fixed 28** (`4c23adf`): every catalogued schema was validated with `jsonschema-go`
  against real `ToDataJSON` output of a fully populated and a zero-valued instance of each event
  struct, and compared with the fields sent (a property never sent, or a field always sent but
  not required, counts as a mismatch). All 28 wrong schemas predate `8ddaed4`; the 79 added there
  passed (the nine scheduled-job and dispatch-job ones read off the code then pass too).

  | Codes | Was | Emitted |
  | --- | --- | --- |
  | `platform:admin:dispatch-pool:{created,updated,archived,deleted}` | `dispatchPoolId`; `clientId`, `rateLimit`, `concurrency` | `poolId` + `code`/`name` |
  | `platform:admin:eventtype:schema-{added,finalised,deprecated}` | `version`; added also `mimeType`, `schemaType` | `specVersion` (finalised + optional `deprecatedVersion`) |
  | `platform:admin:eventtype:updated` | `name` optional | `name` always |
  | `platform:iam:user:created` | `emailDomain`, `name`, `scope`, `isAnchorUser`, `clientId` | `principalId`, `email` |
  | `platform:iam:user:{updated,deactivated,deleted,password-reset-completed}` | updated `email`, deactivated `reason`, reset `email` (never sent); deleted lacked `email`; updated `name` optional | `principalId` (+ `name` / `email` as sent) |
  | `platform:iam:application:{created,updated,activated,deactivated,deleted}` | `applicationType`, `description`, `code` (activate/deactivate), `name` (deleted) | `applicationId` + `code`/`name` as sent |
  | `platform:iam:serviceaccount:{created,updated}` | `applicationId`, `clientIds`, `description`, `clientIdsAdded/Removed` | `serviceAccountId`, `code`, `name` / `serviceAccountId`, `name` |
  | `platform:admin:connection:{created,updated,deleted}` | `endpoint`, `serviceAccountId`, `clientId`, `status`; updated required `code` | `connectionId` + `code`/`name` as sent |
  | `platform:admin:subscription:{created,updated,paused,resumed}` | `connectionId`, `eventTypes`, `clientId`, `eventTypesAdded/Removed`; paused/resumed `code` | `subscriptionId` (+ `code`, `name` as sent) |
  | `platform:iam:client:applications-updated` | arrays | each list `null` when empty |

  The seeder never replaces an attached 1.0, so **migration 072**
  (`072_platform_event_schema_fixes.sql`) replaces each on existing databases while it is still
  exactly what a seed wrote (each code had one seeded form in every earlier seed, checked by
  running each historical `platformEventSchemas`); an edited schema is left alone. Down restores
  the seeded forms. The audit test was temporary and is not committed.
- **Tests**: `seed/emitted_catalogue_test.go` `TestEveryCataloguedEventTypeIsEmitted` (new:
  catalogued ⊆ emitted) beside `TestEveryEmittedEventTypeIsCatalogued` (emitted ⊆ catalogued), so
  drift either way fails; `seed/prune_event_types_pg_test.go` (071 on a scratch embedded
  database: two seeded dead types with schemas, one emitted; bindings by row and code, by code
  only, wildcard, and to an emitted type; types and schemas gone, emitted kept, the two bindings
  archived as they were, the others kept; the seed does not add them back; re-run no-op; Down
  restores the bindings under their ids and drops the archive); `seed/event_schema_fixes_pg_test.go`
  (072: its corrected schemas equal the seed's, two seeded forms spelled out; all 28 seeded then
  replaced, an edited one kept, re-run no-op, the seed attaches no second schema, Down).
- **Wire impact**: `GET /api/event-types` and Sync Platform Events lose the 16 codes; the 28
  schemas served change to the emitted shapes. Event payloads are unchanged.
- **Java/Rust mirror**: drop the same 16 codes from the seed catalogue (and their schemas) and
  remove them from existing databases the same way (071's SQL is portable; archive or delete
  bindings, do not re-point); fix the same 28 schemas to their platform's emitted data (the Go
  field names above are the cross-platform contract only where the platforms already agree;
  check each platform's own event data, e.g. whether its dispatch-pool events send `poolId`);
  add the catalogued ⊆ emitted test.

### F10. Role permission edits not bounded by the caller's own permissions (owner approval 2026-10-11)

- **Found** (in F8): a holder of `role:update` could add `platform:*:*:*`, or any platform
  permission it lacks, to a role it already holds and become super-admin, one edit past the F8
  role ceiling; `role:create` could mint such a role. The SDK role sync wrote whatever permissions
  its body carried, so an application's service account could put `platform:*:*:*` on a role its
  users hold, and the platform role sync could restore a trimmed catalogue role for a caller
  holding that role.
- **Rule** (`role.RequirePermissionsWithinCaller`, `internal/platform/role/ceiling.go`; the
  permission half of Java `RoleCeiling.requirePermissions` and Rust
  `role::ceiling::require_permissions`, same code and wording): a caller may add a permission to a
  role, or remove one from it, only when it holds that permission. Platform permissions only
  (first segment `platform`), matched with `auth.Grants`; holders of `platform:*:*:*` may add or
  remove anything. Only the change is checked (`role.ChangedPermissions`): kept permissions are
  not, a no-op grant or revoke is not, and removal counts. A nil caller holds nothing. Refusal:
  403 `PERMISSION_ABOVE_CALLER`, "You may only add or remove permissions you hold yourself:
  <permissions>".
- **Fix** (`c56a68c`), in the operations, so every route that reaches them is bounded:

  | Operation | Checked | Routes |
  | --- | --- | --- |
  | `CreateRole` | every permission of the new role, after the duplicate check | `POST /api/roles`, `POST /bff/roles` |
  | `UpdateRole` | the change, post-load, after the CODE-role refusal; not when no permissions are sent | `PUT /api/roles/{id}`, `PUT /bff/roles/{roleName}` |
  | `DeleteRole` | every permission of the role (deleting strips it from everyone), after the CODE-role refusal | `DELETE /api/roles/{id}`, `DELETE /bff/roles/{roleName}` |
  | `GrantPermission`, `RevokePermission` | the change (granting a held one or revoking an absent one changes nothing) | `POST /api/roles/{roleName}/permissions/{permission}`, `POST /api/roles/{roleName}/permissions` (body), `DELETE /api/roles/{roleName}/permissions/{permission}` |
  | `SyncRoles` (SDK) | each synced role's change, a new role's permissions, a `removeUnlisted` deletion's permissions; the whole sync refused before any write | `POST /api/applications/{appCode}/roles/sync` |
  | `SyncPlatformRoles` | each catalogue role's change, a new one's permissions, a swept stale one's; whole sync refused | `POST /bff/roles/sync-platform` |
  | permission catalogue (`DELETE /api/roles/permissions/{permission}`, `POST /bff/roles/permissions`) | not a role edit (roles keep their permission strings); unchanged | |
  | seed, fcdev, migrations | direct SQL, system paths; unchanged | |

  `/ui` has no role pages. SPA (`5a712d7`): `isPermissionAboveUser` (`utils/admin-permissions.ts`,
  the same rule; `isRoleAboveUser` builds on it) locks such a permission in the role editor as it
  stands (no toggle, skipped by the per-application select/deselect-all, a lock with the reason and
  a note above the grid), and the role list shows a lock with the reason in place of delete for a
  role holding one. The editor lists only the role's application's catalogue, so on a
  non-platform role its platform permissions are not shown and are sent back unchanged. Create
  Role sends no permissions. The platform role sync's refusal (it cannot be predicted in the
  browser) reaches the global toast.
- **Wire impact**: no shape change, OpenAPI lock unchanged. New 403 `PERMISSION_ABOVE_CALLER` on the
  routes above. Super-admins see no change; an SDK syncing its application's own permissions sees
  none; a platform role sync by a non-super-admin is refused when the catalogue would add or remove
  a platform permission it lacks (a super-admin runs it).
- **Tests** (each failed before the fix): `role/api/permission_ceiling_pg_test.go`
  `TestRolePermissions_BoundedByCallersPermissions` (self-escalation of the caller's own role by
  update and both grants; create; add lacking by grant and update; held wildcard; removal by
  update, revoke and delete; kept permission unchecked; no-op revoke; application permissions for
  a caller holding none; wildcard holder grants, revokes, deletes);
  `shared/bff/roles_permission_ceiling_pg_test.go` (create, update adding and removing, delete:
  403 and the exact envelope); `sdksync/sync_roles_ceiling_pg_test.go` (application permissions
  pass; new and existing role refused, nothing written; removal by list and by
  `removeUnlisted`; unchanged and empty lists pass); `role/operations/ops_pg_test.go`
  `TestSyncPlatformRoles_BoundedByCallersPermissions`; `role/ceiling_test.go`
  `TestPermissionCeiling`, `TestChangedPermissions`; `frontend/tests/admin-permissions.test.ts`
  ("permission ceiling (F10)"). Browser: driven on an isolated fcdev (API 18181, embedded PG
  25433) as an anchor user holding the role permissions and `user:view`: the live API answered 403
  `PERMISSION_ABOVE_CALLER` to adding `platform:*:*:*` to the user's own role (BFF update and
  grant); the editor of a platform DATABASE role locked 37 catalogue permissions, kept the locked
  `user:create` selected on click, toggled the held `user:view`; the list showed the lock in place
  of delete on roles holding `user:create` and the trash on a `user:view`-only role.
- **Java/Rust mirror**:
  - Already match: create, update, delete, grant and revoke (Java `CreateRole`, `UpdateRole`,
    `DeleteRole`, `GrantPermission`, `RevokePermission` → `RoleCeiling.requirePermissions`; Rust
    `role/operations/{create,update,delete,permission_grants}.rs` →
    `ceiling::require_permissions`), same code, status, message and change-only rule.
  - Ordering differs only in which refusal wins: Rust checks in `authorize`, so its ceiling comes
    before `ROLE_EXISTS` and the CODE-role refusal; Go follows Java (after both).
  - Go goes further, to mirror if the owner keeps it: the **SDK role sync** and the **platform
    role sync** are bounded (Java `SyncRoles`, `SyncPlatformRoles` and Rust `role/operations/sync.rs`
    are not). Java and Rust close the SDK path differently: an application's roles may hold only
    that application's permissions (Java `Role.confined`, S1.5, 400
    `PERMISSION_OUTSIDE_APPLICATION`; Rust `role/operations/mod.rs`); a super-admin may cross
    applications through the admin API only. Go has that rule too since F12.

### F11. Function events in the `platform:function:*` family (owner ruling 2026-10-12)

- **Ruling**: the function events use the `platform:function:*` family Java and Rust emit. Go
  emitted 15 `platform:admin:function*` codes. Rename where the data matches; list the rest for
  the owner, no guessing.
- **Compared** (read-only: Java `FunctionEvents.java`, Rust
  `fc-platform-functions/src/function/operations/events.rs`, both `docs/go-mirror/2026-10-11-event-catalogue.md`
  ledgers; Java and Rust emit the same 14 codes with the same data). The two sets are not one
  rename apart (15 against 14, one extra): they are separate designs that share 9 events and
  differ in their data. Subject (`platform.function.{id}`, `platform.function-domain.{id}`) and
  message group match on every pair; the envelope `source` does not (Go `platform:admin`, Java and
  Rust `platform:function`).

  | Go (was) | Go data | Java/Rust | Java/Rust data | Verdict |
  | --- | --- | --- | --- | --- |
  | `platform:admin:function:deleted` | `{functionId, address}` | `platform:function:function:deleted` | `{functionId, address}` | **renamed** |
  | `platform:admin:function:created` | `{functionId, address, name}` | `platform:function:function:created` | `{functionId, address, applicationId, clientId?, runtime}` | owner (1) |
  | `platform:admin:function:updated` | `{functionId, address}` | `platform:function:function:updated` | `{functionId, address, description?, status}` | owner (2) |
  | `platform:admin:function-version:published` | `{functionId, address, number, digest, runtime}` | `platform:function:version:published` | `{functionId, address, versionId, version, digest, pool, signerIssuer?, signerSubject?}` | owner (3) |
  | `platform:admin:function-version:retired` | `{functionId, address, number}` | `platform:function:version:retired` | `{functionId, address, versionId, version}` | owner (3) |
  | `platform:admin:function-alias:set` | `{functionId, address, alias, number}` | `platform:function:alias:changed` | `{functionId, address, alias, versionId, version, previousVersionId?}` | owner (4) |
  | `platform:admin:function-alias:deleted` | `{functionId, address, alias}` | `platform:function:alias:removed` | `{functionId, address, alias, versionId, version}` | owner (4) |
  | `platform:admin:function-setting:{set,deleted}` | `{functionId, kind, key}` | `platform:function:secret:{set,deleted}`; `platform:function:config:updated` | `{functionId, address, key}`; `{functionId, address, keys}` | owner (5) |
  | `platform:admin:function-domain:claimed` | `{domainId, zone, clientId?}` | `platform:function:domain:claimed` | `{domainId, hostname, owner}` | owner (6) |
  | `platform:admin:function-domain:released` | `{domainId, zone}` | `platform:function:domain:released` | `{domainId, hostname, owner}` | owner (6) |
  | `platform:admin:function:promoted` | wiring summary | none (Java/Rust promote emits `alias:changed` for `live`) | | owner (7) |
  | `platform:admin:function-route:{set,deleted}` | `{functionId, routeId, hostname, pathPrefix}` | none | | owner (7) |
  | `platform:admin:function-schedule:wired` | `{functionId, scheduledJobId, code, action}` | none | | owner (7) |
  | none | | `platform:function:version:ready`, `platform:function:policy:updated` | | owner (8) |

- **Fix** (`1aa67fe`): `FunctionDeleted` emits `platform:function:function:deleted` (function
  delete use case); the seed catalogues it (name "Function Deleted", same schema, the same as Java's
  and Rust's) in place of the old code. The other 14 codes are unchanged.
- **Migration 073** (`073_function_event_type_codes.sql`): renames the catalogue row an older
  seed wrote (id, status and schema versions kept; subdomain `function`, aggregate `function`) and
  re-points the subscription bindings naming it, by row or by code (the same event under its new
  code, so re-pointing is right here, unlike 071). When the new code is already a row (this
  build's seeder ran first, or a Java/Rust seeder on the same database), the bindings move to that
  row, a binding that would duplicate one the subscription already has is dropped, and the old row
  is deleted with its schema versions: no unique-constraint failure. Wildcard bindings
  (`platform:admin:function:*`) are patterns and are left alone (such a subscription stops
  receiving function deletions). Stored events and dispatch jobs keep the code they were written
  with. Idempotent; Down renames back where the old code is free (a deleted duplicate or dropped
  binding is not recreated). Plain statements with inline `VALUES`, no temp table.
- **Tests**: `seed/function_event_codes_pg_test.go` on scratch embedded databases:
  `TestMigration073_RenamesFunctionDeleted` (old row with schema; bindings by row and code, by code
  only, wildcard, another function code; renamed with its id and schema, bindings follow, wildcard
  and the other code kept; the seed adds no second row or schema; re-run no-op; Down renames row and
  bindings back) and `TestMigration073_MergesIntoAnExistingNewRow` (both rows present; bindings
  moved to the new row, the duplicate dropped, old row and schema gone, new row's schema kept;
  re-run no-op). `seed/emitted_catalogue_test.go` `TestFunctionEventTypesFollowJavaAndRust`: a
  `platform:function:*` code Go emits must be one of Java/Rust's 14, and the 14 remaining
  `platform:admin:function*` codes are a list that may only shrink (a new one fails).
  `function/operations/ops_pg_test.go` `TestDeleteFunction_HappyPath` checks the code and data.
- **Wire impact**: `GET /api/event-types` and Sync Platform Events list
  `platform:function:function:deleted` instead of `platform:admin:function:deleted`; function
  deletions are stored and dispatched under the new code. OpenAPI lock unchanged. No SDK, SPA or
  `/ui` names any function event code (repo-wide grep, `clients/` included).
- **Java/Rust mirror**: nothing for the renamed event (both already emit it with this data).
- **For the owner** (not changed):
  1. `function:created`: Go sends `name`; Java/Rust send `applicationId`, `clientId?`, `runtime`
     and no name. Rename after Go sends their data (dropping `name`?), or keep Go's code.
  2. `function:updated`: Java/Rust add `description?` and `status`; Go sends neither.
  3. Versions: Go identifies a version by `number`; Java/Rust by `versionId` + `version`. Published
     also differs (`runtime` against `pool` and the signer pair).
  4. Aliases: the verb differs (`set`/`deleted` against `changed`/`removed`) and Java/Rust send
     `versionId`, `version` (`previousVersionId?` on changed). Java/Rust refuse removing `live`;
     Go's removal of `live` unwires (and also emits `promoted`).
  5. Settings: Go has one setting event with `kind` (config, secret, db) per key; Java/Rust split
     secrets (`secret:set`/`deleted` per key, with `address`) from config (`config:updated`, a
     full replacement listing `keys`) and have no `db` kind.
  6. Domains: Go sends `zone` and `clientId?`; Java/Rust send `hostname` and `owner`, the latter on
     release too.
  7. Go-only events, no Java/Rust counterpart: `function:promoted` (the wiring reconciliation
     summary), `function-route:{set,deleted}`, `function-schedule:wired`. Keep under
     `platform:admin:function*`, or name them in the `platform:function:*` family (Java/Rust would
     then need to know the names are taken)?
  8. Java/Rust-only events Go has no operation for: `version:ready` (host heartbeat),
     `policy:updated` (signer policy).
  9. Envelope `source`: every Go function event, the renamed one included, says `platform:admin`;
     Java/Rust say `platform:function`. Not data, left as is; one constant to change.

### F12. Role confinement and principal-sync reach (owner approval 2026-10-12)

- **Found** (F8, F10 "new findings"): any role could hold any application's permissions, so a
  holder of `role:create`/`role:update` could put another application's permissions, or a platform
  permission it holds (which F10's ceiling lets through), on an application role, and an SDK role
  sync could put another application's permissions on its roles. The principal syncs touched any
  existing user by email (a client administrator holding a user permission could rename,
  deactivate or re-role an anchor administrator), the application route accepted any role name
  (another application's roles, and platform roles the F8 ceiling let through: one holding no
  platform permission, or any for a caller holding them), and the platform route let a client
  administrator give its users roles it could not assign through `PUT /api/principals/{id}/roles`.
- **Rule 1, confinement** (`role.RequirePermissionsInApplication`, `internal/platform/role/confinement.go`;
  Java `Role.confined` (S1.5), Rust `role::operations::require_confined`, same code and text): a
  role may hold only its own application's permissions (first segment = the role's application,
  `role.OwningApplicationCode`: its application code, or on a row without one its name's prefix).
  Refusal: 400 `PERMISSION_OUTSIDE_APPLICATION`, "Permission '<p>' does not belong to application
  '<app>'; a role may only hold its own application's permissions" (the first offender). Only what
  a write **adds** is checked (`role.AddedPermissions`, Rust's rule, the owner's "changes only"):
  existing roles holding foreign permissions load and read as before, keep them through edits that
  do not add them (the role editor sends them back unchanged), and can lose them; a removal is
  never refused, and granting a held permission changes nothing. The admin API lets a
  **super-admin** cross applications (Java `Access.crossApplication`, Rust `cross_application`);
  the crossing is logged. It runs before the F10 ceiling (Java's order), so for anyone but a
  super-admin the ceiling is left bounding platform permissions on `platform:` roles.

  | Operation | Checked | Super-admin may cross | Routes |
  | --- | --- | --- | --- |
  | `CreateRole` | every permission, after `ROLE_EXISTS` | yes | `POST /api/roles`, `POST /bff/roles` |
  | `UpdateRole` | the permissions added, after the CODE-role refusal | yes | `PUT /api/roles/{id}`, `PUT /bff/roles/{roleName}` |
  | `GrantPermission` | the permission, unless held | yes | `POST /api/roles/{roleName}/permissions/{permission}`, `POST /api/roles/{roleName}/permissions` |
  | `RevokePermission`, `DeleteRole` | never (removal) | | |
  | `SyncRoles` (SDK) | a new role's permissions, an existing SDK role's added ones (a non-empty list only, as before); whole sync refused | no | `POST /api/applications/{appCode}/roles/sync` |
  | `SyncPlatformRoles` | a new catalogue role's permissions, an existing one's added ones; whole sync refused | no | `POST /bff/roles/sync-platform` |

  The built-in catalogue is all `platform:` roles holding `platform:` permissions, so the platform
  sync is unchanged in practice (`TestSyncPlatformRoles_PermissionsConfinedToTheirApplication`
  syncs it). `/ui` has no role pages. **SPA**: no change; the role editor lists only the role's
  application's catalogue (`/bff/roles/permissions?application=` filters by first segment) and
  prefixes a custom permission with the role's application, so it cannot offer a foreign one; a
  refusal reaches the global toast.
- **Rule 2, target reach** (`administers`, `principal/operations/authz.go`; Java
  `Access.administers`, Rust `principal::operations::sync::administers`): an existing principal is
  touched only within the caller's reach: an anchor reaches everyone; anyone else only CLIENT-scope
  users homed at a client it can access (`auth.CanAccessScope`, so a clientless one only as a
  super-admin). A listed email matching one out of reach refuses the whole sync before any write,
  403 `SYNC_TARGET_FORBIDDEN`, "Not authorised to manage principal '<email>'" (the email
  lower-cased); the `removeUnlisted` sweep skips such users instead (it ranges over every user, so
  refusing would make any non-anchor sweep impossible). Both routes,
  `POST /api/principals/sync` and `POST /api/applications/{appCode}/principals/sync` (there is no
  BFF principal sync). A new email is created as before. An application's provisioned service
  account is anchor-tier, so reach does not narrow it (as in Java and Rust); rule 3 and the F8
  ceiling bound what it hands out.
- **Rule 3, the syncs' roles** (`requireSyncableRoles`, `principal/operations/sync_principals.go`;
  Java `SyncPrincipals.requireSyncableRoles`):
  - Application route, whoever the caller: every role must be the application's, 403
    `ROLE_APP_FORBIDDEN`, "role '<name>' does not belong to application '<app>'" (Java's and
    Rust's text). A role belongs to its application; a name that is not a role belongs to the
    application its prefix names (so `platform:…` and `otherapp:…` are refused whether or not they
    exist). A name that is not a role and has no prefix, or this application's prefix, is accepted
    as before (it grants nothing).
  - Platform route, a non-anchor caller: only roles it could assign through
    `PUT /api/principals/{id}/roles`, application roles of an application the target's home client
    is entitled to, so none on a user the sync creates (no client): 400 `UNKNOWN_ROLE`, 403
    `PLATFORM_ROLE_FORBIDDEN` or `ROLE_APP_FORBIDDEN` "role belongs to an application the client
    cannot access", the existing assignment codes. That check moved to
    `operations.RequireAssignableRoles` / `operations.ClientApplicationIDs`, which the role routes
    now share (`assertAssignableRoles`, `clientAppIDs` delegate). An anchor may name any role, as
    before. `SyncPrincipals` takes the client-config repository for this (the application route
    passes nil).
  - Order per entry: reach, then roles; the F8 ceiling still runs once after the loop. On the
    application route confinement now stops platform roles, and the ceiling stops an
    own-application role a super-admin gave platform permissions.
- **Fix**: `7f87172` (rule 1), `2ab9a97` (rule 2), `55909de` (rule 3).
- **Wire impact**: no shape change, OpenAPI lock unchanged. New 400 `PERMISSION_OUTSIDE_APPLICATION`
  on the role routes and both role syncs above; new 403 `SYNC_TARGET_FORBIDDEN` on both principal
  syncs; new 403 `ROLE_APP_FORBIDDEN` on the application principal sync; on the platform principal
  sync a non-anchor now gets 400 `UNKNOWN_ROLE` / 403 `PLATFORM_ROLE_FORBIDDEN` /
  `ROLE_APP_FORBIDDEN`. Unchanged: super-admins on the admin API, an SDK syncing its own roles and
  permissions and its own users' roles, anchors on the platform principal sync, platform admin
  roles with platform permissions, and reads of every existing role.
- **Tests** (each new test failed before its fix): `role/confinement_test.go`;
  `role/api/permission_confinement_pg_test.go` `TestRolePermissions_ConfinedToTheirApplication`
  (create, update, both grants, a platform role refused an application permission, a held
  platform permission refused on an application role, own-application and platform-role flows,
  super-admin crossing, a legacy role holding foreign permissions read, renamed, re-sent, re-granted,
  revoked and deleted by a non-super-admin); `shared/bff/roles_permission_confinement_pg_test.go`
  (400 and the exact envelope, super-admin crossing, a kept foreign permission re-sent);
  `sdksync/sync_roles_confinement_pg_test.go` (another application's, a held platform and the
  wildcard refused on new and existing roles, a super-admin caller too, nothing written; a role
  holding a foreign permission re-syncs with the same list, none, an own one added, and drops it);
  `role/operations/ops_pg_test.go` `TestSyncPlatformRoles_PermissionsConfinedToTheirApplication`;
  `principal/api/sync_target_reach_pg_test.go` `TestUserSync_TargetsWithinCallersReach` (another
  client's, anchor, partner and clientless users refused, mixed case, a mixed batch writes
  nothing, own-client and new users pass, an anchor reaches all);
  `sdksync/sync_principals_reach_pg_test.go` (refusals, nothing written, the sweep strips the
  in-reach unlisted user and skips the out-of-reach one (mutant: skip removed, fails), an
  anchor-tier caller reaches all); `sdksync/sync_principals_role_app_pg_test.go` (another
  application's, a platform role holding nothing, an unknown foreign-prefixed name, a super-admin
  caller, mixed case; own roles and unprefixed or own-prefixed non-roles pass);
  `principal/api/sync_assignable_roles_pg_test.go` (each code and message, a new user gets no role,
  nothing written; an entitled role, a role-less new user and an anchor pass).
  The F10 ceiling tests used application roles holding platform permissions, which confinement
  now refuses first: they use `platform:` roles instead, and the SDK sync's removal cases get their
  platform permission from a super-admin through the admin API. `sdksync`'s password-hash fixture
  now wires the role repository (an unwired one refuses, as the ceiling's does).
- **Java/Rust mirror**:
  - Already match: confinement's code, status, text and super-admin exception on create, update
    and grant (Java, Rust); target reach on both routes, its code, text and sweep skip (Java, Rust);
    `ROLE_APP_FORBIDDEN` on the application route (Java, Rust); the platform route's assignable
    check (Java).
  - Differ, to mirror if the owner keeps Go's choice: (1) **change-only confinement**: Java checks
    the whole list an update, the SDK sync or the catalogue sync supplies, and a grant of a
    permission the role already holds, so an existing role holding a foreign permission cannot be
    re-saved as it stands by a non-super-admin; Rust checks only what create/update/grant add but
    every permission an SDK sync supplies. (2) Rust's platform role sync does not call `require_confined`, and its
    platform user sync (`sync_users.rs`) has no assignable-role check (found by grep, not run). (3)
    **Unknown role names on the application route**: Java refuses every name that is not a role
    (400 `UNKNOWN_ROLE`); Rust refuses only names prefixed `platform` or an existing application's
    code; Go refuses a non-role by any foreign prefix and accepts unprefixed or own-prefixed
    non-roles (no `UNKNOWN_ROLE` there, so an SDK syncing users before its roles is not broken).
    (4) Java checks the ceiling per entry; Go once after the loop (as noted in Java's
    2026-10-12 ledger).

### F13. Principal-sync passwords and roles, encoded event data, Go SDK (owner approval 2026-10-12)

- **Password hashes** (F12 "new findings"). Both principal syncs
  (`POST /api/principals/sync`, `POST /api/applications/{appCode}/principals/sync`) replaced an
  existing user's password hash with an entry's `passwordHash`: a credential change outside the
  audited password routes. **Fix** (`004f8d4`, as Java `SyncPrincipals` and Rust decision #22,
  owner ruling 2026-09-25): a `passwordHash` is stored only on a user the sync creates; for an
  existing user it is ignored, whoever the caller (an anchor included), and logged
  (`principalId`); the rest of the entry applies. Both responses gained `passwordHashIgnored`
  (Java's name and type: the emails, lower-cased and trimmed, each once, in payload order; omitted
  when empty), read before the sync runs (`operations.PasswordHashesIgnored`, as Java and Rust), so
  a user created concurrently in between is missed there and its hash still not applied. On the
  application route it is a member of the shared `SyncResultResponse`, so every other sync's
  response is unchanged (empty, omitted). An empty `passwordHash` counts as none.
- **An application's sync replaced other applications' synced roles** (F12 "new findings"). The
  merge dropped every `SDK_SYNC` assignment of a listed user before adding the payload's.
  **Fix** (`bdecf5d`, Java `Principal.syncSourcedRoles`, Rust `is_own_sdk_role`): the merge
  replaces only the assignments the sweep would strip, one predicate for both
  (`belongsToThisSync`: `SDK_SYNC` and the role name prefixed `<appCode>:`; every `SDK_SYNC` one on
  the platform route, unchanged). Found while fixing: a named role the user already held from
  another source (`ADMIN_ASSIGNED`, `IDP_SYNC`) was re-inserted as `SDK_SYNC` (the role insert's
  `ON CONFLICT` overwrote the source), so a later `removeUnlisted` sweep stripped an
  administrator's grant; the merge now keeps the existing assignment and its source (Java's rule).
- **Encoded event `data` and audit `operationData`** (Rust SDK README ambiguity 1). The Java,
  TypeScript, Laravel, Rust (`fc-sdk` and the new SDK) and framework outbox writers send them as a
  JSON string holding the encoded value (`"data":"{\"a\":1}"`, Java `writeValueAsString(Map)`,
  TS `JSON.stringify(Record)`, PHP `json_encode(array)`); the Go SDK sends the value itself. The
  outbox processor posts each row's payload unchanged (`internal/outbox/http_dispatcher.go`), and
  the batch endpoints stored the string: `msg_events.data` a JSONB string, which the fan-out copies
  into the job's payload (`internal/stream/fan_out.go`), so the delivery envelope's `data` was a
  string and a `dataOnly` body a quoted string (`dispatchjob/processing`); `aud_logs.operation_json`
  a string, so the audit view's `operationJson` parsed to a string, and the redaction backstop
  (`usecase.RedactAuditJSON`) could not see inside it (a password inside an encoded
  `operationData` was stored; the new test shows it). Java and Rust store it as sent too.
  **Fix** (`8bf67d1`, owner ruling 2026-10-12, unwrap at ingest): `sdk.UnwrapEncodedJSON` on
  `POST /api/events/batch` (`data`) and `POST /api/audit-logs/batch` (`operationData`, before
  redaction). **The rule**: a member that is not a JSON string is stored as sent; a string whose
  text, less surrounding JSON whitespace, starts with `{` or `[` and is exactly one valid JSON value
  is replaced by that value; every other string is stored as sent. At most one level: the result is
  an object or array, never a string, so nothing is unwrapped twice and a value encoded twice is
  kept. **Edge cases**: kept as strings: `"hello"`, `""`, `"42"`, `"true"`, `"null"` (Java's
  encoding of a null map: unwrapping it would make the event `data is required` and refuse it),
  `"\"x\""`, `"{not json"`, `"[1,"`, `"{} trailing"`, `"{}{}"`; unwrapped: `"{}"`, `"[]"` (PHP's
  `json_encode([])`), encoded objects and lists with nested string members left intact. **Accepted
  cost**: a direct API caller that means a string whose text is a JSON object or array gets the
  object; there is no way to tell it from an SDK's encoding (none of the platform's writers sends
  one). Not changed: the singular `POST /api/events` (receives the value itself; a test pins that
  a string stays a string), dispatch-job `payload` (a string by contract, with
  `payloadContentType`), function-emitted events (`function/control/events.go`), the outbox
  processor. Documented in `docs/wire-contract.md`.
  - **Stored rows** (not changed, not inspected: the dev database is off limits). Rows ingested
    from the string-writing SDKs before this fix are `jsonb_typeof(data) = 'string'` in
    `msg_events`, the same text quoted in `msg_events_read.data` (TEXT), the quoted string as
    `msg_dispatch_jobs.payload` (TEXT) of every job fanned out from them, and
    `jsonb_typeof(operation_json) = 'string'` in `aud_logs`. To size it on a database:
    `SELECT count(*) FROM msg_events WHERE jsonb_typeof(data) = 'string' AND left(ltrim(data #>> '{}'), 1) IN ('{', '[')`
    (and `aud_logs` on `operation_json`). **No migration written**: the tables are partitioned
    and large, dispatch jobs and their read rows record what was delivered (rewriting them changes
    history and a redelivery's body), the rule needs a JSON-validity test per row in SQL
    (`pg_input_is_valid(..., 'jsonb')` is Postgres 16+), and the audit rows would also need the
    redaction that could not run on them. Owner to decide (see the question below).
- **Go SDK** (Rust SDK README ambiguity 2, and two found with it). `outboxpgx` wrote snake_case
  audit rows (`entity_type`, `principal_id`, `operation_json`, ...) that the camelCase-only
  `POST /api/audit-logs/batch` refused, every row "principalId is required". **Fix** (`69857fb`):
  the row is the endpoint's item (`entityType`, `entityId`, `operation`, `operationData` as an
  object, `principalId`, `performedAt`), plus `applicationCode` from a new optional
  `Config.ApplicationCode`, which the endpoint requires of an application-scoped credential (an
  application's own service account; without it the row is refused "applicationCode is required
  for an application-scoped credential"). Events stay snake_case (accepted through the aliases).
  `client.ListInstanceLogs` decoded the platform's bare array as `{logs,total}` and failed; the
  runner's `HandlerContext.Log` omitted the required `level`, so every log line was refused.
  **Fix** (`0edbd15`): the array is decoded and wrapped (`Total` = lines returned; the log
  response gained `scheduledJobId`, `clientId`), `Log` and `LogForInstance` send `INFO` unless a
  level is given; the test platform mock now answers as the platform does (204, `level` and
  `message` required). Nothing else under `clients/` vendors the Go SDK (`pkg/fcsdk`). Not
  released or tagged.
- **Dispatch-jobs `clientCode`** (framework `contracts/write-path.md` 7a, Rust SDK README 5):
  **not implemented**, a design question. The change is small (resolve `clientCode` when no
  `clientId` is given, then the batch's existing `CanAccessClient` guard), but "as the events batch
  does" is not one rule: Go's events batch leaves an unknown code unscoped and checks no client
  access at all (a new finding below), Java's leaves it unscoped for an anchor and refuses it, 403,
  for anyone else (S3.2, `requireWritableClient`), and the dispatch batch refuses the whole batch
  where the events batch reports per item. An unknown code silently unscoping a job would deliver
  it with no `X-FlowCatalyst-Client` header. Neither Java nor Rust accepts `clientCode` on dispatch
  jobs. Proposal for the owner: Java's events rule (explicit `clientId` wins; a known code resolves
  and must pass the client guard; an unknown code 403 for a non-anchor and, unlike Java's events,
  also 400 for an anchor, since a job is not a fact to keep), the same on the singular
  `POST /api/dispatch-jobs` (it shares `BatchItem`), mirrored in Java and Rust. **Ruled and
  implemented in F14.**
- **Wire impact**: `passwordHashIgnored` added to `SyncUsersResponse` and `SyncResultResponse`
  (optional, omitted when empty), and both `passwordHash` descriptions; the lock and the SPA's
  generated types were regenerated (`make api-bump`, `pnpm api:generate`). The application sync
  keeps other applications' roles; the batch endpoints store encoded members as objects. No status
  code changed.
- **Tests** (each failed before its fix): `principal/operations/sync_principals_pg_test.go`
  `TestSyncPrincipals_PasswordHashCarry` (now: kept, rest applied, the ignored list normalised and
  deduplicated); `principal/api/sync_password_hash_pg_test.go` and
  `sdksync/sync_principals_password_hash_pg_test.go` (both routes, Java's test case: kept, new user
  takes its hash, the field and its omission); `sdksync/sync_principals_own_roles_pg_test.go`
  (another application's role survives the sync and its sweep, an `ADMIN_ASSIGNED` role keeps its
  source through the sync and the sweep, the platform route still replaces every `SDK_SYNC` role);
  `shared/sdk/encoded_json_test.go` (the rule's cases and idempotence);
  `event/api/encoded_data_pg_test.go` (the Rust SDK's `event-full.json` and `event-minimal.json`
  golden payloads, encoded arrays, kept strings, the singular create unchanged);
  `shared/sdk/audit_encoded_operation_data_pg_test.go` (the Rust SDK's `audit-log-full.json`, a
  password in an encoded `operationData` now redacted, an object and a plain string kept);
  `shared/sdk/go_sdk_audit_pg_test.go` and `event/api/go_sdk_event_pg_test.go` (the Go SDK's
  `outboxpgx.Sink` writes into `outbox_messages` on the embedded database, and the stored payload
  goes through the batch handler: audit accepted for an all-applications and an
  application-scoped credential, redacted object stored; event data stored as sent);
  `pkg/fcsdk/outboxpgx/payload_test.go`; `pkg/fcsdk/client` `TestScheduledJobsListInstanceLogsDecodesTheArray`,
  `TestScheduledJobsLogForInstanceDefaultsTheLevel`; `pkg/fcsdk/scheduledjobs` `TestLogSendsALevel`
  and the level assertion in `TestProcessDispatchesHandlerAndReportsSuccess`. The Rust SDK's Go
  conformance test (`clients/rust-sdk/conformance`) still passes.
- **Java/Rust mirror**: password hashes and own-application roles already match (Go now follows
  them). **To mirror**: the ingest unwrap (Java `IngestApi` and Rust `batch_create_events` /
  `sdk_audit_batch_api.rs` store the string as sent), and the own-role merge's keep-the-existing-
  source rule in Rust (not checked).

### F14. Ingest client guard, dispatch-job `clientCode`, default event `source` (owner rulings 2026-10-12)

The shared helpers are in `internal/platform/shared/sdk/client_code.go` (`ClientCodes`, a
per-request memoised `clientCode` → client id resolver, one `FindByIdentifier` per distinct code;
`NonBlank`; `RequireWritableClient`; `ResolveEventClient`; `ResolveJobClient`). Throughout, a
`clientId` or `clientCode` that is absent, `null`, empty or only whitespace counts as not given
(Rust's `non_blank`; before, a blank `clientId` was stored as `""` for an anchor and refused for
anyone else).

- **Dispatch-jobs `clientCode`** (F13's design question; framework `write-path.md` 7a, Rust SDK
  README 5). **Fix** (`fe25501`): `POST /api/dispatch-jobs/batch` items and the singular
  `POST /api/dispatch-jobs` body accept `clientCode` (the client's identifier slug; never stored).
  **The rule** (`ResolveJobClient`), in order:
  1. a `clientId` wins (the code is then ignored, even one naming another client) and must pass
     the existing guard: 403 `FORBIDDEN` "No access to client: `<clientId>`";
  2. else a `clientCode` that names a client the caller reaches resolves to that client;
  3. a code naming a client the caller does not reach, **or no client**, from a non-anchor: 403
     `FORBIDDEN` "No access to client: `<clientCode>`" (one refusal for both, naming the code: it
     neither confirms that a code exists nor reveals another tenant's client id);
  4. a code naming no client from an anchor: **400 `CLIENT_NOT_FOUND` "Client not found:
     `<clientCode>`"** (the shape Go uses for an unknown reference in a body: `ROLE_NOT_FOUND`
     "Role not found: …", `APPLICATION_NOT_FOUND` "Application not found: …"; 400 is terminal in
     the outbox processor, which retries a 404). A job is never written unscoped because of an
     unknown code: unscoped, it would be delivered with no `X-FlowCatalyst-Client` header;
  5. neither: no client (unchanged; see the open divergence below).

  As before on this route, any refusal rejects the whole batch before anything is written (no
  `items[i]` prefix, as the client guard had none). No platform-scoped refusal was added here (not
  in the ruling). The routes are chi-mounted, outside the OpenAPI lock: no lock change.
- **Events client guard** (F13 "new findings"). `POST /api/events/batch` (and `/bff/events/batch`,
  the same handler) stored any `clientId`, or the client a `clientCode` resolved to, with no
  access check, and wrote a platform-scoped event for any caller; the singular `POST /api/events`
  checked access but let a non-anchor with no clients write a platform-scoped event. **Fix**
  (`abd2ae9`, Java `IngestApi.requireWritableClient`, Rust
  `caller_reach::require_writable_client`, their codes and messages):
  - `RequireWritableClient`: a `clientId` the caller cannot reach is 403 `FORBIDDEN` "No access to
    client: `<clientId>`". With no client: an anchor writes a platform-scoped event; a caller whose
    `clients` is exactly one id (not `*`) writes under that client; anyone else is 403 `FORBIDDEN`
    "clientId is required: a client-scoped caller cannot write platform-scoped rows".
  - Batch (`ResolveEventClient`): a `clientId` wins and goes through `RequireWritableClient`; else a
    `clientCode` naming a client the caller reaches resolves to it; one naming another client or
    none is 403 `FORBIDDEN` "No access to client: `<clientCode>`" for a non-anchor; an anchor's
    unknown code leaves the event platform-scoped (Java's and Rust's rule, F13's "the event is a
    fact"); no client at all goes through `RequireWritableClient`. Order per item: the per-item
    `BAD_REQUEST` checks (type, source, data), then application confinement (per item), then the
    client guard, whose refusal rejects the **whole batch** (nothing written), as Java and Rust.
  - Singular: unchanged default (no `clientId` from a non-anchor with clients: its first client;
    Java's §5 D3), then `RequireWritableClient`. No `clientCode` on the singular (as Java).
  - Application-scoped service accounts are unaffected: every application-provisioned account is
    `ANCHOR` (`serviceaccount/operations/client_reach.go`); the confinement tests still pass.
    A client-linked account (`CLIENT`, one client) now writes its events under its client when it
    names none (before: platform-scoped).
- **Events without `source`** (F13 "new findings", Rust SDK README 10). **Fix** (`4c3654a`): both
  `POST /api/events/batch` and `POST /api/events` default an absent, `null` or blank `source`
  instead of refusing it. **The default**: the event type's `{application}:{subdomain}`, its first
  two `:` segments of the trimmed type (`orders:sales:order:created` → `orders:sales`); the first
  segment alone when there is no non-blank second (`orders` → `orders`). **Evidence**: every
  platform event carries exactly that (`platform:admin` for `platform:admin:client:created`), the
  framework's event metadata rule is source = `{application}:{domain}` (`write-path.md` §6), the
  Go SDK's domain events do the same (`gosdk:sales`), and the framework already defaults a
  dispatch job's source from its code's application segment (7a). It depends on the event alone
  (not on the caller, so all three platforms compute it identically), and the caller's
  application code is not knowable for an all-applications credential. Java and Rust refuse it in
  the batch ("source is required"); Java's singular stores `""`, Rust's singular requires it. A
  type with a blank first segment (`:sales:order:created`) still answers "source is required"
  (batch item `BAD_REQUEST`, singular 400 `VALIDATION`). A given source is stored as sent.
  `CreateEventRequest.source` is no longer required (lock and SPA types regenerated, both
  `source` descriptions state the default). The Rust SDK's `event-minimal.json` now passes the
  batch handler (`encoded_data_pg_test.go`, stored source `orders:sales`).
- **Wire impact**: dispatch-job items and the singular body gained optional `clientCode`; a
  dispatch-job batch with an unknown code is now refused (400 for an anchor, 403 otherwise) where
  the code was ignored; event ingest now 403s writes it accepted (above); `source` optional on
  `CreateEventRequest`, and a missing source succeeds where it was 400/`BAD_REQUEST`. Documented in
  `docs/wire-contract.md` ("SDK ingest: client and source").
- **Tests** (each failed before its fix): `shared/sdk/dispatch_job_client_code_pg_test.go`
  (`TestDispatchJobs_ClientCode`: anchor resolves a code, explicit id wins, blank code ignored,
  unknown code 400 and nothing of the batch written, client-scoped own code resolves, another
  client's code and an unknown code the same 403 naming the code, the singular route's three
  cases); `event/api/client_guard_pg_test.go` (`TestEventIngest_ClientGuard`: the three 403s with
  a valid item before them not written, own id/code/none, explicit id over code, partner and
  no-client callers refused without a client, an anchor's known, unknown and absent code, per-item
  validation before the guard, the singular route); `event/api/source_default_pg_test.go`
  (`TestEventIngest_DefaultsAMissingSource`: absent, `null`, blank, snake_case type, given, one-
  and two-segment types, blank application; the singular route through huma); and
  `event/api/encoded_data_pg_test.go` (the minimal fixture now `SUCCESS`).
- **Java/Rust mirror** (Java `IngestApi`, Rust `fc-platform-messaging` `shared/batch_api.rs`,
  `event/api.rs`, `dispatch_job/api.rs`, `fc-platform-core` `caller_reach.rs`):
  1. Dispatch-job `clientCode` on the batch item and the singular body, with `ResolveJobClient`'s
     rule and codes above (neither accepts the field today).
  2. Event `source` default in both event routes (Java `invalidBatchEventItem` and
     `EventIngestMapper`, which stores `""` on the singular; Rust `batch_api.rs` line ~178 and the
     singular `CreateEventRequest.source: String`).
  3. A `clientCode` that resolves to a client the caller cannot reach: Go answers "No access to
     client: `<clientCode>`"; Java and Rust pass the resolved id to `requireWritableClient`, so
     their message carries the other tenant's client id (and differs from the unknown-code
     message, confirming the code exists). Mirror Go's message.
  4. Java treats only `null` as no `clientId` (a blank one is refused for a non-anchor, stored
     `""` for an anchor); Go and Rust treat blank as absent.
  Already identical: the events guard's statuses, codes and messages, the single-client default,
  whole-batch refusal, the anchor's unknown-code-unscoped rule.

## Decisions taken conservatively (owner to confirm)

1. Lifecycle writes need **update**, not the dedicated seeded permissions that no check reads
   (`user:activate`, `user:deactivate`, `scheduled-job:pause`, `event-type:archive`,
   `process:archive`). `user:assign-roles` is no longer in this list: the owner ruled it
   canonical for user role changes (F7, 2026-10-11).
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
15. Role ceiling (F8): platform permissions only (application permissions are outside it, as in
    Java and Rust); the change is checked, removal included; the mint, provisioning and the
    app-scoped principal sync are bounded too (beyond Java/Rust); the mint reuses
    `ROLE_ABOVE_CALLER`.
16. Unemitted event types (F9 follow-up): subscription bindings to the 16 pruned codes are
    archived (`msg_subscription_event_types_archive`), not deleted outright and not re-pointed at
    the emitted equivalent; a subscription left without bindings keeps its status. The 28 schema
    fixes replace a database's 1.0 only while it is exactly as seeded.
17. Permission ceiling (F10): the same rule as Java and Rust (platform permissions only, the
    change checked, removal and role deletion included); the SDK and platform role syncs are
    bounded too (beyond Java/Rust).
18. Function event codes (F11): only `platform:admin:function:deleted` (same data as Java/Rust)
    was renamed; its bindings follow migration 073, and when the new code is already a row the old
    duplicate row is deleted. Wildcard bindings are not rewritten. The other 14 wait on the list in
    F11.
19. Confinement and sync reach (F12): confinement checks only what a write adds (Rust's rule; Java
    checks the whole supplied list), so existing roles holding foreign permissions keep working; a
    super-admin may cross applications on create, update and grant, never through a sync. The
    application principal sync refuses a non-role name by its foreign prefix and accepts unprefixed
    or own-prefixed non-roles (Java refuses every unknown name, Rust only `platform` or an existing
    application's prefix). The platform principal sync holds a non-anchor to the
    `PUT /api/principals/{id}/roles` rule (Java's), so it cannot give a user it creates any role.

20. Principal syncs and ingest (F13): `passwordHashIgnored` lists trimmed, lower-cased emails, read
    before the sync (Java's and Rust's timing); a named role already held from another source keeps
    that source (Java's). The ingest unwrap takes only an encoded object or array, never a scalar
    (so Java's `"null"` stays a string rather than refusing the event), and leaves the singular
    `POST /api/events` alone. Already-stored string rows are not migrated. The Go SDK gained
    `Config.ApplicationCode` for application-scoped credentials. Dispatch-jobs `clientCode` is not
    implemented (design question in F13; ruled and implemented in F14).
21. Ingest (F14): an anchor's unknown dispatch-job `clientCode` is 400 `CLIENT_NOT_FOUND`, a
    non-anchor's 403 "No access to client: `<clientCode>`" (the same as another tenant's code, and
    the same for events); a code-resolved refusal names the code, not the resolved id (Java and
    Rust name the id); blank `clientId`/`clientCode` count as absent; the dispatch-job routes did
    not gain the platform-scoped refusal. The missing event `source` defaults to the type's
    `{application}:{subdomain}`, not the caller's application.

## New findings, not fixed

Fixed in the second batch (above): the scheduled-job runner reports (F1), event-type `clientId`
(F2), client event names (F3), service accounts (F4), the sync paths' client identifier (F5), the
SPA's sync and role controls (F6); after it, user role changes (F7), the role ceiling (F8) and
the catalogue drift (F9, with its follow-up: unemitted codes pruned, older schemas fixed), the
permission ceiling (F10), role confinement and principal-sync reach (F12), the principal
syncs' password hashes and other applications' roles (F13), and the events client guard and the
missing event source (F14).

- **`DELETE /api/event-types/{id}`** is documented "Archive an event type" and the TS SDK calls it
  `archive()`, but it hard-deletes (now only an archived or never-published type).
- `GET /api/dispatch-jobs` documents a default of 50 rows; the repository's default is 100.
- **Event-type client scoping is dead code** (F2): the read paths filter on a client that is never
  stored.
- ~~**The event-type catalogue lags what is emitted**~~ (F3): the 79 uncatalogued codes were
  catalogued in F9, the 16 never-emitted ones pruned in its follow-up.
- **Route permissions not in the seed**: the SPA's `ROUTE_PERMISSIONS` and the `/ui` nav gate pages
  on codes no role grants (`platform:iam:identity-provider:view`, seeded `platform:iam:idp:view`;
  `platform:iam:oauth-client:*`, seeded `platform:auth:oauth-client:*`;
  `platform:admin:audit:view`, seeded `platform:admin:audit-log:view`; `platform:admin:cors:view`,
  seeded `platform:admin:cors-origin:view`; `platform:admin:settings:view`, not seeded), so only
  wildcard holders reach those pages.
- **The dashboard's OpenAPI sync** is shown to everyone; `POST /bff/developer/sync-platform-openapi`
  needs anchor and `developer:application-openapi:sync` (the same exposure as F6, not in scope).
- ~~**Service-account role assignment is unbounded**~~: fixed in F8.
- ~~**Role permission edits are not bounded**~~: fixed in F10.
- ~~**Role permissions are not confined to the role's application**~~ (F10): fixed in F12.
- **IdP role mappings are not bounded** (F8): a mapping from an IdP group to
  `platform:super-admin` grants it at the next login. Java (`CreateIdpRoleMapping`,
  `DeleteIdpRoleMapping`) and Rust (`create_idp_role_mapping.rs`, identity-provider create/update)
  apply the ceiling.
- ~~**The principal syncs have no target reach or application confinement**~~ (F8): fixed in F12.
- ~~**The application principal sync replaces every `SDK_SYNC` role of a user it lists**~~: fixed
  in F13.
- ~~**The principal syncs overwrite an existing user's password hash**~~: fixed in F13.
- **Rotating a service account's OAuth client secret** (`platform:auth:oauth-client:regenerate-secret`,
  anchor) hands out a credential with the account's whole authority through `client_credentials`,
  with no role ceiling (the mint now has one). Java and Rust not checked.
- ~~**The events batch has no client guard**~~ (F13): fixed in F14.
- **The dispatch-job routes have no platform-scoped refusal** (F13, F14): `POST
  /api/dispatch-jobs/batch` and `POST /api/dispatch-jobs` check access to a given or resolved
  client, but a non-anchor that names none still writes a platform-scoped job (Java and Rust apply
  `requireWritableClient` there too: one client → that client, otherwise 403 "clientId is
  required: …"). Not in the F14 ruling; Java's comment notes an SDK outbox never sends a
  `clientId` on a dispatch job, so a multi-client credential's jobs would be refused.
- ~~**An event without `source` is refused**~~ (F13): fixed in F14 (defaulted).
- **Already-stored encoded event data and audit rows** (F13): not migrated; owner question in F13.

## Not verified

- No browser run of the SPA or `/ui` changes (unit, vitest and integration tests only).
- The Java/Rust ports (paused); the SDKs were not regenerated or released (`make sdk-spec`
  deferred to the SDK release; the lock's only change in the second batch is F2's description).
- Second batch: migration 070 ran on embedded scratch databases only (Up, re-run, Down by
  executing its SQL; through goose on every test database), never on a deployed one; the gated
  `TestDropInSafety` was not run; no browser run of the SPA role and sync controls.
- Routing of a newly created client-scoped pool end to end through a running router.
- F7: no browser run of the SPA role controls. The Java and Rust checks were read, not run
  (Java `Access.requireRoleAssigner`, `PrincipalApi`; Rust `authorization_service.rs`
  requires `USER_ASSIGN_ROLES`).
- F8: no browser run of the SPA pickers' locked roles or the `/ui` and SPA provisioning button;
  no run against a seeded or deployed database (integration tests create their own roles; the
  provisioning test installs `platform:application-service` from the seed catalogue). The Java and
  Rust code was read, not run. fcdev init was not run end to end; its provisioning path is
  covered by the no-caller case of `TestProvisionServiceAccount_GateAndRoleCeiling`.
- F10: the browser run covered the role editor and list only (not the dashboard's role sync card);
  no run against a deployed database. The Java and Rust code was read, not run.
- F12: no browser run (no SPA or `/ui` change); no run against a seeded or deployed database
  (integration tests create their own roles, applications and users). The Java and Rust code was
  read, not run.
- F11: migration 073 ran on embedded scratch databases only (Up, re-run, Down by executing its SQL;
  through goose on every test database), never on a deployed one or one a Java/Rust seeder shared.
  The Java and Rust events were read, not run.
- F13: no run against a seeded or deployed database, and the stored-row query was not run (the dev
  database is off limits); no browser run (the SPA only gained a generated type). The Go SDK was
  exercised through the batch handlers, not through a running outbox processor. The Java and Rust
  code was read, not run.
- F14: integration tests on embedded databases only; no run through a running outbox processor
  or a deployed platform; no browser run (the SPA only gained a generated type, `vue-tsc -b`
  passes). The Java and Rust code was read, not run.
