# Server-rendered UI (`/ui`)

The Vue/PrimeVue SPA is being replaced, area by area, by a server-rendered UI built from the
FlowCatalyst UI kit (`flowcatalyst-framework/ui-kit`: HTML + Tailwind + Datastar) with
[templ](https://templ.guide) components. Plan and rulings: `flowcatalyst-framework/docs/ui-dogfood-plan.md`.
The new UI lives under `/ui` next to the SPA, which stays the default: no SPA route redirects to
`/ui` and no SPA code is removed without the owner's approval, area by area.

Status (2026-10-10): foundations, the Phase 1 kit decisions (row actions, toolbar state part,
flash stack and the Datastar error contract, an icon sprite, now 90 icons), and `/ui/dispatch-pools` and
`/ui/connections` at parity with the SPA pages: list, record drawer, create and edit dialogs,
state actions and delete behind confirmations; the app shell works at phone width and dark mode is
rendered by the server from a theme cookie. Phase 3 (core operations): `/ui/event-types` and
`/ui/subscriptions`, `/ui/applications`, `/ui/clients`, `/ui/scheduled-jobs`, `/ui/dispatch-jobs`,
`/ui/events` and `/ui/processes` at parity (sections below; Processes shows the diagram's source, not
the rendered diagram until 2026-10-24; now drawn by the Mermaid island, see "Islands"). Pages, Datastar streams and assets are compressed
("Compression" below).

## Architecture

```
internal/ui/
  ui.go          Mount: the /ui route group (assets, guards, pages, sign-out)
  ds/            the only package that imports the Datastar Go SDK (datastar-go v1.2.2)
  kit/           templ components, one per kit spec; Props structs mirror the specs' Props tables
  htmlnorm/      HTML normaliser behind the conformance test
  shell/         page layout around the kit app shell; sidebar navigation; error pages
  guard/         CSRF, session negotiation, role-less gate, security headers
  compress/      Brotli/gzip for pages and Datastar streams (middleware), Accept-Encoding negotiation
  pages/         page handlers: dispatch_pools*.go/.templ, connections*.go/.templ and
                 event_types*.go/.templ (list, drawer, form, actions), list.go (the URL as
                 list state: search, status, multi-valued facets, sort, page and rows),
                 clients.go (the client select's search), forms.go (form plumbing),
                 ops.go (PoolOps, ConnectionOps: the use cases), flash.go (the flash key table)
  assets/        embedded kit.css, icon sprite, pinned datastar.js, licences (served at /ui/assets/)
  testdata/fixtures/  the kit's conformance fixtures, vendored
internal/server/wire_ui.go   registerUI: mounts the above with the platform's Authenticator
internal/schemacodegen/      Go port of the SPA's schema code generator, highlighter and zip
                             export (frontend/packages/schema-codegen, utils/schema-highlight.ts,
                             utils/schema-export.ts), byte-identical to the TS on 47 golden inputs
```

Request path: `compress.Handler` → `CorrelationID` → the platform `Authenticator` (same configuration as the `/api`
group, so the `fc_session` cookie is read and resolved the same way) → `SecurityHeaders` → `CSRF`
→ `RequireSession` → `ProfileOnly` → sprite context → page handler. Assets (`/ui/assets/*`) are
public and served before the guards.

Handlers read repositories and run use cases directly (no HTTP hop) and make the **same
permission check as the API handler for the resource** (`auth.CanReadDispatchPools` and the same
`FilterClientScoped` as `GET /api/dispatch-pools`). Mutations run the API's use case through
`usecaseop.Run` with the unit of work and `auth.NewExecutionContext` (`pages.PoolOps`), so
validation, resource authorization (`CheckScopeAccess`), the domain event and the audit record
are the use case's; the page adds nothing to the business rules. Pages take small interfaces
(`PoolStore`, `PoolCommands`, `ClientDirectory`) so handler tests run on fakes and
`*_pg_test.go` (tag `integration`) runs the same handlers on Postgres.

## Toolchain

| Command | What |
|---|---|
| `make ui-generate` | `go tool templ generate -path internal/ui` (templ is pinned by the `tool` directive in go.mod; no global install) |
| `make ui-fmt` | `templ fmt internal/ui` |
| `make ui-verify` | regenerates and fails on a diff, an uncommitted `_templ.go` or an unformatted `.templ`; part of `make ci` and a CI step |
| `make ui-kit-sync` | rebuild `kit.css`, copy sprite/notices/fixtures from the kit (below) |

Generated `*_templ.go` files are committed, like sqlc output. Always generate with
`make ui-generate`: templ records file names relative to `-path`, so generating a single
directory produces a different (stale-looking) file.

golangci-lint skips the generated files by their `Code generated ... DO NOT EDIT` header.

## Conventions

### Components (`internal/ui/kit`)

- **One templ function per kit spec** (`Button`, `Dialog`, `DataTable`, ...), plus parts where
  the spec has them (`TR`, `TH`, `TD`, `SortableHeader`, `EmptyRow`, `FiltersButton`, ...).
- **Props**: `XxxProps` struct; field = the spec's camelCase prop name with a capital first letter,
  `json` tag = the prop name (so fixtures decode straight into it). Enums are typed strings with
  constants (`kit.ButtonOutline`). Booleans default to false, so an on-by-default option is named
  for its opposite (`Persistent` turns dialog light dismiss off).
- **Slots**: the default slot is templ children; named slots are `templ.Component` fields with
  `json:"-"` (`PageHeaderProps.Actions`, `DialogProps.Footer`, `TableToolbarProps.Filters`).
- **Pass-through**: `Class` is appended after the spec's classes (never merged; override with
  `[&]:...`), `Attrs templ.Attributes` go on the part the spec names (Datastar handlers, aria).
- **Markup is the spec's**, class strings copied verbatim. A spec change starts in the framework
  repo (spec + gallery + fixture), then `make ui-kit-sync`, then the templ change.
- **Ids and signal names** that end up inside a Datastar expression go through `kit.ID` /
  `kit.Signal` (panic on anything but `[A-Za-z][A-Za-z0-9_-]*` / camelCase): they are
  programming constants. **User data never goes into a `data-*` expression**; it goes into text or
  plain attributes, which templ escapes.
- Icons: `kit.Icon(kit.IconProps{Name: "x"})` references the sprite at the URL in the context
  (`kit.WithSpriteURL`; the `/ui` group sets the hashed asset URL, tests use the spec's
  `/icons/lucide-sprite.svg`).

### Pages (`internal/ui/pages`)

A page is a struct with its dependencies (repository interfaces + `*shell.Shell`) and a
`Mount(chi.Router)`. The handler, in order:

1. `ac := auth.FromContext(ctx)` and **the same `auth.Can*` check as the API handler**; on error
   `h.Shell.Fail(w, r, err)` (403 page for navigations, bare 403 for Datastar).
2. Parse the view state **from the URL query** into a small struct with defaults for anything
   invalid (`parsePoolQuery`); every filtered/sorted view has a URL (`q.URL()`).
3. Read through the repository / use case; apply the API's client scoping.
4. Build a **view model** (strings ready to render; links computed in Go).
5. If `ds.IsDatastar(r)`: open `ds.NewSSE`, patch the regions that changed by id
   (`sse.Patch(poolsTable(view))`, the toolbar's state part
   `sse.Patch(kit.TableToolbarState(view.toolbar().State()))`), and `sse.ReplaceURL(q.URL())`;
   otherwise `h.Shell.Render(w, r, shell.Page{Title, Path}, poolsPage(view))`.

Templates live next to the handler (`dispatch_pools.templ`): one top-level page component and
one component per patch target, each with a stable id (`pools-table`, `pools-summary`). Props
that both the page and a patch render (the toolbar) are built by one view-model method
(`poolsView.toolbar()`), so they cannot drift.

Mutations are POST only (the CSRF guard relies on GET being safe; native forms can only POST),
go through the use-case layer exactly as the API handler does, pass `{retryMaxCount: 0}` in
their Datastar action, and answer a Datastar request with patches (the changed table, a closed
dialog, `shell.Flash(sse, ...)` for feedback) or a native form post with a 303 redirect and a
flash key. Failures follow "Datastar errors" below. POST routes are mounted behind
`parseForm`: **read the form before opening the SSE stream**. Datastar posts forms as multipart
(`FormData`), and once a handler has flushed its stream headers net/http may refuse further body
reads, so a field read afterwards is silently empty (found in the browser: the drawer did not
refresh after an action; `httptest` does not reproduce it).

### Forms and actions (dispatch pools is the reference; connections follows it)

| URL | GET (navigation) | GET (Datastar) | POST |
|---|---|---|---|
| `/ui/dispatch-pools/new` | the list with the create dialog open | the dialog patched into `#pool-dialog-slot` | create |
| `/ui/dispatch-pools/{id}` | the list with the drawer open | the drawer content (`#pool-drawer`, mode inner), `selPool`, focus on the title | - |
| `/ui/dispatch-pools/{id}/edit` | the list with the edit dialog open | the dialog patched into the slot | update |
| `/ui/dispatch-pools/{id}/{suspend,activate,delete}` | - | - | the action |

- **Form dialogs** (kit `form-dialog.md`): `kit.FormDialog` + one `kit.Field` per control whose
  root id is `{control}-field`, rendered by one function (`poolField`) for the dialog and for the
  error patch. A failed submit patches every field, the form error region
  (`kit.FormDialogError`) and focus (`ds.Focus`), via `patchFormFailure`; use-case error codes map
  to fields (`poolFieldCodes`), the rest goes to the region with `shell.Classify`'s message.
  Success closes the dialog (`ds.CloseDialog`), re-renders the table the user is on (the view
  comes from the `Referer`, which live filtering keeps current) and flashes.
- **Dialog slot: replace, do not morph** (found on the processes page): patching a form dialog into
  its slot with `ds.Inner` morphs the slot's children, so a second dialog on the same page (create,
  closed, then edit: the same `<dialog id>`, the same `data-init`) is morphed in place and its
  `data-init` (`showModal()`) does not run again: it opens non-modal, below the drawer, without the
  focus trap. `patchDialogSlot` (`forms.go`) replaces the slot element, so the dialog is new every
  time. Every page's dialogs go through it (`8b8322e`).
- **Confirmations**: one `kit.ConfirmDialog` per action, shared by the rows and the drawer
  (`kit.OpenConfirm` sets the form's action from the trigger's `data-action`); it posts with
  `kit.SubmitConfirm` and closes at once, the outcome is a flash. Only the dialogs the user may use
  are rendered.
- **Drawer state for the server**: forms carry `<input type="hidden" name="drawer"
  data-attr:value="$selPool">`, so an action refreshes the drawer when it shows that record and
  closes it after a delete.
- **Client select**: `kit.Autocomplete` with `/ui/clients/options` as its source
  (`client.Repository.Search`); the submitted identifier is resolved on the server
  (`resolveClient`). With `CanReadClients` every client is offered; a client-scoped user is offered
  the active clients it can access (another tenant's identifier is "No client with this
  identifier."), as the SPA's `ClientSelect` now is (`/bff/filter-options/clients`); anyone else
  (an anchor without `CanReadClients`) gets a disabled field that says why
  (`docs/platform-bug-fixes.md`).
- **Flash after a native post** (owner ruling: fixed keys): `Shell.SetFlash(w, key)` before the
  303 sets `fc_ui_flash` (HttpOnly, `Path=/ui`, SameSite=Lax, 60s) holding only the key; the next
  page render maps it through `Shell.Flashes` (`pages.FlashMessages`), renders it in the flash
  stack and clears the cookie. Unknown or malformed keys are ignored; no request text is ever
  rendered (`TestPoolCreateNativeFlash`).
- **Connections** (`/ui/connections`, same routes with `pause` instead of `suspend`): create needs
  `CanCreateConnections`, edit/pause/activate `CanUpdateConnections`, delete
  `CanDeleteConnections` (a connection with subscriptions is refused by the use case, 409
  `HAS_SUBSCRIPTIONS`, shown as an error flash). The service account is a native select of
  `serviceaccount.Repository.FindAll` (the SPA's `serviceAccountsApi.list()`, needs
  `CanReadServiceAccounts`; disabled with the reason otherwise); the edit form sends the SPA's PUT
  body (a blank description or external id clears it).
- **Permissions** are the API handlers' checks: create `CanCreateDispatchPools`; update, suspend
  and activate `CanUpdateDispatchPools`; delete `CanDeleteDispatchPools` (`c59e246`); the edit form
  sends a blank description and rate limit as "" and 0, which clear them (`d1158fa`); the drawer and the edit dialog also need `CanReadDispatchPools` and,
  for a client-scoped pool, access to its client (`GET /api/dispatch-pools/{id}`).

### Row actions

`kit.RowActions` (kit `row-actions.md`): at most two inline icon actions, the rest in a native
popover menu; destructive actions last in the menu (the kit adds the separator and the red),
behind a confirmation; an action not allowed **on this row** is shown disabled with
`DisabledReason`; an action the user has no **permission** for is left out (the same `auth.Can*`
check as the API). One `ID` per row (`pool-{id}-actions`), `RowLabel` = the row's name (it goes
into the accessible names). Dispatch pools show Edit inline (disabled with "Archived pools cannot
be edited." for an archived pool) and View details / Suspend… or Activate… / Delete… in the menu
(the drawer's actions, none for an archived pool).

### Filter bars (live)

The toolbar's server-rendered state (hidden sort fields, Clear All, the Filters badge) is one
part, `#{formId}-state` (`kit.TableToolbarState`, id from `kit.ToolbarStateID`). A live answer
patches that part, the table and the URL; never the search box (the user may be typing) or the
filters popover (it may be open). Clear All carries `ClearAttrs: {"data-on:click__prevent":
kit.ClearFilters}`: it empties the search box and the popover's fields, focuses the search box
and submits the form, so the server sees an ordinary request; without Datastar it is a link to
`clearHref` (the view without filters, sort kept).

### Datastar errors

Datastar 1.0.4 applies a response body **only for status 200**; any other status fires the
`datastar-fetch` `error` event and the body is dropped. So:

| Outcome | Who | Answer | The user sees |
|---|---|---|---|
| Success | handler | 200, patches (+ `shell.Flash` success/info) | the change, "Saved." |
| Validation, business rule, not found, permission check | handler | **200**: field errors patched into the form, the dialog's error region, or `shell.Fail` (an error flash with the use-case message; the real status is logged) | the message |
| Unexpected handler error | handler | `shell.Fail`: 200 + "Something went wrong." (logged, no details) | generic message |
| CSRF refusal, no access, role-less (403) | guards | bare 403 | flash stack fallback "The request was refused." |
| No session (401) | guard | bare 401 | navigation to the login page (listener on `<body>`), back to the page after sign-in |
| Panic, 404/405, proxy 5xx | router, proxy | non-200 | fallback "Something went wrong." |
| Network failure | - | none | fallback "The server could not be reached." |

A handler that reached its own logic never returns an empty body or a non-200 to Datastar. Once
it has opened its stream it reports with `Shell.FailSSE(sse, r, err)`. The flash stack
(`kit.FlashStack`, id `flash-stack`, sticky at the top of `<main>`) is a polite live region;
success and info messages remove themselves after 3s/5s, warnings and errors stay until
dismissed and are `role="alert"`. The fallback shows at most one message of each kind.

### Datastar interactions

- Prefer the browser (links, GET forms, `<dialog>`, `popover`); add Datastar on top so the page
  still works as plain links/forms.
- **Filter forms send their fields, not signals**: `@get('/ui/x', {contentType: 'form'})` on the
  form's `submit`, the search input's `input__debounce.300ms` and each filter's `change`. A GET
  with `contentType: 'form'` puts the form fields in the query string, so the server reads one set
  of parameters for the full page and for the live update, and the URL can be replaced with the
  canonical one (`ds.ReplaceURL`). Hidden inputs carry the sort.
- Sort headings are real links (`href` = the sorted URL) with `data-on:click__prevent="@get(el.href)"`.
- Expressions are constants in Go (`liveGet`, `kit.ShowModal(id)`, `kit.CloseDialog`); never
  built from request data.
- Server → page: `ds.SSE` (`Patch`, `PatchHTML`, `Remove`, `PatchSignals`, `Redirect`,
  `ReplaceURL`). `Redirect`/`ReplaceURL` patch a self-running `data-init` element instead of a
  `<script>` because the CSP forbids inline scripts.
- Signals are page-global: name per instance (`selPool`, not `sel`).
- A 401 from any action navigates to the login page (one `data-on:datastar-fetch` listener on
  `<body>` in the layout); every other failure is the flash stack's (above).

### Theme and narrow screens

- **Theme** (kit `app-shell.md`, Theme): `shell.resolveTheme` reads `fc_theme` (the user's choice,
  `light`/`dark`, absent = system) and `fc_scheme` (the system scheme the page last observed) and
  renders `<html class="dark">` plus the `theme`/`dark` signals (`kit.ThemeHTMLAttrs`), so a dark
  page is dark on first paint. `?theme=light|dark` overrides one page (screenshots) and stores
  nothing. The user menu has a System/Light/Dark picker (`kit.ThemeChoices`). Limit, no inline
  script being allowed: in system mode a browser's first page (and the first page after the OS
  scheme changes) renders light and turns dark when Datastar runs; every later page is right on
  first paint (`TestResolveTheme`, `TestPageRendersTheThemeAndTheMobileShell`).
- **Narrow screens** (< 768px): the kit's top bar and off-canvas sidebar (`$sidebarMobileOpen`, always
  closed on load); collapse applies only on wide screens. Tables scroll inside their card; the
  page itself has no horizontal scroll at 390px (checked in headless Chrome, both pages, light and
  dark, dialog and drawer open).

### Event types (`/ui/event-types`)

Mirrors `EventTypeListPage`, `EventTypeCreateDrawer`, `EventTypeDetailDrawer`, `SchemaViewerDialog`
and `EventTypeAddSchemaPage`.

| URL | GET (navigation) | GET (Datastar) | POST |
|---|---|---|---|
| `/ui/event-types` (`?q&app&sub&agg&status&page&rows`) | the list | table, pager, toolbar state, facet summaries, dependent facet options, header actions, URL | - |
| `/ui/event-types/new` | list + create dialog | the dialog | create |
| `/ui/event-types/{id}` | list + drawer | drawer content | - |
| `/ui/event-types/{id}/edit` | list + edit dialog | the dialog | update (name, description) |
| `/ui/event-types/{id}/schemas/{version}` | list + drawer + schema viewer | the viewer dialog | - |
| `/ui/event-types/{id}/add-schema` | the add-schema page | - | add schema (then the drawer) |
| `/ui/event-types/{id}/{archive,delete}`, `.../schemas/{version}/{finalise,deprecate}` | - | - | the action |
| `/ui/event-types/sync-platform` | - | - | Sync Platform Events |
| `/ui/event-types/export` (`?` the view) | the zip download | - | - |

- **Reads** make `GET /api/event-types`'s checks: `CanReadEventTypes`, client scoping (event types
  have no client, so it never narrows; F2 in `docs/platform-bug-fixes.md`), and for a principal
  without the messaging view permission only its own applications' event types (the BFF makes the
  same checks since bug 1's fix). **Actions** make the check of the BFF endpoint the SPA
  calls: create `CanCreateEventTypes`; edit, archive, add schema, finalise and deprecate
  `CanUpdateEventTypes`; delete `CanDeleteEventTypes`; sync anchor scope and the event-type sync
  permission (`CanSyncPlatformEventTypes`).
  Each runs the BFF's use case through `pages.EventTypeOps`.
- **Filters**: the facets are kit `multi-select`s (repeated `app`/`sub`/`agg` parameters, the SPA's URL
  keys); options come from every visible event type of any status, cascade (subdomains of the chosen
  applications, aggregates of both) and chosen values that stop being options are pruned from the
  canonical URL, as the SPA prunes them. A live answer patches each facet's summary and the dependent
  options parts, never the open disclosure. Paging is the SPA's: 100 rows, 50/100/250/500
  (`kit.Paginator`, rows select associated with the toolbar form).
- **Drawer**: the SPA's guards for Archive (CURRENT, every version deprecated) and Delete (ARCHIVED, or
  CURRENT with every version finalising) are kept as disabled buttons with the reason written under
  them; the use cases enforce the same rules (`EventType.CanArchive`/`CanDelete`, bug 5). Edit and Add
  Schema are hidden for an archived event type, as in the SPA.
- **Schema viewer**: `schemacodegen` renders the six tabs (schema, example, TypeScript, PHP, Python,
  Java) on the server, highlighted with escaped text; copy uses the clipboard (`Copied` beside the
  button for 2s instead of a toast).
- **Export Schemas** downloads the zip the SPA builds in the browser, for the view the user is on
  (filters **and** search, all pages); with nothing to export it redirects with the warn flash.
- **Add schema**: the SPA's fields: version (suggested as the SPA does), MIME type, schema type and the
  schema. A JSON Schema must be a JSON document and is sent as one; an XSD or PROTO schema is text,
  sent as a JSON string; the use case stores each with the MIME and schema type given (bug 3).

Differences from the SPA: the create and edit forms are dialogs (the SPA uses the drawer: the
pattern ruled for pools); confirmations use one message per action without the version number;
the sync result flash shows the SPA's two lines as one; the export applies the search too (the SPA
exported the facet-filtered list, ignoring the search box).

### Subscriptions (`/ui/subscriptions`)

Mirrors `SubscriptionListPage`, `SubscriptionCreateDrawer` and `SubscriptionDetailDrawer`. Same routes
as connections (`/new`, `/{id}`, `/{id}/edit`, `/{id}/{pause,resume,delete}`), plus
`/ui/subscriptions/event-type-options?clientScoped=` (the event type list for the Client Scoped switch).

- Reads: `GET /api/subscriptions`'s checks (`CanReadSubscriptions`, client scoping). Actions check the
  permission for the operation: create `CanCreateSubscriptions`; edit, pause, resume
  `CanUpdateSubscriptions`; delete `CanDeleteSubscriptions` (the API's checks since bug 8's fix). Each
  runs the API's use case (`pages.SubscriptionOps`).
- List: search (code, name, connection id, application, client), Application multi-select (options from
  the subscriptions, pruned), status, sortable columns, 100-row pages.
