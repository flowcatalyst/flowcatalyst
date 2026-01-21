# OAuth/OIDC Entities

This document describes the entities that support OAuth 2.0 and OpenID Connect authentication flows.

## OAuthClient

**Collection**: `oauth_clients`

OAuth client application registrations.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | String (TSID) | Primary key |
| `clientId` | String | OAuth client ID (public) |
| `clientName` | String | Display name |
| `clientType` | ClientType | `PUBLIC` or `CONFIDENTIAL` |
| `clientSecretRef` | String | Secret reference (for confidential) |
| `redirectUris` | List\<String\> | Allowed redirect URIs |
| `grantTypes` | List\<String\> | Allowed OAuth grant types |
| `defaultScopes` | List\<String\> | Default scopes |
| `pkceRequired` | boolean | Require PKCE for public clients |
| `applicationIds` | List\<String\> | Associated applications |
| `serviceAccountPrincipalId` | String | Service account for client credentials |
| `active` | boolean | Whether client is active |
| `createdAt` | Instant | Creation timestamp |
| `updatedAt` | Instant | Last update timestamp |

### ClientType Enum

```java
public enum ClientType {
    PUBLIC,       // SPAs, mobile apps (no secret)
    CONFIDENTIAL  // Server-side apps (has secret)
}
```

### Supported Grant Types

- `authorization_code` - Authorization Code flow
- `refresh_token` - Refresh Token grant
- `client_credentials` - Client Credentials flow

### Example Document (Public Client)

```json
{
  "_id": "0HZXEQ5Y8JY5Z",
  "clientId": "acme-spa",
  "clientName": "Acme Single Page App",
  "clientType": "PUBLIC",
  "clientSecretRef": null,
  "redirectUris": [
    "https://app.acme.com/callback",
    "http://localhost:4200/callback"
  ],
  "grantTypes": ["authorization_code", "refresh_token"],
  "defaultScopes": ["openid", "profile", "email"],
  "pkceRequired": true,
  "applicationIds": ["0HZXEQ5Y8JY00"],
  "serviceAccountPrincipalId": null,
  "active": true,
  "createdAt": "2024-01-01T00:00:00Z",
  "updatedAt": "2024-01-01T00:00:00Z"
}
```

### Example Document (Confidential Client)

```json
{
  "_id": "0HZXEQ5Y8JY5Z",
  "clientId": "acme-backend",
  "clientName": "Acme Backend Service",
  "clientType": "CONFIDENTIAL",
  "clientSecretRef": "aws-secretsmanager://acme-backend-secret",
  "redirectUris": [],
  "grantTypes": ["client_credentials"],
  "defaultScopes": ["api:read", "api:write"],
  "pkceRequired": false,
  "applicationIds": ["0HZXEQ5Y8JY00"],
  "serviceAccountPrincipalId": "0HZXEQ5Y8JY11",
  "active": true,
  "createdAt": "2024-01-01T00:00:00Z",
  "updatedAt": "2024-01-01T00:00:00Z"
}
```

### Methods

```java
// Check if redirect URI is allowed
boolean allowed = oauthClient.isRedirectUriAllowed(uri);

// Check if grant type is allowed
boolean allowed = oauthClient.isGrantTypeAllowed("authorization_code");

// Check if this is a service account client
boolean isServiceAccount = oauthClient.isServiceAccountClient();
```

---

## AuthorizationCode

**Collection**: `authorization_codes`

OAuth authorization codes for code exchange.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `code` | String (TSID) | Authorization code (PK) |
| `clientId` | String | OAuth client ID |
| `principalId` | String | Authenticated principal |
| `redirectUri` | String | Redirect URI used |
| `scope` | String | Granted scopes |
| `codeChallenge` | String | PKCE code challenge |
| `codeChallengeMethod` | String | PKCE method (S256) |
| `nonce` | String | OpenID nonce |
| `state` | String | OAuth state parameter |
| `contextClientId` | String | FlowCatalyst client context |
| `createdAt` | Instant | Creation timestamp |
| `expiresAt` | Instant | Expiration timestamp |
| `used` | boolean | Whether code was exchanged |

### Example Document

```json
{
  "_id": "0HZXEQ5Y8JY5Z",
  "clientId": "acme-spa",
  "principalId": "0HZXEQ5Y8JY00",
  "redirectUri": "https://app.acme.com/callback",
  "scope": "openid profile email",
  "codeChallenge": "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
  "codeChallengeMethod": "S256",
  "nonce": "abc123",
  "state": "xyz789",
  "contextClientId": "0HZXEQ5Y8JY11",
  "createdAt": "2024-01-15T10:30:00Z",
  "expiresAt": "2024-01-15T10:40:00Z",
  "used": false
}
```

### Methods

```java
// Check if code is expired
boolean expired = authCode.isExpired();  // expiresAt < now

// Check if code is valid (not expired and not used)
boolean valid = authCode.isValid();
```

### Security Notes

