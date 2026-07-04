# Verify FlowCatalyst UI/API changes

## Launch

- An fc-dev instance is often already running on :8080 (embedded PG on :15432) — check with
  `lsof -nP -iTCP:8080 -sTCP:LISTEN` and **do not kill it** (it may be the owner's).
- Frontend changes: serve the working tree via Vite proxying to the running API —
  `cd frontend && VITE_BACKEND_PORT=8080 pnpm exec vite --port 4200` (background it).
  After adding a dependency, start with `--force`; expect the dep optimizer to reload pages
  mid-flow on first visit of each lazy route — warm every route once before asserting.
- No API running: `bin/fc-dev start` (built by `make go-build`; needs `make frontend` first
  because the SPA is embedded at compile time).

## Login

Seeded dev admin: `admin@flowcatalyst.local` / `DevPassword123!` (from `fc-dev start` defaults).
The login is two-step: fill the email input → click "Continue" → wait for
`input[type="password"]` → fill → submit → lands on `/dashboard`.

## Drive

No Playwright in the repo. Install `playwright-core` in the session scratchpad and use system
Chrome: `chromium.launch({ channel: "chrome", headless: true })`. Screenshot to scratchpad.

Useful selectors: `.fc-table-toolbar` (list toolbar), `.fc-filter-popover` (filter popup opened
by the Filters button), `.p-drawer.entity-drawer` (detail/create drawers), `.p-confirmdialog`
(discard/confirm), `.sidebar-profile-popover` (profile menu), `.p-datatable-tbody tr` (rows;
action buttons are the row's `button` elements).

Flows worth driving for list/detail changes: filter popup (badge + Clear All + URL params;
selecting inside the popup must NOT close it — nested dropdowns use appendTo="self"); drawer
open over list (instant, no slide animation by design); Escape/backdrop close; dirty guard
(`?edit=true` then Escape); deep-link `/module/:id` in a fresh navigation; browser Back closes
drawer preserving query.

## Gotchas

- The embedded SPA in a running binary is stale until `make frontend && make go-build`.
- PrimeVue theme CSS is injected after app CSS: overriding theme rules on `.p-drawer` etc.
  needs higher specificity, not just equal (see EntityDrawer.vue global styles).
- Base font is 12.6px, so theme `rem` sizes are ~0.79× the usual pixel values.