- Create: the SPA's sections; event types are a searchable multi-select of CURRENT event types with a
  CURRENT schema matching the Client Scoped switch (the switch re-fetches the list and clears the
  choice, as the SPA does; the client field shows only for a client-scoped subscription). The bindings
  (id, code, current version) are built from the event types offered. Connection, dispatch pool and the
  event types are offered only to users who may read them (disabled with the reason otherwise).
- The pool code is looked up by id when a subscription has none stored (rows created before bug 11's
  fix; create and update store it now), so the Pool column and the drawer are not empty.

Differences from the SPA: dialogs instead of drawer forms; **Queue** is a select of the two values the
use case accepts (as the SPA now offers, bug 10); the create form sends the Client Scoped switch and
the Sequence (the API takes both on create, bug 9); the create form's "+ create connection" nested
dialog is not ported (create the connection on the Connections page first); the edit form's connection
is a select of active connections (the SPA's is a free-text id) and a blank choice leaves the
connection unchanged (the API now treats the SPA's blank id the same way, bug 12).

### Applications (`/ui/applications`)

Mirrors `ApplicationListPage`, `ApplicationCreateDrawer` and `ApplicationDetailDrawer`. Routes as
connections (`activate`/`deactivate`/`delete`), plus `/{id}/provision-service-account` and
`/{id}/provision-login-client` (POST).

- Reads: `GET /api/applications`'s checks (`CanReadApplications`, application confinement for an
  application service account; the drawer `CanReadApplication`). Actions check the permission for the
  operation (create, update for edit/activate/deactivate, delete; the API's checks since bug 13's
  fix); provisioning is anchor-only, as the API.
- Login-client provisioning runs `operations.ProvisionLoginClient`, the API handler's code moved into
  the operations package (`d5fdf43`) so both run the same thing.
- **Shown-once credentials**: the provisioning answer patches a dialog carrying the client id and
  secret (never a URL, a redirect or a flash; a native post gets the page itself, responses are
  `no-store`). It cannot be dismissed except by "I've saved the credentials", which removes it from
  the page; Chrome lets a second Escape through a prevented `cancel`, so a close that was not the button
  reopens it.
- The drawer's Documentation section lists the pages the application synced (`appdocs`, when the user
  may read platform documentation, as the SPA's `GET /api/docs`), linking to the SPA's documentation
  pages (not ported).

Differences from the SPA: dialogs for create/edit; the type is a kit toggle group (the SPA's
SelectButton); **redirect URIs are a textarea, one per line** (the SPA adds and removes chips; blank
lines and repeats are dropped); Delete is disabled with the reason "Deactivate the application first."
while active (the use case refuses it too, `APPLICATION_ACTIVE`, bug 14).

### Clients (`/ui/clients`)

Mirrors `ClientListPage`, `ClientCreateDrawer` and `ClientDetailDrawer`. Routes as connections
(`activate`/`suspend`/`deactivate`), plus `POST /ui/clients/{id}/applications` (the picker's Save).

- Every check is the API's, per operation and anchor-only (`CanReadClients`, `CanCreateClients`,
  `CanUpdateClients`, `CanActivateClients`, `CanSuspendClients`, `CanDeactivateClients`); the
  applications picker is the anchor-only `PUT /api/clients/{id}/applications` (`UpdateClientApplications`);
  without it the drawer lists the enabled applications read-only.
- The list is the SPA's: search over identifier and name, no filters; here the search covers every
  client and pages 100 rows (the SPA now does the same in the browser; the API is not paged, bug 16).
- The applications picker is the kit pick list (`pick-list.md`): ticked applications are posted as a
  list; Save stays disabled until the choice changes.
- **Login Branding** links to the SPA's page (`/clients/{id}/theme`): its theme editor is shared with
  the platform settings and moves with them (Phase 5).
- Suspend and Deactivate send the SPA's fixed reasons ("Manual suspension", "Manual deactivation").
  Deactivate sets the client INACTIVE and keeps it (bug 15); Activate reactivates it.

Differences from the SPA: dialogs for create/edit; the picker is the kit's dual pane (checkboxes and a
mirrored list) rather than PrimeVue's move-between-lists.

Templ gotcha found here: `{ attrs... }` writes a `templ.Attributes` map in **sorted** order, and Datastar
applies attributes in document order, so a `data-init` that reads a signal declared by `data-signals` in
the same map runs first and sees nothing (the picker's Save was enabled on load). Write such pairs as
explicit attributes, in order, before the spread.

### Scheduled jobs (`/ui/scheduled-jobs`)

Mirrors `ScheduledJobListPage`, the create and detail drawers, `ScheduledJobInstanceListPage` and
`ScheduledJobInstanceDetailPage`. Routes: the list (`?q&clients&applications&statuses&page&rows`),
`/new`, `/{id}`, `/{id}/edit`, `/{id}/{pause,resume,fire,archive,delete}` (POST), the firings page
`/{id}/instances?status&trigger&page&rows` and one firing `/instances/{instanceId}`.

- Reads are the BFF's (`/bff/scheduled-jobs`): `CanReadScheduledJobs`, filters in SQL with paging (20
  rows by default), a non-anchor user confined to their clients, platform jobs and firings anchor-only
  (not found otherwise), the "running" flag per row, client and application names. The filter options
  are the BFF's (Platform-scoped for anchors, active clients the user can access, active applications).
- Actions check the permission for the operation: create `CanCreateScheduledJobs`; edit, pause,
  resume, archive `CanUpdateScheduledJobs` (the API's checks since bug 17's fix); fire
  `CanFireScheduledJobs`; delete `CanDeleteScheduledJobs`. Pause, resume and Fire Now are immediate
  (as the SPA); archive and delete ask first; archive and delete close the drawer.
- The form keeps the SPA's client-side checks as field errors (code, name, a scope, at least one cron,
  JSON payload); the use case's cron and URL errors land on their fields.

Differences from the SPA: dialogs for create/edit; **cron expressions are a textarea, one per line**
(the SPA adds and removes rows); the firings and firing pages are server pages linked from the drawer
(as in the SPA), with the paginator.

### Dispatch jobs (`/ui/dispatch-jobs`)

