# Portal apps, portal-user lifecycle, and the platform access gate

What changed in the portal identity plane and in platform access control
(September 2026), why, and where it lives. Builder-facing detail is in
`docs/portal-implementation-guide.md`; the architecture overview is
`docs/published/40-portal-users.md`.

**Reimplementing this elsewhere (e.g. Java)?** Use
`docs/portal-apps-reimplementation-spec.md` — the normative spec: exact
schema, operation algorithms, wire shapes, error codes, events, gate order,
and acceptance scenarios.

## Summary

| Area | Before | After |
|---|---|---|
| Portal apps | One implicit portal per client (`oauth_clients.portal_client_id`) | Named **portal apps** per client (`pta_`), several per client; users **granted per app** |
| Knowing which portal | Not reported | id_token carries `portal_app_code`, `portal_app_id`, `portal_client_id` |
| Invites | Platform UI had an invite button | Invites come **only from the portal app** (`POST /api/portal-users` + `portalAppCode`) |
| User status | UI showed the immutable *Source* column as "Invited" — never changed | Derived **Invited / Invite expired / Active / Suspended** |
| Finding users | Unpaginated list, client-side sort | Server-side `TERM%` search on email + name, app filter, pagination |
| OAuth client for a portal | Created by hand, flagged by hand | **Provisioned automatically** with the portal app; credentials shown once |
| Users with no platform role | Could open the dashboard; ~10 endpoints leaked data; a zero-role ANCHOR user passed most checks | **Profile only** — backend 403 `NO_PLATFORM_ROLE`, SPA shows nothing else |
| Client switcher | SPA called a non-existent route | Fixed |

## 1. Portal apps

A client may run several portals for its customers (e.g. a customer portal
and a supplier portal). Each is a **portal app**:

- `portal_apps` — `id` (`pta_…`), `client_id`, `code` (unique per client,
  lower-case letters/digits/`-`/`_`, immutable), `name`, `description`,
  `active`.
- `portal_identity_apps` — the grant table `(identity_id, portal_app_id,
  source, granted_at)`, cascading on delete of either side.
- `oauth_clients.portal_app_id` — links a portal OAuth client to the app it
  fronts. `portal_client_id` must equal the app's client. A portal OAuth
  client **without** an app link is a *legacy client-wide* portal: no grant
  check and no app code (kept so existing portals keep working).

**Identity model (owner decision):** a portal identity stays one per
(client, email) — one password across that client's portals — and is
granted access per app.

### Login gate

A login through an app-linked OAuth client requires the identity to hold
that app's grant:

