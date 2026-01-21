# Authentication Guide

FlowCatalyst supports multiple authentication methods: internal username/password, external OIDC providers, and service account authentication. This guide covers configuration and usage.

## Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                    Authentication Methods                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐ │
│  │  Internal   │  │    OIDC     │  │   Service Account       │ │
│  │ (password)  │  │  (SSO/IDP)  │  │  (machine-to-machine)   │ │
│  └──────┬──────┘  └──────┬──────┘  └───────────┬─────────────┘ │
│         │                │                     │               │
│         └────────────────┼─────────────────────┘               │
│                          ▼                                      │
│              ┌─────────────────────┐                           │
│              │   Principal Entity  │                           │
│              └──────────┬──────────┘                           │
│                         │                                       │
│                         ▼                                       │
│              ┌─────────────────────┐                           │
│              │   JWT Access Token  │                           │
│              └─────────────────────┘                           │
└─────────────────────────────────────────────────────────────────┘
```

## Internal Authentication

Username/password authentication for users without external IDP.

### Configuration

No special configuration required - internal auth is always available.

### User Registration

```http
POST /api/auth/register
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "SecurePassword123!",
  "name": "John Doe"
}
```

### Login

```http
POST /api/auth/login
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "SecurePassword123!"
}
```

Response:
```json
{
  "accessToken": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
  "refreshToken": "dGhpcyBpcyBhIHJlZnJlc2ggdG9rZW4...",
  "expiresIn": 3600,
  "tokenType": "Bearer"
}
```

### Password Hashing

Passwords are hashed using Argon2id:
- Memory cost: 65536 KB
- Time cost: 3 iterations
- Parallelism: 4 threads
- Hash length: 32 bytes

## OIDC Authentication

Integration with external identity providers (Keycloak, Azure AD, Okta, etc.).

### Domain-Based Configuration

Configure auth per email domain using `ClientAuthConfig`:

```json
{
  "emailDomain": "acme.com",
  "configType": "CLIENT",
  "primaryClientId": "0HZXEQ5Y8JY5Z",
  "authProvider": "OIDC",
  "oidcIssuerUrl": "https://login.microsoftonline.com/tenant-id/v2.0",
  "oidcClientId": "your-client-id",
  "oidcClientSecretRef": "aws-secretsmanager://acme-oidc-secret"
}
```

### OIDC Login Flow

```
1. User enters email (acme.com domain)
2. FlowCatalyst looks up ClientAuthConfig
3. User redirected to external IDP
4. User authenticates with IDP
5. IDP redirects back with code
6. FlowCatalyst exchanges code for tokens
7. User identity validated/created
8. FlowCatalyst tokens issued
```

### Multi-Tenant OIDC

For IDPs with per-tenant URLs (Azure AD):

```json
{
  "emailDomain": "*.onmicrosoft.com",
  "oidcMultiTenant": true,
  "oidcIssuerPattern": "https://login.microsoftonline.com/[^/]+/v2.0"
}
```

### External Base URL

When running behind a proxy, configure the external URL for callbacks:

```properties
flowcatalyst.auth.external-base-url=https://api.example.com
```

## Service Account Authentication

For machine-to-machine authentication (webhooks, APIs).

### Creating Service Accounts

```http
POST /api/service-accounts
Content-Type: application/json
Authorization: Bearer {admin-token}

{
  "code": "my-service",
  "name": "My Backend Service",
  "clientIds": ["0HZXEQ5Y8JY5Z"],
  "roles": ["webhook-sender"]
}
```

Response includes credentials:
```json
{
  "id": "0HZXEQ5Y8JY00",
  "code": "my-service",
  "apiKey": "fc_sk_live_abc123...",
  "signingSecret": "whsec_xyz789..."
}
```

### API Key Authentication

```http
GET /api/events
Authorization: Bearer fc_sk_live_abc123...
```

### Webhook Signing

Outbound webhooks are signed with HMAC-SHA256:

```
X-FlowCatalyst-Signature: sha256=abc123...
X-FlowCatalyst-Timestamp: 1705312200
```

Verification:
```python
import hmac
import hashlib

