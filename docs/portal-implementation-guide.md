# Building a portal on FlowCatalyst

How to use the platform's **portal identity plane** to authenticate and
manage a client's portal end-users — in general (any stack), with the
Laravel SDK, and with the TypeScript SDK.

Companion docs: `docs/portal-identity-plan.md` (the architecture decision),
`docs/portal-reference-implementation.md` (punch-list for the first
reference app).

---

## 1. Concepts

A **portal** is an application a FlowCatalyst client (e.g. VALUE_LOGISTICS)
runs for *their* customers (e.g. TIGER_BRANDS). Those customers' users are
**portal identities** — a population completely separate from platform
users:

- One `portal_identities` row per **(client, email)**. The same human at two
  clients' portals is two identities with independent credentials; a
  platform employee with the same email is a third, unrelated record.
- Portal identities have **no platform authority whatsoever**: no roles, no
  client access, no API access. What a portal user *means* (their
  organization, their portal role) is entirely the portal app's data, keyed
  by the identity id.
- The platform owns *proving who they are*: passwords (platform password
  policy, hashing, reset/invite machinery), or federation to the customer
  org's own IdP.
- The portal plane has **its own entry points** (`/portal/*`) and **never
  touches the platform session** (`fc_session`). There is no silent SSO into
  a portal; every portal login is a fresh authentication. The portal app
  runs its own session from the id_token.

Identity ids are branded TSIDs with the `ptu_` prefix (e.g.
`ptu_0H5X…`). Use them — not emails — as your join key.

## 2. Platform setup (once per portal)

All three are configurable in the platform dashboard; the third can also be
managed via API.

1. **A portal OAuth client** (*Identity & Access → OAuth Clients*):
   - Type: **CONFIDENTIAL** for a server-side portal app (the backend
     exchanges the code and keeps the secret — client authentication plus
     PKCE, keep *Require PKCE* on); **PUBLIC** only for a portal whose
     browser performs the token exchange itself. Grant `authorization_code`.
   - Redirect URI: your portal's OAuth callback (e.g.
     `https://portal.example.com/flowcatalyst/callback`).
   - **Portal owner client** set to the operating client. This flag is what
     admits the OAuth client to `/portal/authorize` — without it the portal
     plane refuses it, and with it the client stops being usable for
     ordinary platform logins.
2. **A service account** for the portal backend, holding the built-in
   **`platform:portal-administrator`** role. That role (permissions
   `platform:iam:portal-user:view` + `platform:iam:portal-user:manage`) is
   all the authority the portal-user admin API needs. The same role given
   to one of the client's administrators lets them manage portal users in
   the platform UI (*Client Administration → Portal Users*); client scope
   confines each holder to their own client.
3. **Identity providers for federated customer orgs** (optional, per org):
   an IdP row with its **portal binding** (`portalClientId`) set to the
   operating client, and the org's email domains claimed. A portal-bound
   IdP serves *only* that client's portal flows; don't also route employee
   logins to it via email-domain mappings.

## 3. The login flow (any stack)

The portal plane speaks standard **OAuth 2.0 authorization-code + PKCE**;
only the entry endpoint differs from a normal OIDC integration.

```
Portal app                        FlowCatalyst platform
----------                        ---------------------
1. Redirect the browser to:
   GET {platform}/portal/authorize
       ?response_type=code
       &client_id=<portal OAuth client_id>
       &redirect_uri=<registered redirect>
       &scope=openid profile email
       &state=<random>                     2. Validates client (must be
       &nonce=<random>                        portal-flagged) + redirect_uri
       &code_challenge=<S256>                 + PKCE, parks the request,
       &code_challenge_method=S256            shows its portal login page:
                                              - email first
                                              - domain owned by a bound IdP
                                                → SSO handshake with the org
                                                IdP (JIT-creates the identity
                                                on first login)
                                              - otherwise → password form
3. Browser lands on
   {redirect_uri}?code=…&state=…           (or ?error=access_denied&… )

4. POST {platform}/oauth/token             5. Answers with:
   grant_type=authorization_code              access_token (authority-free)
   code=…  redirect_uri=…                     id_token   (the identity proof)
   client_id=…  code_verifier=…               NO refresh_token — ever

6. Verify the id_token (platform JWKS), read its claims,
   resolve your own membership data, create YOUR session.
```

