# Authentication & Authorization Entities

This document describes the core authentication and authorization entities in FlowCatalyst.

## Principal

**Collection**: `auth_principals`

Unified identity model for users and service accounts.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | String (TSID) | Primary key |
| `type` | PrincipalType | `USER` or `SERVICE` |
| `scope` | UserScope | `ANCHOR`, `PARTNER`, or `CLIENT` |
| `clientId` | String | Home client (for CLIENT scope) |
| `applicationId` | String | Associated application (optional) |
| `name` | String | Display name |
| `active` | boolean | Whether principal is active |
| `userIdentity` | UserIdentity | Embedded user details (for USER type) |
| `roles` | List\<RoleAssignment\> | Assigned roles |
| `createdAt` | Instant | Creation timestamp |
| `updatedAt` | Instant | Last update timestamp |

### PrincipalType Enum

```java
public enum PrincipalType {
    USER,     // Human user account
    SERVICE   // Service account for machine-to-machine auth
}
```

### UserScope Enum

```java
public enum UserScope {
    ANCHOR,   // Platform admin - access to ALL clients
    PARTNER,  // Partner - access to ASSIGNED clients
    CLIENT    // End user - bound to SINGLE client
}
```

### Example Document

```json
{
  "_id": "0HZXEQ5Y8JY5Z",
  "type": "USER",
  "scope": "CLIENT",
  "clientId": "0HZXEQ5Y8JY00",
  "name": "John Doe",
  "active": true,
  "userIdentity": {
    "email": "john@example.com",
    "emailDomain": "example.com",
    "idpType": "INTERNAL",
    "passwordHash": "$argon2id$...",
    "lastLoginAt": "2024-01-15T10:30:00Z"
  },
  "roles": [
    {
      "roleName": "client-admin",
      "assignmentSource": "MANUAL",
      "assignedAt": "2024-01-01T00:00:00Z"
    }
  ],
  "createdAt": "2024-01-01T00:00:00Z",
  "updatedAt": "2024-01-15T10:30:00Z"
}
```

---

## UserIdentity (Embedded)

Embedded in Principal for USER type principals.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `email` | String | User's email address |
| `emailDomain` | String | Extracted domain for matching |
| `idpType` | IdpType | `INTERNAL` or `OIDC` |
| `externalIdpId` | String | External IDP user identifier |
| `passwordHash` | String | Argon2id hash (for INTERNAL auth) |
| `lastLoginAt` | Instant | Last login timestamp |

### IdpType Enum

```java
public enum IdpType {
    INTERNAL,  // Username/password authentication
    OIDC       // External OIDC provider
}
```

---

## RoleAssignment (Embedded)

Tracks role assignments for principals.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `roleName` | String | Name of assigned role |
| `assignmentSource` | String | How role was assigned (MANUAL, IDP, SDK) |
| `assignedAt` | Instant | When role was assigned |

---

## AuthRole

**Collection**: `auth_roles`

Role definitions with associated permissions.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | String (TSID) | Primary key |
| `applicationId` | String | Owning application (optional) |
| `applicationCode` | String | Application code (denormalized) |
| `name` | String | Role name (unique within application) |
| `displayName` | String | Human-readable name |
| `description` | String | Role description |
| `permissions` | Set\<String\> | Permission strings |
| `source` | RoleSource | `CODE`, `DATABASE`, or `SDK` |
| `clientManaged` | boolean | Can be modified by clients |
| `createdAt` | Instant | Creation timestamp |
| `updatedAt` | Instant | Last update timestamp |

### RoleSource Enum

```java
public enum RoleSource {
    CODE,      // Defined in application code
    DATABASE,  // Created via API/UI
    SDK        // Synced from SDK definitions
}
```

### Permission String Format

Permissions follow a hierarchical format:
```
{subdomain}:{context}:{aggregate}:{action}
```

Examples:
- `platform:admin:clients:read`
- `platform:admin:clients:write`
- `orders:client:orders:create`
- `orders:client:orders:view`

### Example Document

```json
{
  "_id": "0HZXEQ5Y8JY5Z",
  "applicationId": "0HZXEQ5Y8JY00",
  "applicationCode": "ecommerce",
  "name": "order-manager",
  "displayName": "Order Manager",
  "description": "Can manage orders and view customers",
  "permissions": [
    "orders:client:orders:create",
    "orders:client:orders:view",
    "orders:client:orders:update",
    "customers:client:customers:view"
  ],
  "source": "SDK",
  "clientManaged": false,
  "createdAt": "2024-01-01T00:00:00Z",
  "updatedAt": "2024-01-01T00:00:00Z"
}
```

---

## AuthPermission

**Collection**: `auth_permissions`

Permission definitions (registry of valid permissions).

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | String (TSID) | Primary key |
| `applicationId` | String | Owning application |
| `name` | String | Permission string |
| `displayName` | String | Human-readable name |
| `description` | String | Permission description |
| `source` | PermissionSource | `SDK` or `DATABASE` |
| `createdAt` | Instant | Creation timestamp |

### PermissionSource Enum

```java
public enum PermissionSource {
    SDK,       // Registered via SDK
    DATABASE   // Created via API/UI
}
```

---

## IdpRoleMapping

**Collection**: `idp_role_mappings`

Maps external IDP roles to internal roles (security boundary).

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | String (TSID) | Primary key |
| `idpRoleName` | String | External role name from IDP |
| `internalRoleName` | String | Internal FlowCatalyst role name |
| `createdAt` | Instant | Creation timestamp |

### Security Purpose

This explicit allowlist ensures:
- Only mapped IDP roles grant internal permissions
- Prevents privilege escalation via IDP manipulation
- Audit trail of role mappings

### Example Document

```json
{
  "_id": "0HZXEQ5Y8JY5Z",
  "idpRoleName": "Azure-Admin-Group",
  "internalRoleName": "client-admin",
  "createdAt": "2024-01-01T00:00:00Z"
}
```

---

## AuditLog

**Collection**: `audit_logs`

Audit trail for sensitive operations.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | String (TSID) | Primary key |
| `entityType` | String | Type of affected entity |
| `entityId` | String | ID of affected entity |
| `operation` | String | Operation performed |
| `operationJson` | String | Operation details as JSON |
| `principalId` | String | Who performed the operation |
| `performedAt` | Instant | When operation occurred |

### Example Document

```json
{
  "_id": "0HZXEQ5Y8JY5Z",
  "entityType": "Principal",
  "entityId": "0HZXEQ5Y8JY00",
  "operation": "CREATE_USER",
  "operationJson": "{\"email\": \"john@example.com\", \"scope\": \"CLIENT\"}",
  "principalId": "0HZXEQ5Y8JY11",
  "performedAt": "2024-01-15T10:30:00Z"
}
```

---

## Common Queries

### Find principal by email

```java
principalRepository.find("userIdentity.email", email).firstResult();
```

### Find principals with role

```java
principalRepository.find("roles.roleName", roleName).list();
```

### Find active principals for client

```java
principalRepository.find("clientId = ?1 and active = true", clientId).list();
```

### Find roles for application

```java
authRoleRepository.find("applicationId", applicationId).list();
```

### Find IDP role mapping

```java
idpRoleMappingRepository.find("idpRoleName", externalRoleName).firstResult();
```

## See Also

- [OAuth Entities](oauth-entities.md) - OAuth/OIDC entities
- [Client Entities](client-entities.md) - Multi-tenancy entities
- [Authentication Guide](../guides/authentication.md) - Auth system guide
- [Authorization Guide](../guides/authorization.md) - RBAC guide
