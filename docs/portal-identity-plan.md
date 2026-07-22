# Portal identity: platform authenticates, portal apps authorize

Status: DECIDED 2026-07-22 (owner) · Phase 1 implemented
Related: docs/2fa-implementation-plan.md (Go-only divergence precedent), TODO(oidc-by-provider) in oauthapi/authorize.go (closed by Phase 1)

## The decision

FlowCatalyst clients (e.g. VALUE_LOGISTICS) need portals for THEIR customers
(e.g. VODACOM-as-VALUE's-customer, TIGER_BRANDS), including federation to those
customers' own IdPs and self-service user management by those customers'
admins (B2B2B).

Two rejected extremes:

- **Full platform org model** (orgs/memberships/org claims in the platform):
  puts *product* semantics (what a sub-tenant is, who may manage whom) into
  the IdP, where every future auth audit, token shape, SDK, and parity effort
  carries it forever. Irreversible once org claims ship in tokens.
- **Auth built into the portal app**: forks the identity plane — a second
  password store, second MFA/lockout/reset/audit implementation, federation
  built twice, always worse-audited than the platform's.

**The split we chose: the platform owns *proving who someone is* (credentials,
MFA, backoff, federation handshakes, audit). The portal app owns *what they
mean* (organizations, memberships, portal roles, delegated admin).** This
continues the identity-token decision (interactive logins yield authority-free
access tokens; apps run their own sessions from the id_token) and matches the
Laravel SDK native-login bridge pattern the portal will be built on.

Consequences:

- The platform never learns what an "organization" is. TIGER_BRANDS never
  appears in platform tables. VODACOM keeps exactly one `tnt_clients` row (its
  own direct tenancy); its presence in VALUE's portal is portal-side data.
- Portal end-users exist in the platform as **inert identities**: USER
  principals with `scope=CLIENT, client_id=NULL`, no roles, no client-access
  grants, no application access. Every existing authorization surface treats
  them as "no access" (CanAccessClient false, buildClients empty), and
  interactive tokens carry no authority anyway. The portal's membership table
  is the only thing that grants them meaning.
- One human = one principal (global email uniqueness). A VODACOM employee who
  is both a direct platform user and a portal user is ONE principal whose
  authority depends on which door they enter (direct dashboard vs portal OAuth
  client).
- Reversibility: if a second consumer of org structure ever appears (another
  app sharing orgs, machine credentials for a portal org), the portal's
  membership tables become the seed data for promoting orgs into the platform.
  The reverse migration (retracting org claims from tokens/SDKs) would not be
  possible — which is why we start app-side.

## What the platform provides (and nothing else)

1. **Provider-direct OIDC login** — `GET /auth/oidc/login?provider_id=idp_…`
   (and `/oauth/authorize?...&provider=idp_…`, closing the
   TODO(oidc-by-provider) parity gap). Lets a portal send "log this user in
   against TIGER_BRANDS' IdP" without any email-domain mapping. IdP routing is
   EXPLICIT (the portal names the provider); the global
   `tnt_email_domain_mappings` table plays no part, so portal IdPs never
   collide with (or leak into) the client-employee domain-routing path.
2. **Null-client JIT provisioning** — a successful provider-direct login whose
   email has no principal auto-provisions the inert identity described above
   (`CreatePortalUser` operation). No role sync runs (no mapping → no
   role-mapping surface); portal roles are portal data.
3. **Trust binding for provider-direct logins** —
   `identity_providers.allowed_email_domains` is the binding:
   - multi-tenant IdP (Entra common etc.): non-empty allowed_email_domains is
     REQUIRED (fail closed at login start), and the id_token's email domain
     must match one entry. Without this, any tenant of the shared IdP could
     mint identities.
   - single-tenant IdP: the IdP itself bounds the population; the
     allowed-domain check still applies when the list is non-empty.
   All other verification (issuer/pattern, audience, nonce, #EXT# guest
   rejection, state single-use) is identical to the mapping-based flow.
4. **(Phase 2 — next) Ensure/invite API** — `POST /api/principals/portal`
   {email, name?} → {principalId, created}: idempotently ensure a null-client
   principal exists and send an invite (password-reset) email. Called by the
   portal backend's service account (client_credentials → API-authority
   token). Gated on user-admin permission; target is always null-client so
   RequireUserAdmin's anchor rules apply. Needed for invite-based password
   users; SSO-only orgs work without it.
5. **(Deferred) Upstream group relay** — an optional claim relaying the org
   IdP's raw group names on the id_token so the portal can map them to portal
   roles itself. V1 portals use invite/manual membership instead.

## What the portal app owns

- `organizations` (owner client implied by the portal deployment),
  `org_memberships` (principal sub/email ↔ org, portal role, source
  MANUAL|INVITE|IDP_SYNC) — ordinary app tables in the portal DB.
- Delegated admin: an org-admin membership edge scopes every management
  operation to that org. One level, no graph walks. Authority follows the
  edge, never the legal entity: VODACOM's portal org-admin has zero authority
  over `clt_VODA`'s direct tenancy even though both are "VODACOM".
- Login flow: portal starts OAuth (one OAuth client per portal, bound to the
  owner client), optionally with `provider=` for org-federated users; on
  callback it reads the id_token, resolves the user's membership(s), refuses
  users with no membership, and creates its own session. Multi-org users get
  an app-side org switcher (later phase).
- Design portal tables with promotion in mind (stable principal ids as the
  join key, org rows carrying an external identifier) so a future platform
  org model can be seeded from them.

## Phase 1 implementation (this repo, DONE)

- `bridge`: `ResolveByProviderID` (shared cache with the domain path);
  `/auth/oidc/login` accepts `provider_id` as an alternative to `domain`;
  callback branches on `email_domain_mapping_id = ''` (empty-string sentinel —
  the column is NOT NULL, no migration): provider-direct logins enforce
  allowed_email_domains, JIT-provision null-client principals, and skip role
  sync. Mapping-based logins are byte-for-byte unchanged.
- `principal/operations`: `CreatePortalUser` — the deliberate null-client
  create (CreateUser's CLIENT-requires-clientId validation stays intact for
  admin/API paths).
- `oauthapi/authorize.go`: `?provider=` no longer errors; it chains into the
  bridge with the stashed OAuth params exactly like the SPA login path, so the
  downstream app's code is issued on callback.
- Existing invariants that make the inert-identity posture safe (all verified
  in code this session): interactive access tokens are authority-free
  (`token_use=identity`, rejected by the API middleware), refresh families die
  at 7 days absolute, session cookies resolve authorization fresh from the DB
  per request.

## Phase 2 (next)

- `POST /api/principals/portal` ensure/invite endpoint + invite email.
- Portal reference implementation on the Laravel SDK (native-login bridge +
  org tables), including the org-membership gate on login.
- Org-IdP self-service: portal admin surface collects IdP config and manages
  the platform `identity_providers` row via the platform API (scoped service
  account).

## Phase 3 (on demand)

- Upstream group relay claim; multi-org switcher; SAML org IdPs via
  crewjam/saml behind the same bridge seam; promotion of orgs into the
  platform if and only if a second consumer appears.