- **Password login** — refused with `403 NO_PORTAL_ACCESS` ("You don't have
  access to this portal"), checked *after* the password so it reveals
  nothing to someone who can't prove the credential.
- **SSO** — a first login JIT-creates the identity **and grants the app it
  came through**; an existing identity without the grant gets
  `error=access_denied`.
- **Code redemption** (`/oauth/token`) re-checks the grant, so a revocation
  between code issuance and redemption kills the code.
- An **inactive** app refuses every login and every new grant.

### id_token claims

Portal logins now carry `portal_client_id`, plus `portal_app_code` and
`portal_app_id` for an app-linked OAuth client. The code matches the
`portalAppCode` the portal sends on the admin API. SDK accessors:
Laravel `FlowCatalystUser::getPortalAppCode()` / `getPortalAppId()` /
`getPortalClientId()`; TypeScript `principal.portal = { clientId,
appCode?, appId? }`.

### Automatic OAuth-client provisioning

`POST /api/portal-apps {clientId, code, name, description?, redirectUris?,
clientType?}` creates the app **and** its portal OAuth client in one
transaction (`CreateAppWithOAuthClient`):

- portal-flagged for the client, linked to the app
- `authorization_code` only (portal logins never get refresh tokens), PKCE
  required, scopes `openid profile email`
- `CONFIDENTIAL` by default (server-side portal with a secret); `PUBLIC`
  for browser-only portals
- the callback URL(s) registered (absolute http(s), no wildcards)

The response carries `oauthClientId`, `oauthClientRowId`, `clientType` and,
for CONFIDENTIAL, `clientSecret` — **returned exactly once** (stored
hashed). The Portal Apps page shows these with copy buttons, the portal
endpoints (`/portal/authorize`, `/oauth/token`, JWKS, discovery) and a
ready-to-paste Laravel `.env` block.

**Deleting an app deletes its OAuth client(s)** in the same transaction
(`DeleteApp`). They are deliberately not unlinked: an unlinked portal OAuth
client becomes a legacy client-wide portal, which would silently widen who
can sign in through it.

Creating a portal app — and so its OAuth client — is gated by the
client-delegable `platform:iam:portal-user:manage` permission (the
`platform:portal-administrator` role). The OAuth client is bounded: the
caller's own client, portal-flagged, never `apiAccess`. Editing it later
(callback URLs, secret rotation) remains an anchor action under
*Identity & Access → OAuth Clients*.

### OAuth-client drawers

The create/edit drawers gain a **Portal app** picker, scoped to the chosen
portal owner client. The controller resolves `portalAppId` to its client
and refuses a conflicting `portalClientId` (`PORTAL_APP_CLIENT_MISMATCH`);
a portal app can only be linked to a portal client
(`PORTAL_APP_REQUIRES_PORTAL_CLIENT`). Clearing the portal owner also
unlinks the app.

## 2. Portal users

### Status that moves

The "stuck at INVITED" report was the *Source* column (`INVITE` vs `JIT`),
which never changes. Status is now **derived, never stored**, so it cannot
drift:

| State | Condition |
|---|---|
| `SUSPENDED` | status is `DISABLED` |
| `ACTIVE` | a password has been created, or the user has signed in (incl. every JIT identity) |
| `INVITE_EXPIRED` | never completed, and the 72h set-password link has lapsed |
| `INVITED` | otherwise — invite outstanding (SSO invites never expire) |

`portal_identities` gains `invited_at` / `invite_expires_at`, written by
the invite path (`MarkInvited`) each time an invite is delivered or a link
minted. Migration 053 backfills them from live invite tokens, else from
`created_at + 72h`. The raw `status` (ACTIVE/DISABLED) is unchanged on the
wire; `state` is added.

### Search and pagination

`GET /api/portal-users?clientId=&q=&portalAppCode=&page=&size=`:

- `q` — case-insensitive **prefix** (`TERM%`) on email and name; LIKE
  metacharacters are literal. Prefix only: `jones` does not find
  "Pat Jones". Backed by `text_pattern_ops` indexes on `(client_id, email)`
  and `(client_id, lower(name))`.
- `portalAppCode` — only users granted that app (unknown code → 404).
- `page` (0-based) / `size` (default **100**, max 1000); the response adds
  `total`, `page`, `size`. **Behaviour change:** the list used to return
  everything — callers that relied on that must page through `total`.
- Each row adds `state`, `apps [{id, code, name, source, grantedAt}]`,
  `invitedAt`, `inviteExpiresAt`.

### Invites and grants

- `POST /api/portal-users` accepts `portalAppCode` (case-insensitive): the
  identity is granted that app (other grants untouched), the invite's
  default redirect prefers that app's OAuth client, and the response echoes
  `portalAppCode` and `state`.
- `POST /api/portal-users/{id}/apps {clientId, portalAppCode}` grants;
  `DELETE /api/portal-users/{id}/apps/{portalAppCode}?clientId=` revokes one
  portal while the identity and its other portals stay. `DELETE
  /api/portal-users/{id}` still offboards from every portal of the client.
- The platform UI's **invite button is removed**: the portal owns the
  membership half of an invite, so invites come from the portal app.

### Portal Users page

*Portal → Portal Users* (moved out of Client Administration): client
picker, portal-app filter, debounced server-side search, server
pagination, a status tag with invite dates on hover, app chips (removable,
with confirmation), suspend/reactivate, and delete.

## 3. Profile-only access for users without a platform role

**Rule (owner decision):** a user with no platform role may see only their
own profile. A portal login sees nothing — portal identities never get a
platform session at all.

An audit found the rule was not enforceable per-handler: about ten
authenticated endpoints had no permission check (e.g. `/bff/roles`,
`/bff/event-types`, `/api/email-domain-mappings/lookup`,
`/api/audit-logs/batch`), and because `requirePermission`/`IsAdmin`
short-circuit on the anchor tier, a zero-role ANCHOR user passed almost
every check.

**Backend** — `middleware.ProfileOnlyWithoutRole`, mounted right after
`Authenticator` so it covers every huma and chi route in the authenticated
group. A context that is a **USER** with **no roles and no permissions**
gets `403 {code: "NO_PLATFORM_ROLE"}` except on:

- `/auth/*` — me, change-password, login history, 2FA, passkeys, logout,
  client selection, the login flows
- `/portal/*` — the portal plane's own login routes
- `GET /api/me`

Principal type now rides on `AuthContext.PrincipalType`: session cookies
are always `USER`; bearers take the access token's `type` claim (parsed by
`sessiontoken`). Service accounts are exempt — application service accounts
authorize through their `applications` claim, not roles. Unauthenticated
requests pass through to the handlers' own checks (the group also hosts the
public login flows). Dev test-header contexts are exempt unless they send
`X-FC-Test-Principal-Type: USER`.