def verify_signature(payload, timestamp, signature, secret):
    message = f"{timestamp}.{payload}"
    expected = hmac.new(
        secret.encode(),
        message.encode(),
        hashlib.sha256
    ).hexdigest()
    return hmac.compare_digest(f"sha256={expected}", signature)
```

## OAuth 2.0 Support

FlowCatalyst can act as an OAuth 2.0 authorization server.

### Supported Flows

| Flow | Use Case |
|------|----------|
| Authorization Code + PKCE | SPAs, mobile apps |
| Authorization Code | Server-side apps |
| Client Credentials | Machine-to-machine |
| Refresh Token | Token renewal |

### Authorization Code Flow (with PKCE)

```http
GET /oauth/authorize?
  response_type=code&
  client_id=my-spa&
  redirect_uri=https://app.example.com/callback&
  scope=openid profile&
  state=xyz&
  code_challenge=E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM&
  code_challenge_method=S256
```

### Token Exchange

```http
POST /oauth/token
Content-Type: application/x-www-form-urlencoded

grant_type=authorization_code&
code=abc123&
redirect_uri=https://app.example.com/callback&
client_id=my-spa&
code_verifier=dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk
```

### Client Credentials Flow

```http
POST /oauth/token
Content-Type: application/x-www-form-urlencoded
Authorization: Basic {base64(client_id:client_secret)}

grant_type=client_credentials&
scope=api:read api:write
```

## Token Claims

### Access Token Claims

```json
{
  "sub": "0HZXEQ5Y8JY5Z",
  "iss": "https://api.flowcatalyst.io",
  "aud": "flowcatalyst",
  "exp": 1705315800,
  "iat": 1705312200,
  "scope": "ANCHOR",
  "clients": ["*"],
  "roles": ["platform-admin"],
  "clientId": null,
  "name": "Admin User",
  "email": "admin@flowcatalyst.local"
}
```

### Client Claim Values

| Scope | `clients` Claim |
|-------|-----------------|
| ANCHOR | `["*"]` (access all) |
| PARTNER | `["client-1", "client-2"]` (assigned list) |
| CLIENT | `["client-1"]` (single client) |

## Role Mapping

### IDP Role Mapping

Map external IDP roles to internal roles:

```http
POST /api/auth/idp-role-mappings
Content-Type: application/json

{
  "idpRoleName": "Azure-Admin-Group",
  "internalRoleName": "client-admin"
}
```

This creates an explicit allowlist - only mapped roles grant permissions.

### Role Sync

Roles can be synced from IDP on login:
1. User authenticates via OIDC
2. IDP returns user's groups/roles
3. Groups matched against `IdpRoleMapping`
4. Matched roles assigned to principal

## Security Best Practices

### Token Security

1. **Short access token lifetime** - Default 1 hour
2. **Secure refresh tokens** - Stored as hashes, rotated on use
3. **PKCE required** - For public clients
4. **HTTPS only** - Tokens only transmitted over TLS

### Password Requirements

1. Minimum 8 characters
2. Mixed case recommended
3. Numbers/symbols recommended
4. Common passwords blocked

### Session Management

1. **Logout all sessions**
   ```http
   POST /api/auth/logout-all
   Authorization: Bearer {token}
   ```

2. **Revoke specific token**
   ```http
   POST /api/auth/revoke
   Content-Type: application/json

   {"token": "refresh-token-here"}
   ```

## Configuration Reference

```properties
# JWT Configuration
smallrye.jwt.sign.key.location=private-key.pem
smallrye.jwt.verify.key.location=public-key.pem
mp.jwt.verify.issuer=https://api.flowcatalyst.io

# Token Lifetimes
flowcatalyst.auth.access-token-lifetime=3600
flowcatalyst.auth.refresh-token-lifetime=604800

# External URL (for OAuth callbacks)
flowcatalyst.auth.external-base-url=https://api.example.com

# OIDC (when FlowCatalyst validates external tokens)
quarkus.oidc.auth-server-url=https://auth.example.com
quarkus.oidc.client-id=flowcatalyst
```

## See Also

- [Authorization Guide](authorization.md) - Role-based access control
- [Auth Entities](../entities/auth-entities.md) - Authentication entities
- [OAuth Entities](../entities/oauth-entities.md) - OAuth/OIDC entities
- [Multi-Tenancy](../architecture/multi-tenancy.md) - Tenant model