Key properties:

- **id_token claims**: `sub` = the portal identity id (`ptu_…`), `email`,
  `name`, `aud` = your client_id, `nonce`, and an **empty `roles` claim** —
  portal roles are your data, never the platform's.
- **The access token is deliberately useless** for platform APIs
  (`token_use=identity`). All platform calls from your backend use the
  service account (client_credentials).
- **No refresh tokens.** Your app session *is* the session; when it ends,
  send the user through `/portal/authorize` again.
- **Errors** come back on your redirect URI as standard OAuth params
  (`error=access_denied` for a suspended identity, etc.).
- Suspension/deletion of an identity bites at the next code issuance or
  redemption. Ending your own app session on offboarding is your job.

## 4. Managing portal users (admin API)

All endpoints live under `/api/portal-users`, authenticated with your
service account's bearer token, and are scoped by `clientId` (the operating
client). Requests from callers without the portal-administrator role (or
anchor tier) are refused.

### Ensure / invite

```
POST /api/portal-users
{
  "clientId": "clt_…",
  "email": "pat@customer.example",
  "name": "Pat Jones",                    // optional, applied on create
  "returnInviteLink": true,               // optional, see below
  "redirectUri": "https://portal.example.com/auth/entry"  // optional
}
→ { "identityId": "ptu_…", "created": true, "invited": false,
    "inviteUrl": "https://platform…/auth/reset-password?token=…" }
```

- **Idempotent.** Re-calling for an existing identity reactivates a
  suspended one and re-mints/re-sends the invite while the identity has no
  password yet. Identities that can already sign in are left alone.
- **Invite delivery** — two modes:
  - Default: the platform emails its own set-password invite (requires the
    platform's SMTP to be configured).
  - `returnInviteLink: true`: nothing is emailed; the response carries the
    72-hour set-password URL and **you** send your own fully-branded email
    around it. The URL is a live bearer credential — never log it.
- **`redirectUri`** is followed after the user sets their password, so they
  land straight back in your login flow. It must exactly match a registered
  redirect URI of one of the client's portal OAuth clients (open-redirect
  defence). Point it at wherever you start `/portal/authorize`. **When
  omitted** (e.g. invites sent from the platform UI), the platform defaults
  it to the portal's origin, derived from the portal OAuth client's
  registered redirect URI — the invitee still lands back at the portal.
- SSO-only orgs don't need invites at all — the first IdP login JIT-creates
  the identity. Call ensure anyway when you want the identity id up front
  (e.g. to create a membership before first login).

### List, suspend, reactivate, delete

```
GET    /api/portal-users?clientId=clt_…
→ { "portalUsers": [ { "identityId", "email", "name", "status",
                       "source", "hasPassword", "lastLoginAt",
                       "createdAt", "updatedAt" } ] }

POST   /api/portal-users/{identityId}/deactivate   { "clientId": "clt_…" }
POST   /api/portal-users/{identityId}/activate     { "clientId": "clt_…" }
DELETE /api/portal-users/{identityId}?clientId=clt_…
```

- **Deactivate** blocks all portal login (password and SSO — an SSO login
  never self-reactivates) but keeps the row for reactivation.
- **Delete** is offboarding: the identity is simply gone. Nothing else in
  the platform references it.
- Offboarding sequence in your app: remove your membership row, kill your
  session, then delete the platform identity.

## 5. Laravel SDK

The Laravel SDK provides both halves: the browser login flow and the typed
admin client.

### Login flow

