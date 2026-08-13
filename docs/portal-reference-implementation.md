# Portal reference implementation: what's needed

Status: SDK groundwork DONE 2026-08-13 · reference app NOT STARTED
Parent: docs/portal-identity-plan.md (the decision + platform surface)

The platform side (Phases 1+2) and the SDK groundwork are done. This is the
punch-list for building the first portal on the Laravel SDK.

## Already available (nothing to build)

Platform (merged to main):

- Provider-direct login: `GET /oauth/authorize?...&provider=idp_…` chains into
  the OIDC bridge for the named IdP — no email-domain mapping involved.
- Null-client JIT provisioning: first successful provider-direct login creates
  the inert principal (`scope=CLIENT, client_id=NULL`, no roles, no access).
- Ensure/invite: `POST /api/principals/portal` `{email, name?}` →
  `{principalId, created}` — idempotent; sends the invite (password-reset)
  email while the user has no way to sign in yet. Caller: the portal backend's
  service account (client_credentials); gated on user-admin permission.
- Trust binding: `identity_providers.allowed_email_domains` (REQUIRED for
  multi-tenant IdPs — fail closed).

SDKs (regenerated 2026-08-13, pending release):

- Generated clients now expose the ensure/invite endpoint:
  `ensurePortalUser(PortalUserRequest)` (Laravel `Generated/Client.php`,
  TS `sdk.gen.ts`).
- Laravel login flow passes provider-direct params: `?provider=idp_…` on the
  login route (or `FLOWCATALYST_OIDC_PROVIDER` /
  `flowcatalyst.oidc.provider` as the app-wide default), plus `?prompt=`
  passthrough.
- Native-login bridge (`native_login` opt-in): DatabaseOidcUserHandler upserts
  a local user from the id_token and starts a native Laravel session.

## Platform setup (per portal deployment, no code)

1. An OAuth client for the portal: grant `authorization_code`, redirect URI
   `https://portal.example.com/flowcatalyst/callback`, bound to the owner
   client.
2. A service account for the portal backend with the user-admin permission
   (for ensure/invite) — scoped, not anchor-wide, where possible.
3. One `identity_providers` row per federated customer org, with
   `allowed_email_domains` set (mandatory for multi-tenant IdPs like Entra
   common).

## What the portal app builds

1. **Org tables** — `organizations` (carry an external identifier for future
   promotion) and `org_memberships` (principal id as the join key ­— NOT
   email — plus portal role and source MANUAL|INVITE|IDP_SYNC).
2. **Membership gate** — a custom `OidcUserHandler` wrapping the database
   handler: on callback, resolve membership by the id_token `sub`; NO
   membership → refuse the login (do not create a session). This gate is the
   portal's entire authorization boundary; the platform will happily
   authenticate humans who mean nothing to this portal (e.g. a dashboard user
   who wandered in — see gotchas).
3. **Login UX** — per-org entry points that hit the SDK login route with
   `?provider=idp_…` for federated orgs; plain login (no provider) for
   password/invite users.
4. **Invite flow** — org-admin enters an email → portal calls
   `ensurePortalUser` → stores `{principalId, org, role}` membership. Safe to
   repeat (idempotent; re-invites while the user can't sign in).
5. **Delegated admin** — org-admin membership edge scopes all management to
   that org. One level, no graph walks.
6. **Later phase** — multi-org switcher; org-IdP self-service (portal admin
   manages the `identity_providers` row via the platform API).

## Deployment rules

- **Sibling subdomain, never the platform's hostname.** Same origin would let
  portal XSS drive the platform BFF with a co-resident `fc_session`, and the
  platform SPA owns `/`, `/oauth/*`, `/auth/*`, `/api/*`, `/bff/*`. Same
  registrable domain is fine — `fc_session` is host-only (no Domain
  attribute).
- Portal session is the portal's own Laravel cookie — name it distinctly
  (`SESSION_COOKIE=portal_session`); it never interacts with `fc_session`.

## Gotchas (verified in code)

- **SSO precedence:** `/oauth/authorize` reuses a fresh `fc_session` BEFORE
  honouring `provider=` (authorize.go:133 vs :162). A browser with a live
  platform session skips the org IdP entirely. Send `prompt=login` with
  `provider=` when a fresh IdP handshake is required.
- **Dashboard users pass authentication.** A platform employee visiting the
  portal authenticates fine and reaches the callback; only the membership
  gate turns them away. Design the "no membership" page accordingly.
- **No role sync on provider-direct logins** — deliberate. Portal roles live
  in `org_memberships`; upstream group relay is deferred (Phase 3).
- **Interactive tokens are authority-free** (`token_use=identity`): the portal
  cannot call platform APIs with the user's access token. All platform calls
  go through the portal's service account.
- **One human = one principal** (global email uniqueness): the same email may
  already exist as a direct platform user; ensure/invite returns that
  principal (`created: false`) rather than a new one. Membership rows attach
  portal meaning to it; its platform authority is untouched.

## Prerequisite before starting

Cut SDK releases so the reference app consumes the new surface: Laravel
(> 0.8.20) and TS (> 0.9.11). This regen also carries the previously-pending
clientId removal and Time-schema (`{}` → string) changes.