Mirrors `DispatchJobListPage`, `DispatchJobDetailDrawer`, `RequeueByFilterDialog` and `RequeueSelectedDialog`.
Routes: the list (`?clients&applications&subdomains&aggregates&codes&statuses` (repeated), `q` (source), `group`,
`eventTypes` (comma list, at most 10), `subscription`, `connection` (exact, or ending in `*`), `window`
(`1h|3d|7d|31d`; none is the last 24 hours), `from`/`to` (YYYY-MM-DD), `lens=stuck`, `dir=asc`, `size`, `after`
(keyset position)), `/{id}` (drawer), `/{id}/sign`, `/{id}/requeue`, `/{id}/redrive`, `/{id}/cancel`,
`/{id}/complete`, `/{id}/payload`, `/{id}/audit` (GET, fills the drawer's Audit section), `/requeue` (ticked ids;
`step=plain|confirm|redrive`), `/requeue-by-filter` (GET: the confirm step, `step=redrive` for the second step;
POST: the run; the filter is the URL's query), `/requeue-errors` (the error preset).

- Every route makes the API's checks (`internal/platform/dispatchjob/api`): `dispatch-job:view`, SQL
  tenant scoping (`operations.VisibleTo`), per-job client scope on the drawer; **Sign needs
  `dispatch-job:view-raw`** and is omitted without it (as the SPA now does, bug 20). Requeue (row, drawer, selection, by filter) runs `ResendDispatchJobs` / `ResendByFilter` as the
  API does, with the API's gate (`dispatch-job:requeue`; Edit payload needs `dispatch-job:edit-payload`, Cancel job `dispatch-job:cancel`, Mark completed `dispatch-job:complete`, Redrive `dispatch-job:redrive-completed` on top of requeue; each control is hidden without its permission and the server stays the guard). A row or the drawer offers Requeue only for a status the API resets (PENDING, FAILED, ERROR, CANCELLED, EXPIRED, as `jobActions` in the SPA) and Redrive only on a COMPLETED job. The list, the filter dialog and its run carry the API's time window: no dates means the last 24 hours, at most `FC_DISPATCH_JOB_MAX_WINDOW_DAYS` (31); QUEUED and PROCESSING jobs are never reset, COMPLETED ones only by a confirmed redrive. The outcome flash says what the guard left alone ("2 jobs in flight (QUEUED or PROCESSING) left alone").
- **Window bar** (preset buttons, "Showing the last 24 hours", a "default window" tag when the server chose it) and two **quick views**: Failed (FAILED and ERROR) and Stuck (PENDING and QUEUED created more than five minutes ago: `lens=stuck` caps the window's end at now minus 5 minutes, only ever lowering a chosen end date; picking other statuses by hand drops the lens). A requeue by filter acts on the lens too.
- **Redrive** (re-delivering COMPLETED jobs) is the SPA's double confirmation, server-driven. Ticked rows that include COMPLETED ones, and a row's or the drawer's Redrive, open a dialog for a holder of `redrive-completed` (anyone else requeues at once and is told the COMPLETED ones were left alone): step 1 offers "Requeue N other jobs" or "Also redrive N COMPLETED jobs…"; step 2 asks for the word `REDRIVE` and the number of jobs the run resets, and a wrong entry runs nothing and says why on the field. The by-filter dialog does the same from its review step (`step=redrive`; the number is the eligible plus the COMPLETED count, sent as `confirmCount` with `redrive=1`; a changed match goes back to the review).
- **Edit payload** is in the drawer for a PENDING or FAILED job (the CodeMirror island, JSON when the content type is). **Audit** (audit-log view permission) loads when the drawer's section is opened; the **subscription and connection** lines link to their pages when the caller may read them (the bare id otherwise, as the SPA does best effort).
- **Created from/to stay UTC days** (wave 1 ruling, 2026-10-24: conservative). The SPA uses the browser's zone; a server-rendered page does not know it, and sending it would make the 31-day window depend on DST and on a hidden form field. A UTC day is always 24 hours, so a 31-day range is never refused by a day the user cannot see. The label says "UTC days", times in the table stay in the viewer's zone.

- **The filtered view's URL is what "Requeue all matching" acts on**: its link carries the view's
  filter (not size, sort or position) to `/requeue-by-filter?…`; the confirm step previews that filter
  (`SummarizeMatches`: count, by status, eligible, out of scope, the 10,000 cap) and its form posts to
  the same query with `confirmCount` = the eligible count shown. `FILTER_MATCH_CHANGED` re-renders the
  step with the new counts and the SPA's warning (HTTP 200 for Datastar, 409 natively); a run's outcome
  is the SPA's toast (requeued / remaining / nothing).
- Selection: native checkboxes associated with the `#dj-selection` POST form; one Datastar expression
  on the `<tbody>` counts them into `$djSel` (the "Requeue selected (n)" label) and sets the header
  box. A full page whose rows are all ticked shows the select-all banner, which leads to the
  requeue-by-filter step.
- Live filtering as the other lists; changing the applications, subdomains or aggregates patches the
  dependent multi-selects' options. Links that change the filters (a row's group, the error preset)
  send `X-FC-Toolbar: full` and get the whole toolbar back, so the popover's fields follow; a typed
  search never does (it would drop keystrokes in flight).
- **Paging**: the SPA has none ("most recent N", size 50–1000). The page keeps the size select and
  adds keyset paging over (created_at, id): **Older** carries `after=<unix µs>.<id>` of the last row,
  **Newest** returns to the first page (kit `cursor-pager`). `FilterParams.Cursor` was added to the
  repository for it (the list's ORDER BY gained `id` as a tie-breaker); the match count and the
  requeue walk ignore it.
- The table is patched in **replace** mode (not morph): a morph resets each `<time>` to its UTC
  fallback without re-running the localisation (kit `local-time.md`); the drawer's body likewise.
- The distinct statuses and codes (the API's filter-options, scoped to the caller's tenants since bug
  18's fix) are cached for 30 s per process and tenant scope: the SPA reads them once per visit, the
  live list on every request, and the two `DISTINCT`s cost ~10 ms each over 100k jobs.

Differences from the SPA:
- The application, subdomain and aggregate filters have options (the segments of the distinct codes,
  each narrowed by the levels chosen above it), as the SPA's now are (bug 19). A parent change keeps the
  children that still apply instead of clearing them.
- Created from/to are **UTC days** (the SPA uses the browser's time zone); the label says so. Times
  are shown in the viewer's locale and zone (kit `local-time`), as the SPA does.
- Selection is cleared by every list refresh (the SPA keeps the ticked rows still visible). The
  select-all banner offers "Requeue all matching jobs…" (the confirm step shows the count) rather than
  first counting and "selecting" all matching jobs.
- "Requeue everything in error" is a button beside "Requeue all matching" (the SPA's split-button menu).
- URL arrays are repeated parameters (`statuses=FAILED&statuses=ERROR`), not the SPA's comma list.

### Events (`/ui/events`)

Mirrors `EventListPage` and `EventDetailDrawer` (read-only). Routes: the list
(`?clients&applications&subdomains&aggregates&types` (repeated), `q` (source), `size`, `after`) and
`/{id}` (drawer). The API's checks (`internal/platform/event/api`): `event:view`, SQL tenant scoping
(platform events plus the caller's clients), per-event client access on the drawer (403 otherwise).
Event ids are untyped TSIDs and may start with a digit, so the path parameter is checked against its
own pattern (`[A-Za-z0-9_-]{1,64}`) and never enters an expression. The whole row opens the drawer
(one delegated handler on the `<tbody>` clicks the row's Event ID link).

Shared with dispatch jobs (`pages/firehose.go`): result size, keyset paging (Older/Newest;
`event.FilterParams.Cursor` added, `id` tie-breaker in the order), facets derived from the distinct
event types (cached 30 s), the replace-mode table patch and local times.

Differences from the SPA: the aggregate filter has options and the subdomain, aggregate and type
options follow the levels chosen above them (the SPA's lists are static and its aggregates empty); the
filters have search boxes (the SPA's have `filter`) ; paging as above; the Context Data section shows
the event's context, read from the write table as `GET /api/events/{id}` now does (bug 22).

### Processes (`/ui/processes`)

Mirrors `ProcessListPage`, `ProcessDetailDrawer`, `ProcessCreatePage` and `ProcessEditPage`. Routes as
connections: the list (`?q&app&subdomain&status&page&rows`, the SPA's URL keys), `/new`, `/{id}`,
`/{id}/edit`, `/{id}/{archive,delete}` (POST).

- Every check is the API's, per operation (`internal/platform/process/api`, per-operation since
  `1f83887`): `CanReadProcesses` for the list and the drawer, create `CanCreateProcesses`, edit and
  archive `CanUpdateProcesses`, delete `CanDeleteProcesses`. Each runs the API's use case
  (`pages.ProcessOps`). Processes are global (no client scoping, as the API).
- List: as the SPA, every process of both statuses is read once and filtered on the server: search
  over code and name, Application, Subdomain (options follow the chosen application; a live answer
  patches them) and Status selects, 50-row pages (25/50/100/250). No sortable columns (the SPA has
  none; the order is by code). A chosen application or subdomain that is not an option is dropped
  from the canonical URL.
- Drawer: description, Edit and Archive for a CURRENT process, Delete for an ARCHIVED one (the SPA's
  guards, which the use cases enforce since bug 23's fix), tags. **The diagram is drawn by the Mermaid
  island** (2026-10-24, owner ruling: vendor Mermaid; see "Islands"), with the source kept below it in a details block;
  a non-Mermaid diagram type shows its source with a note. The SVG/PNG downloads are not in the new UI (the current UI
  has them).
- Create and edit are form dialogs with the SPA's fields: code (create only), name, description, tags
  (comma-separated: trimmed, blanks dropped; an emptied field clears them, as the SPA sends `[]`) and
  the Mermaid source (verbatim). The SPA's own check (code and name required) is a field error; the use
  case's code errors (`INVALID_CODE_FORMAT`, `CODE_EXISTS`) land on Code. Create opens the new process
  in the drawer, as the SPA navigates to it.

Differences from the SPA: dialogs instead of full-page editors; **Archive asks for confirmation** (the
SPA archives at once; owner ruling: archive and delete go through the shared confirmation); the row
Edit is disabled with "Archived processes cannot be edited." for an archived process (the SPA's pencil
opens its editor, bug 24); no SVG/PNG download (the diagram itself is drawn by the Mermaid island since 2026-10-24);
the flash says "Process created" etc. without the code (flash keys carry no request text).

### Configuration (`/ui/platform/config`)

Mirrors `PlatformConfigPage` (Platform, Settings, Configuration; shown with `platform:admin:config:view`).
Routes: the list (`?app&q&sort&dir&page&rows`), `/new?app` and `/edit?app&section&property&client` (the dialog),
and POSTs `/save` (add and edit are one upsert, as `PUT /api/config/...`), `/delete` and `/reveal`
(the property is named in the query string: `app`, `section`, `property`, `client` = the client's id).

- **One rule set with the API.** The access rules moved out of the API handlers into
  `platformconfig.Service` (`internal/platform/platformconfig/service.go`): `AuthorizeRead`,
  `AuthorizeUpdate`, `AuthorizeWrite`, `AuthorizeRevealApp`/`RevealReach`/`AuthorizeReveal`, `ResolveClientID`,
  `Delete` and `Reveal`. Both the huma handlers and this page call it, so view, update, delete and reveal,
  the application confinement, "client resolved only after the permission check" and the below-anchor rules
  cannot drift. Set runs the `SetProperty` use case, as the API.
- The application select offers `platform` and the active applications the viewer may read (the SPA's
  `applicationsApi.list({activeOnly})`); an application-scoped account is not offered `platform`. The table
  (section, property, scope, type, value, description, updated) searches section, property and description,
  sorts, and pages 25 rows. A client-scoped value shows the client's identifier.
- **Secrets are never in the page.** The row builder does not read a secret's value at all: the table shows a
  lock and bullets, the edit dialog never prefills it, and the secret password input has no value to echo, so a
  failed save re-renders without it. Changing a secret needs a new value (as the SPA). The type is a toggle
  group; the value control swaps between a textarea and the kit password input by a signal, and the server
  reads the one that matches the chosen type.
- Edit and Delete are offered when the viewer passes the API's write rule for the application
  (`AuthorizeWrite`); Reveal when the viewer holds a reveal permission that reaches the application and
  that row's client (`AuthorizeRevealApp`, `RevealReach`). The server decides again on every post: a forged
  save, delete or reveal from a view-only or update-only caller is refused and changes nothing.
- **Reveal.** A confirmation says it is recorded in the audit log under the caller's name, then
  `platformconfig.Service.Reveal` runs the API's audited path (permission, confinement, client access, audit row
  or refusal). The value comes back in that response only, in a dialog (Copy and Close) patched into
  `#cfg-reveal-slot`: not in a URL, a signal, a flash or the table. The dialog removes itself from the page when
  it is closed (Close, Escape; no light dismiss), so no plaintext stays in the DOM. The response is
  `Cache-Control: no-store` (the stream helper sets `no-cache` itself when it starts; the `noStore` writer
  stamps the header as the response begins). A native post gets the page with the dialog, also uncacheable.
- Errors follow "Datastar errors": validation lands on its field (section, property, value, an unknown
  client "No client with this identifier."), a refusal on a Datastar delete or reveal is an error flash, a
  refused save is the dialog's error region.

Differences from the SPA: add and edit are form dialogs (the same fields); a secret's reveal sits in a modal
that cannot be dismissed by the mask; Delete and Reveal ask first through the shared confirmation dialogs (the
SPA's PrimeVue confirm); the flash says "The property was saved/deleted" without the coordinates (flash keys
carry no request text); the nav entry uses the `list` icon (the kit sprite has no sliders icon).

Not verified in a browser: the signal-driven value swap, the reveal dialog's self-removal and the Copy button
are covered by the rendered markup in tests only.

## Compression

`internal/ui/compress` (the first middleware of the page group) compresses `text/html` and
`text/event-stream` with Brotli (level 5) or gzip (level 6), whichever `Accept-Encoding` prefers
(q-values honoured, `q=0` refuses, Brotli on a tie). Every response of those types gets `Vary:
Accept-Encoding`, compressed or not; anything else passes untouched (a response that already has a
`Content-Encoding`, the event-type zip export, plain-text refusals, 204/304). The decision is taken
when the header is committed, including by the first `Flush` (how the Datastar SDK starts a
stream). Every `Flush` flushes the encoder and then the connection, so Datastar events still arrive
one at a time (`TestStreamFlushesEachEvent` sends an event, waits until the client has decoded it,
then sends the next; it fails, mutation-tested, when the encoder is not flushed). The encoder is
closed (gzip trailer, last Brotli block) and pooled when the handler returns. Security headers, the
CSRF guard and session negotiation are unchanged (`ui_test.go`: a compressed page keeps CSP,
`no-store` and `X-Frame-Options`; a cross-site post is still refused).

Not used, and why:
- **The Datastar SDK's `datastar.WithCompression`** (the owner's first suggestion for streams): it
  never closes its encoder, so every stream ends without the gzip trailer or the final Brotli block (a
  strict decoder reports `unexpected EOF`; checked with the SDK 1.2.2), sets no `Vary`, and builds all
  four encoders (zstd included) on every request. `ds.NewSSE` is unchanged; the middleware compresses
  its stream.
- **chi's `middleware.Compress`**: when a handler flushes before writing (the Datastar SDK does), the
  header goes out without `Content-Encoding` and the body is then gzipped anyway (checked: the client
  received gzip bytes with no encoding header, plus a "superfluous WriteHeader" log). It also matches
  `gzip;q=0` as accepting gzip and sets `Vary` only on compressed responses.

**Assets** (`/ui/assets/`, outside the page group, so never compressed twice): text files (CSS, JS,
SVG, notices) are compressed once per coding at the best level on their first request in it and served
by `Accept-Encoding`, with `Vary`, the right `Content-Length` and their own ETag per variant (a
Brotli ETag does not validate the gzip variant). kit.css 105 KB → 16.6 KB, datastar.js 34 KB → 12.2 KB,
the sprite 25 KB → 3.5 KB (Brotli).

Levels: on the 1,000-row dispatch-jobs stream (2.9 MB, seeded data), Brotli 4 → 49 KB in 5.9 ms,
5 → 38 KB in 7.7 ms, 6 → 37 KB in 7.8 ms; gzip 5 → 91 KB in 2.9 ms, 6 → 83 KB in 3.8 ms (Apple M4 Pro).

## CSRF

Sessions are the `fc_session` cookie (HttpOnly, `SameSite=Lax`, Secure outside dev), so
state-changing `/ui` requests need cross-site protection. Three layers:

1. **SameSite=Lax** keeps the cookie off cross-site POSTs, `fetch`es and iframes; only top-level
   GET navigations carry it, and GET handlers never change state.
2. **`guard.CSRF`** (middleware on the whole group) for every method but GET/HEAD/OPTIONS, built
   on Go's `net/http.CrossOriginProtection`: `Sec-Fetch-Site` must be `same-origin` (or `none`,
   user-initiated); without it the `Origin` must match the `Host` (or the configured public base
   URL, `FC_JWT_ISSUER`/`FC_EXTERNAL_BASE_URL`, for proxies that rewrite Host). This covers what
   Lax does not: same-site sibling subdomains and browsers that ignore SameSite.
3. Stricter than the standard library: an unsafe request carrying **neither** header is refused
   (every browser sends at least one on a POST; anything else is not a browser and should use
   `/api` with a bearer token). `Datastar-Request: true` alone is not accepted as proof.

No token is needed: Datastar's `fetch` and native form posts both carry `Sec-Fetch-Site`/`Origin`,
so forms need no hidden field and actions no header. Rejections are 403 and logged
(`ui: cross-origin request refused`). Tests: `internal/ui/guard/guard_test.go` (17 cases; the
check was mutation-tested by disabling each half once) and `internal/ui/ui_test.go` (a
cross-site sign-out is refused and keeps the session).

Related headers (`guard.SecurityHeaders`): CSP `script-src 'self' 'unsafe-eval'` (Datastar
compiles expressions with `Function`; no inline scripts or handlers), `style-src 'self'
'unsafe-inline'` (kit inline styles), `connect-src 'self'`, `frame-ancestors 'none'`, plus
`X-Frame-Options: DENY`, `nosniff`, `Referrer-Policy: same-origin`, `Cache-Control: no-store`.

Session negotiation (`guard.RequireSession`), as the TS SDK's `requireSession`: a GET/HEAD
navigation accepting HTML without a session is redirected (302) to
`/auth/login?redirect=<path>`; a Datastar action, fetch or form post gets 401. A role-less user
gets 403 (same rule as the API's `ProfileOnlyWithoutRole`). Sign-out is `POST /ui/logout`
(clears the cookie like `/auth/logout`, then 303 to the login page).

After signing in, the SPA's login returns to the `/ui` page in `?redirect=`
(`frontend/src/api/auth.ts` `redirectAfterLogin` → `safeUiRedirect` in `api/uiRedirect.ts`): the
value is parsed against the page's origin and followed only when it stays same-origin and its
normalised path is `/ui` or under `/ui/` (no open redirect; `..`, `//host`, backslashes and
encoded dots are refused; `frontend/tests/ui-redirect.test.ts`). OIDC interaction and OAuth
round-trips keep precedence. SPA routes in `?redirect=` are still ignored, as before.

## Conformance fixtures

`flowcatalyst-framework/ui-kit/fixtures/<component>/<case>.{json,html}` (format and normalisation
rules: `ui-kit/fixtures/README.md`), vendored to `internal/ui/testdata/fixtures`.
`TestConformance` (`internal/ui/kit/conformance_test.go`) renders each fixture's props and slots
and compares with the golden through `htmlnorm` (HTML5 fragment parse in the right context,
comments dropped, attributes sorted, whitespace collapsed, class order kept). It fails for a
fixture directory without a renderer, a renderer without fixtures, an unknown prop (strict
decoding) or an unused slot. 158 fixtures pass. Run against the framework checkout directly:
`UI_KIT_FIXTURES=../flowcatalyst-framework/ui-kit/fixtures go test ./internal/ui/kit/`.

Adding a component: spec + Props table + gallery example + fixtures in the framework repo
(commit there), `make ui-kit-sync`, the templ component, a `renderers` entry, `make ui-generate`,
tests green.

## Adding a page

1. `internal/ui/pages/<area>.go` (struct, `Mount`, handler as above) and `<area>.templ`.
2. Mount it in `internal/ui/ui.go`; add its dependency to `ui.Config` and pass it in
   `internal/server/wire_ui.go`.
3. In `internal/ui/shell/nav.go` set the entry's `ui` path, so the sidebar links to the new page
   (only `/ui`'s own sidebar changes; the SPA is not touched).
4. A handler test with a fake repository (full page, Datastar patches, permission denied,
   client scoping); see `dispatch_pools_test.go`.
5. `make ui-generate ui-kit-sync` (new Tailwind classes need the stylesheet rebuilt), then
   `make ui-verify lint test-unit`.

## Islands (Mermaid and CodeMirror)

Two parts need real client code and are web elements loaded by a script tag **only on the pages that use them**
(owner ruling 2026-10-24; no Vite or Node in the product build). Both files are committed under
`internal/ui/assets/static/`, served compressed like the other assets, with their licences.

| Part | Files | Page loads |
|---|---|---|
| `kit.Diagram` (`<fc-diagram>`) | `fc-diagram.js` (ours, 4.2 kB) and `mermaid.min.js` (unmodified `dist/mermaid.min.js` of npm `mermaid@12.1.0`, sha256 `6484afc3...18b2`, `assets.MermaidSHA256`) | `<script type="module" src={ assets.URL("fc-diagram.js") }>`; the element fetches the Mermaid bundle itself, once, on the first diagram |
| `kit.CodeEditor` (`<fc-code-editor>`) | `fc-code-editor.js` (CodeMirror 6 with JSON and YAML, built by `tools/vendor-codemirror`, `assets.CodeEditorSHA256`) | `<script defer src={ assets.URL("fc-code-editor.js") }>` |

Both render the server's text as the fallback (a `<pre>` source, a `<textarea>`), so a page without script is
usable. Used by the Processes drawer (diagram) and the dispatch job payload editor (JSON when the content type is).
A page that patches such a part in (a drawer, a dialog) must carry the script in its page template, not in the
patched fragment.

Mermaid runs with `securityLevel: "strict"` and `suppressErrorRendering`; a source it rejects leaves the drawing
hidden and the source open. The drawing never carries a Datastar attribute (labels are author-controlled HTML, and
Datastar evaluates `data-init`/`data-effect`/`data-on:*` on any node added to the page, with `'unsafe-eval'`): the
view is `data-ignore`, Mermaid renders into a scratch node inside it (never `document.body`) with
`dompurifyConfig: { ALLOW_DATA_ATTR: false, FORBID_ATTR: [data-<datastar plugin>...] }`, and the finished SVG is
scrubbed of Datastar attributes in an inert `<template>` before it is moved in (kit `diagram.md`, Behaviour). **The CSP is unchanged**: the bundle is a same-origin script (`script-src 'self'`),
Mermaid's inline `<style>` and `style=""` fit the existing `style-src 'unsafe-inline'`, and it does not use
`eval` (`'unsafe-eval'` stays only for Datastar). Checked in headless Chrome under the exact policy.
Transfer sizes (Brotli, best): `mermaid.min.js` 5.49 MB raw, 1.18 MB; `fc-code-editor.js` 340 kB, 97 kB;
`fc-diagram.js` 1.8 kB, 0.7 kB. The first request in a coding compresses at the best level (about 6 s for
Mermaid, once per process); later requests and revalidations are served from memory.

`<fc-code-editor>` replaces its textarea with a hidden input of the same `name` (and `data-bind*` attributes) and
the editor, and writes every change into the input with a bubbling `input` event, so forms, Datastar signals and
dirty-form checks work. It does not follow later writes to the input.

Upgrading Mermaid: `npm pack mermaid@X`, copy `package/dist/mermaid.min.js` and `package/LICENSE`, then change
`assets.MermaidVersion`, `assets.MermaidSHA256` and this table together. Upgrading CodeMirror (run only then):

```
cd tools/vendor-codemirror && npm ci && npm run build   # after editing the exact versions in package.json
```

The build is reproducible; copy the printed `shasum -a 256` of `fc-code-editor.js` into `assets.CodeEditorSHA256`
and commit the bundle with `LICENSE-codemirror.md` (regenerated by the script). `node_modules` is not committed.

## Refreshing the kit

`make ui-kit-sync` (`scripts/ui-kit-sync.sh`, `KIT_DIR=` to point elsewhere) runs the kit's own
`build.sh` (downloads its pinned Tailwind 4.3.2 standalone CLI on first use), builds
`internal/ui/assets/static/kit.css` from the kit's `theme/kit.css` plus this repo's
`internal/ui/**/*.{templ,go}`, copies the sprite, the Lucide licence and the kit's
third-party notices, copies `fixtures/` to `internal/ui/testdata/fixtures`, checks
`datastar.js` against its sha256 and writes `internal/ui/assets/KIT_SOURCE` (kit commit,
Tailwind and Datastar versions, the sprite's icon count and sha256).

Icons: a new icon is a line in the kit's `icons/sprite-icons.txt` and `icons/build-sprite.sh`
(pinned lucide-static 1.52.0), committed in the kit, then the sync. `TestSpriteMatchesKitSource`
fails when the embedded sprite is not the synced one; `TestEveryIconUsedIsInTheSprite` fails
when a `.templ`/`.go` file under `internal/ui` names an icon the sprite lacks. Commit the kit change first (the script warns when the kit has
uncommitted changes), then commit the synced files with `KIT_SOURCE`.

Classes used only in Go templates exist in the stylesheet only after a sync, so run it after
adding a page. Datastar is vendored unmodified (1.0.4, the version the kit pins; sha256
`727844ad…cf91`, checked by `TestDatastarIsThePinnedFile`); upgrading it means changing the URL
and hash in the script and `assets.DatastarSHA256` together, after the kit moves.

## Kit defects and gaps found (also in the kit's PORTING-NOTES.md)

Fixed in the kit (framework commit `b8fde39`): duplicate `type` attributes on four gallery inputs
and in `input.md` (email and file inputs rendered as text); `<th>` without `scope="col"` in the
gallery and `data-table.md`; skeletons without `aria-hidden`; the gallery's form-field error not
linked; the app shell's page header without the actions wrapper; `select.md` eliding the
checkmark URI; `table-toolbar.md` rendering `value=""`; `_conventions.md` placing DELETE signals
in the body (Datastar 1.0.4 sends them in the query, like GET).

Recorded, not changed:
- `filter-select`: the gallery's handler is `el.form && el.form.requestSubmit()` (its select is
  outside a form), the spec's is `el.form.requestSubmit()`; the templ port follows the spec.
- The gallery's confirm dialog has `data-on:submit="el.closest('dialog').close()"`; Datastar
  prevents the default of a form submit it handles, so that form never posts. Demo-only; the spec
  (native POST) is what the port implements.
- Gallery screenshots predate the fixes above (the file input now renders as one).

Gaps the pilot found, and their state after Phase 1 step 1 (kit commits `1317a2f`, `2dd6ff6`):
1. **Icons: done.** 86 icons (was 38; 90 since the theme picker and the phone top bar), built by the kit's `icons/build-sprite.sh`; the sidebar
   uses the Lucide counterparts of the SPA's PrimeIcons (no approximations); Create Pool has `plus`.
2. **Row actions: done.** Kit spec `row-actions.md`; the proposed part is gone.
3. **Table toolbar live updates: done.** The `{formId}-state` part (above). This also fixed a pilot
   bug: a live sort click did not update the hidden sort fields, so the next search lost the sort.
4. **App shell on narrow screens: done** (above).
5. **Form controls for create/edit: partly done.** Form dialog, autocomplete/client select,
   number input (`input` type number), form section, detail field. Open: multi-select with
   search, password with strength meter, secret-reference input (`SecretRefInput`),
   permission-denied modal.
6. **Dark mode without the light flash: done, with one limit** (system mode's first page in a
   browser; above).
7. **Flash messages: done** (`shell.Flash` for Datastar actions; the fixed-key cookie for a native
   post/redirect/get, above).
8. **Datastar errors: done** (the contract and fallback above).
9. **Login `?redirect=`: done** for `/ui` targets.

## Phase 1 CRUD (dispatch pools, connections, shell): verification

Isolated instance (own embedded Postgres on 25432 with a temp data dir, API on 18080, metrics on
19090, `HOME`/`XDG_*` in a temp dir; the user's instance untouched), Chrome:
- Pools, desktop, light and dark: create dialog (native `required` first, then the use case's
  "already exists" patched onto Code with focus there; the dirty state survived the error patch);
  Escape on a changed form asks to discard; the client select narrowed by the server search;
  success closed the dialog, flashed, re-rendered the table and opened the new pool in the drawer;
  deep link `/ui/dispatch-pools/{id}` opens the drawer; Suspend from the drawer and from the row
  menu (drawer closed) through the shared confirmation; edit from the drawer (Save disabled until a
  change, focus back on Edit after saving, drawer refreshed); Delete closed the drawer. No console
  messages.
- Connections, desktop: create with the service account select and the client select; flash,
  table, drawer.
- Phone width (390×844, headless Chrome over CDP with the dev test headers): both pages, light and
  dark, menu open/Escape (focus to the close button and back), create dialog, drawer; no
  horizontal page scroll, no console errors or warnings.
- Theme: Dark in the picker → the next server response has `class="dark"`; System with a light OS →
  no class.
- Found in the browser and fixed: a form read after the SSE headers were flushed came back empty
  (multipart from Datastar), so the drawer did not refresh; `.fc-main`'s base rule overrode the
  phone layout.

**Not exercised in a browser**: the 403 "refused" fallback from a real guard refusal (only the
listener, with a dispatched `datastar-fetch` error event; a same-origin page cannot fail CSRF), the
native (no-JavaScript) post/redirect/get path (unit tests cover it), system mode with a dark OS
(unit test), Firefox/Safari, screen readers, touch.

## Not verified (earlier phases)

- Browsers other than Chrome (headless, via the DevTools protocol), screen readers, touch.
- The visual check ran against an isolated instance (own Postgres on a temp data dir, own ports,
  `HOME` pointed at a temp dir so no credential files were written to the user's cache): login
  with the dev bootstrap admin, the pools page in light and dark, live search, the status filter
  in the popover, sort, sidebar collapse and the sign-out CSRF check. No console or CSP errors.
- Phase 1 step 1, same kind of instance, Chrome, light and dark: signing in from
  `/ui/dispatch-pools?status=ACTIVE` returned to that URL; live Clear All (filter cleared, sort
  kept, Clear All and badge gone, URL replaced, focus in the search box); Clear All appearing while
  typing; row menus (position, disabled reasons, separator); the fallback for a 404/405 Datastar
  request (one "failed" message for two failures) and for a stopped server (offline). In the kit
  gallery: menu keyboard use (Tab into the items, Escape and dialog close return focus to the
  trigger). **Not exercised in a browser**: the 403 "refused" fallback (no way to make a
  same-origin Datastar request fail CSRF from the page), a server-patched success flash (the
  pilot has no mutation yet), screen readers.

## Phase 3 (core operations): verification, platform bugs, owner decisions

### Event types: verification (2026-10-10)

- Unit tests (`event_types_test.go`, fakes): list, facets cascading and pruning, paging, permissions
  per action, application confinement, drawer guards, schema viewer, create/edit/add-schema with
  use-case errors on fields, every action, sync tally, export zip contents.
  `event_types_pg_test.go` (tag `integration`, embedded Postgres): create → edit → add schema →
  finalise → export → viewer → deprecate → archive → delete through the real use cases, 7 audit rows.
- Chrome, isolated instance (ports 18080/19090, embedded PG 25432, own data dir and `HOME`), desktop:
  live facets (summary patched, popover and disclosure stay open, URL replaced), Clear All, drawer,
  schema viewer tabs, create with a duplicate code (error on the code group, focus on Application)
  then a new code (dialog closed, drawer opened, flash), add-schema page, finalise / deprecate /
  archive / delete from the drawer through the confirmations, sync (tally flash), export (zip).
  Dark mode on desktop. Headless Chrome over CDP at 390×844, light and dark: list, filters open,
  drawer, viewer, create dialog; no horizontal page scroll, no console errors or warnings.
- Not exercised in a browser: the native (no-JavaScript) posts (unit tests cover them), the copy
  button (clipboard permission in automation), Firefox/Safari, screen readers.

### Platform bugs found

All fixed on 2026-10-10; each entry's fix, commit, wire impact, tests and the Java/Rust mirror notes
are in `docs/platform-bug-fixes.md` (with the decisions taken conservatively and the further
findings). The descriptions below are as found.

1. **`/bff/event-types` reads have no authorization**: `GET /bff/event-types`, `GET /bff/event-types/{id}`
   and the filter endpoints (`filters/applications|subdomains|aggregates`) check no permission and
   apply neither client scoping nor application confinement (`GET /api/event-types` does all three).
   The UI uses the API's checks.
2. **Multi-value facets filter by the first value only**: the SPA sends repeated `application=`,
   `subdomain=`, `aggregate=` parameters; the BFF list and filter endpoints read `q.Get` (one value),
   and `FindWithFilters` takes one value per facet. Choosing two applications in the SPA shows the
   first one's event types. The UI filters in memory with set semantics.
3. **Add schema stores the schema double-encoded, and ignores MIME and schema type**: the SPA sends the
   textarea's text as a JSON string (`"schema": "{...}"`), `json.RawMessage` keeps the quotes, so
   the version holds a JSON string, not the document (the viewer and the export then see a string).
   `bffAddSchemaRequest.MimeType`/`SchemaType` are never passed on: every version is
   `application/schema+json`/`JSON_SCHEMA` whatever the user chose.
4. **Stored XSS in the SPA's schema viewer**: `highlightJson` (`frontend/src/utils/schema-highlight.ts`)
   does not escape before inserting into `v-html`; a schema containing markup in a string (anyone who
   can add a schema version, including SDK service accounts) runs in the viewer's page. The Go port
   escapes.
5. **Archive and delete rules live only in the SPA**: `canArchive` (every version deprecated) and
   `canDelete` (archived, or every version finalising) are checks in `EventTypeDetailDrawer.vue`;
   `ArchiveEventType` and `DeleteEventType` accept any event type.
6. Sync Platform Events reports every catalogue type as "updated" on each run when nothing changed
   (seen: "73 updated (73 total)"; the schema tally says 73 unchanged).
7. The SPA's row pencil opens an archived event type in edit mode although its drawer hides Edit for
   archived types.

8. **Subscription writes use the any-of gate**: `POST/PUT /api/subscriptions`, pause and resume check
   `CanWriteSubscriptions` (create, update or delete), so a delete-only role can create, edit, pause
   and resume (the dispatch pool API was fixed for the same in `c59e246`; connections already split).
9. The SPA's create body sends `clientScoped` and `sequence`; `CreateSubscriptionRequest` has neither,
   so the leniency net drops them: no subscription created through the SPA or the API is ever
   client-scoped, and its sequence is always 99.
10. The SPA's Queue field is free text (placeholder "default"); the use case accepts only
    DEFAULT/HIGH_PRIORITY (ignoring case) and answers 400 `INVALID_QUEUE` otherwise.
11. `dispatchPoolCode` is stored only by the SDK sync: a subscription created in the SPA or through
    `POST /api/subscriptions` has `dispatchPoolId` and no code, so the SPA's Pool column is empty for it.
12. The SPA's edit sends `connectionId: ""` when the field is blank, which stores an empty string (the
    use case sets any provided value; it cannot clear the connection).

13. **Application writes use the any-of gate**: create, update, activate and deactivate check
    `CanWriteApplications` (create, update or delete).
14. Deleting an active application is blocked only by the SPA (the button is disabled);
    `DeleteApplication` refuses only applications still referenced.

15. **Client "Deactivate" deletes the client**: `POST /api/clients/{id}/deactivate` ("soft delete")
    runs `DeleteClient`, which removes the row (`ClientDelete`); the SPA promises "Soft delete this
    client. Can be reactivated later."
16. The SPA's client list sends `page`/`pageSize` to `GET /api/clients`, which ignores them and returns
    every client; "Next" shows the same list again (and the search filters only what was loaded).

17. **Scheduled-job writes use the any-of gate**: create, update, pause, resume and archive check
    `CanWriteScheduledJobs` (create, update or delete).

18. **`GET /api/dispatch-jobs/filter-options` is not tenant-scoped**: `DistinctValues` reads the whole
    projection, so a client user with `dispatch-job:view` sees every tenant's codes, client ids, pool
    ids and subscription ids. The /ui page uses the statuses and codes the same way (for parity) and
    shows only accessible clients.
19. The SPA's application, subdomain and aggregate filters never have options: the filter-options
    endpoint does not return those facets and the SPA sets them to `[]` (its own comment says so).
20. The SPA shows **Sign** to every viewer; the API requires `dispatch-job:view-raw` (403 otherwise).
21. `FindWithFilters` caps `size` at 1,000 by falling back to **100** for anything larger (`limit > 1000
    → 100`), so `size=1001` returns fewer rows than `size=1000`.
22. **The SPA's event drawer never shows Context Data**: it reads `event.contextData`, the API sends
    `context`; and `GET /api/events/{id}` reads `msg_events_read`, which has no context column, so the
    field is always empty anyway (the debug raw view reads the write table and has it).

23. **Process edit/delete rules live only in the SPA**: the drawer offers Edit and Archive for a CURRENT
    process and Delete for an ARCHIVED one, but `UpdateProcess` edits an archived process and
    `DeleteProcess` deletes a current one (the API, and so the SDK, can do both).
24. The SPA's row pencil opens the full-page editor for an archived process, although its drawer hides
    Edit for archived processes (as bug 7 for event types).
25. The SPA's process edit sends `description: ""` for a blank field, which stores an empty string, not
    NULL (the use case sets any provided value; both UIs show "—" for either). The /ui page sends the
    same body.

### Processes and compression: verification (2026-10-10)

- Unit tests (`processes_test.go`: list, facets with the dependent subdomain options and pruning, the
  search fields, empty states, permissions per operation incl. delete-only and update-only users, the
  drawer for current/archived/non-Mermaid processes with the source escaped, create and edit bodies
  (tags parsing, description clearing, verbatim source), field errors, the replace-mode dialog patch,
  archive/delete with drawer refresh/close, an internal error, the native post's flash) and
  `processes_pg_test.go` (embedded Postgres: create with tags and source, duplicate code on its field,
  facets, drawer, edit clearing tags and description, archive, delete; 4 audit rows).
  `internal/ui/compress` (negotiation table, pages, pass-through cases, the event-by-event stream over
  a real connection for Brotli and gzip) and `ui_test.go` (compressed page with the security headers,
  live stream, CSRF refusal, the asset variants and their ETags).
- Chrome on the isolated instance (ports 18080/19090, embedded PG 25432, own data dir and `HOME`):
  live application filter (subdomain options patched, popover open, URL replaced), drawer, Archive and
  Delete through the confirmations (flash, drawer refreshed / closed, table re-rendered), create with a
  bad code (error on Code, focus there) then a good one (dialog closed, drawer opened, tags parsed,
  markup in the source escaped), edit clearing the tags, "Open in the current UI" (the SPA's drawer
  renders the diagram). Dark mode on desktop. Headless Chrome at 390×844, light and dark: list,
  filters, drawer, create and edit dialogs; no horizontal page scroll, no console problems. Found and
  fixed in the browser: the second dialog of a visit opened non-modal (above, "Dialog slot"), the
  link in the drawer's notice wrapped one word per line.
- Not exercised in a browser: the native (no-JavaScript) posts (unit tests cover them), Firefox/Safari
  (their decoding of the compressed streams in particular), screen readers.

### Subscriptions: verification (2026-10-10)

Unit tests (`subscriptions_test.go`: list, filters, sort, permissions per operation incl. a delete-only
user, client scoping, drawer, actions, create with scope switching, bindings, field errors, edit body)
and `subscriptions_pg_test.go` (create with a real pool and finalised event type, edit, pause, resume,
delete; 5 audit rows; pool code shown). Chrome on the isolated instance: Client Scoped switch
re-fetching the event types and showing the client field, create through the real use case (drawer
opened, flash), edit (queue to High priority), pause from the drawer. Headless Chrome at 390×844, light
and dark: list, drawer, create (multi-select open, summary updated in the browser), edit; no horizontal
scroll, no console problems.

### Applications: verification (2026-10-10)

Unit tests (`applications_test.go`: list with type/status/search, permissions per operation incl.
delete-only and client users, drawer sections, create/edit bodies, actions, provisioning of both
credentials incl. the native post answering with the page, redirect-URI parsing) and
`applications_pg_test.go` (create, edit, deactivate/activate, service account provisioned and
attached, CONFIDENTIAL login client with its secret shown, delete refused while referenced). Chrome on
the isolated instance: create with the type toggle (help text follows it) and the logo MIME field
appearing with a logo; service account and login client provisioning (dialog, Escape does not dismiss
it, the button removes it, drawer shows Provisioned). Headless Chrome 390×844, light and dark: list,
drawer, create dialog; no horizontal scroll, no console problems.

### Clients: verification (2026-10-10)

Unit tests (`clients_page_test.go`: list and search, a client user refused, per-operation permissions,
the picker's posted list, every action with the fixed suspend reason, create/edit with the identifier
error on its field) and `clients_page_pg_test.go` (create, edit, suspend with reason, activate, enable
an application, deactivate deleting the row). Chrome on the isolated instance: create (drawer opened),
picker (Save disabled until a change, back to disabled when undone, filter, count, mirrored list),
Save through the use case. Headless Chrome 390×844, light and dark: list, drawer, create; no
horizontal scroll, no console problems.

### Scheduled jobs: verification (2026-10-10)

Unit tests (`scheduled_jobs_test.go`: facets, paging, a client user confined and refused platform
jobs and firings, drawer, every action and the fire flash, per-operation permissions, create and edit
bodies and errors, the firings list and one firing with its logs) and `scheduled_jobs_pg_test.go`
(create, edit, pause, resume, fire, the firing listed, archive, delete). Chrome on the isolated
instance: create (drawer opened), Fire Now (flash with the instance, recent firings updated), firings
page, firing page (the delivery error). Headless Chrome 390×844, light and dark: list, drawer, create,
firings; no horizontal scroll, no console problems.

### Dispatch jobs: verification and measurements (2026-10-10)

Unit tests (`dispatch_jobs_test.go`: URL round trip incl. cursor and junk values, list with
client/identifier/platform cells, group link, metadata preview, a user without the permission refused,
a client user scoped in SQL, live patches incl. dependent options and the full-toolbar header, cursor
paging, drawer with attempts latest-first and the payload formatted as `formatJson` does, Sign only
with view-raw, another tenant's job refused, requeue of ticked rows (deduplicated), one row, natively;
requeue-by-filter preview/run/cap/changed/bad count, the error preset, no-filter warning) and
`internal/platform/dispatchjob` `TestFindWithFilters_CursorPages` (both directions, ties at one instant).
Chrome on the isolated instance with **100,000 seeded jobs** (`msg_dispatch_jobs`, projected):
selection count and header state, Requeue selected (flash, rows reset), row requeue, status / group /
date filters (URL follows, popover fields, bound on focus), Clear All, the group link (toolbar
re-rendered, other filters kept), Requeue all matching (16 jobs, through `ResendByFilter`), the error
preset (24,982 matching, 10,000 cap message), Older/Newest, the created sort, drawer open/switch/
requeue (times stay localised), Sign (plan, headers, verify command). Headless Chrome 1440×900 and
390×844, light and dark: list, filters, drawer, sign, selection, confirm step; no horizontal page
scroll, no console problems.

Measured on an Apple M4 Pro, loopback, 100k jobs in the projection:

| | 200 rows | 1,000 rows |
|---|---|---|
| templ render of the table (`BenchmarkDispatchJobsTable`) | 1.8 ms, 2.4 MB allocated | 8.4 ms, 11.9 MB allocated |
| server time, live list request (Datastar, incl. query) | 4.7 ms | 18 ms |
| response size (uncompressed / gzip) | 576 KB / 18 KB | 2.8 MB / 71 KB |
| browser: filter click → rows localised (median) | 78 ms (response 17 ms, apply ~60 ms) | 280 ms (response 45 ms, apply ~235 ms) |
| full page load (DCL / load) | 27 ms / 44 ms | 180 ms / 320 ms |

For comparison the SPA's `GET /api/dispatch-jobs` answers 200 rows in 1.8 ms (110 KB JSON, 5 KB
gzip) and 1,000 in 4.5 ms (512 KB). Before the cache, the two `DISTINCT` queries made a live request
13 ms (200 rows). The sizes above are uncompressed; `/ui` is compressed since 2026-10-10 (below).

**With compression** (re-measured 2026-10-10, same machine, 100,000 seeded jobs; curl medians of 10,
headless Chrome medians of 6 page loads and 10 live filter changes; Chrome negotiates Brotli):

| 1,000 rows | before (uncompressed) | Brotli | gzip |
|---|---|---|---|
| full page on the wire (decoded 2.93 MB) | 2.93 MB | 43 KB | 88 KB |
| live patch on the wire (decoded 2.90 MB) | 2.90 MB | 39 KB | 84 KB |
| server, full page (curl, loopback) | 19.7 ms | 21.8 ms | 20.5 ms |
| server, live request (curl, loopback) | 20.4 ms | 22.4 ms | 21.3 ms |
| Chrome, loopback: page response / DCL | 33–38 ms / 146–210 ms | 44–59 ms / 171–178 ms | 29–35 ms / 196–220 ms |
| Chrome, loopback: filter click → rows applied | 249–274 ms | 297–300 ms | 264–270 ms |
| Chrome, 50 Mbit/s + 20 ms: page response / DCL | 495 ms / 550 ms | 52 ms / 257 ms | - |
| Chrome, 50 Mbit/s + 20 ms: filter click → rows applied | 722 ms | 327 ms | - |

| 200 rows | before | Brotli | gzip |
|---|---|---|---|
| full page / live patch on the wire | 633 KB / 603 KB | 16 KB / 12 KB | 25 KB / 20 KB |
| server, live request (curl, loopback) | 4.6 ms | 5.4 ms | 5.3 ms |
| Chrome, 50 Mbit/s + 20 ms: filter click → rows applied | 198 ms | 82 ms | - |

On loopback Brotli costs time at 1,000 rows: ~2 ms of server time by curl (the encoder runs while the
stream is written), but 25–50 ms more from click to applied rows in Chrome (the browser's decode of
2.9 MB; gzip is within the ±25 ms run-to-run noise). At 200 rows the difference is lost in the noise.
On any real link the transfer it saves dominates (50 Mbit/s: 722 → 327 ms). The seeded rows are regular, so real data compresses somewhat less. Most of the browser's apply time at 1,000
rows is Datastar scanning the new rows' attributes (each row carries a `data-init` for its time, two
links' `data-on:click` and the row requeue's).

### Events: verification and measurements (2026-10-10)

Unit tests (`events_test.go`: list cells, a user without the permission refused, a client user scoped
and refused another tenant's event, live patch with the dependent options, URL round trip, cursor
paging, drawer, payload formatting incl. a JSON string holding JSON, a bad id) and
`events_pg_test.go` (facets from the distinct types narrowed by the chosen subdomain, the list, the
drawer); `TestFindWithFilters_CursorPages` in `internal/platform/event`. Chrome on the isolated
instance with **100,000 seeded events**: size change, application filter on/off, Older, row click
opening the drawer; headless Chrome 1440×900 and 390×844, light and dark (list, filters, drawer), no
horizontal page scroll, no console problems.

| | 200 rows | 1,000 rows |
|---|---|---|
| templ render of the table (`BenchmarkEventsTable`) | 0.8 ms | 4.0 ms |
| server time, live list request (incl. query) | 2.1 ms | 7.0 ms |
| response size (uncompressed / gzip) | 225 KB / 8 KB | 1.0 MB / 27 KB |
| browser: filter click → rows localised | 38 ms (apply ~28 ms) | 112 ms (apply ~90 ms) |
| full page load (DCL / load) | 12 ms / 19 ms | 19 ms / 99 ms |

The SPA's `GET /api/events` answers in 0.9 ms (200 rows, 67 KB) and 2.2 ms (1,000 rows, 339 KB). An
event row costs ~1 KB of HTML and ~0.09 ms to apply; a dispatch-job row ~2.9 KB and ~0.23 ms (its
checkbox, two live links and the requeue button each carry Datastar attributes; delegating them to the
`<tbody>` as the selection count already is would cut both, not done).

### Wave 1 parity pass 2026-10-24

Method: for each area, `git log --since=2026-10-10 -- frontend/src/pages/<area> internal/platform/<area>` (plus
`internal/ui`, since several commits the same afternoon as a port already changed the `/ui` page), each behaviour change read
against the page. Dispatch jobs is caught up (section above); the other nine:

- **Dispatch pools: checked.** Edit clearing the rate limit and description (d1158fae) and a client-scoped user picking its own client (108894d6) already changed `/ui` in the same commits; the F19 pool ownership and the identifier stored on create are not on the page.
- **Connections: checked.** The F17 sync fix and `PUT serviceAccountId` are API-only (the SPA's edit does not send it, so neither does the dialog); nothing in the SPA since 2026-10-10.
- **Events: checked.** The detail's context data (bug 22) and the 1,000 size clamp are in; F20's application-scoped reads (the application-service view permission with confinement to the caller's applications) are API-only: `/ui` still requires the messaging view permission, the stricter reading, and an application service account does not sign in to it.
- **Processes: changed.** The SPA's edit page sends a stale link to an archived process back to the drawer (10a5ac1e); the edit form now answers `PROCESS_ARCHIVED` ("Archived processes cannot be edited.") instead of opening. Edit, archive and delete gating by status were already in (ccf2dd52).
- **Clients: checked.** Deactivate-sets-INACTIVE (ff65ec46) changed `/ui` in its own commit; the SPA's move to browser paging (db4e7766) is what `/ui` already did (100 rows, 50/100/250/500).
- **Event types: checked.** The archive and delete rules and their reasons, the sync gate (a3414cc9) and the schema fixes are in. Create is now anchor-reach only at the use case (0a8df193); the New button is still shown to a client-scoped holder of `event-type:create`, as in the SPA, and the refusal reaches the form as a flash.
- **Subscriptions: checked.** Queue select, `clientScoped` and `sequence` on create, the pool code, the blank connection on update and the per-operation permissions were all in the port or the same-day fixes; F30 (sync no-ops) is API-only.
- **Applications: checked.** The one SPA commit (90e41132) gates Provision Service Account on anchor reach, `service-account:create` and `application:update`; `/ui` has had that gate since 8b1cb0fa, with the same "No service account" text. Its role locks (`isRoleAboveUser`, 403 `ROLE_ABOVE_CALLER`) are in the user and service-account drawers, which are wave 4; F18/F21 are role-API changes.
- **Scheduled jobs: checked.** Per-operation permissions (210e4b55) changed `/ui` in the same commit; the application-service firings report and F31 are API-only.

Not done: none of this was opened in a real browser (light, dark, narrow) yet; the dispatch jobs additions are covered by
handler tests (`dispatch_jobs_redrive_test.go`) and the Postgres page test (the Stuck lens, the ticked-row dialog and a redrive
by filter against real rows), not by a headless run. The acceptance run in a browser stays before the owner is asked to switch the area.

### Owner decisions needed

- **Add schema, archive/delete guards, Sync Platform Events, subscription permissions and create
  body, client deactivate, filter-options scoping, process guards, dialog slot morph**: decided and
  done 2026-10-10 (`docs/platform-bug-fixes.md`, including the choices taken conservatively there).
- **Findings of the first batch, second batch**: done 2026-10-10 (F1 to F6 in
  `docs/platform-bug-fixes.md`, with the choices taken conservatively). For the UIs: the SPA now
  shows Sync Platform Events, the role sync and the role editors only to users the BFF admits
  (`frontend/src/utils/admin-permissions.ts`); `/ui` needed no change (its sync button already
  follows `CanSyncPlatformEventTypes`, and it has no role pages; the nav's Roles and Permissions
  entries link to the SPA under the view permissions the reads need). New findings there: the SPA's
  route permissions and this nav use several codes no role grants (identity providers, OAuth
  clients, audit log, CORS, settings), and the SPA's dashboard OpenAPI sync is shown to everyone.
- **Event types and Subscriptions sidebar entries** now link to their `/ui` pages in the `/ui` sidebar
  (the SPA's own navigation is unchanged).
- **Dispatch jobs paging**: the page adds keyset paging (Older/Newest) that the SPA does not have, with a
  `Cursor` field and an `id` tie-breaker in the repository's list query. Keep?
- **Dispatch jobs date range in UTC days** (the SPA: the browser's zone). Pass the viewer's zone (a
  hidden field or cookie) instead?
- **Compression for `/ui`: done** (owner ruling 2026-10-10), but **not with the two components the
  ruling named**: the Datastar SDK's stream compression leaves every stream unterminated and chi's
  middleware sends gzip bytes without `Content-Encoding` after an early flush (both checked; "Compression"
  above), so one small middleware on the standard library and `andybalholm/brotli` (already in the module
  graph through the Datastar SDK; now a direct requirement) compresses pages and streams. Keep, or should
  streams be left to the SDK once it closes its encoder?
- **Dispatch Jobs and Events sidebar entries** now link to their `/ui` pages in the `/ui` sidebar.
- **Processes: ported with the diagram as source** (owner ruling 2026-10-10: no vendored Mermaid). **Resolved
  2026-10-24:** Mermaid is vendored and the diagram is drawn (see "Islands"). The sidebar entry links to `/ui/processes`.

## Functions: trace switches and trace log (wave 2, part 2)

Pages: `function_trace.go|templ` (switches, log, stream), `function_trace_form.go` (enable dialog),
`function_trace_view.go` (URL state, record view models). Spec: the SPA's `FunctionTraceControls.vue`,
`FunctionTraceLogPage.vue`, `functionTrace.ts` and `docs/function-trace-design.md`.

| URL | GET | POST |
|---|---|---|
| `/ui/functions/{id}/traces` | the log page; a Datastar GET patches the log and, while a window is live and the page is not paused, stays open as a stream | - |
| `/ui/functions/{id}/traces/more?cursor=` | Datastar: appends the next (older) page and replaces `#trace-more` | - |
| `/ui/functions/{id}/trace/switches` | Datastar: the switches again (a countdown reached zero) | - |
| `/ui/functions/{id}/trace/{context}/enable` | the enable dialog (slot `#trace-dialog-slot`; a navigation gets the log page with it open) | enable |
| `/ui/functions/{id}/trace/{context}/disable` | - | disable (no confirmation, as in the SPA) |

- **Embedding.** The function drawer calls `FunctionTrace.Switches(ctx, fnID, showLogLink)`, which returns
  `TraceSwitches(fnID, view)`: one card per context (name, built-in tag, description, Live/Off, countdown or
  "until switched off", sample rate), Enable/Change and Disable, the slot for the dialog and, with `showLogLink`,
  "View trace logs". Patch target `#fn-trace-switches`. `ui.Config.FunctionTrace` / `FunctionTraceOps` back it; `ui.Mount`
  builds the handler once. A post made from the log page carries `page=traces` and comes back to it; from the drawer, to `/ui/functions/{id}`.
- **Permissions** are the API's: the page needs `trace:view` (else a 403 page; the switches component shows the SPA's notice
  and reads nothing); Enable/Change/Disable need `trace:manage` and are left out without it; `http` also needs `trace:view-bodies`
  (shown disabled with the SPA's reason, refused again on the post). Bodies of `http` records are withheld without
  `trace:view-bodies`: the view model never keeps them (and an `http` record is never printed raw), so they are in no response.
  The function address under the title needs `function:view`.
- **Enable dialog.** Duration (5, 15, 30, 60 within the context's cap), "No time limit" (disables the duration), sample rate 1 to 1000.
  Field errors from the use case (`TRACE_WINDOW_TOO_LONG`, `TRACE_MINUTES_INVALID`, `TRACE_SAMPLE_RATE_INVALID`) land on the field.
- **Log.** Filters (context, kind, level, time, reference, correlation id, invocation id) are the URL (`?kind=http&since=60`) and apply
  live; reference, correlation and invocation cells are chips that filter to their value (the value rides in a data attribute, never in an
  expression). Rows are native `<details>` with `data-ignore-morph`: a record never changes, so refresh ticks leave the rows the viewer
  opened alone. `http` records expand to Request and Response panels (headers sorted, JSON pretty-printed, base64 labelled, truncation noted,
  copy button); other records show their message and pretty data. Load more appends 100 older records (a plain link works without Datastar).
- **Stream.** The filter bar's own request is the stream (`@get(el.action, {contentType: 'form'})`): Datastar cancels the bar's
  earlier request when a new one starts, so there is one stream per page. It patches status, context options, error line, records and
  "Load more" every 5 s while any window is live, then ends by itself when the window closes, the viewer leaves, or after 30 minutes
  (the page then reads "paused"; Resume reopens). Pause and Resume are the form's submitter (`pause=1`, `resume=1`), the paused state a hidden field.
  Enabling or disabling on the page resubmits the bar so the stream matches the new state.
- **Differences from the SPA.** Counts per invocation and the order of rows are as loaded; refresh ticks do not update the count of rows
  already shown. A new filter, Refresh or a restart replaces the list (rows collapse; the SPA kept them expanded). The copy buttons copy
  the text shown (pretty JSON), the SPA copied the raw body. "Back to function" links to `/ui/functions/{id}` (the drawer's deep link).
  The SPA's countdown is a client clock; here it is a client tick on a server-rendered label, and at zero the card asks for the switches again.
- **Kit.** No new kit part; `make ui-kit-sync` was run so `kit.css` has the classes the templates use.
- **Tests.** `function_trace_test.go` (permission matrix, forged posts, bodies and secrets never in a response, stream start/continue/end,
  paging, validation) and `function_trace_pg_test.go` (`-tags=integration`: enable, cap refusal, no limit, disable, forged posts, redaction,
  against the real use cases). Not done: the area has not been opened in a real browser (light, dark, narrow).

## Functions: list and drawer (wave 2, part 1)

Pages: `functions.go|templ` (list, filters, drawer, tabs), `functions_detail.go` (the drawer's view model, describe
parsing), `functions_form.go` (create and edit dialogs), `functions_actions.go` (versions and aliases, settings, routes,
delete), `functions_ops.go` (`FunctionCommands`, `FunctionOps`: the use cases). Spec: the SPA's `FunctionListPage.vue`,
`FunctionCreateDrawer.vue`, `FunctionDetailDrawer.vue` and `functionHelpers.ts`; the rules are `internal/platform/function/api`.
The trace switches and log are part 2 (above); the drawer embeds them through `FunctionTrace.Switches`.

| URL | GET | POST |
|---|---|---|
| `/ui/functions?q&application&client&page&rows` | the list (search is an address prefix; 100 rows by default) | - |
| `/ui/functions/new` | the create dialog | create |
| `/ui/functions/{id}?tab=` | the list with the drawer open; a Datastar GET patches the drawer | - |
| `/ui/functions/{id}/edit` | the edit dialog | update |
| `/ui/functions/{id}/delete` | the delete dialog (typed address) | delete |
| `/ui/functions/{id}/aliases/new?version=` | the point-alias dialog | `/aliases` (save), `/aliases/{name}/delete` (remove; live is the demote) |
| `/ui/functions/{id}/versions/{n}/...` | - | `promote` (the live alias, no confirmation, as the SPA), `retire` |
| `/ui/functions/{id}/settings/{config,secrets,db}/{new,edit}?key=` | the set dialog | `/settings/{kind}` (save), `/settings/{kind}/delete?key=` |
| `/ui/functions/{id}/routes/new`, `/routes/{routeId}/edit` | the route dialog | `/routes` (save), `/routes/{routeId}/delete` |

- **Reads** are the API's: `CanReadFunctions`; a caller below anchor is confined in SQL to platform functions and its
  clients (the list), and `auth.CheckScopeAccess` on the function for the drawer and every dialog. The drawer reads the
  versions, aliases, settings and routes of the function with the same check.
- **Writes** make the check of the endpoint they mirror, again on every post (a control that is not shown is not the guard):
  create, edit, delete, retire and config values `CanWriteFunctions` ("manage"); promote, point alias, remove alias
  `CanPromoteFunctions`; secrets and databases `CanManageFunctionSecrets`; routes `CanManageFunctionRoutes`. The claimed zones
  are listed with `CanManageFunctionDomains`. The use cases then check the function's client (`SCOPE_FORBIDDEN`, a 403 flash or
  the form's error), so a client user cannot touch another client's function or create a platform-owned one. A caller who may
  write but not read gets the flash only (the drawer and table are not re-read).
- **Drawer tabs** (kit `TabList`, `?tab=` kept in the address bar): Overview (details, Diagnostics, Danger Zone), Versions (versions
  and aliases), What it declares (endpoints, subscriptions, schedules, config/secret/db names, HTTP allow-list, emits, dispatches,
  from the live version, else the newest), Settings, Routes. Every action posts the drawer's record and tab (hidden fields from
  the signals), so the answer refreshes the drawer where the user is.
- **Secrets are write-only.** A secret or database row shows `set` or `missing`; the repository does not return the value and the
  row builder never reads one. The dialog is the kit password input with no value, a failed save does not echo it (config values
  are echoed: they are plain text), the use case encrypts it and masks it in the audit, and the success flash names the key only.
  `TestFunctionPagesNeverCarrySecretValues` serves every tab and dialog from a store that holds a secret and a DSN, and the
  integration test searches `aud_logs`, `msg_events` and every answer of a whole run for them.
- **Versions.** Promote to live for a ready version that is not live (the live one offers Demote…, which removes the live alias and
  unwires the function after a confirmation); Point alias… for any ready version; Retire… behind a confirmation, disabled with the
  reason for a retired version or one an alias still points at. The promote and demote flashes say what the wiring changed ("Version 2 is now live — pool
  fn-billing-invoice-pdf · 2 subscriptions created"), built on the server from the use case's summary; a native post gets the fixed key.
- **Delete** is the typed-address confirmation (the SPA's, `fcdev fn delete`'s): the dialog shows the version count and the live
  version; the server compares the trimmed text with the address exactly (case matters) and refuses anything else with the field
  error, whatever the dialog did. After the commit the artifacts no other function references are removed from the store, best
  effort, as the API does.
- **Create and edit.** Application select (all applications; disabled with the reason without `CanReadApplications`), name (the
  SPA's rule, checked again by the use case), client (the shared client autocomplete), runner pool, warm, service account (active
  accounts and the current one; disabled with the reason without `CanReadServiceAccounts`, and then the account is left alone
  because a disabled field is not posted), four limits (blank = default on create, unchanged on edit). Edit sends only what changed:
  description, pool (a blank pool clears it), service account set or cleared. Use-case codes (address exists, limit ceilings,
  service-account refusals) land on their field.

Differences from the SPA: dialogs for create and edit (the SPA edits in the drawer); the drawer is five tabs, not one scroll; the
row's Edit opens the edit dialog (the SPA opened the drawer in edit mode); the create dialog does not preview the address (the
help line says it is `{application code}.{name}`); a blank description now clears it (the SPA's `description || undefined`
could not); Promote to live is not offered on the version that is already live; the **Routes tab and claimed zones are new**
(the SPA has no routes UI; the API and the runner do): a route is hostname, path prefix, what it serves (live or one alias) and
alias prefixes, its hostname and path read-only when editing because they are its key; the client filter is a select of the
active clients the caller can access.

SPA behaviours that were bugs, not copied: edit could not clear the description; retiring an aliased version was offered and
failed with a toast; Promote to live was offered on the version already live.

Not verified in a browser: the page has not been opened in light, dark and narrow widths, and the tab signal round trip (a
tab kept across an action) is covered by the rendered markup in tests only. Tests: `functions_test.go` (permission matrix of the
drawer, forged posts for every write with and without the permission, dialogs refused, secrets never in a response, create and edit
bodies, promote/demote/retire, settings, routes, typed-address delete, native fallbacks) and `functions_pg_test.go`
(`-tags=integration`: the whole life of a function against the real use cases with encryption and audit checks, and client scope).
The sidebar's Functions entry links to `/ui/functions`; two shell tests that used it as the example of an unported entry now use Audit Log.

## Portal: apps and users (wave 4)

Mirrors `PortalAppsPage` and `PortalUsersPage` (sidebar Portal, shown with `platform:iam:portal-user:view`). Routes:
`/ui/portal-apps?client=` (the list), `/portal-apps/new?client=` and `/portal-apps/{id}/edit?client=` (the dialogs), POSTs
`/portal-apps/new`, `/{id}/edit`, `/{id}/delete`, `/{id}/assign-unassigned`; `/ui/portal-users?client=&app=&q=&page=&rows=`,
`/portal-users/{id}/grant?client=` (the dialog), POSTs `/{id}/grant`, `/{id}/suspend`, `/{id}/reactivate`, `/{id}/delete`
and `/{id}/apps/{code}/revoke`. The client is the page's scope: a native select above the table (a change is a
navigation, so every confirmation and dialog of a page carries the one client it was rendered for as a hidden field).

- **Permissions are the API's.** A read is `auth.CanReadPortalUsers(ac, client)` (an anchor, or access to the client plus
  the view or the manage permission); every change is `auth.CanManagePortalUsers(ac, client)` (an anchor, or access to the
  client plus the manage permission). The check runs in the handler on the client the post names (`manageView`), before
  anything is read, as in `internal/platform/portalidentity/api`; the portal use cases are Authorize-Public by design, so the
  page is the gate here as the API handler is. A viewer sees no action column, no New, no dialogs; a forged post from a viewer,
  from a manager of another client, or from a caller with no permission is refused and runs nothing. An app or identity of
  another client is "not found" under the client named (the use cases scope by client as well).
- **Client select.** An anchor with the view-clients permission picks from every client (the SPA's `clientsApi.list`); an
  anchor without it gets an empty select but may still open `?client=<id>`; a client-scoped caller is offered the clients it can
  access (named, or by id when it cannot read them) and lands on its first one.
- **Apps.** Name and description, code, the OAuth clients fronting the app (link to the SPA's OAuth client page, copy
  `client_id`), user count, status. The warning banner counts users with no portal app. Assign unassigned users is a menu
  action, disabled with the reason when nobody is unassigned or the app is inactive (the SPA disabled it silently); Delete
  explains that the app's OAuth clients go with it. Both ask first.
- **Create** runs `CreateAppWithOAuthClient` as the API: code (lower-cased), name, description, callback URLs one per line,
  client type. **The credentials dialog is the one place the secret exists**: client secret (confidential only), client id,
  the four endpoints and the Laravel environment snippet, each with a copy button, in the create response only
  (`Cache-Control: no-store`, also for a native post, which gets the page itself instead of post/redirect/get). It cannot be
  dismissed by Escape or the mask, and "Done" removes it from the page, so the plaintext does not stay in the DOM. It is in no
  URL, flash, signal or table patch, and the later list does not have it. A missing callback URL is warned about with a link to
  the OAuth client. The endpoints use the origin the browser addressed (its `Origin` header on the post).
- **Edit** has name, description and the active switch (the code is fixed); a blank description clears it.
- **Users.** The API's prefix search on email and name (debounced), the app filter (one of the client's apps, or "No portal app"),
  25/50/100 rows, newest first. State tag with the SPA's hover detail, app chips with the grant's source and date, source
  (Invite or SSO sign-in), last login, created. Row actions: Grant portal app (disabled with the reason when no active app is
  left to grant), Suspend or Reactivate, Delete; each chip has a remove button. Grant resolves the app by code among the
  client's, refuses an inactive app, as the API.
- Errors follow "Datastar errors": a refusal of the form is its error region (the code and the callbacks land on their
  fields), a refused action an error flash.

Differences from the SPA: dialogs instead of PrimeVue dialogs; Suspend and Reactivate ask first (the SPA toggled at once);
the confirmations have one message per action without the app or user named; the SPA showed the row actions of Portal Users to
every viewer (the API refused them), here they need the manage permission; the app filter is hidden for a client without apps
(the SPA disabled it); an app filter that is not one of the client's apps is dropped from the URL.

SPA behaviours that were bugs, not copied: the row actions of Portal Users were offered without the manage permission;
Assign unassigned users was disabled without saying why.

Not verified in a browser: the pages have not been opened in light, dark and narrow widths, and the credentials dialog's
self-removal and copy buttons are covered by the rendered markup in tests only. Tests: `portal_apps_test.go` and
`portal_users_test.go` (permission matrix, forged posts from a viewer, another client's manager and a caller without the
permission for every write, the credentials shown once with no-store, create and edit bodies, filters and paging, grant and
revoke rules, escaping) and `portal_pg_test.go` (`-tags=integration`: the whole life of an app, its OAuth client and its users
against the real use cases, the secret stored hashed and in no later answer, audit rows, refused posts writing nothing).
The sidebar's Portal Apps and Portal Users entries link to `/ui/portal-apps` and `/ui/portal-users`.

## Service accounts (`/ui/service-accounts`)

Mirrors `ServiceAccountListPage`, `ServiceAccountCreateDrawer` and `ServiceAccountDetailDrawer`. Routes: the list
(`?q&client&status&sort&dir&page&rows`), `/new`, `/{id}` (drawer), `/{id}/edit`, `/{id}/roles` (GET: the dialogs; POST: the
save), and POST-only `/{id}/{regenerate-token,regenerate-secret,token,delete,applications,all-applications}` and
`/{id}/connections/{connectionId}/{pause,activate}`.

- **Gates** are the API's, the same code (`serviceaccount/operations/gate.go`: `RequireAnchorWriter`, `RequireApplicationReach`,
  `AuthorizeWrite`, `AuthorizeCreate`; the API handlers call them too). Reads need `service-account:view`. Every write needs the
  permission for the operation **and anchor scope** (owner decision #19): create `create`; edit, roles, both regenerations, the
  mint, application access `update`; delete `delete`. An application-scoped caller (no all-applications access) reaches only the
  accounts bound to its applications, and creates none (the form has no application). Controls are hidden without the gate, and
  every route makes the check itself before it loads anything, so a forged post is refused without telling a real id from an
  invented one. The role ceiling (F8) is the use case's: `AssignRolesToServiceAccount` and `MintAdminToken` (moved out of the API
  handler into `operations`, `MintDeps`) refuse a role above the caller, 403 `ROLE_ABOVE_CALLER`, shown in the dialog or as a flash.
- **Shown once.** Create (OAuth client id and secret, webhook auth token and signing secret), Regenerate Token, Regenerate Secret
  and Generate Token answer with a dialog in that response only: `Cache-Control: no-store` (also stamped over the stream
  helper's `no-cache`, `noStore`), a copy button per value, in no URL, flash, signal, table or drawer patch; a native post gets
  the page itself (post/redirect/get would lose the values). The create dialog cannot be dismissed by Escape or the mask (the
  SPA's `closable=false`; the same reopen-on-close trick as the applications page); the other three can, and every way of closing
  removes the dialog from the page, so the plaintext does not stay in the DOM. The stored webhook secrets are encrypted at rest and
  never rendered; the drawer shows the public OAuth client id only.
- **Confirmations** are the SPA's `credentialConfirm` texts (regenerate token and secret: danger, the old value stops working at
  once and receivers still holding it reject deliveries; mint: warn, a live bearer for one hour, earlier tokens are not revoked,
  recorded in the audit log) and the delete dialog's list of what goes with the account. The SPA has no typed confirmation here.
  The regenerations have **no overlap window** (the 24-hour previous-secret overlap belongs to OAuth client secret rotation, in
  the OAuth Clients area, not in this drawer).
- **List.** Search (name, code), the single Client filter (needs the client list: anchor with `client:view`; without it the column
  shows ids and the filter is left out), Status, sort by name, code or created, 100-row pages. Clients column: none = "All", one or
  two by name, more as "first +N more".
- **Drawer.** Information (Edit), Webhook Credentials, API Access (the mint, disabled with the reason while the account is
  inactive), Roles (Manage Roles), Application Access, Connections, Danger Zone.
- **Roles dialog.** A kit multi-select with chips and search (role names contain a colon, which the pick list's ids do not allow).
  Roles holding platform permissions the user lacks are **locked** as in the SPA: not offered, and the ones the account holds are
  listed with a lock and posted back unchanged so saving does not remove them. Needs `role:view` for the role list.
- **Application access** is the linked SERVICE principal's, which the SPA drives through the principal API: reading needs
  `user:view`, writing `user:update` and, here, also the account gate above (stricter than the SPA: its drawer is anchor-only, so the
  client-administrator bounding of the principal API never applies). The all-applications switch (immediate, keeps the explicit
  grants) can be turned on only by a caller holding all-applications access; turning off one's own is refused (the principal API
  asks for a confirmation the switch never sends). The pick list saves the explicit grants.
- **Connections** are those whose service account this is (visible under the connections API's client scoping), Pause (asks) and
  Activate with `connection:update`; a connection of another account is not found. New Connection links to `/ui/connections/new`.

Differences from the SPA: dialogs for create and edit (the SPA uses the drawer); the edit form starts from the tier the client
links give when no scope is stored (the SPA started from ANCHOR, and saving cleared the links); an anchor account is created and
saved without clients (the SPA sent a stale selection, which the platform refused); the list has no Roles column (the list API
does not carry roles, so the SPA's column was always empty) and the connections table no Endpoint column (the field does not
exist); flash texts do not count roles or applications (a flash key carries no request data); the pick list replaces the
move-between-lists dialogs.

Not in the SPA, so not here: **Activate and Deactivate** (the drawer has neither; the API has a deactivate use case but no
activate, so a deactivated account could never come back, and the page does not offer a one-way button), and application
provisioning (the Applications page offers it).

Not verified in a browser: the pages have not been opened in light, dark and narrow widths; the credentials dialog's
self-removal, the code suggested from the name, the scope-driven client field and the copy buttons are covered by the rendered
markup in tests only. Tests: `service_accounts_test.go` (permission matrix with forged posts from a viewer, a non-anchor holder of
every permission and an application-scoped caller on every route, the controls per permission, the list filters, create and edit
command bodies, the shown-once dialogs with no-store and no secret in a signal, the roles lock, application access rules,
connections) and `service_accounts_pg_test.go` (`-tags=integration`: create with the stored credentials shown, edit and the
principal's reach following the links, the role ceiling, regeneration, the audited mint, application access, forged posts by a
non-anchor caller writing nothing, delete). Mutation checks: dropping the permission-and-anchor gate from the shared write gate
fails the matrix test (a forged edit and application-access post reach the use case), and dropping the `noStore` stamp fails the
three tests of the shown-once dialogs.
The sidebar's Service Accounts entry links to `/ui/service-accounts`.

## Ingestion: tabs, sets and test connection (wave 3, second half)

Mirrors the Nuxt control plane's drawer sections (`IngestionDrawer`, `IngestionFailures`, `IngestionHistory`), `SetsPage` and
`ConnectionTestPanel`. All of it is mounted only with `FC_INGESTION_ENABLED` (`ui.Config.IngestionEnabled`); with the flag off
the routes do not exist and answer 404. Files: `ingestion_tabs*`, `ingestion_sets*`, `ingestion_testrun*` in `internal/ui/pages`.

**Drawer panels** (`IngestionTabs`). The drawer embeds `IngestionActivityTab(id)`, `IngestionFailuresTab(id)` and
`IngestionHistoryTab(id)`: a placeholder with `data-init="@get(el.dataset.url)"`, which asks for its panel at
`GET /ui/ingestions/{id}/activity|failures|history` (a tab that is never opened is never read; a navigation to a panel goes to
the ingestion's own page). Every route makes the by-id checks of `GET /api/ingestions/{id}/...`: `CanReadIngestions`, the
ingestion exists, `ingestion.CheckReach` (client and application).

- **Activity**: the route state each runtime last reported (status tag, the version it runs, warned when it is not the live one,
  the reported time, the error), then the exchanges (completed, failed, records, jobs) over the last 24 hours and 7 days, the
  last exchange and the last failure. These are new reads: `Repository.ListRouteStates` and `Repository.ActivityOf`
  (`internal/platform/ingestion/activity.go`, queries in `ingestion_reads.sql`); a metric window counts in the period its start
  falls in.
- **Failures**: newest first by id, 20 a page, `?after=<id>` appends the next page under the ones shown. The kind as people say it
  (Payload rejected, Not dispatched, Source error, Login refused), the runtime and version, the message, "After N records".
  A notice says whether payloads are kept. **The excerpt is shown only while the ingestion keeps failed payloads**, even if an
  older row still holds one. The cursor must look like an id; anything else is refused before the store is asked.
- **History**: the audit rows of the ingestion (the API's `.../history`), newest first, 20 a page by offset. The audit log names an
  operation by its command's type (`UpdateCommand`, `SaveVersionCommand`, ...), which is what the verbs map from. The rows hold the
  command, not the state before it, so "Changed: name, source" is computed by comparing the definition a command carried with the
  newest older command that carried one; **only field names are ever printed, never definition content** (a definition holds
  secret references and addresses). A create has no changed list.

**Sets** (`IngestionSets`, `/ui/ingestion-sets` and `/ui/ingestion-sets/{name}`). Reading is the API's `canReadSets`: the view
permission **and** a caller that reaches every client and application (a client-tier or application-scoped caller is refused with
`INGESTION_SET_ANCHOR_ONLY`; the sidebar offers the entry to anchor-tier callers with the view permission). The list has the owner counts and last-applied
time; a set's page lists its kept applies newest first (who, when, items and files, going back marks, the changes that did
something as tags that open the ingestion, a removed one has no link) and loads **the files** of an apply on first open
(`.../applies/{id}/files`, from the archive; an archive no longer kept is "the set does not keep apply"). A read-only note says
applying is done with `fcdev ingest apply`; there is no apply here.

- **Go back to this** (on every apply but the newest; needs `platform:ingestion:set:apply`): the button opens a confirmation whose
  plan is a **dry run** of the same use case (`GET .../reapply`, nothing is written), with the SPA's wording that it lasts until the
  set is applied again. Confirming posts `.../reapply`: a new apply with `goingBackTo`, the lists patched, a flash naming the apply
  ("already is as ... left it" when nothing changed). **Release the set** (the same permission, shown while the set owns
  ingestions) asks first and posts `.../release`. The POSTs make the API's check (the apply permission; the use cases check reach
  per ingestion), so they answer a caller that may not list sets with the message only, never with the sets.
- Native posts redirect to the set with a fixed flash key (`ingestion-set-reapplied`, `ingestion-set-released`).

**Test connection** (`IngestionTests`, needs `platform:ingestion:ingestion:test`). `IngestionTestPanel(action, fromForm)` is the
button and the place the outcome goes (`#ing-test-result`). `POST /ui/ingestions/{id}/test` tests a saved ingestion's live
definition; `POST /ui/ingestions/test` tests the definition a form holds and exists only when `ui.Config.IngestionTestSubject`
(reads the form into application, client and definition) is set. The unsaved path makes the checks of a save and of
`POST /api/ingestion-tests`: the definition validates, the client's scope, the application's reach, and **the permission a custom
kind declares** (`camel-route` needs `platform:ingestion:ingestion:route`). The POST queues the test and answers with a box that
asks for `GET /ui/ingestion-tests/{id}/stream`, which polls the row every second and patches the box when something changed:
waiting (with "no runtime carries this kind" **at once** when none does), a runtime is trying, the outcome with its steps, records
and excerpt, or **expired** once the two minutes are up; it ends when the test does or the viewer leaves. A new test's box has its
own id, so the stream of an earlier test patches nothing. The stream makes `GET /api/ingestion-tests/{id}`'s checks (test
permission, the test's client and application reach). The excerpt is the one the platform kept when the runtime answered (none
unless the ingestion keeps payloads). A native post queues nothing and goes back to the list.

Differences from the Nuxt pages: the audit-based history has no version column (the audit row does not hold the version); failures
are kept 30 days here (`FailuresRetention`), the Nuxt page said 14; the unsaved test needs the form's reader (below); the Nuxt "Show
files" button is a `<details>` that loads on first open.

Not verified in a browser: the pages have not been opened in light, dark and narrow widths, and the streamed test has been
exercised against Postgres through `httptest`, not with a running runtime. Hand-offs: the drawer (the other half) must embed
`IngestionTestPanel("/ui/ingestions/"+id+"/test", false)` for a saved ingestion (offered with the test permission) and, for the form,
`IngestionTestPanel("/ui/ingestions/test", true)` together with a `Subject` that parses its form.

Tests: `ingestion_tabs_test.go`, `ingestion_sets_test.go` and `ingestion_testrun_test.go` (permission matrices with client-tier and
application-scoped callers, forged ids and cursors, forged posts that must run nothing, escaping, paging, the plan being a dry run,
the stream's states in order, expiry, the kind permission) and the `*_pg_test.go` files (`-tags=integration`: the new reads with real
windows, failures paged by cursor, the history from real audit rows, sets applied through the use case with the real archive, going
back and releasing, test connection with the real poll and report, the required permission read from the kinds a runtime reported).
Mutation checks: showing the excerpt without the keep-payloads condition fails `TestIngestionFailuresShowAnExcerptOnlyWhilePayloadsAreKept`;
dropping the custom-kind permission check from the unsaved test fails `TestIngestionTestOfAKindThatNeedsAPermission`; dropping the
set-apply permission from going back fails `TestIngestionSetForgedPostsChangeNothing`.

## Authorization: roles and permissions (wave 4)

Pages: `roles.go|templ` (list, drawer, delete, platform sync), `roles_form.go|templ` (create dialog, editor page, the
new-permission builder), `roles_ops.go` (`RolesCommands`, `RolesOps`: the use cases; `PermissionCatalogue`, `PermissionStore`:
the catalogue), `roles_flash.go` (flash keys, registered in `init` so the shared flash table is not edited),
`permissions.go|templ` (list and create dialog). Spec: the SPA's `RoleListPage.vue`, `RoleDetailDrawer.vue`,
`RoleEditPage.vue`, `PermissionListPage.vue`, `admin-permissions.ts` and the BFF handlers `internal/platform/shared/bff/roles.go`
(the SPA calls `/bff/roles`, so its gates are the spec, not `/api/roles`'s read confinement).

| URL | GET | POST |
|---|---|---|
| `/ui/roles?q&app&source&page&rows` | the list (search over name, display name, description; 100 rows by default) | - |
| `/ui/roles/new` | the create dialog | create |
| `/ui/roles/{name}` | the list with the drawer open; a Datastar GET patches the drawer | - |
| `/ui/roles/{name}/edit` | the editor page (a role that cannot be edited returns to its drawer with a warning) | save |
| `/ui/roles/{name}/edit/add-permission` | - | adds a new permission to the editor (writes nothing) |
| `/ui/roles/{name}/delete` | - | delete (confirmation dialog) |
| `/ui/roles/sync-platform` | - | the platform role sync (no confirmation, as the SPA's dashboard button) |
| `/ui/permissions?q&app&ctx&action&page&rows` | the list | - |
| `/ui/permissions/new` | the create dialog | define a permission |

Role names contain `:`, so they are the path segment as they are (the SPA encoded them); a segment that is not a canonical role name
(`role.ValidateName`) is a 404 and never reaches a query or an attribute.

- **Gates** are the BFF's, checked again on every post: reads need `role:view` (a 403 page without); create (`POST /bff/roles`, defining
  a permission) `role:create`, edit `role:update`, delete `role:delete`, the platform sync all three, each **with anchor scope on top**
  (`RequireAnchor`), so a client user holding the permissions sees no write control and a forged post is refused. The controls follow the
  same functions (`rolesPermsFor`). The use cases then apply what the page cannot: the permission ceiling (F10, `PERMISSION_ABOVE_CALLER`),
  the application confinement (F12, `PERMISSION_OUTSIDE_APPLICATION`), the code-role refusal, and the name rules (F18).
- **Only admin-created roles** (source `DATABASE`) are edited or deleted: the SPA offers neither for code-defined or SDK-registered roles,
  and the page refuses the posts too (`409 ROLE_NOT_ADMIN_CREATED`; the use cases refuse a code-defined role only). The editor GET for such
  a role, or without `role:update`, returns to the drawer with the SPA's "Not Editable" warning.
- **Locks.** A role holding a platform permission the caller lacks (`role.PermissionsAboveCaller`, the SPA's `isRoleAboveUser`) shows a
  disabled Delete with the SPA's reason instead of the trash. In the editor, every platform permission the caller lacks is locked: not in the
  multi-select, listed under "N locked permissions" with the SPA's explanation; the ones the role holds are posted back as hidden fields so
  saving keeps them. A forged post that drops or adds one is refused by the use case and its message lands on the permissions field
  (tested with and without the lock). A super-admin has no locks.
- **The permission picker** is the kit multi-select with search and chips (`kit.MultiSelect`, one value per checked permission) with
  Select all and Clear. The offered list is the application's catalogue (`bff.PermissionCatalog`, exported from the BFF for this: the platform
  builtins, the permissions declared by roles and the `iam_permissions` table), plus what the role holds and what was added in this editing.
  A role holds only its own application's permissions (F12), so there is one group; the SPA's per-application grid and checkbox collapse into it.
- **New permission** (non-platform roles only): application code, three segments (lowercased and slugified when an input loses focus, the SPA's
  `slugifySegment`) and Add. The SPA adds it to the list client-side; here Add posts the form to `add-permission`, which answers with the picker and
  the builder again (the new permission checked). It reaches the role only with Save. Enter in the builder presses Add, Enter in the picker's
  search does nothing, so neither saves the role. Without script, Add is a native submit that re-renders the page.
- **Create permission** (the permissions page): the four segments must be lowercase tokens (`bff.ValidPermissionSegment`, the BFF's rule) and the
  code new (the SPA keeps Create disabled for an existing one; here the field says so); the write is the BFF's `PermissionRepo.Upsert`, no use case,
  no event, as the BFF does it. An application code with an underscore fails the rule, as it does at the BFF.
- **Credentials and secrets:** this area has none. Nothing here is shown once or kept in page state.
- **Kit:** no new part and no kit change; `make ui-kit-sync` was run for the new classes. Exported from the BFF for the catalogue and the segment rule:
  `bff.PermissionEntry`, `bff.PermissionCatalog`, `bff.ValidPermissionSegment` (append-only in `roles.go`).

Differences from the SPA: the delete confirmation is the shared dialog ("Delete this role?"), not one that names the role; the drawer lists every
permission (the SPA paged by 10); the application and source filters are multi-selects; the picker shows the permission code only (descriptions are
on the permissions page); Save is not disabled while nothing changed; "Sync Platform Roles" is on the roles page (the SPA has it on the dashboard);
the permissions list keeps the catalogue's order.

SPA behaviours that were bugs, not copied: the editor sent a blank description as "unchanged", so a description could never be cleared; a blank display
name was sent as "unchanged" and silently kept, here it is a field error.

Tests: `roles_test.go` and `permissions_test.go` (permission matrix incl. the anchor gate and a client user, forged posts for every write with and without
the permission, locks and the retained locked fields, create/edit/delete/sync bodies and error mapping, the add-permission rules, the page contract,
native fallbacks) and `roles_pg_test.go` (`-tags=integration`: the real use cases, the ceiling and confinement refusals through the page, audit counts,
the platform sync, the catalogue). Mutation checks: with `RequireAnchor` removed from `rolesWrite`, four tests fail (the matrix, the forged posts, the
refusal redirects, the permissions matrix); with the hidden locked fields removed from the editor, `TestRolesEditPage` and the Postgres test fail.
Not verified in a browser: the area has not been opened in light, dark and narrow widths, and the Datastar expressions (Select all, the Enter guard, the
slug on blur, the picker patch) are covered by the rendered markup only.

## Developer portal and Developer Users (wave 4)

Pages: `developer.go|templ` (applications list, application page with its three tabs, API versions, the OpenAPI download),
`developer_docs.go` (the OpenAPI reader behind the API docs tab: operations grouped by tag, `$ref` resolution, one operation's parameters,
body and responses), `developer_users.go|templ` (`DeveloperUsers`, `DeveloperUserOps`: the use cases). Spec: the SPA's
`frontend/src/pages/developer/*` and the BFF handlers `internal/platform/shared/bff/developer.go`; the Developer Users page's endpoints are
`internal/platform/principal/api` (`/api/principals/developer-users`, `.../{id}/developer-credential`, `.../{id}/roles`).

| URL | GET | POST |
|---|---|---|
| `/ui/developer` | the applications | - |
| `/ui/developer/applications/{id}?tab=api-docs\|event-types\|processes` | the application, the chosen tab rendered in place, the others as placeholders | - |
| `/ui/developer/applications/{id}/tabs/{tab}` | a tab's content: the lazy load and every filter or selection change (Datastar patches the panel and the address; without script it redirects to the page) | - |
| `/ui/developer/applications/{id}/operation?m&p&n` | one operation's detail (Datastar patches its row; without script a page of its own) | - |
| `/ui/developer/applications/{id}/openapi.json` | the current document as a download | - |
| `/ui/developer/applications/{id}/versions` | the API versions | - |
| `/ui/developer-users?q&sort&dir&page&rows` | the developer users | - |
| `/ui/developer-users/grant`, `.../grant/search?q` | the Grant Developer Role dialog; its search | - |
| `/ui/developer-users/{id}/confirm/{rotate\|revoke\|remove}` | the confirmation dialog, naming the user | - |
| `/ui/developer-users/{id}/{grant\|remove-role\|credential\|revoke}` | - | the action |

- **Gates** are the BFF's and the principal API's, made again on every request (the portal is read-only, so there is nothing to post): every portal route
  needs `CanReadDeveloperPortal` (anchor scope and `application-openapi:view`); the processes tab also needs the process list's own `process:view`, which
  the portal's permission does not imply, and says so in the tab when it is missing (no process data is read). Developer Users: the list `user:view`
  (`CanReadPrincipals`), like the principal list restricted for a non-anchor caller to the principals of the clients they reach; setting, rotating and revoking a
  credential is `requireDeveloperCredentialAccess` (the caller's own record needs `api-credential:manage`, anyone else's `user:update`, then the use case checks the
  target is a developer in scope); granting and removing the role need `user:assign-roles` **and anchor scope** (see the differences). The SPA shows every control;
  here a control the caller cannot use is left out, and the handler refuses its post and its confirmation dialog anyway.
- **API docs without Swagger UI.** Swagger UI is not vendored. The tab lists the current document's operations grouped by tag (the document's tag order, then the
  rest; method, path, summary, deprecated), each row a closed `<details>` that loads its detail the first time it is opened (a path from the document is a plain
  attribute, never in an expression): parameters (path-level and operation-level merged), request body, responses, and each media type's schema as JSON in the
  event type viewer's code block, local `$ref`s inlined (a recursive reference is left as `{"$ref": ...}` where it recurses, depth and expansions are capped). Descriptions
  are rendered Markdown (sanitised). Download OpenAPI is a link to the formatted document under the SPA's file name. What this loses against Swagger UI: the models
  section and the schema tree (collapsible types with required marks and constraints; the JSON is the resolved schema, in key order of the encoder, not the document's),
  generated example values, the servers and security-scheme display, anchor links per operation, and in-page search of operations (the SPA's Swagger UI had its filter off too).
  "Try it out" is off in the SPA and absent here.
- **Tabs load when shown.** The tab bar is `kit.TabList` (`?tab=` keeps the tab in the address). The chosen tab is rendered with the page; the others are placeholders with
  `data-on-intersect__once` that fetch `.../tabs/{tab}` the first time the tab becomes visible, so a large OpenAPI document is not read for someone who looks at event types.
  Event types and processes keep their filters and selection in the query (`etsub`, `etagg`, `etstatus`, `et`; `psub`, `pstatus`, `proc`); status defaults to CURRENT and
  an empty `etstatus=` means every status, as the SPA's cleared select.
- **Event types tab:** the SPA's master-detail: the table (code chips, name, description, status; the first row selected), and under it the selected event type's viewer, which is the event
  types area's schema viewer (`toETViewer`: schema, example, TypeScript, PHP, Python, Java from `schemacodegen`, with the copy button), opening the CURRENT version, else FINALISING, else the first.
- **Processes tab:** the SPA's row opened the process page. Here a row selects the process and its diagram is drawn under the table by the Mermaid island (other diagram types show the source), with
  Open process (to `/ui/processes/{id}`) and, for a caller with `process:update` and a process that is not archived, Edit; the row's pencil is the same link. The diagram is not drawn until the
  island's script has loaded (the page loads it).
- **Developer Users.** The role changes run `AssignRoles` with the user's roles plus (or minus) `platform:developer`, as the SPA's replace-the-set call does, from the principal as loaded now. The
  credential is `SetDeveloperCredential` / `RevokeDeveloperCredential`. Removing the role also revokes the credential, best effort, as the SPA does (so re-granting does not bring the old secret
  back); a failure of the revoke is logged and does not undo the removal. Confirmations are the SPA's wording and name the user, so they are dialogs fetched per user (`.../confirm/...`), not the
  shared confirm dialog; setting a first secret asks nothing; rotating, revoking and removing the role confirm.
- **Credentials and secrets.** A new developer secret is shown once: the credential action answers with the dialog in that response, which is `Cache-Control: no-store`, with a Copy button, the
  `client_id`, and "Copy this secret now. It will not be shown again."; the value is in no URL, flash, signal or redirect (a native post gets the page itself, as post/redirect/get would lose it), and
  the dialog removes itself from the page however it is closed (Done, Escape). Nothing else renders it: not the list, the confirmations, the search or a later response. The stash that hands the
  plaintext over is the use case's (`PopDevClientSecret`, two minutes, once).
- **Kit:** no new part and no kit change (the tab list, Mermaid island, Markdown renderer and code block are used as they are); every class used is already in `kit.css`. Shared files touched, appended or
  anchored only: `internal/ui/ui.go` (Config fields and the two Mount lines), `internal/server/wire_ui.go`, `internal/ui/shell/nav.go` (the Applications and Developer Users entries now point to `/ui`),
  `internal/ui/pages/flash.go` (three keys for the native fallbacks).

Differences from the SPA: the API docs tab as above; a processes row selects instead of navigating; the event type filters are in the toolbar's popover; the Developer Users controls follow the caller's
permissions; the grant, remove and revoke toasts name the user on a Datastar answer and not after a native post; the list is paged and searched on the server; the Developer Users entry in the sidebar is
`/ui/developer-users` (not under `/ui/developer`, which would mark two entries current). **Role changes are anchor only**: the principal API lets a non-anchor administrator with `user:assign-roles` assign
application-scoped roles of reachable applications and refuses `platform:developer` to them (`operations.RequireAssignableRoles`, a platform role), a bound the handler applies and `AssignRoles` does not, so the page,
which runs the use case, requires anchor scope instead of repeating the bound: it refuses what the API refuses for this one role. The portal pages are not filtered by the application access of the principal (`iam_principal_application_access`) because the BFF does not
filter either, whatever its comment says. Sync Platform OpenAPI (`POST /bff/developer/sync-platform-openapi`) is not on these SPA pages (the SPA's dashboard calls it) and is not ported here.

Tests: `developer_test.go` (permission matrix over every route for a caller without the permission and a non-anchor, the tabs and their state in the URL, the processes tab without `process:view`, the
docs rendering, `$ref` handling and the recursion cut, the download name, versions), `developer_users_test.go` (list, search, sort and scope; the controls per caller; forged posts for all four actions
by callers without the permission, including `user:update` for a role change and a client administrator holding `user:assign-roles`; the confirmations; the secret shown once, `no-store`, in no other
response) and `developer_pg_test.go` (`-tags=integration`: the portal over a synced and re-synced document, an event type and a process; the users flow through the real use cases with the secret
returned once and only its keyed hash stored, the role removal taking the credential, audit rows). Mutation checks: removing the anchor requirement from the role-change gate fails
`TestDeveloperUsersControls`, `TestDeveloperUsersForgedPosts` and `TestDeveloperUsersGrantDialog`; removing the `no-store` wrapper from the credential action fails
`TestDeveloperUsersSecretShownOnce`; removing the process list's permission check from the processes tab fails `TestDeveloperProcessesTab`.
Not verified in a browser: the area has not been opened in light, dark and narrow widths, and the Datastar expressions (`data-on-intersect__once` on the placeholders, `data-on:toggle__once` on the
operation rows, the dialogs' `data-on:close`) are covered by the rendered markup only.

## Ingestions: list, form and drawer (wave 3, part 1)

`/ui/ingestions`, mounted and linked from the sidebar (Platform: Ingestions, Ingestion sets) **only when
`FC_INGESTION_ENABLED` is on** (`ui.Config.IngestionEnabled`, `shell.Features`); off, every route here is a 404. The
Nuxt control plane (`inhance/mono-repo/apps/ingest-control-plane/web`) was the specification. Files:
`internal/ui/pages/ingestions*.go|templ`.

| URL | GET (navigation) | GET (Datastar) | POST |
|---|---|---|---|
| `/ui/ingestions` (`?q&application&client&kind&status&after`) | the list | table, pager, toolbar state, URL | - |
| `/ui/ingestions/new` | list + create dialog | the dialog | create |
| `/ui/ingestions/{id}` (`?tab&a&b`) | list + drawer | drawer content | - |
| `/ui/ingestions/{id}/edit` (`?from=N`) | list + drawer + dialog | the dialog | save a version |
| `/ui/ingestions/{id}/{pause,resume,delete}`, `/versions/{n}/live` | - | - | the action |
| `/ui/ingestions/{id}/compare?a&b` | the drawer on Versions | both definitions | - |
| `/ui/ingestions/{id}/definition.yaml` | the definition file download | - | - |
| `/ui/ingestions/{id}/tabs/{activity,failures,history}` | redirect to the drawer | the tab's placeholder | - |

- **Checks are the API's**: view `GET /api/ingestions` (reach in SQL through `ingestion.ListReach`: a client user its
  clients, an application's account its application; by id `ingestion.CheckReach`); create `CanCreateIngestions`;
  a change `CanUpdateIngestions`, and `CanPromoteIngestions` for "make live when saved"; pause and resume update;
  remove delete; make live promote. Controls are left out without the permission, the server decides again on every
  post, and the use case owns reach, the set that owns an ingestion (`INGESTION_SET_OWNED`) and the kind's permission.
- **The form is drawn from data** (`ingestions_kinds.go`): each source kind is a list of fields (text, number,
  duration, secret NAME, choice, switch, lines, code, name: value lines), each with its JSON path in the definition and
  a condition. One reader makes the definition JSON from the post (`ingsBuildDefinition`, then the strict
  `ingestion.ParseDefinition`), one reads a stored definition back (`ingsValuesOf`; a round-trip test covers every
  kind), and a validation error's path (`source.auth.password`, `...password.name`, `source.headers.1.name`) finds
  its control. Durations are written `30s`, `5m`, `2h`, `1d` or ISO and stored ISO. A secret field holds a name
  (`kit.SecretRefInput`), never a value. Pool, connection and service account are codes, checked by the API's resolver
  at save (`INGESTION_TARGET_INVALID` lands on the field it names).
- **Conditional fields**: every kind is a `<fieldset>`, and so is every branch (auth kind, track, results, rate limit,
  cursor, own access key). `data-show` shows it and `data-attr:disabled` disables the rest, so what is not chosen is
  neither posted nor checked by the browser (a hidden control with a pattern would otherwise block the submit
  without a message). No kind field carries the HTML `required` attribute for that reason; the server asks.
  A database poll has no split (each row is a record) and is saved with `none`.
- **Kinds runtimes report** come from `ingestion.Repository.LiveKinds`, as the API's source-kinds list. A kind that
  declares a required permission (camel-route: `platform:ingestion:ingestion:route`) is **not offered** without it, and
  changing such an ingestion is shown disabled with the reason; the use case refuses it again. An extension's
  `settings` are read as the Nuxt `SourceKind` field list (`[{name,label,description,type,required,defaultValue,
  choices,min}]`, or `{"fields": [...]}`; types TEXT, MULTILINE, INTEGER, BOOLEAN, DURATION, URL, SECRET, CHOICE,
  LIST), because the platform passes `settings` through untouched and the runtime contract does not fix its shape.
  Field names that are not a setting name are skipped. A kind with no description is edited as a JSON object
  (camel-route's is its route, in the YAML editor). A stored ingestion of a kind no runtime reports is still editable.
- **Drawer**: Overview (set notice, "no runtime carries this kind" notice, details, actions), Definition (the
  definition file in the read-only editor, Export YAML), Versions (live badge, notes, who and when, Make live,
  Change from this version, New version, Compare: both definitions side by side), and Activity, Failures and History,
  which are `IngestionTabs`' (`IngestionActivityTab` and its siblings). Those three are empty slots until the tab is
  on show (`data-on-intersect__once`): the slot asks `/tabs/{tab}`, which makes the drawer's checks and patches the
  tab's own placeholder in, and the placeholder asks for its panel. A set's ingestions are read-only (no change,
  new version, make live or remove); pause and resume still work.
- **A change is a saved version** (`POST /api/ingestions/{id}/versions`): the live one keeps running until a version
  is made live, here with the switch (promote) or from the Versions tab. The dialog says which number it will be.
- **Not built here**: test connection (`ingestion_testrun*`) and the sets pages (`ingestion_sets*`) belong to the other
  half; the Nuxt "Keep payloads" toggle is the form's switch (it is part of a version).

Tests: `ingestions_test.go` (fakes: list, filters, cursor paging, reach, drawer, permission matrix, set-owned,
kind permission, form rendering, create and save with field errors, forged posts, every kind's round trip),
`ingestions_pg_test.go` (tag `integration`: the real use cases, reach, set ownership, camel-route and the route
permission), `ingestions_flag_test.go` (flag off is a 404, the sidebar entries) and `shell/nav_ingestion_test.go`.
Mutation checks (run, seen to fail, restored): skipping the permission check of `runAction` fails
`TestIngestionForgedPostsAreRefused`; dropping `ListReach` from the list fails `TestIngestionsListPermissions`.

## Users (wave 4)

Pages: `users.go|templ` (list, filters, drawer frame), `users_detail.go` (the drawer's view model, the shared "after a change" answer, the assignable roles), `users_form.go` (create, email check, edit), `users_actions.go` (state and account actions, password, roles, application and client access, CSV import), `users_dialogs.templ`. Spec: the SPA's `UserListPage`, `UserCreateDrawer`, `UserDetailDrawer`/`UserDetailBody`, and for a client administrator `ClientUsersPage`, `ClientUserCreateDrawer`, `ClientUserDetailDrawer` and `UserImportDialog`.

| URL | GET | POST |
|---|---|---|
| `/ui/users?q&status&client&roles&sort&dir&page&rows` | the list (100 rows; 50/100/250/500) | - |
| `/ui/users/new` | the create dialog | create |
| `/ui/users/check-email?email=` | Datastar: the email's domain check, patched into `#usr-form-domain` | - |
| `/ui/users/{id}` | the list with the drawer open; Datastar patches the drawer | - |
| `/ui/users/{id}/edit` | the edit dialog | name, and for an anchor the type and client |
| `/ui/users/{id}/{activate,deactivate,delete,send-reset,reset-2fa}` | - | the action |
| `/ui/users/{id}/reset-password` | the dialog | set the password |
| `/ui/users/{id}/roles`, `/applications` | the picker dialogs | replace the set |
| `/ui/users/{id}/all-applications/{grant,restrict}` | - | the all-applications flag |
| `/ui/users/{id}/clients/new`, `.../clients`, `.../clients/{clientId}/revoke` | the grant dialog | grant, revoke |
| `/ui/users/import`, `/import/template.csv` | the import dialog, the template | the import (multipart) |

- **One rule set with the API.** The principal API's handler bodies hold the gates (client scoping, `blockNonClientTarget`, a client administrator's role and application bounds, the all-applications and self-restriction rules, the role ceiling) and the use cases hold the rest (the password policy, the last-administrator guard). Rather than copy them, `principalapi.State` has exported in-process entry points (`internal/platform/principal/api/ui_entry.go`) that run each handler unchanged with the caller on the context; the page calls those (`pages.UserService`). `internal/server/wire_ui_users.go` builds the state for the routes and the page alike. The page also makes the coarse permission check itself first, so a control that is hidden is never the only guard, and a forged post by a caller without the permission is refused before the service is asked (and again by it).
- **Two audiences.** A platform administrator (anchor) gets the SPA's platform page. Anyone else gets the client administrator's: its title, no Type column, the client filter only with several clients, the create form with an optional password (CLIENT users only, the client implicit with one), and a drawer without client access, the all-applications switch or delete, as the SPA. Both sidebar entries (User Management, Client Administration) link to `/ui/users`.
- **Controls by permission.** Create and import `user:create`; Edit (name), applications, password, send reset, reset 2FA, activate and deactivate `user:update` (Edit is also offered to an anchor, who may change the type and client); roles `user:assign-roles` plus `role:view` for the list; delete `user:delete`, anchor only; client access, type and client anchor scope; granting all-applications access only to a caller holding it.
- **Roles.** A multi-select with search and chips, as the SPA's `assignableRoles`: every role for an anchor-scope user or one with all applications, else the roles of the applications the user can access plus those already assigned. Roles whose platform permissions the caller lacks (the ceiling, `ROLE_ABOVE_CALLER`) are not offered; the ones the user already holds are listed as kept and posted back unchanged.
- **Credentials.** No page or answer carries a password or a stored hash: the password fields have no value to echo and a failed save re-renders them empty (`TestUserPagesNeverCarryCredentials`, the integration test searches every answer for the passwords and the stored hash). The **sign-in link** is minted only when the create form's switch is on ("Show the link instead of emailing it", INTERNAL users who set their own password); then nothing is emailed and the link is shown once, in a dialog patched into the create answer, `Cache-Control: no-store`, with a Copy button; the dialog removes itself however it is closed, and the link is in no signal, URL or flash. A native post gets the page itself, since a redirect would lose the link.
- **Edit** compares against the stored user: the name is sent when it changed, the association when the type or client did (`ANCHOR` is `*`, `CLIENT` changes the home client, `PARTNER` promotes), and a CLIENT or PARTNER without a client is refused before anything is written (the SPA saved the name first).
- **Import.** A dialog with its own multipart form (a file must reach the server from a post without script): client, CSV file of Full Name, Email and pipe-separated Roles (a header row is skipped; at most 1000 rows). The report lists the rows not created.

Differences from the SPA: dialogs instead of drawer forms; the drawer is one column as in the SPA but the pickers are multi-selects, not dual panes; revoking client access asks first (the SPA revokes at once); the all-applications switch is a Grant/Restrict button, and restricting your own flag asks (as the SPA); the sign-in link switch is new (the API's `returnInviteLink`); the confirmations name no user (shared dialogs). Passkey administration: the SPA's users pages have none. The permission-denied dialog was not needed (a refusal is the flash).

Not verified in a browser: the area has not been opened in light, dark and narrow widths, the email check's round trip, the scope/client swap in the edit dialog, the chips in the dialogs and the file upload through Datastar are covered by rendered markup and the multipart tests only. Tests: `users_test.go` (permission matrix of the drawer, forged posts for every write, dialogs refused, sign-in link on request only, credentials never in a response, list URL state, edit mapping, import) and `users_pg_test.go` (`-tags=integration`: the real API handlers: create with and without the link, edit and association modes, roles, applications, password, delete and the last-administrator guard, a client administrator confined to its client and bounded in roles, view-only forged posts, the role ceiling).

## Platform pages A: audit log, login attempts, CORS origins (wave 4)

Pages: `audit_log.go|templ`, `login_attempts.go|templ`, `cors_origins.go|templ`. Spec: the SPA's `AuditLogListPage`, `LoginAttemptListPage` and `CorsOriginsPage`; the old pages stay and the three sidebar entries now link to `/ui`.

| URL | GET | POST |
|---|---|---|
| `/ui/audit-log?entityType&operation&applications&clients&after` | the list (100 per page, newest first, keyset) | - |
| `/ui/audit-log/{id}` | the list with the drawer open; Datastar patches the drawer | - |
| `/ui/login-attempts?attemptType&outcome&q&from&to&after` | the list (100 per page, newest first, keyset) | - |
| `/ui/login-attempts/{key}` | the drawer; `key` is `<attempted_at in unix microseconds>.<id>` | - |
| `/ui/cors-origins?q&sort&dir&page&rows` | the list (10/25/50 per page) | - |
| `/ui/cors-origins/new`, `/ui/cors-origins/{id}/delete` | the add dialog, the removal confirmation (it names the origin) | add, remove |

- **Gates, as the API's.** Audit log: `platform:admin:audit-log:view` and nothing else (the log is not tenant-scoped, so a client-scoped holder reads all of it, as through the API). Login attempts: anchor scope plus `platform:admin:login-attempt:view`. CORS origins: anchor plus `cors-origin:view` to read, `:create` to add, `:delete` to remove; Add Origin and the remove action are hidden without them and the POSTs and dialogs check again. The use cases carry no resource-level authz (the controller's coarse check is the whole rule), so the page's check is the guard.
- **Audit log.** Filters as the SPA's: entity type and operation (selects of the distinct values, cached 30 s like the other firehose lists), application and client (multi-selects). The SPA lists only applications that appear in the log; names come from the applications list when the caller may read applications (`application:view`), else the id is shown. The drawer shows the SPA's dialog (time, entity type, entity id, operation, performed by, principal id, application, client, pretty-printed operation data).
- **Login attempts.** Filters as the SPA's: attempt type, outcome, identifier (exact match, the toolbar search), date range. The drawer shows the row; the identifier and principal id link to `/ui/users/{id}` or `/ui/service-accounts/{id}` when the caller may read principals and the principal resolves (the SPA's link resolution; any failure leaves plain text). A service account token's identifier is an OAuth client id, which has no `/ui` page, so it stays plain text. There is no read by id, so the drawer finds the row by its own time in the key.
- **CORS origins.** Search (origin or description), sort and paging in memory, as the SPA. Add validates as the SPA does (required; must start with `http://` or `https://`; lower-cased; blank description is none), then the use case's own format and uniqueness rules answer on the field. The removal confirmation names the origin and repeats the 5-minute cache warning; toasts are the SPA's sentences.

Differences from the SPA: the date range of login attempts is whole UTC days (two date inputs, like the dispatch jobs list), because the SPA sends `datetime-local` values that `GET /api/login-attempts` does not parse as RFC 3339 (it ignores them); the audit page has no export, principal filter or date range because neither the SPA page nor `GET /api/audit-logs` offers any (`platform:admin:audit-log:export` is seeded but nothing uses it); the sidebar entries keep the SPA's permission strings (`audit:view`, `cors:view`), which are not the API's, as in `navigation.ts`.

No credential or secret is shown on these pages (user agents and IP addresses are the most sensitive fields).

Not verified in a browser: the three pages have not been opened in light, dark or narrow widths, and the row click, live filters and dialogs are covered by rendered markup and handler tests only. Tests: `audit_log_test.go`, `login_attempts_test.go`, `cors_origins_test.go` (list, filters and URL state, keyset paging, drawers, permission matrices including forged posts, validation) and the `*_pg_test.go` files (`-tags=integration`: the real repositories and use cases, audit rows written by add and remove, a forged remove deletes nothing). Mutation checks (run, seen to fail, restored): skipping the delete permission check fails `TestCorsOriginsPermissions`; swapping the audit permission for the sidebar's `audit:view` fails `TestAuditLogList`.

## Sign-in pages A: login, SSO landing, forgot and reset password, logout, portal login (`/ui/auth/*`)

Pages: `auth_signin.go|templ` (the login steps, the SSO landing, logout, the context, the theme), `auth_reset.go|templ` (forgot, reset and set password), `auth_signin_portal.go|templ` (the portal login). Spec: the SPA's `pages/auth/*` (`LoginPage`, `CallbackPage`, `ForgotPasswordPage`, `ResetPasswordPage`, `LogoutPage`, `PortalLoginPage`) and the second-factor step of `TwoFactorChallenge`.

**Additive, not switched on.** The SPA's `/auth/login` and every existing auth endpoint are unchanged and stay the default (`guard.LoginPath`, the emailed reset and invite links and `GET /portal/authorize` still point at the SPA). The new pages are reached only by their `/ui/auth/*` URL; there is no flag: nothing links to them and no redirect leads there. The owner switches later by pointing `guard.LoginPath`, the email links (`ExternalBaseURL` paths) and `portalauth.State.LoginPagePath` at these URLs.

| URL | GET | POST |
|---|---|---|
| `/ui/auth/login?redirect&client&oauth&interaction&...` | the email step | `/ui/auth/login/email`, `.../password`, `.../2fa`, `.../setup` (the steps below) |
| `/ui/auth/callback?redirect&error&error_description` | where the OIDC bridge returns after SSO | - |
| `/ui/auth/forgot-password?email&...` | the form | the form (the answer is the same for any address) |
| `/ui/auth/reset-password?token`, `/ui/auth/set-password?token` | the form (reset, or the invite framing), or why the link is unusable | set the password |
| `/ui/auth/logout` | the confirmation | sign out |
| `/ui/auth/portal-login?flow` | the portal login | `/ui/auth/portal-login/email`, `.../password`, `.../reset` |

- **No second implementation of the rules.** Every step is one in-process call to the public endpoint the SPA calls (`/auth/check-domain`, `/auth/login`, `/auth/2fa/verify`, `/auth/2fa/challenge/email`, `/auth/password-setup/request`, `/auth/password-reset/{request,validate,confirm}`, `/auth/logout`, `/portal/auth/*`, `/api/public/login-theme`), dispatched through the platform's own router (`ui.Config.SignInUpstream`, wired to the root router in `wire_ui.go`) with the browser's host, remote address, `Origin`/`Sec-Fetch-Site`, `User-Agent` and cookies. Hence the per-(email, IP) backoff and the per-email ceiling (429 with `Retry-After`, shown as "try again in N seconds"), the SSO enforcement, the Argon2id timing equalisation and one answer for an unknown user and a wrong password, the 2FA gate and its own backoff, the login CSRF guard (`CredentialCSRF`), the portal rate limit and the password policy are the endpoints' own, and a fix to them reaches the new pages. The endpoint's `Set-Cookie` headers (the session cookie, the trusted-device cookie) go to the browser unchanged, so the attributes are the platform's builders'. Deviation from "use cases directly, no HTTP hop": there is a dispatch but no HTTP hop, and re-using the handlers is what makes "same lockout, same cookies" true by construction.
- **Steps.** Email (SSO discovery: an external domain redirects 303 to `/auth/oidc/login?domain=...` with the OAuth round trip as `oauth_*` and `return_url=/ui/auth/callback?...`; an internal user who never set a password is offered the emailed link; everyone else, including an unknown address, gets the password step), password, second factor (authenticator code, emailed code or recovery code, "remember this device" when the domain allows it), setup link, forgot, reset/set password (policy, factor code, invite redirect, the session an invite establishes), logout (the confirmation is a GET, signing out a POST), portal login (email, SSO hand-off or password, forgot password).
- **After sign-in** (the SPA's `redirectAfterLogin` order): an OIDC interaction (`/oidc/interaction/{uid}/login`, the uid is checked), an OAuth round trip (`/oauth/authorize?` + the forwarded fields), a `?redirect=` that `safeUIRedirect` accepts (the port of `safeUiRedirect`: same origin, path `/ui` or under it after cleaning; `..`, encoded dots and slashes, `//host`, backslashes, control characters and schemes are refused), else `/ui/`. The sign-in context rides between steps as hidden fields (never the password). A reset's `redirectUri` and a portal's `redirectUrl` are the values the endpoints validated when the invite or flow was made; the page follows only an http(s) URL or a same-site path from them.
- **CSRF and headers.** The group is outside the session guards but behind `guard.CSRF` (an unsafe request needs `Sec-Fetch-Site` same-origin or a matching `Origin`; neither is refused, so a form on another site cannot sign a victim in to the attacker's account), `guard.SecurityHeaders` (CSP, no framing, no-store, same-origin referrer) and compression. The CSP's `img-src` is widened to `https:` on these pages only, for a tenant's logo. The SSO landing is the one route that reads the session cookie (to tell a finished sign-in from a refused one); a stale cookie only means "not signed in".
- **Branding.** The theme is the one the SPA's login page uses: `GET /api/public/login-theme?client=<identifier>` (platform theme with the client's layered over, field by field), asked for the `?client=` the relying party passed through `/oauth/authorize`, carried page to page. Values are validated before they reach a style attribute (plain colours and gradients only; no `url(`), the logo is an https or same-site URL, or the tenant's SVG shown as an image data URL (it cannot run script), and the tenant's CSS is applied as the SPA does (a `<` is stripped so it cannot close its element). The reset and portal pages follow the SPA: a portal token or flow shows no platform brand.
- **Secrets.** Passwords and the reset token are never written back into a response except the reset token in the reset form's hidden field (the page the emailed link opened); an invalid or expired link does not echo it. The pending 2FA token is a short-lived signed token that grants no session; it rides in the challenge form. Every page is `no-store`.

Differences from the SPA: native forms (a refused step re-renders the page with the status code: 401, 403, 422, 429) instead of in-page state, so the page works without script (the password toggle and strength meter use Datastar); the OIDC `CallbackPage` code exchange (client `platform-admin-ui`, `sessionStorage` state) is not ported: the platform's SSO is the bridge's server-side callback, which this area's `/ui/auth/callback` lands after (no OAuth client of that name exists in the platform); passkey sign-in and forced 2FA enrolment (QR code, recovery codes) are not in these pages: a password that owes enrolment, and a reset on a domain that compels 2FA, are handed to the current sign-in (`/auth/login`) with a message; the two-factor pages of `auth_mfa*.go` are a separate, cookie-carried implementation that this area does not call.

Tests: `auth_signin_test.go`, `auth_reset_test.go`, `auth_signin_portal_test.go` (unit, a fake of the endpoints: the redirect validation table incl. the SPA's cases, the landing order, each step, uniform refusals, lockout display, cookie pass-through, cross-site posts refused, theme sanitising, secrets never echoed) and `auth_signin_pg_test.go` (`-tags=integration`: the whole platform router on Postgres: a real sign-in and the cookie it sets, an unknown user and a wrong password answered alike, the backoff locking an address out, a forged cross-site post getting no session, forgot password answered alike, a bad reset token, sign-out).

Not verified in a browser: the pages have not been opened in a browser at all (light styling, narrow widths, the password toggle and strength meter, autofocus, the SSO round trip to a real identity provider, the 2FA remember-device cookie round trip). New Tailwind classes: none (layout is inline style plus classes already in `kit.css`), so `make ui-kit-sync` was not run.

## Sign-in pages B: passkeys and two-factor (`/ui/auth/*`)

Pages: `auth_mfa.go` (the challenge, the token hand-over, the session), `auth_mfa_enroll.go` (enrolment after the password), `auth_mfa_security.go` (the signed-in security page), `auth_mfa_qr.go` (the QR code), `auth_passkey.go` (the passkey sign-in page, the passkey list and removal), `auth_mfa.templ`, `auth_mfa_security.templ`; the browser half is `static/fc-webauthn.js`. Spec: the SPA's `TwoFactorChallenge`, `TwoFactorSetup`, `TwoFactorSection`, `PasskeysSection` and `api/webauthn.ts`, and the `/auth/2fa/*` and `/auth/webauthn/*` endpoints.

**Additive, not switched on.** The SPA's sign-in, `/auth/2fa/*` and `/auth/webauthn/*` are unchanged and stay the default; nothing links to these pages and no redirect leads there. They are mounted when `ui.Config.TwoFactor` is set (it is, in `wire_ui.go`).

| URL | GET | POST |
|---|---|---|
| `/ui/auth/2fa?next&method` | the challenge (needs the challenge cookie) | `/ui/auth/2fa/verify` (method, code, remember), `/ui/auth/2fa/email` (send the code) |
| `/ui/auth/2fa/enroll?next` | the method menu (enroll token) | `.../enroll/totp`, `.../enroll/totp/confirm`, `.../enroll/email`, `.../enroll/email/confirm`, `.../enroll/done` |
| `/ui/auth/2fa/start`, `/ui/auth/2fa/enroll/start` | - | take the token `/auth/login` answered with (`token`, `next`), keep it in the challenge cookie, redirect to the page |
| `/ui/auth/passkey?next&email` | "Sign in with a passkey" (loads `fc-webauthn.js`) | - (the script calls `/auth/webauthn/authenticate/*`) |
| `/ui/auth/security` (session) | methods, recovery codes, remembered devices, passkeys | `.../security/2fa/{totp,email}` (+ `/confirm`), `.../2fa/{method}/remove`, `.../2fa/recovery-codes`, `.../2fa/devices/{id}/revoke`, `.../passkeys/{id}/remove` |

- **Same services, same rules.** The pages call `mfa.Service`, the `mfatoken` issuer, `twofa.Policy`, the login-attempt backoff (`loginbackoff.Check`, per (email, IP), shared with the password step; wrong codes are recorded as failed `USER_LOGIN` attempts), `notify.Notifier` and the audit repository directly, as the login endpoint's handlers do, and mint the session with the same `provider.MintSessionToken` and cookie builder. The rules copied from the endpoint's handlers rather than shared (they are unexported there): the usable factors are the confirmed ones narrowed to the domain's allowed set when it compels 2FA; a method the domain does not allow cannot be enrolled (403); a user whose domain compels 2FA cannot remove their last factor (409); recovery codes exist only for authenticator-app users and are generated at first TOTP enrolment; a burnt recovery code emails the user; remember-this-device is honoured only when the domain allows it, with the `__Host-fc_td`/`fc_td` cookie the login endpoint reads.
- **No new way around 2FA.** A session is minted at exactly two places: `verify` (a code that the service accepts, for the pending token's user) and `enroll/done` (the enroll token's user holds a factor the domain accepts, or the domain no longer compels one). The between-step token is the `mfatoken` JWT (HS256, rejected by the session middleware) in an HttpOnly cookie scoped to `/ui/auth`; a pending token does nothing on the enrol pages and an enroll token nothing on the challenge, a bad, expired or other-user token redirects to the default sign-in, and the cookie ends with the sign-in. `next` is validated twice (when taken, when used): `/ui` or under, no scheme or host, no dot segments, no encoded dots, slashes or backslashes, no control characters.
- **Hand-over from the password step.** Sign-in pages A call `/auth/login` in-process and handle `mfa_required` inline, and send `enrollment_required` to the current sign-in. To use these pages instead, post the answer's `mfaToken` (or `enrollToken`) as `token` to `/ui/auth/2fa/start` (or `.../enroll/start`) with the `next`; the endpoint checks the token's purpose and user before setting the cookie. The agreed `ContinueAfterPassword`/`CompleteSignIn` functions were not needed: the decision stays in `/auth/login`, and `AuthMFA.completeSignIn` issues the session (JWT, cookie, success attempt) the way `completeLogin` does.
- **Secrets are shown once.** The TOTP key and the QR code are in the response of the begin step; a wrong code re-renders the setup from the `uri` the form echoes (a hidden field of the page that showed it; only drawn, never trusted: the confirm is against the secret the service stored, and a `uri` that is not an `otpauth://totp/` URI with a secret draws nothing). Recovery codes are in the response that creates them (first enrolment, regenerate) and nowhere else: not a URL, cookie, signal or later page. Both pages are `no-store` (`guard.SecurityHeaders`), copy is a button that reads the element's text (`$mfaCopied` only says "copied"), and "I've saved them" leaves for a page without them.
- **QR code on the server.** `qrSVG` draws `boombuler/barcode/qr` (already in `go.mod` through `pquerna/otp`, which the service uses for its PNG; now a direct dependency) as inline SVG: one `<path>` of unit squares at error-correction M on a white ground with the four-module quiet zone (a dark theme must not swallow it). The markup is made of integers only. `TestQRSVGIsTheEncodersMatrix` compares the drawing module by module with the encoder's matrix.
- **Passkeys.** `fc-webauthn.js` (one committed IIFE bundle, 10 kB, loaded only by the passkey sign-in page and the security page) is `@simplewebauthn/browser` 13.3.0 (the version the SPA uses) plus about 100 lines of glue, built by `tools/vendor-webauthn` (`npm ci && npm run build`; exact versions, esbuild 0.28.2, reproducible; licence in `static/LICENSE-webauthn.md`; sha256 in `assets.WebAuthnSHA256`, checked by `TestIslandBundlesArePinned`; never part of the normal build). It drives `navigator.credentials` and posts to the platform's `/auth/webauthn/*` endpoints with the SPA's JSON (the endpoints verify, apply the backoff, record the attempt and set the session cookie, so a passkey sign-in is the endpoint's, not a copy); it ends by navigating to the validated `next`, or by reloading the security page. A passkey sign-in meets no second factor, as on those endpoints; a password sign-in still does, a passkey notwithstanding. List and removal are server side: the security page lists the user's credentials and removes one by the revoke use case after the same ownership check as `DELETE /auth/webauthn/credentials/{id}` (another user's id is a 404 and removes nothing).
- **Security page.** Self-service only: every action acts on the session's principal; a form field naming another user changes nothing (device revocation is scoped to the owner in the service). Remove method, remove passkey and regenerate recovery codes confirm in a native dialog (the SPA confirms none of them; destructive and credential actions here do). Reached by URL: the sidebar has no entry for it (the SPA's navigation has none either; the profile link of the user menu still goes to the SPA's `/profile`).

Differences from the SPA: native forms with the status in the response (401 wrong code, 422 empty or non-matching code, 429 backoff) instead of in-page state; a wrong enrolment code re-renders the setup instead of keeping it in memory; no conditional-UI autofill for passkeys; a user with no platform role cannot reach the security page (the `/ui` role-less gate; their profile lives in the SPA).

Tests: `auth_mfa_test.go` (unit, fakes of the services: token purposes and bad tokens, the method offer incl. a domain-narrowed set, verify success/failure/forged method/burnt recovery codes, remember-device only when allowed, email challenge, the token hand-over, enrolment menu and forged methods, the enroll token never ending in a session without a confirmed factor, secrets shown once, the security page's last-factor guard, domain gates, session requirement for every POST, self-service scoping incl. another user's device and passkey, `authNext` table, passkey page), `auth_mfa_qr_test.go`, `internal/ui/twofactor_mount_test.go` (mounted only when wired, no session needed for the sign-in pages, session required for the security page, cross-site posts refused), and `auth_mfa_pg_test.go` (`-tags=integration`: real `mfa.Service`, TOTP with generated codes and the replay guard, 10 recovery codes shown once and usable once, remember-device cookie and owner-scoped revocation, backoff 429 refusing even the right code, the email factor, and the passkey ceremony driven with the repository's software authenticator against the real `/auth/webauthn/*` handlers: register, sign in (session cookie set), a wrong key refused, the security page listing and a second user unable to remove the credential). Mutation checks (run, seen to fail, restored): dropping the factor requirement of `enroll/done` fails `TestEnrollmentIsTheOnlyWayPastTheEnrollToken`; dropping the passkey ownership check fails `TestSelfServiceActsOnlyOnTheSessionsUser`.

Not verified in a browser: none of these pages has been opened in one (layout, dark mode, narrow widths, the confirm dialogs, the copy button) and `fc-webauthn.js` has not been run in a browser: no virtual authenticator was driven through Chrome; the server path it calls was exercised headlessly (above) and the bundle is pinned and reproducible. New Tailwind classes: none (checked against `kit.css`), so `make ui-kit-sync` was not run.

## Platform pages B: Documentation, Theme, Names, Raw views (wave 5)

The SPA's `DocsPage`, `LoginThemeSettingsPage`, `ClientLoginThemePage`, `PlatformNamesSettingsPage` and the two debug
pages, added beside the SPA (nothing of it is touched; the sidebar entries link to `/ui`). Files:
`pages/docs*`, `theme_settings*`, `names_settings*`, `debug_*`.

| Page | Routes | Gate (the API's) |
|---|---|---|
| Documentation | `/ui/platform/docs` (opens the first platform page), `/ui/platform/docs/{source}/{slug}?q=` | `platform:admin:docs:view` (`auth.CanReadPlatformDocs`) |
| Theme | `/ui/platform/settings/theme` (`?defaults=1`), POST same, POST `/preview` | read: `platformconfig.Service.List` (config view, confinement, grants); save: `AuthorizeUpdate`, then the `SetProperty` use case |
| Login Branding | `/ui/clients/{id}/theme`, POST same, POST `/preview` | `CanReadClients` for the page; save: `AuthorizeUpdate`, `ResolveClientID` (unknown client refused), `SetProperty`; switch off: `Service.Delete` |
| Names | `/ui/platform/settings/names` (`?default=1`), POST same | as Theme |
| Raw Events | `/ui/debug/events?size=`, `/ui/debug/events/{id}` (drawer) | `platform:messaging:event:view-raw` (not scoped to a client, as the API) |
| Raw Dispatch Jobs | `/ui/debug/dispatch-jobs?size=`, `/ui/debug/dispatch-jobs/{id}` | `platform:messaging:dispatch-job:view-raw`; a job opened from the list is checked against the viewer's clients as `GET /api/dispatch-jobs/{id}/raw` does |

- **Documentation.** The listing and page lookups were lifted out of the huma handlers into `docsapi.State.Index`,
  `PlatformPage` and `ApplicationPage` (no permission check inside); the handlers and this page both gate on
  `CanReadPlatformDocs` and call them. Markdown goes through `internal/ui/md` (raw HTML is dropped, `javascript:` links
  removed). ` ```mermaid ` fences are cut out of the source before rendering and drawn by the Mermaid island
  (`kit.Diagram`); a fence of another language that mentions mermaid is code. As the SPA's click handler, a link to a
  sibling `.md` file opens that page of the same source and an external link opens in a new tab (rewritten on the
  sanitised output, so text that merely looks like a link is not touched). Navigation between pages is plain links;
  the filter (title or slug, case-insensitive) patches the navigation only. A page that is not there is a message in
  the pane with a 404, the list stays. `{source}` and `{slug}` must be names (`[A-Za-z0-9_.-]`), so they are safe in a
  URL and an expression.
- **Theme and Login Branding** share one editor (`thmView`, `thmEditor`). Both read and write the platform-config row
  (`platform/login/theme`, the client's override with the client's id) through `platformconfig.Service` and the
  `SetProperty` use case, so the access rules are those of `PUT /api/config/platform/login/theme`; the stored value is the
  SPA's `JSON.stringify(theme)` (blank optional fields null) and `GET /api/public/login-theme` serves it (tested end
  to end). The override starts from the platform theme; the switch off deletes the override rather than storing a copy.
  The colour pickers write into their text fields (primary and background also regenerate the gradient, as the SPA).
  A viewer without write access sees the values with the controls disabled and no Save.
- **Live preview that runs nothing.** The preview is rendered by the server (`POST .../preview`, debounced on form
  input; nothing is read or saved) from values it vouches for: colours and the gradient are validated and fall back to
  the built-in look, text is escaped, an inline SVG logo is shown as an `<img src="data:image/svg+xml;base64,...">` (an
  image cannot run script; the SPA injects it with `v-html`), an SVG with script or handlers is not shown, a logo
  address is named, not loaded (the admin CSP is `img-src 'self' data:`), and **custom CSS is never applied** to the admin
  page. Only the preview's own stylesheet-free inline styles are used, so no kit stylesheet rebuild was needed.
- **Validation on save** (field errors, nothing stored): colours must be hex or `rgb()`/`rgba()` (the rule `branding`
  uses for emails), the gradient a CSS gradient of colours and stops, the logo URL http(s), the inline SVG an `<svg>`
  element of at most 64 kB without `<script>`, `foreignObject`, `iframe`, `embed`, `object`, `on*=` or `javascript:`, the
  logo height a whole number from 20 to 120 (the field's own range), text fields at most 500 characters, custom CSS
  at most 64 kB. The SPA's editor enforces none of this beyond the height's input attributes.
- **Names.** One field; empty is "Platform name cannot be empty." (the SPA's message), at most 200 characters; the
  stored name is what `branding.PlatformName` reads for emails, 2FA and passkeys.
- **Raw views.** The most recent N rows of the write-side tables (`FindRecentRaw`, N one of 50/100/200/500/1000, default
  200), no filtering or paging, and a drawer (the SPA's dialog) reading one row (`FindRawByID` / `FindByID`).

Differences from the SPA: the diagram has no click-to-enlarge lightbox (`kit.Diagram` scales to the column and keeps
its source below); a view-only user sees disabled controls where the SPA shows editable ones that fail on save; the
Dispatch Jobs debug link is shown for `dispatch-job:view-raw` (the API's permission) where the SPA's route guard names
`dispatch-job:view`; the sidebar's Theme and Names still require `platform:admin:settings:view`, which no role is seeded
with (docs/platform-bug-fixes.md), as in the SPA; the pages themselves are gated by the API's rules, not that code.

Tests: `docs_test.go`, `theme_settings_test.go`, `names_settings_test.go`, `debug_test.go` (unit, fakes: the permission matrix with
forged posts from a viewer and from nobody, the SPA's PUT bodies, validation, the preview's escaping, SVG and
custom-CSS handling, mermaid splitting, link rewriting, the filter, client scoping of a raw job) and
`docs_pg_test.go`, `theme_settings_pg_test.go`, `debug_pg_test.go` (`-tags=integration`: the real `docsapi.State`,
`platformconfig.Service` and `SetProperty`; what the pages save is what `/api/public/login-theme` serves and
`branding.PlatformName` reads, the override layers and is deleted, a client user without grants and a viewer change
nothing; the raw views read the write tables). Mutation checks (run, seen to fail, restored): dropping the update gate of the
names post fails `TestNamesPermissionMatrix`; dropping the script check of the preview's SVG fails `TestThemePreview`.

Not verified in a browser: the Datastar interplay (the debounced preview post, the picker handlers writing into the text
fields, the switch showing and hiding the fields, the navigation filter patch, the drawers) and the Mermaid island on a
docs page are covered by the rendered markup in tests only.

## Authentication admin A: identity providers and email domains (wave 5)

Mirrors `IdentityProviderListPage`, `IdentityProviderCreateDrawer`, `IdentityProviderDetailDrawer` and
`EmailDomainMappingListPage`, `EmailDomainMappingCreateDrawer`, `EmailDomainMappingDetailDrawer`. Both areas are additive: the SPA
pages stay. Routes: `/ui/identity-providers` and `/ui/email-domains`, each with the list (`?q&sort&dir&page&rows`), `/new`, `/{id}`
(drawer), `/{id}/edit`, and POST-only `/{id}/delete`. Email domains filter on `?status=` (the scope, ANCHOR, PARTNER, CLIENT) and
`?idp=` (a contains match on the provider's name, as the SPA's text filter).

- **Gates** are the API's: every identity provider and mapping permission is anchor-only (`auth.anchorWith`). Reads need `view`;
  create, update, delete their own permission; the provider move needs the mapping `update`. The edit dialog needs `view` and
  `update` (the SPA loads the record, then PUTs). Controls are hidden without the permission and every route makes the check itself
  before it loads anything, so a forged post is refused without telling a real id from an invented one (the matrix tests post
  every write as a no-permission caller, a viewer and a non-anchor holder of every permission). The seeded `internal` provider's
  Delete is disabled with the reason (the use case refuses it too).
- **Client secret: write-only.** The page never reads `oidcClientSecretRef` into a view: the drawer says "Configured" or "Not
  configured" (the API's `hasClientSecret`), the edit form says whether one is set and leaves the field blank (blank keeps the
  stored secret). The SPA's `SecretRefInput` is reproduced: a select of where it is held (Local Encrypted, AWS Secrets Manager, AWS
  Parameter Store, GCP Secret Manager, HashiCorp Vault) and one value. Local Encrypted takes the secret itself in `kit.PasswordInput`
  and sends `encrypt:` plus it; the API handler's inline encryption (`encryption.EncryptSecretRef`, errors mapped to the same
  `ENCRYPTION_NOT_CONFIGURED` / `UNSUPPORTED_SECRET_SCHEME` validations) is done by `IdpOps`, so the use case sees what the API's
  does. The others take a reference in a text field and send `<scheme>://<value>`. The kit's `SecretRefInput` was **not** used: it
  holds the NAME of a secret under a pattern (`store:name`) that cannot express the scheme references the API accepts or the local
  encrypted kind the SPA offers. A refused form never echoes a typed secret (the failure patch leaves the password field to the
  browser; it is patched only when the secret is itself the field at fault, and then empty).
- **Domains and scope.** The SPA's chip input is a textarea (one domain per line; commas and spaces also split), lower-cased,
  de-duplicated, each checked with the SPA's pattern. Once domains are listed the scope choice appears (Anchor or Client; Partner
  mappings are made on the Email Domains page); Client shows the primary client (the platform's client select). An edit never
  infers a scope (the provider stores none). Scope codes (`MAPPING_SCOPE_REQUIRED`, `INVALID_MAPPING_SCOPE`,
  `PRIMARY_CLIENT_NOT_ALLOWED`) show next to the scope, `PRIMARY_CLIENT_REQUIRED` on the client. The create command is the SPA's
  POST body, the edit command its PUT body (the whole domain set always; the secret only when entered).
- **Releasing domains** (the SPA's "Remove Email Domains" dialog): an edit that drops a domain is answered with the warning, the
  domains and a required checkbox; the use case runs only when `confirmRelease=yes` is posted. This is stricter than the SPA (its
  confirmation was client-side only).
- **Role sync.** OIDC providers only. The allowed roles are a kit multi-select (needs `role:view`; without it the roles already
  allowed stay as stored and a posted role is ignored); a posted role not in the list is refused before the use case.
- **Delete** keeps the SPA's confirmation; the use case's refusal while domains still route to the provider (409
  `DOMAINS_STILL_MAPPED`) is the error flash.
- **Email domain mapping form.** Create: domain (the SPA's pattern, lower-cased), provider, scope (default Client), primary client
  (Client **and Partner**), the tenant when the chosen provider is multi-tenant (required), and for a provider that is not OIDC the
  two-factor policy (require, methods, remember device, 1 to 365 days). The parts follow the select through signals (the OIDC and
  multi-tenant provider ids). Edit: the domain and the scope are shown, not editable; the provider select, the client, the tenant and the
  policy are. A changed provider is answered with the SPA's "Change Identity Provider" text (to an internal provider the SSO users
  are converted and must reset their password; to an OIDC provider password login is disabled) and a required checkbox; only
  then does `MoveMappingToProvider` run, followed by the update, and the count converted is flashed as the SPA's toast does. The
  tenant and the two-factor parts follow the provider the mapping will be on.
- **Not in the SPA, so not here:** activate and deactivate of a provider, a test connection and redirect information for a
  provider. The SPA pages have none of these (the API has no such routes either).

SPA behaviours that were bugs, recorded and not copied:

1. The mapping edit form offers a Scope Type and sends it, but `PUT /api/email-domain-mappings/{id}` has no scope (`UpdateCommand`);
   the body is dropped. Changing Client to Anchor therefore cleared the client and left a CLIENT mapping with none. The scope is read-only
   here; **owner decision needed** if a scope change should be possible (it needs an API change).
2. Create sends a primary client for the CLIENT scope only, but the use case also requires it for PARTNER, so a partner mapping could
   never be created from the SPA. The client field is shown and required for Partner.
3. The list's Primary Client column shows the raw id. The names are shown when the client list can be read (id otherwise).
4. The SPA's edit sends `oidcIssuerPattern: null` for a blank pattern, which the API reads as "unchanged": a pattern cannot be cleared.
   Kept as it is (the page sends the same body); a clear would need the API to take an empty string.
5. The SPA's tenant and two-factor parts follow the provider the mapping had before a move; here the one it will have.

Differences from the SPA: dialogs for create and edit (the SPA uses the drawer); the domain chip input is a textarea; the provider
and mapping lists page at 100 rows like the SPA's paginator; the role pick list is a multi-select with chips and search.

Not verified in a browser: the pages have not been opened in light, dark and narrow widths; the signals that show and hide the OIDC,
scope, client, tenant, two-factor and secret parts (`data-show` expressions, including the id lists tested with `includes`), the password
field's reveal toggle and the multi-select chips are covered by the rendered markup in tests only.
Tests: `identity_providers_test.go` and `email_domains_test.go` (permission matrices with forged posts, controls per permission, list
filters and sorts, create and update command bodies, the secret kinds and that neither the stored nor the typed secret is in any
answer, the release and move confirmations, field errors from the use case's codes, native posts) and
`identity_providers_pg_test.go` (`-tags=integration`: create with encrypted storage and the domains' mappings and client link, the scope
refusal, the confirmed release falling back to the internal provider, delete blocked while mapped, a mapping with a two-factor policy,
the confirmed move, forged posts by a viewer and a non-anchor caller writing nothing and no audit rows). Mutation checks: dropping the
`CanDeleteIdentityProviders` gate fails the permission matrix (the forged delete reaches the use case), and bypassing the release
confirmation fails `TestIdentityProvidersUpdateReleasesDomainsOnlyWhenConfirmed`.
The sidebar's Identity Providers and Email Domains entries link to `/ui/identity-providers` and `/ui/email-domains`.

## Authentication admin B: OAuth clients and reset approvals (wave 5)

Pages: `oauth_clients.go` (list, drawer, actions), `oauth_clients_form.go` (create and edit dialogs, rotation, the shown-once dialogs), `oauth_clients_ops.go` (`OAuthClientOps`: the use cases), `oauth_clients.templ`; `reset_approvals.go`, `reset_approvals.templ`. Spec: the SPA's `OAuthClientListPage`, `OAuthClientCreateDrawer`, `OAuthClientDetailDrawer` and `ResetApprovalsPage`.

**Additive, not switched on.** The SPA's pages stay the default (the old UI is kept so it can be used if necessary); the sidebar entries now carry a `ui` path (`/ui/oauth-clients`, `/ui/reset-approvals`), as the other migrated areas, and each page says it is a preview.

| URL | GET (navigation) | GET (Datastar) | POST |
|---|---|---|---|
| `/ui/oauth-clients` (`?q&sort&dir&page&rows`) | the list | table, pager, toolbar state, URL | - |
| `/ui/oauth-clients/new` | list + create dialog | the dialog | create |
| `/ui/oauth-clients/{id}` | list + drawer | drawer content | - |
| `/ui/oauth-clients/{id}/edit` | list + edit dialog | the dialog | update |
| `/ui/oauth-clients/portal-app-options?portalClientId=` | - | the Portal app field for that owner | - |
| `/ui/oauth-clients/{id}/{activate,deactivate,delete}` | - | - | the action |
| `/ui/oauth-clients/{id}/{rotate-secret,rotate-secret-now,revoke-previous-secret}` | - | - | the secret action |
| `/ui/reset-approvals` | the queue | the queue (the Refresh button) | - |
| `/ui/reset-approvals/{id}/{approve,deny}` | - | - | the decision |

- **Gates are the API's.** Every OAuth client route is anchor-only and needs the permission of its operation, checked before anything is read: list and drawer `CanReadOAuthClients`; create `CanCreateOAuthClients`; edit, activate, deactivate `CanUpdateOAuthClients` (the edit dialog also needs view); delete `CanDeleteOAuthClients`; rotate, rotate without overlap and revoke the previous secret `CanRotateOAuthClientSecrets` (`regenerate-secret`, as the API's `rotate-secret`, `regenerate-secret` and `revoke-previous-secret`). Controls are hidden without the permission (and the confirmation dialogs are not rendered); a forged post is refused by the page before the use case runs. A caller who may create or rotate but not view gets the secret dialog and neither the table nor the drawer (`TestOAuthClientsPermissionMatrix`, `TestOAuthClientRotation`). Use cases are the API's (`pages.OAuthClientOps` runs `authops.CreateOAuthClient` and friends through the unit of work). The portal app rule moved out of the API handler into `authapi.ResolvePortalApp`, which both call. Reset approvals call `resetapi.State.Pending/Approve/Deny` in-process (the huma handlers are thin wrappers of the same methods): the `user:update` permission, the queue confined to the caller's clients (an anchor sees all), the per-request client check (`RequireUserAdmin`: a client administrator cannot decide another client's or a platform user's request) and the single decision (`ALREADY_DECIDED`) are the API's.
- **Secrets are shown once.** The create (CONFIDENTIAL) and rotate responses carry a modal with the secret (the client id and secret on create), copy buttons, and the warning "will not be shown again". The create dialog cannot be dismissed except by "I've copied the secret" (a close by anything else reopens it, as for the application login client); the rotation dialog can. Every way of closing removes the dialog from the page. The response is `Cache-Control: no-store` (the `noStore` writer), a native post gets the page itself (post/redirect/get would lose the secret), and the secret is in no URL, signal, redirect, flash, drawer or later page (tests assert it appears once and never in a signal, the drawer or the list). The stored reference (`SecretRef`, `PreviousSecretRef`) is never read into a view. The integration test verifies the dialog's secret against the stored hash with the key, and that a forged rotation changes nothing.
- **Rotation.** "Rotate Secret" is the SPA's (body-less `rotate-secret`): the new secret works at once and the old one for the 24-hour overlap, with the SPA's confirmation text. The drawer also shows what the API reports and the SPA ignores: while an overlap is open, "Previous secret still accepted" with its end and whether the old secret was used since (`previousSecretExpiresAt`, `previousSecretLastUsedAt`), and **Revoke Previous Secret** (confirmed) ends it now. **Rotate Without Overlap** (`graceSeconds: 0`, confirmed, danger) is for a secret believed compromised. `regenerate-secret` is the SDK's alias of `rotate-secret`, so the page has no separate action for it.
- **Form.** Fields and checks are the SPA's: name; type (create only; PUBLIC forces PKCE on and the PKCE box follows the type); redirect URIs (required on create; on edit required for the `authorization_code` and `refresh_token` grants), post-logout redirect URIs, allowed origins (stored as scheme and host, a path is refused); grant types and default scopes (multi-selects; a value the client holds that the form cannot offer is kept); associated applications (searchable multi-select of active applications, offered only to who may list them, as the SPA's call); portal owner client (offered only to who may list clients) and, for an owner, the portal app of that client (the select re-fetches on change); API access (disabled for a portal client; the use case refuses the combination). Create sends the SPA's body (optional fields only when set); edit sends everything, an emptied list clearing it and an emptied owner clearing the owner and its app. Use-case codes land on their field (`CLIENT_NAME_REQUIRED`, `INVALID_CLIENT_TYPE`, `PORTAL_API_ACCESS_CONFLICT`, `PORTAL_APP_*`).
- **List and drawer.** Search over name, client id and application names; sort by name, client id, type, status, created; 100-row pages (50/100/250/500). Row: Edit inline; View details, Deactivate… or Activate…, Delete… in the menu. The drawer is the SPA's three sections (configuration with the copy button for the client id, client secret for CONFIDENTIAL clients, activation). Owner client and portal app names need the permissions of the lists the SPA reads them from; otherwise the ids are shown.
- **Reset approvals.** The SPA's table (user, requested, expires, Approve and Deny) with a Refresh button; decisions answer with the queue as it now stands and the SPA's toast ("Approved", "Denied"); like the SPA they are immediate, with no confirmation. A request decided meanwhile shows the API's "request is no longer pending".

Differences from the SPA: the three URL lists are textareas, one per line (the SPA adds and removes chips); create and edit are dialogs (the pattern ruled for pools; create opens the new client's drawer, as the SPA navigates to it); **Activate and Deactivate ask for confirmation** (the SPA toggles at once; deactivating stops every login through the client); Delete is in the row menu only, as the SPA; the flash messages carry no client name (flash keys carry no request text); the overlap display, Revoke Previous Secret and Rotate Without Overlap are not in the SPA; the portal app select is a Datastar-patched field rather than a client-side filter. The sidebar entry for OAuth clients required the permission `platform:iam:oauth-client:view`, which no role holds (the API's is `platform:auth:oauth-client:view`): corrected, so a non-wildcard administrator sees the entry.

Tests: `oauth_clients_test.go` and `reset_approvals_test.go` (unit, fakes of the use cases and of the API state: list, search and sort; the permission matrix with forged posts for six caller shapes across every route, controls hidden; drawer and overlap; create confidential/public with the SPA's bodies, validation and the use case's refusals, the shown-once dialog; edit bodies; rotation, no-store, no secret in a signal or a later page, a rotate-only caller; state actions; portal app options; the reset queue and decisions) and `oauth_clients_pg_test.go`, `reset_approvals_pg_test.go` (`-tags=integration`: the real use cases and the real API state on Postgres: the secrets shown are the stored ones, the 24-hour overlap and its revocation, immediate cutover, portal rules, audit, forged posts changing nothing; the reset queue confined by client, forged and repeated decisions refused, the link sent only on approval). Mutation checks (run, seen to fail, restored): dropping the rotate permission check fails `TestOAuthClientsPermissionMatrix` (a viewer's forged rotate ran and showed the secret); dropping the `noStore` writer fails `TestOAuthClientCreateConfidentialShowsTheSecretOnce` and `TestOAuthClientRotation`.

Not verified in a browser: none of these pages has been opened in one (layout, dark mode, narrow widths, the dialogs, the copy buttons, the Datastar-driven PKCE and API-access switches and the portal app re-fetch, the persistent create dialog's reopen on Escape); they are covered by the rendered markup and the server responses in the tests only. New Tailwind classes: none beyond the kit's, so `make ui-kit-sync` was not run.
