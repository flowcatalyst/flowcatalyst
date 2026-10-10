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
`/ui/subscriptions`, `/ui/applications` and `/ui/clients` at parity (sections below); the other
core areas are not ported yet.

## Architecture

```
internal/ui/
  ui.go          Mount: the /ui route group (assets, guards, pages, sign-out)
  ds/            the only package that imports the Datastar Go SDK (datastar-go v1.2.2)
  kit/           templ components, one per kit spec; Props structs mirror the specs' Props tables
  htmlnorm/      HTML normaliser behind the conformance test
  shell/         page layout around the kit app shell; sidebar navigation; error pages
  guard/         CSRF, session negotiation, role-less gate, security headers
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

Request path: `CorrelationID` → the platform `Authenticator` (same configuration as the `/api`
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
- **Confirmations**: one `kit.ConfirmDialog` per action, shared by the rows and the drawer
  (`kit.OpenConfirm` sets the form's action from the trigger's `data-action`); it posts with
  `kit.SubmitConfirm` and closes at once, the outcome is a flash. Only the dialogs the user may use
  are rendered.
- **Drawer state for the server**: forms carry `<input type="hidden" name="drawer"
  data-attr:value="$selPool">`, so an action refreshes the drawer when it shows that record and
  closes it after a delete.
- **Client select**: `kit.Autocomplete` with `/ui/clients/options` as its source (the SPA's
  `ClientSelect` search: `CanReadClients`, `client.Repository.Search`); the submitted identifier
  is resolved on the server (`resolveClient`). Without `CanReadClients` the field is disabled and
  says why.
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
- **Permissions** are the API handlers' checks: create, update, suspend and activate need
  `CanWriteDispatchPools` (the API's coarse gate: any of create/update/delete), delete
  `CanDeleteDispatchPools`; the drawer and the edit dialog also need `CanReadDispatchPools` and,
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

- **Reads** make `GET /api/event-types`'s checks: `CanReadEventTypes`, client scoping, and for a
  principal without the messaging view permission only its own applications' event types (the SPA's
  BFF list has none of these, see "Platform bugs found"). **Actions** make the check of the BFF
  endpoint the SPA calls: create `CanCreateEventTypes`; edit, archive, add schema, finalise and
  deprecate `CanUpdateEventTypes`; delete `CanDeleteEventTypes`; sync anchor scope (`RequireAnchor`).
  Each runs the BFF's use case through `pages.EventTypeOps`.
- **Filters**: the facets are kit `multi-select`s (repeated `app`/`sub`/`agg` parameters, the SPA's URL
  keys); options come from every visible event type of any status, cascade (subdomains of the chosen
  applications, aggregates of both) and chosen values that stop being options are pruned from the
  canonical URL, as the SPA prunes them. A live answer patches each facet's summary and the dependent
  options parts, never the open disclosure. Paging is the SPA's: 100 rows, 50/100/250/500
  (`kit.Paginator`, rows select associated with the toolbar form).
- **Drawer**: the SPA's guards for Archive (CURRENT, every version deprecated) and Delete (ARCHIVED, or
  CURRENT with every version finalising) are kept as disabled buttons with the reason written under
  them; the use cases do not enforce them (owner decision below). Edit and Add Schema are hidden for an
  archived event type, as in the SPA.
- **Schema viewer**: `schemacodegen` renders the six tabs (schema, example, TypeScript, PHP, Python,
  Java) on the server, highlighted with escaped text; copy uses the clipboard (`Copied` beside the
  button for 2s instead of a toast).
- **Export Schemas** downloads the zip the SPA builds in the browser, for the view the user is on
  (filters **and** search, all pages); with nothing to export it redirects with the warn flash.
- **Add schema**: version (suggested as the SPA does) and the schema. The schema must be a JSON
  document and is sent as one; the SPA's MIME type and schema type fields are not shown because the
  use case ignores them (every version is stored as JSON Schema, `application/schema+json`).

Differences from the SPA: the create and edit forms are dialogs (the SPA uses the drawer: the
pattern ruled for pools); confirmations use one message per action without the version number;
the sync result flash shows the SPA's two lines as one; the export applies the search too (the SPA
exported the facet-filtered list, ignoring the search box); facet options come from the event types
the user may see (the SPA's filter endpoints list every application).

### Subscriptions (`/ui/subscriptions`)

Mirrors `SubscriptionListPage`, `SubscriptionCreateDrawer` and `SubscriptionDetailDrawer`. Same routes
as connections (`/new`, `/{id}`, `/{id}/edit`, `/{id}/{pause,resume,delete}`), plus
`/ui/subscriptions/event-type-options?clientScoped=` (the event type list for the Client Scoped switch).

- Reads: `GET /api/subscriptions`'s checks (`CanReadSubscriptions`, client scoping). Actions check the
  permission for the operation: create `CanCreateSubscriptions`; edit, pause, resume
  `CanUpdateSubscriptions`; delete `CanDeleteSubscriptions` (the API itself still admits any of the three
  for every write: bug 8). Each runs the API's use case (`pages.SubscriptionOps`).
- List: search (code, name, connection id, application, client), Application multi-select (options from
  the subscriptions, pruned), status, sortable columns, 100-row pages.
- Create: the SPA's sections; event types are a searchable multi-select of CURRENT event types with a
  CURRENT schema matching the Client Scoped switch (the switch re-fetches the list and clears the
  choice, as the SPA does; the client field shows only for a client-scoped subscription). The bindings
  (id, code, current version) are built from the event types offered. Connection, dispatch pool and the
  event types are offered only to users who may read them (disabled with the reason otherwise).
- The pool code is looked up by id when a subscription has none stored (bug 11), so the Pool column and
  the drawer are not empty for subscriptions created in a UI.

Differences from the SPA: dialogs instead of drawer forms; **Queue** is a select of the two values the
use case accepts (Default, High priority; the SPA's free text fails for anything else, bug 10);
**Sequence** is not offered (the API drops it, bug 9); the create form's "+ create connection" nested
dialog is not ported (create the connection on the Connections page first); the edit form's connection
is a select of active connections (the SPA's is a free-text id) and a blank choice leaves the
connection unchanged (the SPA stores an empty id, bug 12).

### Applications (`/ui/applications`)

Mirrors `ApplicationListPage`, `ApplicationCreateDrawer` and `ApplicationDetailDrawer`. Routes as
connections (`activate`/`deactivate`/`delete`), plus `/{id}/provision-service-account` and
`/{id}/provision-login-client` (POST).

- Reads: `GET /api/applications`'s checks (`CanReadApplications`, application confinement for an
  application service account; the drawer `CanReadApplication`). Actions check the permission for the
  operation (create, update for edit/activate/deactivate, delete; the API admits any of the three for
  create, update, activate and deactivate: bug 13); provisioning is anchor-only, as the API.
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
while active (the SPA only disables it; the use case does not require it: bug 14).

### Clients (`/ui/clients`)

Mirrors `ClientListPage`, `ClientCreateDrawer` and `ClientDetailDrawer`. Routes as connections
(`activate`/`suspend`/`deactivate`), plus `POST /ui/clients/{id}/applications` (the picker's Save).

- Every check is the API's, per operation and anchor-only (`CanReadClients`, `CanCreateClients`,
  `CanUpdateClients`, `CanActivateClients`, `CanSuspendClients`, `CanDeactivateClients`); the
  applications picker is the anchor-only `PUT /api/clients/{id}/applications` (`UpdateClientApplications`);
  without it the drawer lists the enabled applications read-only.
- The list is the SPA's: search over identifier and name, no filters; here the search covers every
  client and pages 100 rows (the SPA asks for server pages the API ignores: bug 16).
- The applications picker is the kit pick list (`pick-list.md`): ticked applications are posted as a
  list; Save stays disabled until the choice changes.
- **Login Branding** links to the SPA's page (`/clients/{id}/theme`): its theme editor is shared with
  the platform settings and moves with them (Phase 5).
- Suspend sends the SPA's fixed reason ("Manual suspension").

Differences from the SPA: dialogs for create/edit; the picker is the kit's dual pane (checkboxes and a
mirrored list) rather than PrimeVue's move-between-lists; the Deactivate texts say what the platform
does (bug 15) instead of "Soft delete ... Can be reactivated later".

Templ gotcha found here: `{ attrs... }` writes a `templ.Attributes` map in **sorted** order, and Datastar
applies attributes in document order, so a `data-init` that reads a signal declared by `data-signals` in
the same map runs first and sees nothing (the picker's Save was enabled on load). Write such pairs as
explicit attributes, in order, before the spread.

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

### Platform bugs found (recorded, not fixed)

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

### Owner decisions needed

- **Add schema** sends the document (refusing text that is not JSON) instead of the SPA's JSON
  string, and omits the MIME/schema type fields the server ignores (bug 3). Keep, or mirror the SPA?
- **Archive/delete guards** are shown as disabled buttons with reasons, as in the SPA, though the
  server does not enforce them (bug 5). Should the use cases enforce them?
- **Sync Platform Events** is allowed to any anchor user (the BFF's `RequireAnchor`), with no
  event-type permission. Should it need `event-type:sync` or `:manage`?
- **Event types and Subscriptions sidebar entries** now link to their `/ui` pages in the `/ui` sidebar
  (the SPA's own navigation is unchanged).
- **Subscription API permissions** (bug 8): split the any-of gate per operation as for dispatch pools?
- **Client deactivate** (bug 15): should it set INACTIVE (as the SPA says) instead of deleting? The /ui
  page states what happens today.
- **Subscription create body** (bug 9): should the API accept `clientScoped` (and `sequence`), or should
  the SPA stop sending them?