```dotenv
FLOWCATALYST_BASE_URL=https://your-platform.example.com

# Portal login (browser flow)
FLOWCATALYST_OIDC_ENABLED=true
FLOWCATALYST_OIDC_CLIENT_ID=<portal OAuth client_id>
FLOWCATALYST_OIDC_CLIENT_SECRET=<secret>   # confidential client (recommended server-side)
FLOWCATALYST_OIDC_PORTAL=true          # <— routes login through /portal/authorize

# Backend API access (portal-user management)
FLOWCATALYST_CLIENT_ID=<service account client id>
FLOWCATALYST_CLIENT_SECRET=<service account secret>
```

With `FLOWCATALYST_OIDC_PORTAL=true` the SDK's login route
(`/flowcatalyst/login`) redirects to `/portal/authorize` instead of
`/oauth/authorize`; the callback and token exchange are unchanged.
`provider`/`prompt` are ignored in portal mode — the platform's portal
login page routes org IdPs from the email domain by itself.

Implement your membership gate in a custom `OidcUserHandler` — the `sub`
claim is the portal identity id:

```php
final class PortalUserHandler implements OidcUserHandler
{
    public function handleAuthenticatedUser(FlowCatalystUser $user): mixed
    {
        // $user->sub is the portal identity id (ptu_…) — your join key.
        $memberships = OrgMembership::where('identity_id', $user->sub)->get();
        if ($memberships->isEmpty()) {
            abort(403, 'No portal access. Ask your administrator for an invite.');
        }
        session()->put('portal_user', [
            'id' => $user->sub, 'email' => $user->email, 'name' => $user->name,
        ]);
        return $user;
    }

    public function handleLogout(): void { session()->flush(); }
    public function getPostLoginRedirect(): string { return '/'; }
    public function getPostLogoutRedirect(): string { return '/'; }
}
```

(Bind it in a service provider; leave `FLOWCATALYST_NATIVE_LOGIN` off — the
database handler's local-user upsert targets platform employees, not portal
identities.)

Give the portal its own distinctly-named session cookie
(`SESSION_COOKIE=portal_session`) and deploy on its own hostname (a sibling
subdomain of the platform is fine; the same hostname is not).

### Managing portal users

