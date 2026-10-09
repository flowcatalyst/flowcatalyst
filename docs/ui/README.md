# Server-rendered UI (`/ui`)

The Vue/PrimeVue SPA is being replaced, area by area, by a server-rendered UI built from the
FlowCatalyst UI kit (`flowcatalyst-framework/ui-kit`: HTML + Tailwind + Datastar) with
[templ](https://templ.guide) components. Plan and rulings: `flowcatalyst-framework/docs/ui-dogfood-plan.md`.
The new UI lives under `/ui` next to the SPA, which stays the default: no SPA route redirects to
`/ui` and no SPA code is removed without the owner's approval, area by area.

Status (2026-10-09): foundations plus one read-only page, `/ui/dispatch-pools`.

## Architecture

```
internal/ui/
  ui.go          Mount: the /ui route group (assets, guards, pages, sign-out)
  ds/            the only package that imports the Datastar Go SDK (datastar-go v1.2.2)
  kit/           templ components, one per kit spec; Props structs mirror the specs' Props tables
  htmlnorm/      HTML normaliser behind the conformance test
  shell/         page layout around the kit app shell; sidebar navigation; error pages
  guard/         CSRF, session negotiation, role-less gate, security headers
  pages/         page handlers (dispatch_pools.go + dispatch_pools.templ)
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
`FilterClientScoped` as `GET /api/dispatch-pools`).

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
   (`sse.Patch(poolsTable(view))`), and `sse.ReplaceURL(q.URL())`; otherwise
   `h.Shell.Render(w, r, shell.Page{Title, Path}, poolsPage(view))`.

Templates live next to the handler (`dispatch_pools.templ`): one top-level page component and
one component per patch target, each with a stable id (`pools-table`, `pools-summary`).

Mutations (later pages) are POST/PUT/PATCH/DELETE only (the CSRF guard relies on GET being
safe), go through the use-case layer exactly as the API handler does, and answer a Datastar
request with patches (`ds.Append("#flash-stack")` for a banner, the changed row, a closed dialog)
or a native form post with a 303 redirect.

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
  `<body>` in the layout).

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

## Conformance fixtures

`flowcatalyst-framework/ui-kit/fixtures/<component>/<case>.{json,html}` (format and normalisation
rules: `ui-kit/fixtures/README.md`), vendored to `internal/ui/testdata/fixtures`.
`TestConformance` (`internal/ui/kit/conformance_test.go`) renders each fixture's props and slots
and compares with the golden through `htmlnorm` (HTML5 fragment parse in the right context,
comments dropped, attributes sorted, whitespace collapsed, class order kept). It fails for a
fixture directory without a renderer, a renderer without fixtures, an unknown prop (strict
decoding) or an unused slot. 112 fixtures pass. Run against the framework checkout directly:
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
Tailwind and Datastar versions). Commit the kit change first (the script warns when the kit has
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

Gaps the pilot needs (not invented silently):
1. **Icons**: the sprite has 38 icons. Missing for the navigation and the pilot pages: plus,
   trash, external-link, eye, server, database, bell, send, clock, box, building, globe, mail,
   lock, link, history, book, log-in, palette, tag, wrench, network/sitemap, zap, house. The
   sidebar uses approximations (`shell/nav.go`); "Create Pool" has no icon.
2. **Row actions**: no spec. Implemented as a PROPOSED part (`kit.RowActions`: a right-aligned
   cell of `.fc-icon-btn` links/buttons, fixture `row-actions/proposed`); needs a spec decision
   (plus a menu variant for more than two actions).
3. **Table toolbar live updates**: no stable patch targets. The Filters button is now its own part
   (`FiltersButton`, re-rendered via the selector `#{formId} [popovertarget]`; patching the
   popover would reset its position while open), but **Clear All** appears/disappears
   conditionally, so after a live filter it is stale until the next navigation. Proposal: an
   always-present `#{formId}-clear` container.
4. **App shell on narrow screens**: `app-shell` has no mobile layout (the SPA has a top bar and an
   off-canvas nav; the Topcoat `sidebar` has a mobile sheet, but it is not the FlowCatalyst shell).
5. **Form controls for the pilot's create/edit**: multi-select with search, a client
   select/filter (the SPA's `ClientSelect`/`ClientFilter`), AutoComplete, number input,
   password with strength meter, secret-reference input (`SecretRefInput`), notification banner
   stack, permission-denied modal. Connections needs the client select and the secret-ref input.
6. **Dark mode** starts from `prefers-color-scheme` via a signal, so a dark-mode user sees one
   light frame before Datastar loads (no inline script is allowed). Proposal: a theme cookie the
   server renders as the `dark` class.
7. **Flash messages**: the kit ported the banner markup but not the flash-cookie plumbing (NP);
   the first page with a mutation needs a small flash helper (cookie set on redirect, read once).
8. **Datastar errors**: the kit has no convention for non-2xx answers. The shell turns 401 into a
   navigation to login; a 403/500 on an action currently shows nothing (proposal: a shell-level
   listener that appends an error banner to `#flash-stack`).
9. Platform, not kit: the SPA login ignores `?redirect=`, so after signing in from a `/ui` link the
   user lands on the SPA's start page, not back on `/ui`.

## Not verified

- Browsers other than Chrome (headless, via the DevTools protocol), screen readers, touch.
- The visual check ran against an isolated instance (own Postgres on a temp data dir, own ports,
  `HOME` pointed at a temp dir so no credential files were written to the user's cache): login
  with the dev bootstrap admin, the pools page in light and dark, live search, the status filter
  in the popover, sort, sidebar collapse and the sign-out CSRF check. No console or CSP errors.