- Codes expire after 10 minutes
- Codes can only be used once (`used` flag)
- PKCE required for public clients
- Must validate redirect URI matches

---

## RefreshToken

**Collection**: `refresh_tokens`

OAuth refresh tokens for token renewal.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `tokenHash` | String | SHA-256 hash of token (PK) |
| `principalId` | String | Token owner principal |
| `clientId` | String | OAuth client ID |
| `contextClientId` | String | FlowCatalyst client context |
| `scope` | String | Token scopes |
| `tokenFamily` | String | Token family ID for rotation |
| `createdAt` | Instant | Creation timestamp |
| `expiresAt` | Instant | Expiration timestamp |
| `revoked` | boolean | Whether token is revoked |
| `revokedAt` | Instant | When token was revoked |
| `replacedBy` | String | Hash of replacement token |

### Example Document

```json
{
  "_id": "a1b2c3d4e5f6...",
  "principalId": "0HZXEQ5Y8JY00",
  "clientId": "acme-spa",
  "contextClientId": "0HZXEQ5Y8JY11",
  "scope": "openid profile email",
  "tokenFamily": "fam_123456",
  "createdAt": "2024-01-15T10:30:00Z",
  "expiresAt": "2024-01-22T10:30:00Z",
  "revoked": false,
  "revokedAt": null,
  "replacedBy": null
}
```

### Token Rotation

Refresh tokens support rotation for security:

1. Token is used to get new access token
2. Old token is marked `revoked = true`
3. New token created with same `tokenFamily`
4. Old token's `replacedBy` points to new token hash

### Methods

```java
// Check if token is expired
boolean expired = refreshToken.isExpired();

// Check if token is valid (not expired and not revoked)
boolean valid = refreshToken.isValid();
```

### Security Notes

- Tokens stored as SHA-256 hash (never plaintext)
- Token families enable detecting token reuse attacks
- Revoking one token in family should revoke all

---

## OidcLoginState

**Collection**: `oidc_login_state`

OIDC login flow state for CSRF protection.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `state` | String | State parameter (PK) |
| `emailDomain` | String | User's email domain |
| `authConfigId` | String | Auth config used |
| `nonce` | String | OIDC nonce |
| `codeVerifier` | String | PKCE code verifier |
| `returnUrl` | String | Where to redirect after login |
| **OAuth Request Context** | | |
| `oauthClientId` | String | Requesting OAuth client |
| `oauthRedirectUri` | String | OAuth redirect URI |
| `oauthScope` | String | Requested scopes |
| `oauthState` | String | OAuth state from client |
| `oauthCodeChallenge` | String | Client's PKCE challenge |
| `oauthCodeChallengeMethod` | String | PKCE method |
| `oauthNonce` | String | Client's nonce |
| **Timestamps** | | |
| `createdAt` | Instant | Creation timestamp |
| `expiresAt` | Instant | Expiration timestamp |

### Example Document

```json
{
  "_id": "state_abc123xyz",
  "emailDomain": "acme.com",
  "authConfigId": "0HZXEQ5Y8JY00",
  "nonce": "nonce_xyz789",
  "codeVerifier": "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk",
  "returnUrl": "https://app.acme.com/dashboard",
  "oauthClientId": "acme-spa",
  "oauthRedirectUri": "https://app.acme.com/callback",
  "oauthScope": "openid profile email",
  "oauthState": "client_state_123",
  "oauthCodeChallenge": "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
  "oauthCodeChallengeMethod": "S256",
  "oauthNonce": "client_nonce_456",
  "createdAt": "2024-01-15T10:30:00Z",
  "expiresAt": "2024-01-15T10:45:00Z"
}
```

### Purpose

This entity captures the full context of an OIDC login flow:

1. User initiates login from OAuth client
2. State created with all context
3. User redirected to external IDP
4. IDP redirects back with state
5. State validated and original context restored
6. Flow completed back to OAuth client

### Methods

```java
// Check if state is expired
boolean expired = loginState.isExpired();
```

---

## Common Queries

### Find OAuth client by client ID

```java
oauthClientRepository.find("clientId", clientId).firstResult();
```

### Find authorization code

```java
authCodeRepository.findById(code);
```

### Find refresh token by hash

```java
refreshTokenRepository.findById(tokenHash);
```

### Find tokens for principal

```java
refreshTokenRepository.find(
    "principalId = ?1 and revoked = false",
    principalId
).list();
```

### Revoke token family

```java
refreshTokenRepository.update(
    "revoked = true, revokedAt = ?1",
    Instant.now()
).where("tokenFamily", tokenFamily);
```

### Find OIDC login state

```java
oidcLoginStateRepository.findById(state);
```

### Cleanup expired states

```java
oidcLoginStateRepository.delete("expiresAt < ?1", Instant.now());
```

## See Also

- [Auth Entities](auth-entities.md) - Authentication entities
- [Client Entities](client-entities.md) - Multi-tenancy entities
- [Authentication Guide](../guides/authentication.md) - Auth system guide