**SPA** — `canAccessPath` denies every route but `/profile` for a user with
no roles and no permissions (`isRoleless`), so the sidebar is empty and
every navigation lands on Profile. The previously unmapped portal routes are
now mapped to `platform:iam:portal-user:view`.

## 4. Client switcher fix

The SPA's `switchClient` posted to `/auth/client/{id}` with no body; the
backend serves `POST /auth/client/switch {clientId}`. It now calls the
correct route. (`switchClient` currently has no callers, so this was latent.)

## Where the code lives

| Concern | Location |
|---|---|
| Migration (apps, grants, OAuth link, invite dates, search indexes) | `internal/migrate/sql/053_portal_apps.sql` |
| Identity entity, derived state, grants | `internal/platform/portalidentity/entity.go` |
| Portal app aggregate + repository | `internal/platform/portalidentity/app.go` |
| App operations incl. `CreateAppWithOAuthClient`, `DeleteApp` | `internal/platform/portalidentity/app_operations.go` |
| Ensure / grant / revoke operations | `internal/platform/portalidentity/operations.go` |
| Search, grant sync, `MarkInvited` | `internal/platform/portalidentity/repository.go` |
| Admin API (`/api/portal-users`, `/api/portal-apps`) | `internal/platform/portalidentity/api/api.go` |
| Password-login app gate | `internal/platform/portalauth/endpoints.go` |
| SSO app gate + JIT grant | `internal/platform/auth/bridge/login_endpoint.go` |
| Redemption re-check + portal claims | `internal/platform/auth/oauthapi/portal_token.go`, `authservice.GeneratePortalIDToken` |
| OAuth client `portalAppId` | `internal/platform/auth/{entity,repository}.go`, `auth/api`, `auth/operations/oauth_client.go` |
| Profile-only gate | `internal/platform/shared/middleware/profile_only.go`, `shared/auth.AuthContext.IsRoleless` |
| SPA | `frontend/src/pages/portal/PortalAppsPage.vue`, `PortalUsersPage.vue`, `stores/permissions.ts`, `config/navigation.ts`, OAuth client drawers |
| SDKs | regenerated clients; Laravel `FlowCatalystUser`, TS `fastify/oidc/claims.ts` + `principal.ts` |

## Tests

- `portalidentity/entity_test.go` — state derivation, grant/revoke, code rules
- `shared/middleware/profile_only_test.go` — the gate (role-less CLIENT and
  ANCHOR users, admins, service accounts, anonymous)
- `portalauth/portal_app_gate_pg_test.go` — password-login gate, grant,
  inactive app, revoke, delete-with-OAuth-client
- `auth/oauthapi/portal_app_token_pg_test.go` — `portal_app_*` claims,
  revocation kills an outstanding code
- `portalidentity/api/search_pg_test.go` — prefix search, escaping, app
  filter, pagination, INVITED → ACTIVE, INVITE_EXPIRED
- `portalidentity/api/create_app_pg_test.go` — provisioning, one-time
  secret, PUBLIC, duplicate code writes nothing, bad callback refused

## Follow-ups (not done)

- Let client administrators edit a portal app's callback URLs and rotate
  its secret from *Portal Apps* (today that is an anchor action on the
  OAuth client).
- Infix/word-start name search if prefix-only proves too strict.
- 2FA for portal password users remains deferred.
