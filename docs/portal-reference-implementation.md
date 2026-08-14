# Portal reference implementation: what's needed

Status: portal identity plane (Phase 2.5 v2) BUILT 2026-08-13 · reference app NOT STARTED
Parent: docs/portal-identity-plan.md (the decision + platform surface)

Portal end-users are a SEPARATE identity population from platform users: one
`portal_identities` row per (client, email), platform-implemented, with
initiation endpoints independent of the employee auth surface. This is the
punch-list for building the first portal on the Laravel SDK.

## Already available (nothing to build)

Platform:

- **`GET /portal/authorize`** — the portal plane's OAuth entry (standard
  authorization-code + PKCE params). Serves ONLY portal-flagged OAuth
  clients; never consults `fc_session` (every login is a fresh
  authentication); parks the request and sends the user to the platform's
  portal login page.
- **Portal login page** (platform SPA, `/portal/login`): email-first —
  `POST /portal/auth/check-domain` routes a domain owned by an IdP *bound to
  this client's portal* to SSO, everything else to password;
  `POST /portal/auth/login` verifies the portal identity's password and
  returns the code redirect. Uniform 401s (no account enumeration),
  per-(client, email) rate limiting, no cookies.
- **Portal SSO** — `GET /portal/auth/oidc/login` starts the org-IdP
  handshake through the shared OIDC bridge; the callback enforces the IdP's
  allowed email domains, JIT-creates the portal identity on first login
  (source JIT; suspended identities are refused, never reactivated), issues
  the code, and never writes `fc_session`.
- **Shared `POST /oauth/token`** — the portal app redeems its code as usual.
  Portal subjects (`ptu_…`) get an id_token minted from the portal identity
  (sub = identity id, email, name, EMPTY roles claim), an authority-free
  access token, and **never a refresh token** — the portal app's own session
  is the session.
- **Admin API `/api/portal-users`** (client-delegable: anchor, or the
  `platform:portal-administrator` role + access to the client — see
  Platform setup below):
  - `POST` `{clientId, email, name?, returnInviteLink?, redirectUri?}` →
    `{identityId, created, invited, inviteUrl?}` — idempotent ensure +
    set-password invite (re-mints while the identity has no password).
    `returnInviteLink` hands back the 72h link so the portal sends its own
    branded email (the URL is a live credential — never log it);
    `redirectUri` (exact-match against the client's portal OAuth clients'
    registered URIs) is followed after set-password, landing the user back
    in the portal's login.
  - `GET ?clientId=` — list (email, name, status, source, hasPassword,
    lastLoginAt).
  - `POST {id}/deactivate` / `{id}/activate` `{clientId}` — suspension.
  - `DELETE {id}?clientId=` — offboarding: the identity row is simply
    deleted. Nothing else references it.
- **Set-password page** — the shared platform reset page; portal invite
  tokens branch to the portal identity (password policy enforced, no 2FA
  gate — deferred for portal users) and then follow the invite's
  `redirectUri`.

SDKs (regenerated, pending release): `ensurePortalUser`, `listPortalUsers`,
`activate/deactivatePortalUser`, `deletePortalUser` in both generated
clients; the Laravel login flow's `?provider=`/`?prompt=` passthrough remains
for non-portal OIDC apps (portals point at `/portal/authorize` instead).

## Platform setup (per portal deployment, no code)

1. An OAuth client for the portal: type PUBLIC (PKCE), grant
   `authorization_code`, redirect URI
   `https://portal.example.com/flowcatalyst/callback`, and **Portal owner
   client** set to the operating client (this is what admits it to
   `/portal/authorize`). Configurable in the dashboard's OAuth client editor.
2. A service account for the portal backend holding the seeded
   **`platform:portal-administrator`** role (permissions
   `platform:iam:portal-user:view` + `:manage`) — that role is the entire
   authority `/api/portal-users` needs. The same role given to a client
   administrator lets them manage their client's portal users in the
   platform UI (Client Administration → Portal Users); their client scope
   confines them to their own client. Platform admins (anchor) pass
   everywhere without the role.
3. One `identity_providers` row per federated customer org with its **portal
   binding** (`portalClientId`) set to the operating client and its email
   domains claimed (mappings). A portal-bound IdP serves ONLY that client's
   portal flows.

## What the portal app builds

1. **Org tables** — `organizations` (carry an external identifier) and
   `org_memberships` (portal identity id — the id_token `sub` — as the join
   key, plus portal role and source MANUAL|INVITE|IDP_SYNC).
2. **Login** — point the OAuth flow at `GET /portal/authorize` (not
   /oauth/authorize). On callback, exchange the code, authenticate the user
   from the id_token (`sub` = portal identity id), resolve membership in
   your own tables; no membership → refuse (SSO JIT users may arrive before
   any membership exists — decide between "request access" UX or
   pre-created memberships via the invite flow).
3. **Invite flow** — org-admin enters an email → call `ensurePortalUser`
   with `returnInviteLink: true` and a `redirectUri` pointing at your OAuth
   entry → send your own branded email around the returned `inviteUrl` →
   store `{identityId, org, role}` membership. Idempotent; safe to repeat.
4. **Offboarding** — remove the membership + kill your session (immediate),
   then `DELETE /api/portal-users/{identityId}?clientId=…` so the identity
   is gone (an outstanding authorization code dies at redemption; there are
   no refresh tokens). Suspension: the deactivate endpoint.
5. **Delegated admin** — org-admin membership edge scopes management to that
   org. One level, no graph walks.

## Deployment rules

- **Sibling subdomain, never the platform's hostname** (origin isolation;
  the platform SPA owns `/`). Same registrable domain is fine.
- Portal session is the portal's own cookie; the portal plane sets no
  cookies at all on the platform side.

## Gotchas (verified in code)

- **No SSO reuse, by design**: a platform employee's `fc_session` is
  invisible to `/portal/authorize` — there is no silent SSO into portals and
  no `prompt=login` dance. Every portal login authenticates fresh.
- **Two planes, two identities**: the same email as a platform employee and
  as a portal user are unrelated records with independent credentials.
  Deleting one never touches the other.
- **Suspension bites at the next code issuance/redemption** (portal logins
  are short chains; there are no refresh tokens). Killing the portal's own
  session immediately is the portal's job.
- **JIT never reactivates**: a suspended identity attempting SSO gets
  access_denied; only the admin API (ensure/activate) reactivates.
- **Portal-bound IdPs are invisible to the employee plane's flows** only by
  configuration — don't ALSO create employee email-domain-mappings routing
  general logins to a portal IdP.
- **The portal cannot call platform APIs with the user's tokens** (identity
  tokens are authority-free). All platform calls use the portal's service
  account.

## Prerequisite before starting

Cut SDK releases so the reference app consumes the new surface: Laravel
(> 0.8.20) and TS (> 0.9.11). The regen also carries the previously-pending
clientId removal and Time-schema (`{}` → string) changes.
