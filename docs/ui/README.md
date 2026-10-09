# Server-rendered UI (`/ui`)

The Vue/PrimeVue SPA is being replaced, area by area, by a server-rendered UI built from the
FlowCatalyst UI kit (`flowcatalyst-framework/ui-kit`: HTML + Tailwind + Datastar) with
[templ](https://templ.guide) components. Plan and rulings: `flowcatalyst-framework/docs/ui-dogfood-plan.md`.
The new UI lives under `/ui` next to the SPA, which stays the default: no SPA route redirects to
`/ui` and no SPA code is removed without the owner's approval, area by area.

Status (2026-10-09): foundations, the Phase 1 kit decisions (row actions, toolbar state part,
flash stack and the Datastar error contract, an 86-icon sprite) `/ui/dispatch-pools` and
`/ui/connections` at parity with the SPA pages: list, record drawer, create and edit dialogs,
state actions and delete behind confirmations.

## Architecture

```
internal/ui/
  ui.go          Mount: the /ui route group (assets, guards, pages, sign-out)
  ds/            the only package that imports the Datastar Go SDK (datastar-go v1.2.2)
  kit/           templ components, one per kit spec; Props structs mirror the specs' Props tables
  htmlnorm/      HTML normaliser behind the conformance test
  shell/         page layout around the kit app shell; sidebar navigation; error pages
  guard/         CSRF, session negotiation, role-less gate, security headers
  pages/         page handlers: dispatch_pools*.go/.templ and connections*.go/.templ (list,
                 drawer, form, actions), list.go (the URL as list state),
                 clients.go (the client select's search), forms.go (form plumbing),
                 ops.go (PoolOps, ConnectionOps: the use cases), flash.go (the flash key table)
  assets/        embedded kit.css, icon sprite, pinned datastar.js, licences (served at /ui/assets/)
  testdata/fixtures/  the kit's conformance fixtures, vendored
internal/server/wire_ui.go   registerUI: mounts the above with the platform's Authenticator
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
decoding) or an unused slot. 142 fixtures pass. Run against the framework checkout directly:
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
1. **Icons: done.** 86 icons (was 38), built by the kit's `icons/build-sprite.sh`; the sidebar
   uses the Lucide counterparts of the SPA's PrimeIcons (no approximations); Create Pool has `plus`.
2. **Row actions: done.** Kit spec `row-actions.md`; the proposed part is gone.
3. **Table toolbar live updates: done.** The `{formId}-state` part (above). This also fixed a pilot
   bug: a live sort click did not update the hidden sort fields, so the next search lost the sort.
4. **App shell on narrow screens**: open (`app-shell` has no mobile layout).
5. **Form controls for create/edit: partly done.** Form dialog, autocomplete/client select,
   number input (`input` type number), form section, detail field. Open: multi-select with
   search, password with strength meter, secret-reference input (`SecretRefInput`),
   permission-denied modal.
6. **Dark mode** flashes light once before Datastar loads: open (proposal: a theme cookie the server
   renders as the `dark` class).
7. **Flash messages: done** (`shell.Flash` for Datastar actions; the fixed-key cookie for a native
   post/redirect/get, above).
8. **Datastar errors: done** (the contract and fallback above).
9. **Login `?redirect=`: done** for `/ui` targets.

## Not verified

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