The generated client (the `FlowCatalyst` facade's `generated()` accessor)
carries the full admin surface:

```php
use FlowCatalyst\Generated\Model\PortalUserRequest;
use FlowCatalyst\Generated\Model\PortalUserClientBody;

$api = FlowCatalyst::generated();

// Invite (portal-managed email): returns the invite URL for YOUR mailer.
$req = (new PortalUserRequest())
    ->setClientId($clientId)
    ->setEmail('pat@customer.example')
    ->setName('Pat Jones')
    ->setReturnInviteLink(true)
    ->setRedirectUri('https://portal.example.com/auth/entry');
$result = $api->ensurePortalUser($req);
// $result->getIdentityId(), ->getCreated(), ->getInviteUrl()

// List / suspend / reactivate / offboard
$list = $api->listPortalUsers(['clientId' => $clientId]);
$api->deactivatePortalUser($identityId, (new PortalUserClientBody())->setClientId($clientId));
$api->activatePortalUser($identityId, (new PortalUserClientBody())->setClientId($clientId));
$api->deletePortalUser($identityId, ['clientId' => $clientId]);
```

## 6. TypeScript SDK

The TS SDK provides the typed admin client AND, for Fastify apps, the full
browser login flow: the `@flowcatalyst/sdk/fastify` auth plugin gains
**portal mode** — set `portal: true` and its login route enters through
`/portal/authorize` (callback, token exchange, session cookie, and id_token
verification unchanged; the session principal's id is the `ptu_…` identity):

```ts
import { flowcatalystAuth } from '@flowcatalyst/sdk/fastify';

await app.register(flowcatalystAuth, {
  baseUrl: process.env.FC_BASE_URL!,
  clientId: process.env.FC_OIDC_CLIENT_ID!,      // portal OAuth client
  clientSecret: process.env.FC_OIDC_CLIENT_SECRET!,
  portal: true,                                   // <— portal identity plane
  expectedAudience: process.env.FC_OIDC_CLIENT_ID!,
  cookie: { secret: process.env.SESSION_SECRET! },
});
```

Non-Fastify stacks wire the standard OAuth dance from §3 with any OIDC
tooling (or plain fetch), pointing the authorize step at
`/portal/authorize` and verifying the id_token against
`{platform}/.well-known/jwks.json`:

### Managing portal users

```ts
import { FlowCatalystClient } from '@flowcatalyst/sdk';
import {
  ensurePortalUser, listPortalUsers,
  activatePortalUser, deactivatePortalUser, deletePortalUser,
} from '@flowcatalyst/sdk';

const fc = new FlowCatalystClient({
  baseUrl: process.env.FC_BASE_URL!,
  clientId: process.env.FC_CLIENT_ID!,         // service account
  clientSecret: process.env.FC_CLIENT_SECRET!,
});

// Invite (portal sends its own email around inviteUrl)
const ensured = await fc.request((client, headers) =>
  ensurePortalUser({
    client, headers,
    body: {
      clientId, email: 'pat@customer.example', name: 'Pat Jones',
      returnInviteLink: true,
      redirectUri: 'https://portal.example.com/auth/entry',
    },
  }),
);

// List / lifecycle
const users = await fc.request((client, headers) =>
  listPortalUsers({ client, headers, query: { clientId } }));
await fc.request((client, headers) =>
  deactivatePortalUser({ client, headers, path: { id }, body: { clientId } }));
await fc.request((client, headers) =>
  deletePortalUser({ client, headers, path: { id }, query: { clientId } }));
```

### Login flow sketch (framework-agnostic)

```ts
// 1. Start: redirect the browser
const url = new URL(`${platform}/portal/authorize`);
url.search = new URLSearchParams({
  response_type: 'code', client_id: portalClientId,
  redirect_uri: callbackUrl, scope: 'openid profile email',
  state, nonce, code_challenge: challenge, code_challenge_method: 'S256',
}).toString();
reply.redirect(url.toString());

// 2. Callback: exchange + verify (e.g. with jose). A CONFIDENTIAL client
// authenticates the exchange (Basic auth shown; client_secret in the form
// body also works). A PUBLIC client omits the secret entirely.
const tokens = await fetch(`${platform}/oauth/token`, {
  method: 'POST',
  headers: {
    'content-type': 'application/x-www-form-urlencoded',
    authorization: 'Basic ' +
      Buffer.from(`${portalClientId}:${portalClientSecret}`).toString('base64'),
  },
  body: new URLSearchParams({
    grant_type: 'authorization_code', code, redirect_uri: callbackUrl,
    client_id: portalClientId, code_verifier: verifier,
  }),
}).then(r => r.json());

const { payload } = await jwtVerify(tokens.id_token, jwks, {
  issuer: platform, audience: portalClientId,
});
// payload.sub = ptu_… identity id; check payload.nonce === nonce.
// Resolve YOUR membership for payload.sub; none → refuse.
// Create your own session. There is no refresh_token to store.
```

## 7. Security & operational notes

- **Invite URLs are credentials** (72h, single-use set-password). Treat the
  `inviteUrl` like a password: deliver it, don't persist or log it.
- **Rate limiting**: the password endpoint is throttled per (client, email)
  (`FC_RL_PORTAL_LOGIN_PER_15MIN`, default 10). Uniform 401s prevent
  account enumeration.
- **2FA for portal password users is deferred**; org-federated users
  inherit MFA from their own IdP. Prefer SSO for security-sensitive orgs.
- **Login flows expire after 15 minutes** — a stale portal login page tells
  the user to return to the portal, which just restarts `/portal/authorize`.
- **Suspension latency**: platform-side suspension blocks the next login /
  code redemption; your app session survives until you end it.
- **Deployment**: own hostname (sibling subdomain fine), own session
  cookie. The portal plane sets no cookies on the platform side.
- **Membership promotion**: keep an external identifier on your org rows
  and the `ptu_` id as the membership key, so a future platform org model
  could be seeded from your tables.
