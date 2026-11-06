# Java hand-off: `/ui` becomes the default console (SPA URLs redirect)

Go implements it in the commit that adds this file (2026-11-06). Owner ruling, "landing + redirects":
`/` and post-login land on `/ui`, and each Vue SPA page URL redirects to its `/ui` page. The SPA code stays
in the binary. A setting turns the redirects off so a rollback needs no release. Mirror this in the Java
platform once its `/ui` pages cover the same areas (redirect only the SPA routes whose `/ui` page exists).

## The setting

`FC_CONSOLE` = `ui` (default) | `spa`. Read once at startup, case-insensitive; an unknown value logs a warning
and means `ui`. `spa` = today's behaviour (no redirects; the SPA serves its pages). Documented in
`docs/environment-variables.md` section 1.

## Where it hooks

The SPA is the server's NotFound fallback (and the GET method-mismatch fallback, so `GET /auth/login`, whose
POST twin is an API route, reaches it). The redirect is a wrapper around that handler, so it only ever sees
requests no other route matched. Go: `internal/consoleredirect` (wraps `frontend.Handler()` in `cmd/fcdev` and
`cmd/fc-server`). Map: `internal/consoleredirect/routes.go`.

## Behaviour rules

- Only `GET` and `HEAD` are redirected. Other methods pass through unchanged.
- Pass through unchanged: `/ui` and `/ui/...`, API prefixes, and the SPA's static files (anything under
  `/assets/`, any file the embedded dist contains other than `index.html`, any path with a static-asset
  extension), so `FC_CONSOLE=spa` and the SPA's own assets keep working.
- Response: `302 Found`, `Location` = mapped `/ui` path + the original raw query string unchanged,
  `Cache-Control: no-store` (switching the setting back takes effect at once).
- A path that is no SPA page (the SPA's own catch-all sent these to `/dashboard`) goes to `/ui/`.
- Path parameters are copied segment by segment from the escaped request path. A segment is refused (the
  request goes to `/ui/`) when empty, longer than 256, `.`/`..` (raw or `%2e`), containing `%2f`, `%5c`, `%00`,
  a backslash, a control character or a space. Paths longer than 2048 go to `/ui/`. A trailing slash is the
  same page. The Location always starts `/ui/`, is never built from a query value, never `//`.
- Query parameter names are identical between the SPA and `/ui` pages (`token`, `client`, `email`, `reset`,
  `oauth` + the OAuth fields, `interaction`, `flow`), so emailed links keep working with no translation.
  The one translated parameter is the sign-in `redirect` (on the `/ui/auth/*` targets only): the value is an
  SPA path (e.g. `/dispatch-jobs?status=ERROR`), rewritten through the same map to its `/ui` path (own query
  kept) and re-encoded; a value already under `/ui` is kept (`/ui` validates it with `safeUIRedirect`);
  anything else (unmapped, absolute URL, `//host`, backslash, control characters) is dropped. `/ui` only
  accepts `/ui` redirects, which is why this is needed.

## The map (SPA -> `/ui`)

Exact pages unless marked "list". `{x}` segments are copied.

- `/`, `/dashboard`, and any unknown non-file path -> `/ui/`
- `/auth`, `/auth/login` -> `/ui/auth/login`; `/auth/forgot-password`, `/auth/reset-password`,
  `/auth/set-password` -> same under `/ui`; `/logout` -> `/ui/auth/logout`; `/portal/login` -> `/ui/auth/portal-login`
- `/applications[/new|/{id}]`, `/clients[/new|/{id}|/{id}/theme]`, `/users[/new|/{id}]` -> same under `/ui`
- `/client-administration/users[/new|/{id}]` -> `/ui/users[/new|/{id}]`
- `/identity/service-accounts[/new|/{id}]` -> `/ui/service-accounts...`; `/identity/portal-users` -> `/ui/portal-users`;
  `/identity/portal-apps` -> `/ui/portal-apps`; `/identity/developer-users` -> `/ui/developer-users`
- `/authorization/roles` and `/roles` -> `/ui/roles`; `/authorization/roles/{name}[/edit]` -> `/ui/roles/{name}[/edit]`;
  `/authorization/permissions` -> `/ui/permissions`
- `/authentication/identity-providers[/new|/{id}]` -> `/ui/identity-providers...`; `/authentication/domain-idps` ->
  `/ui/identity-providers` (list)
- `/authentication/email-domain-mappings[/new|/{id}]` -> `/ui/email-domains...`; `/authentication/anchor-domains` ->
  `/ui/email-domains` (list)
- `/authentication/reset-approvals` and `/{id}` -> `/ui/reset-approvals` (list for `{id}`: no detail page)
- `/authentication/oauth-clients[/new|/{id}]` -> `/ui/oauth-clients...`
- `/event-types[/create -> new|/{id}|/{id}/add-schema]`, `/scheduled-jobs[/create -> new|/{id}|/{id}/instances|/instances/{instanceId}]`,
  `/processes[/create -> new|/{id}|/{id}/edit]`
- `/subscriptions`, `/connections`, `/dispatch-pools` (`/new`, `/{id}`), `/functions` (`/new`, `/{id}`, `/{id}/traces`),
  `/dispatch-jobs[/{id}]`, `/events[/{id}]` -> same under `/ui`
- `/platform/cors` -> `/ui/cors-origins`; `/platform/audit-log` -> `/ui/audit-log`; `/platform/docs` -> `/ui/platform/docs`;
  `/platform/docs/{source}` -> `/ui/platform/docs` (list: no per-source page); `/platform/docs/{source}/{slug}` -> same
  under `/ui`; `/platform/login-attempts` -> `/ui/login-attempts`; `/platform/settings/theme|names` -> `/ui/platform/settings/theme|names`;
  `/platform/settings/config` -> `/ui/platform/config`; `/platform/debug/events|dispatch-jobs` -> `/ui/debug/events|dispatch-jobs`
- `/developer`, `/developer/applications/{id}[/versions]`, `/profile` -> same under `/ui`

Server-side links that stay SPA-shaped and rely on the redirect (so the setting stays meaningful): the OAuth
authorize endpoint's `/auth/login?oauth=true&...` redirect; emailed `/auth/reset-password?token=` and
`/auth/set-password?token=` links; the portal's `/portal/login` page; the bridge's post-login `/dashboard` target.

## Tests (Go)

`internal/consoleredirect/consoleredirect_test.go`: every mapping (GET and HEAD), every table entry reachable,
query kept byte for byte, the exact `/auth/login?oauth=true&...` shape from `authorize.go`, `redirect`
translation and dropping, non-GET and asset passthrough, `FC_CONSOLE` parsing and `spa` passthrough, hostile
paths (encoded slashes, `//evil`, `..`, long paths, CR/LF) never giving a Location outside `/ui/`.
`internal/ui/consoleredirect_routes_test.go`: walks the `/ui` router and asserts every mapped target is a GET route,
so a `/ui` rename breaks the build. `frontend/handler_test.go`: `ServesAsIs`.
