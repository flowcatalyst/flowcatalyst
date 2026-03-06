# FlowCatalyst OIDC Provider

FlowCatalyst ships its own OIDC/OAuth 2.0 authorization server built on [`oidc-provider`](https://github.com/panva/node-oidc-provider). It is **not** a proxy to Keycloak, Azure AD, or Auth0 — it **is** the provider. Applications authenticate against FlowCatalyst directly.

## Discovery

The standard OIDC discovery document is available at:

```
GET {issuer}/.well-known/openid-configuration
```

Where `issuer` is configured via `EXTERNAL_BASE_URL` (or `OIDC_ISSUER` if set explicitly). For example:

```
https://platform.yourcompany.com/.well-known/openid-configuration
```

## Endpoints

All OIDC endpoints live under the `/oidc` prefix:

| Endpoint | Path |
|---|---|
| Authorization | `{issuer}/oidc/authorize` |
| Token | `{issuer}/oidc/token` |
| Userinfo | `{issuer}/oidc/me` |
| JWKS | `{issuer}/oidc/jwks` |
| Introspection | `{issuer}/oidc/token/introspection` |
| Revocation | `{issuer}/oidc/token/revocation` |
| End session | `{issuer}/oidc/session/end` |

> **Note:** The authorization endpoint is `/oidc/authorize`, not `/oidc/auth`. The `/auth/*` path prefix is reserved for the platform's own login, logout, and user API routes.

## Grant Types

| Grant type | Use case |
|---|---|
| `authorization_code` | Interactive user login (browser-based apps) |
| `client_credentials` | Server-to-server (service accounts) |
| `refresh_token` | Refresh an access token without re-authenticating |

PKCE (`S256`) is **required** for all public clients (those using `token_endpoint_auth_method: none`).

## Scopes

| Scope | Claims included |
|---|---|
| `openid` | `sub` + all FlowCatalyst custom claims |
| `profile` | `name`, `updated_at` |
| `email` | `email`, `email_verified` |
| `offline_access` | Enables refresh token issuance |

## Token Format

All access tokens are **signed JWTs (RS256)** — never opaque strings. This is enforced via resource indicators regardless of whether the client requests a specific audience.

Tokens can be verified using the public keys at the JWKS endpoint. The key ID (`kid`) in the token header identifies which key to use.

## Custom Claims

FlowCatalyst adds the following custom claims to both the **access token** and **ID token / userinfo**. They are always present when the `openid` scope is requested — no additional scope is needed.

### `type`

The principal type.

| Value | Description |
|---|---|
| `USER` | A human user |
| `SERVICE_ACCOUNT` | A machine/API client |

### `scope`

The principal's tenant scope (not to be confused with OAuth scopes).

| Value | Description |
|---|---|
| `ANCHOR` | Platform-level admin — has access to all tenants |
| `CLIENT` | Scoped to a single tenant/client |
| `PARTNER` | Cross-tenant access (configured per grant) |

### `client_id`

The internal ID of the tenant/client this principal belongs to. `null` for ANCHOR-scoped users.

```json
"client_id": "01JWXYZ123456789ABCDE"
```

### `roles`

Array of role names assigned to the principal. Format: `"<applicationCode>:<ROLE_NAME>"`.

```json
"roles": ["integral:ADMIN", "integral:USER", "flowcatalyst:PLATFORM_ADMIN"]
```

The application code prefix identifies which application the role belongs to. The role name after the colon is uppercase.

### `applications`

Derived from `roles` — the unique set of application codes this principal has any role in. Useful for quick access checks without inspecting individual roles.

```json
"applications": ["integral", "flowcatalyst"]
```

### `clients`

The tenant(s) this principal is authorized to act on behalf of. Format: `"<internalId>:<humanIdentifier>"`.

| Scope | Value |
|---|---|
| `ANCHOR` | `["*"]` — all tenants |
| `CLIENT` | `["01JWXYZ...:acme-corp"]` — single tenant |
| `PARTNER` | `[]` |

The `<humanIdentifier>` part is the client's `identifier` field — a short, readable slug set when the client is created (e.g. `"acme-corp"`, `"inhance"`).

### Full example — access token payload

```json
{
  "sub": "01JWXYZ123456789ABCDE",
  "iss": "https://platform.yourcompany.com",
  "aud": "https://platform.yourcompany.com",
  "iat": 1700000000,
  "exp": 1700003600,
  "type": "USER",
  "scope": "CLIENT",
  "client_id": "01JWabc987654321FGHIJ",
  "roles": ["integral:ADMIN"],
  "applications": ["integral"],
  "clients": ["01JWABC987654321FGHIJ:acme-corp"]
}
```

### Full example — service account token (`client_credentials`)

```json
{
  "sub": "01JWXYZ999999999SERVICE",
  "iss": "https://platform.yourcompany.com",
  "aud": "https://platform.yourcompany.com",
  "iat": 1700000000,
  "exp": 1700003600,
  "type": "SERVICE_ACCOUNT",
  "scope": "CLIENT",
  "client_id": "01JWABC987654321FGHIJ",
  "roles": ["integral:READ_ONLY"],
  "applications": ["integral"],
  "clients": ["01JWABC987654321FGHIJ:acme-corp"]
}
```

> For `client_credentials` tokens, `sub` is the service account's principal ID (not the OAuth `client_id`).

## Connecting Your Application

### Step 1 — Register an OAuth client in FlowCatalyst

Via the FlowCatalyst admin API or UI, create an OAuth client with:

- **Redirect URIs** — all URIs your app will redirect to after login
- **Post-logout redirect URIs** — all URIs your app will redirect to after logout
- **Grant types** — `authorization_code` for browser apps, `client_credentials` for server-to-server
- **Token endpoint auth method** — `none` (public/PKCE) or `client_secret_basic` / `client_secret_post` (confidential)

### Step 2 — Configure your OIDC client library

Point your OIDC client at the discovery URL:

```
https://{platform-domain}/.well-known/openid-configuration
```

Recommended scopes: `openid profile email offline_access`

### Step 3 — Read claims from the token

After a successful login, decode the access token (or call the userinfo endpoint) to read the claims above. The `roles`, `applications`, and `clients` claims are the primary authorization signals.

```js
// Example: check if user has integral admin role
const isIntegralAdmin = token.roles.includes("integral:ADMIN");

// Check if user can access any part of the integral application
const hasIntegralAccess = token.applications.includes("integral");

// Get the tenant identifier
const tenant = token.clients[0]?.split(":")[1]; // "acme-corp"
```

## Redirect URI Wildcard Support

Registered redirect URIs support wildcard patterns in the hostname segment to support multi-tenant or multi-environment deployments without registering every individual URI:

```
https://qa-*.yourcompany.com/callback
https://*.preview.yourcompany.com/auth/callback
```

Exact URI matching is always tried first; wildcard matching is the fallback.

## Logout

See [oidc-logout.md](./oidc-logout.md) for the full RP-initiated logout flow. Always redirect to `/oidc/session/end` rather than only clearing local tokens, otherwise the provider session persists and the user will be silently re-authenticated on the next authorization request.

## Key Rotation

Tokens are signed with RS256. The signing key ID (`kid`) is embedded in the token header. The JWKS endpoint always exposes all current verification keys, so zero-downtime key rotation is supported — the old public key remains in JWKS until all tokens signed with it have expired.

Rotate signing keys using the CLI:

```bash
flowcatalyst rotate-keys [--key-dir <path>] [--keep <n>]
```
