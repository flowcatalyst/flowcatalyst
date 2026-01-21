# Client & Multi-Tenancy Entities

This document describes the entities that support FlowCatalyst's multi-tenant architecture.

## Client

**Collection**: `auth_clients`

The primary tenant entity representing an organization or account.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | String (TSID) | Primary key |
| `name` | String | Display name |
| `identifier` | String | URL-safe unique identifier |
| `status` | ClientStatus | Current status |
| `statusReason` | String | Reason for status change |
| `statusChangedAt` | Instant | When status last changed |
| `notes` | List\<ClientNote\> | Administrative notes |
| `createdAt` | Instant | Creation timestamp |
| `updatedAt` | Instant | Last update timestamp |

### ClientStatus Enum

```java
public enum ClientStatus {
    ACTIVE,     // Client is operational
    INACTIVE,   // Client is disabled (soft delete)
    SUSPENDED   // Client is temporarily suspended
}
```

### Example Document

```json
{
  "_id": "0HZXEQ5Y8JY5Z",
  "name": "Acme Corporation",
  "identifier": "acme-corp",
  "status": "ACTIVE",
  "statusReason": null,
  "statusChangedAt": null,
  "notes": [
    {
      "text": "Client onboarded via partner program",
      "timestamp": "2024-01-01T00:00:00Z",
      "addedBy": "admin@flowcatalyst.local",
      "category": "ADMIN_NOTE"
    }
  ],
  "createdAt": "2024-01-01T00:00:00Z",
  "updatedAt": "2024-01-15T10:30:00Z"
}
```

---

## ClientNote (Embedded)

Embedded in Client for administrative history.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `text` | String | Note content |
| `timestamp` | Instant | When note was added |
| `addedBy` | String | Who added the note |
| `category` | String | Note category |

### Categories

- `STATUS_CHANGE` - Status transition notes
- `ADMIN_NOTE` - General administrative notes
- `BILLING` - Billing-related notes
- `SUPPORT` - Support ticket references

---

## ClientAccessGrant

**Collection**: `client_access_grants`

Grants PARTNER scope users access to specific clients.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | String (TSID) | Primary key |
| `principalId` | String | Partner user principal ID |
| `clientId` | String | Client being granted access to |
| `grantedAt` | Instant | When access was granted |
| `expiresAt` | Instant | Optional expiration |

### Example Document

```json
{
  "_id": "0HZXEQ5Y8JY5Z",
  "principalId": "0HZXEQ5Y8JY00",
  "clientId": "0HZXEQ5Y8JY11",
  "grantedAt": "2024-01-01T00:00:00Z",
  "expiresAt": "2024-12-31T23:59:59Z"
}
```

### Usage

```java
// Find all clients a partner can access
List<ClientAccessGrant> grants = grantRepository
    .find("principalId", principalId)
    .list();

List<String> clientIds = grants.stream()
    .filter(g -> g.expiresAt == null || g.expiresAt.isAfter(Instant.now()))
    .map(g -> g.clientId)
    .toList();
```

---

## AnchorDomain

**Collection**: `anchor_domains`

Email domains that automatically grant ANCHOR scope.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | String (TSID) | Primary key |
| `domain` | String | Email domain (e.g., "flowcatalyst.local") |
| `createdAt` | Instant | Creation timestamp |

### Example Document

```json
{
  "_id": "0HZXEQ5Y8JY5Z",
  "domain": "flowcatalyst.local",
  "createdAt": "2024-01-01T00:00:00Z"
}
```

### Usage

```java
// Check if domain is anchor domain
boolean isAnchor = anchorDomainRepository
    .find("domain", emailDomain)
    .firstResultOptional()
    .isPresent();
```

---

## ClientAuthConfig

**Collection**: `client_auth_config`

Per-domain authentication configuration.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | String (TSID) | Primary key |
| `emailDomain` | String | Email domain to match |
| `configType` | AuthConfigType | `ANCHOR`, `PARTNER`, or `CLIENT` |
| `primaryClientId` | String | Main client for this domain |
| `additionalClientIds` | List\<String\> | Additional client associations |
| `grantedClientIds` | List\<String\> | For PARTNER scope |
| `authProvider` | AuthProvider | `INTERNAL` or `OIDC` |
| `oidcIssuerUrl` | String | OIDC issuer URL |
| `oidcClientId` | String | OIDC client ID |
| `oidcMultiTenant` | boolean | Multi-tenant OIDC support |
| `oidcIssuerPattern` | String | Regex for multi-tenant issuers |
| `oidcClientSecretRef` | String | Secret reference |
| `createdAt` | Instant | Creation timestamp |
| `updatedAt` | Instant | Last update timestamp |

### AuthConfigType Enum

```java
public enum AuthConfigType {
    ANCHOR,   // Users from this domain get ANCHOR scope
    PARTNER,  // Users from this domain get PARTNER scope
    CLIENT    // Users from this domain get CLIENT scope
}
```

### AuthProvider Enum

```java
public enum AuthProvider {
    INTERNAL,  // Username/password authentication
    OIDC       // External OIDC provider
}
```

### Example Document (OIDC Client)

```json
{
  "_id": "0HZXEQ5Y8JY5Z",
  "emailDomain": "acme.com",
  "configType": "CLIENT",
  "primaryClientId": "0HZXEQ5Y8JY00",
  "additionalClientIds": [],
  "grantedClientIds": [],
  "authProvider": "OIDC",
  "oidcIssuerUrl": "https://login.microsoftonline.com/tenant-id/v2.0",
  "oidcClientId": "app-client-id",
  "oidcMultiTenant": false,
  "oidcIssuerPattern": null,
  "oidcClientSecretRef": "aws-secretsmanager://acme-oidc-secret",
  "createdAt": "2024-01-01T00:00:00Z",
  "updatedAt": "2024-01-01T00:00:00Z"
}
```

### Example Document (Internal Partner)

```json
{
  "_id": "0HZXEQ5Y8JY5Z",
  "emailDomain": "partner.example.com",
  "configType": "PARTNER",
  "primaryClientId": null,
  "additionalClientIds": [],
  "grantedClientIds": ["0HZXEQ5Y8JY00", "0HZXEQ5Y8JY11"],
  "authProvider": "INTERNAL",
  "createdAt": "2024-01-01T00:00:00Z",
  "updatedAt": "2024-01-01T00:00:00Z"
}
```

---

## Application

**Collection**: `auth_applications`

Applications and integrations registered in the platform.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | String (TSID) | Primary key |
| `type` | ApplicationType | `APPLICATION` or `INTEGRATION` |
| `code` | String | Unique application code |
| `name` | String | Display name |
| `description` | String | Application description |
| `iconUrl` | String | Icon URL for UI |
| `defaultBaseUrl` | String | Default API base URL |
| `serviceAccountId` | String | Associated service account |
| `active` | boolean | Whether application is active |
| `createdAt` | Instant | Creation timestamp |
| `updatedAt` | Instant | Last update timestamp |

### ApplicationType Enum

```java
public enum ApplicationType {
    APPLICATION,  // First-party application
    INTEGRATION   // Third-party integration/connector
}
```

### Example Document

```json
{
  "_id": "0HZXEQ5Y8JY5Z",
  "type": "APPLICATION",
  "code": "ecommerce",
  "name": "E-Commerce Platform",
  "description": "Main e-commerce application",
  "iconUrl": "https://cdn.example.com/icons/ecommerce.png",
  "defaultBaseUrl": "https://api.ecommerce.example.com",
  "serviceAccountId": "0HZXEQ5Y8JY00",
  "active": true,
  "createdAt": "2024-01-01T00:00:00Z",
  "updatedAt": "2024-01-01T00:00:00Z"
}
```

---

## ApplicationClientConfig

**Collection**: `application_client_config`

Per-client configuration overrides for applications.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | String (TSID) | Primary key |
| `applicationId` | String | Application ID |
| `clientId` | String | Client ID |
| `enabled` | boolean | Whether app is enabled for client |
| `baseUrlOverride` | String | Override default base URL |
| `configJson` | String | Custom configuration as JSON |
| `createdAt` | Instant | Creation timestamp |
| `updatedAt` | Instant | Last update timestamp |

### Example Document

```json
{
  "_id": "0HZXEQ5Y8JY5Z",
  "applicationId": "0HZXEQ5Y8JY00",
  "clientId": "0HZXEQ5Y8JY11",
  "enabled": true,
  "baseUrlOverride": "https://custom-api.acme.com",
  "configJson": "{\"feature_flags\": {\"beta_features\": true}}",
  "createdAt": "2024-01-01T00:00:00Z",
  "updatedAt": "2024-01-15T10:30:00Z"
}
```

---

## Common Queries

### Find client by identifier

```java
clientRepository.find("identifier", identifier).firstResult();
```

### Find active clients

```java
clientRepository.find("status", ClientStatus.ACTIVE).list();
```

### Find auth config for domain

```java
clientAuthConfigRepository.find("emailDomain", domain).firstResult();
```

### Find partner's accessible clients

```java
List<ClientAccessGrant> grants = grantRepository
    .find("principalId = ?1 and (expiresAt is null or expiresAt > ?2)",
          principalId, Instant.now())
    .list();
```

### Find applications for client

```java
List<ApplicationClientConfig> configs = appConfigRepository
    .find("clientId = ?1 and enabled = true", clientId)
    .list();
```

## See Also

- [Auth Entities](auth-entities.md) - Authentication entities
- [Multi-Tenancy Architecture](../architecture/multi-tenancy.md) - Tenant model
- [Authentication Guide](../guides/authentication.md) - Auth system guide
