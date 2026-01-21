# FlowCatalyst Platform - Architecture Documentation

This document provides complete architecture documentation for the FlowCatalyst Platform module, enabling reimplementation in any language without reading the source code.

## Table of Contents

1. [Overview](#overview)
2. [Core Domains](#core-domains)
3. [Multi-Tenant Architecture](#multi-tenant-architecture)
4. [Authorization Model (RBAC)](#authorization-model-rbac)
5. [Authentication & Identity Providers](#authentication--identity-providers)
6. [Event System](#event-system)
7. [Dispatch Job System](#dispatch-job-system)
8. [Use Cases & Commands Pattern](#use-cases--commands-pattern)
9. [Secret Management](#secret-management)
10. [Audit Logging](#audit-logging)
11. [BFF Layer](#bff-layer)
12. [Data Structures](#data-structures)
13. [Configuration](#configuration)

---

## Overview

The FlowCatalyst Platform module is a comprehensive Identity Access Management (IAM), Authorization, and Event Distribution system. It provides:

- **Multi-tenant architecture** with three user access scopes (ANCHOR, PARTNER, CLIENT)
- **RBAC (Role-Based Access Control)** with code-first permission definitions
- **OAuth2/OIDC authentication** with support for external IDPs and internal password-based auth
- **Dispatch job system** for webhook delivery and async task execution
- **Audit logging** for compliance and tracing
- **Secret management** with multiple provider backends (AWS, GCP, HashiCorp Vault, encrypted)
- **Service account lifecycle management** with webhook credentials

### Technology Stack

- **Framework**: Quarkus 3.x
- **Database**: MongoDB with Panache
- **Authentication**: SmallRye JWT, OIDC
- **Secrets**: AWS Secrets Manager, GCP Secret Manager, HashiCorp Vault, local encryption
- **REST**: Quarkus REST (RESTEasy Reactive)

---

## Core Domains

### Principal (Auth Identity)

**Entity**: `Principal` (MongoDB collection: `auth_principals`)

Unified identity model for both users and service accounts:

| Field | Type | Description |
|-------|------|-------------|
| `id` | String | TSID Crockford Base32 |
| `type` | PrincipalType | `USER` or `SERVICE` |
| `scope` | UserScope | `ANCHOR`, `PARTNER`, or `CLIENT` |
| `clientId` | String | Home client for CLIENT/PARTNER scope |
| `applicationId` | String | For SERVICE type - the owning application |
| `name` | String | Display name |
| `active` | Boolean | Whether principal is active |
| `createdAt` | Instant | Creation timestamp |
| `updatedAt` | Instant | Last update timestamp |
| `userIdentity` | UserIdentity | Embedded for USER type |
| `serviceAccount` | ServiceAccount | Embedded for SERVICE type |
| `roles` | List\<RoleAssignment\> | Denormalized role assignments |

**Key Patterns**:
- **Denormalized roles**: `roles = [RoleAssignment(roleName, assignmentSource, assignedAt)]`
- TSID-based IDs for sortability and JavaScript safety
- Embedded user/service account details for single document lookup

### UserIdentity (Embedded in Principal)

| Field | Type | Description |
|-------|------|-------------|
| `email` | String | User's email address |
| `emailDomain` | String | e.g., "company.com" |
| `idpType` | IdpType | `INTERNAL` or `OIDC` |
| `externalIdpId` | String | Subject from OIDC token |
| `passwordHash` | String | Argon2id hash (INTERNAL auth only) |
| `lastLoginAt` | Instant | Last login timestamp |

### ServiceAccount (Embedded in Principal)

| Field | Type | Description |
|-------|------|-------------|
| `code` | String | Unique identifier, e.g., "tms-service" |
| `description` | String | Description of the service account |
| `lastUsedAt` | Instant | Last usage timestamp |

### Standalone ServiceAccount Entity

**Entity**: `ServiceAccount` (MongoDB collection: `service_accounts`)

For webhook credentials:

| Field | Type | Description |
|-------|------|-------------|
| `id` | String | TSID |
| `code` | String | Unique code |
| `name` | String | Display name |
| `description` | String | Description |
| `clientIds` | List\<String\> | Clients this account can operate in |
| `applicationId` | String | If created for an application |
| `active` | Boolean | Active status |
| `webhookCredentials` | WebhookCredentials | Auth token + signing secret |
| `roles` | List\<RoleAssignment\> | Denormalized roles |
| `lastUsedAt` | Instant | Last usage |
| `createdAt` | Instant | Creation time |
| `updatedAt` | Instant | Last update |

### Client Entity

**Entity**: `Client` (MongoDB collection: `auth_clients`)

Tenant/organization representation:

| Field | Type | Description |
|-------|------|-------------|
| `id` | String | TSID |
| `name` | String | Client name |
| `identifier` | String | Unique slug |
| `status` | ClientStatus | `ACTIVE`, `SUSPENDED`, etc. |
| `statusReason` | String | e.g., "ACCOUNT_NOT_PAID" |
| `statusChangedAt` | Instant | When status changed |
| `notes` | List\<ClientNote\> | Audit trail |
| `createdAt` | Instant | Creation time |
| `updatedAt` | Instant | Last update |

### Application Entity

**Entity**: `Application` (MongoDB collection: `auth_applications`)

| Field | Type | Description |
|-------|------|-------------|
| `id` | String | TSID |
| `type` | ApplicationType | `APPLICATION` or `INTEGRATION` |
| `code` | String | Unique prefix for roles/permissions |
| `name` | String | Display name |
| `description` | String | Description |
| `iconUrl` | String | Icon URL |
| `defaultBaseUrl` | String | Default base URL |
| `serviceAccountId` | String | References standalone ServiceAccount |
| `active` | Boolean | Active status |
| `createdAt` | Instant | Creation time |
| `updatedAt` | Instant | Last update |

**Application Types**:
- **APPLICATION**: User-facing (TMS, WMS) - users log in
- **INTEGRATION**: Third-party adapters (Salesforce, SAP) - for event/subscription scoping

### AnchorDomain Entity

**Entity**: `AnchorDomain` (MongoDB collection: `anchor_domains`)

Email domains with god-mode access:

| Field | Type | Description |
|-------|------|-------------|
| `id` | String | TSID |
| `domain` | String | e.g., "flowcatalyst.tech" |
| `createdAt` | Instant | Creation time |

Users from anchor domains automatically get ANCHOR scope.

---

## Multi-Tenant Architecture

### UserScope Enum

Controls which clients a user can access:

| Scope | Description | Client Access |
|-------|-------------|---------------|
| `ANCHOR` | Platform admin | Access ALL clients (`["*"]` in token) |
| `PARTNER` | Partner user | Access explicitly granted clients |
| `CLIENT` | Client-bound user | Access home client + optional additional |

### Scope Derivation

1. Check if email domain is in `AnchorDomain` collection → `ANCHOR`
2. Check `ClientAuthConfig` for email domain:
   - `configType=ANCHOR` → `ANCHOR`
   - `configType=PARTNER` → `PARTNER`
   - `configType=CLIENT` → `CLIENT`
3. Explicit override on `Principal.scope`

### ClientAuthConfig Entity

**Entity**: `ClientAuthConfig` (MongoDB collection: `client_auth_config`)

| Field | Type | Description |
|-------|------|-------------|
| `id` | String | TSID |
| `emailDomain` | String | e.g., "acmecorp.com" |
| `configType` | AuthConfigType | `ANCHOR`, `PARTNER`, or `CLIENT` |
| `primaryClientId` | String | Required for CLIENT type |
| `additionalClientIds` | List\<String\> | Exceptions for CLIENT users |
| `grantedClientIds` | List\<String\> | For PARTNER type |
| `authProvider` | AuthProvider | `INTERNAL` or `OIDC` |
| `oidcIssuerUrl` | String | OIDC issuer |
| `oidcClientId` | String | OIDC client ID |
| `oidcMultiTenant` | Boolean | Multi-tenant OIDC |
| `oidcIssuerPattern` | String | For multi-tenant validation |
| `oidcClientSecretRef` | String | Encrypted secret reference |

### ClientAccessGrant Entity

**Entity**: `ClientAccessGrant` (MongoDB collection: `client_access_grants`)

Grants PARTNER users access to specific clients:

| Field | Type | Description |
|-------|------|-------------|
| `id` | String | TSID |
| `principalId` | String | Principal receiving access |
| `clientId` | String | Client being granted |
| `grantedAt` | Instant | When granted |
| `expiresAt` | Instant | Optional expiration |

---

## Authorization Model (RBAC)

### Permission Definition

**Format**: `{application}:{context}:{aggregate}:{action}`

**Examples**:
- `platform:iam:user:create`
- `tms:orders:order:view`
- `wms:inventory:stock:update`

```
interface PermissionDefinition {
    application: String   // "platform", "tms", etc.
    context: String       // "iam", "admin", "messaging", etc.
    aggregate: String     // "user", "role", "event-type", etc.
    action: String        // "view", "create", "update", "delete"
    description: String

    toPermissionString() -> "{application}:{context}:{aggregate}:{action}"
}
```

### Role Definition

**Format**: `{application}:{role-name}`

**Examples**:
- `platform:iam-admin`
- `tms:dispatcher`

```
interface RoleDefinition {
    application: String
    roleName: String
    permissions: Set<PermissionRecord>
    description: String

    toRoleString() -> "{application}:{roleName}"
}
```

### AuthRole Entity

**Entity**: `AuthRole` (MongoDB collection: `auth_roles`)

| Field | Type | Description |
|-------|------|-------------|
| `id` | String | TSID |
| `applicationId` | String | Owning application |
| `applicationCode` | String | Denormalized for queries |
| `name` | String | Full name, e.g., "platform:tenant-admin" |
| `displayName` | String | Human-readable name |
| `description` | String | Description |
| `permissions` | Set\<String\> | Permission strings |
| `source` | RoleSource | `CODE`, `DATABASE`, or `SDK` |
| `clientManaged` | Boolean | Sync to IDPs |
| `createdAt` | Instant | Creation time |
| `updatedAt` | Instant | Last update |

**RoleSource**:
- `CODE`: From annotated Java classes, synced at startup
- `DATABASE`: Created by admins via UI
- `SDK`: Registered by external applications

### AuthPermission Entity

**Entity**: `AuthPermission` (MongoDB collection: `auth_permissions`)

| Field | Type | Description |
|-------|------|-------------|
| `id` | String | TSID |
| `application` | String | Application code |
| `context` | String | Context code |
| `aggregate` | String | Aggregate code |
| `action` | String | Action code |
| `permissionString` | String | Derived: app:context:aggregate:action |
| `source` | PermissionSource | `CODE` or `SDK` |
| `description` | String | Description |
| `createdAt` | Instant | Creation time |

### Permission Registry

In-memory registry scanned at startup:
- All permissions from annotated classes
- All roles from annotated classes
- Validates role permissions reference valid permission definitions
- Provides fast lookup by string representation

### Authorization Service

```
hasPermission(principalId, permissionString): boolean
hasPermission(principal, permissionString): boolean
requirePermission(principalId, permissionString): void  // throws ForbiddenException
```

**Important**: This service ONLY validates RBAC. Tenant isolation and business rules must be enforced in application logic.

---

## Authentication & Identity Providers

### AuthMode Enum

| Mode | Description |
|------|-------------|
| `EMBEDDED` | Full IdP: token issuance, OAuth2/OIDC endpoints |
| `REMOTE` | Token validation only: delegates to external IdP |

### AuthProvider Enum

| Provider | Description |
|----------|-------------|
| `INTERNAL` | Username/password stored in FlowCatalyst |
| `OIDC` | External IDP (Keycloak, Okta, Entra, etc.) |

### IdpType Enum

| Type | Description |
|------|-------------|
| `INTERNAL` | Password-based |
| `OIDC` | External IDP (subject stored as externalIdpId) |

### OAuthClient Entity

**Entity**: `OAuthClient` (MongoDB collection: `oauth_clients`)

| Field | Type | Description |
|-------|------|-------------|
| `id` | String | TSID |
| `clientId` | String | Unique, e.g., "fc_0HZXEQ5Y8JY5Z" |
| `clientName` | String | Display name |
| `clientType` | ClientType | `PUBLIC` (SPAs) or `CONFIDENTIAL` (servers) |
| `clientSecretRef` | String | Encrypted secret for CONFIDENTIAL |
| `redirectUris` | List\<String\> | Must match exactly in auth flow |
| `grantTypes` | List\<String\> | e.g., "authorization_code", "client_credentials" |
| `defaultScopes` | String | e.g., "openid profile email" |
| `pkceRequired` | Boolean | Always enforced for PUBLIC |
| `applicationIds` | List\<String\> | User-facing apps this client serves |
| `serviceAccountPrincipalId` | String | If service account client |
| `active` | Boolean | Active status |
| `createdAt` | Instant | Creation time |
| `updatedAt` | Instant | Last update |

### JWT Token Claims

| Claim | Description |
|-------|-------------|
| `sub` | Principal ID |
| `type` | PrincipalType (USER or SERVICE) |
| `groups` | Role names (for session tokens) |
| `clients` | Client IDs or `["*"]` for ANCHOR |
| `iat` | Issued at |
| `exp` | Expires at |
| `iss` | Issuer (usually "flowcatalyst") |

### IDP Role Sync

**Entity**: `IdpRoleMapping` (MongoDB collection: `idp_role_mappings`)

| Field | Type | Description |
|-------|------|-------------|
| `id` | String | TSID |
| `idpType` | IdpType | IDP type |
| `idpRoleName` | String | Role name from OIDC token |
| `platformRoleName` | String | e.g., "platform:iam-admin" |
| `createdAt` | Instant | Creation time |

---

## Event System

### Event Entity

**Entity**: `Event` (MongoDB collection: `events`)

The core event document stored in MongoDB, based on CloudEvents specification:

| Field | Type | Description |
|-------|------|-------------|
| `id` | String | TSID Crockford Base32 |
| `specVersion` | String | e.g., "1.0" |
| `type` | String | Event type code (e.g., "operant:execution:trip:started") |
| `source` | String | Origin system identifier |
| `subject` | String | Qualified aggregate ID (e.g., "operant.execution.trip.1234") |
| `time` | Instant | When the event occurred |
| `data` | String | Event payload (JSON string) |
| `correlationId` | String | Distributed tracing |
| `causationId` | String | Event that caused this |
| `deduplicationId` | String | Idempotency key |
| `messageGroup` | String | For ordering guarantees |
| `contextData` | List\<ContextData\> | Key-value pairs for search/filtering |
| `clientId` | String | Multi-tenant scoping (nullable) |

### EventType Entity

**Entity**: `EventType` (MongoDB collection: `event_types`)

| Field | Type | Description |
|-------|------|-------------|
| `id` | String | TSID |
| `code` | String | Globally unique code (e.g., "operant:execution:trip:started") |
| `name` | String | Human-friendly name |
| `description` | String | Description |
| `specVersions` | List\<SpecVersion\> | Schema versions |
| `status` | EventTypeStatus | `ACTIVE`, `ARCHIVED` |
| `createdAt` | Instant | Creation time |
| `updatedAt` | Instant | Last update |

**EventType Code Format**: `{APPLICATION}:{SUBDOMAIN}:{AGGREGATE}:{EVENT}`

### SpecVersion (Embedded in EventType)

| Field | Type | Description |
|-------|------|-------------|
| `version` | String | e.g., "1.0", "2.0" |
| `schemaType` | SchemaType | `JSON_SCHEMA` |
| `schema` | String | JSON Schema content |
| `status` | SpecVersionStatus | `FINALISING`, `FINALISED`, `DEPRECATED` |
| `createdAt` | Instant | Creation time |
| `finalisedAt` | Instant | When finalised |

### Subscription Entity

**Entity**: `Subscription` (MongoDB collection: `subscriptions`)

Defines how events are dispatched to a target endpoint:

| Field | Type | Description |
|-------|------|-------------|
| `id` | String | TSID |
| `code` | String | Unique within client scope |
| `name` | String | Display name |
| `description` | String | Description |
| `clientId` | String | Owning client (nullable = anchor-level) |
| `clientIdentifier` | String | Denormalized for queries |
| `eventTypes` | List\<EventTypeBinding\> | Event types to listen to |
| `target` | String | Webhook URL |
| `queue` | String | Queue name for message routing |
| `customConfig` | List\<ConfigEntry\> | Custom key-value config |
| `source` | SubscriptionSource | `API` or `UI` |
| `status` | SubscriptionStatus | `ACTIVE`, `PAUSED`, `ARCHIVED` |
| `maxAgeSeconds` | int | Max job age (default 86400 = 24h) |
| `dispatchPoolId` | String | Rate limiting pool |
| `dispatchPoolCode` | String | Denormalized |
| `delaySeconds` | int | Delay before dispatch (default 0) |
| `sequence` | int | Ordering within message group (default 99) |
| `mode` | DispatchMode | `IMMEDIATE`, `NEXT_ON_ERROR`, `BLOCK_ON_ERROR` |
| `timeoutSeconds` | int | Target response timeout (default 30) |
| `maxRetries` | int | Max retry attempts (default 3) |
| `serviceAccountId` | String | Webhook credentials |
| `dataOnly` | boolean | true = raw payload, false = envelope |
| `createdAt` | Instant | Creation time |
| `updatedAt` | Instant | Last update |

### DispatchPool Entity

**Entity**: `DispatchPool` (MongoDB collection: `dispatch_pools`)

Controls rate limiting and concurrency for dispatch jobs:

| Field | Type | Description |
|-------|------|-------------|
| `id` | String | TSID |
| `code` | String | Unique within client scope |
| `name` | String | Display name |
| `description` | String | Description |
| `rateLimit` | int | Maximum dispatches per minute |
| `concurrency` | int | Maximum concurrent dispatches (≥1) |
| `clientId` | String | Owning client (nullable = anchor-level) |
| `clientIdentifier` | String | Denormalized |
| `status` | DispatchPoolStatus | `ACTIVE`, `ARCHIVED` |
| `createdAt` | Instant | Creation time |
| `updatedAt` | Instant | Last update |

### DomainEvent Interface

Base interface for all domain events (immutable records):

| Field | Type | Description |
|-------|------|-------------|
| `eventId` | String | TSID |
| `eventType` | String | {app}:{domain}:{aggregate}:{action} |
| `specVersion` | String | e.g., "1.0" |
| `source` | String | e.g., "platform:iam" |
| `subject` | String | {domain}.{aggregate}.{id} |
| `time` | Instant | Event timestamp |
| `executionId` | String | Single use case execution |
| `correlationId` | String | Distributed tracing |
| `causationId` | String | Causal chain |
| `principalId` | String | Who triggered it |
| `messageGroup` | String | Ordering group |

### Domain Events

**Principal Events**:
- `UserCreated`: New user created
- `UserUpdated`: User info changed
- `UserActivated`/`UserDeactivated`: Status change
- `UserDeleted`: User removed
- `RolesAssigned`: Roles assigned to user
- `ClientAccessGranted`/`ClientAccessRevoked`: Partner user access changed

**Application Events**:
- `ApplicationCreated`/`Updated`/`Deleted`: Application lifecycle
- `ApplicationActivated`/`Deactivated`: Status change
- `ServiceAccountProvisioned`: Auto-created service account

**Role Events**:
- `RoleCreated`/`Updated`/`Deleted`: Role lifecycle
- `RolesSynced`: Roles synced from IDP

---

## Dispatch Job System

### DispatchJob Entity

**Entity**: `DispatchJob` (MongoDB collection: `dispatch_jobs`)

| Field | Type | Description |
|-------|------|-------------|
| `id` | String | TSID |
| `externalId` | String | External reference |
| `source` | String | Origin system |
| `kind` | DispatchKind | `EVENT` or `TASK` |
| `code` | String | Event type or task code |
| `subject` | String | Aggregate reference |
| `eventId` | String | Triggering event |
| `correlationId` | String | Distributed tracing |
| `metadata` | List\<DispatchJobMetadata\> | Key-value pairs |
| `targetUrl` | String | Webhook URL |
| `protocol` | DispatchProtocol | `HTTP_WEBHOOK` |
| `headers` | Map\<String, String\> | Custom headers |
| `payload` | String | JSON payload |
| `payloadContentType` | String | Content type |
| `dataOnly` | Boolean | true = raw payload, false = with envelope |
| `serviceAccountId` | String | Webhook credentials reference |
| `clientId` | String | Owning client |
| `subscriptionId` | String | Triggering subscription |
| `mode` | DispatchMode | `IMMEDIATE`, `NEXT_ON_ERROR`, `BLOCK_ON_ERROR` |
| `dispatchPoolId` | String | Rate limiting pool |
| `messageGroup` | String | FIFO ordering |
| `sequence` | Integer | Ordering within group |
| `timeoutSeconds` | Integer | Default 30s |
| `status` | DispatchStatus | Status |
| `attempts` | List\<DispatchAttempt\> | Delivery history |

### DispatchStatus Lifecycle

```
PENDING → QUEUED → IN_PROGRESS → COMPLETED
                 ↓             ↓
              (retry)      ERROR
                 ↓
               QUEUED

Any state → CANCELLED (manual intervention)
```

### Webhook Credentials

**WebhookCredentials** (embedded in ServiceAccount):

| Field | Type | Description |
|-------|------|-------------|
| `authType` | WebhookAuthType | `BEARER` or `BASIC` |
| `authTokenRef` | String | Encrypted secret reference |
| `signingSecretRef` | String | Encrypted HMAC-SHA256 secret |
| `signingAlgorithm` | SignatureAlgorithm | `HMAC_SHA256` |
| `createdAt` | Instant | Creation time |
| `regeneratedAt` | Instant | Last regeneration |

### Webhook Headers

| Header | Description |
|--------|-------------|
| `Authorization` | `Bearer {authToken}` or `Basic {base64}` |
| `X-FlowCatalyst-ID` | Dispatch job ID |
| `X-FlowCatalyst-SIGNATURE` | HMAC-SHA256({timestamp + body}) |
| `X-FlowCatalyst-TIMESTAMP` | Unix timestamp |

### Payload Modes

**dataOnly = true (default)**:
```http
POST /webhook
Authorization: Bearer token
X-FlowCatalyst-ID: 0HZXEQ5Y8JY5Z
X-FlowCatalyst-SIGNATURE: ...
X-FlowCatalyst-TIMESTAMP: 1704067800

{original payload JSON}
```

**dataOnly = false**:
```http
POST /webhook
...same headers...

{
  "id": "0HZXEQ5Y8JY5Z",
  "kind": "EVENT",
  "code": "order.created",
  "subject": "order:12345",
  "eventId": "0HZXEQ5Y8JY00",
  "timestamp": "2024-01-15T10:30:00Z",
  "data": {original payload}
}
```

---

## Use Cases & Commands Pattern

### Command/UseCase Structure

```
record MyCommand(
    field1: String,
    field2: String,
    ...
)

class MyUseCase {
    execute(command: MyCommand, context: ExecutionContext): Result<MyEvent>
}
```

### Result Type

```
Result<T> {
    // Success:
    Result.success(T value)

    // Failure with error:
    Result.failure(UseCaseError error)

    // UseCaseError subtypes:
    - ValidationError(code, message, details)
    - BusinessRuleViolation(code, message, details)
    - NotFoundError(code, message)
    - ConflictError(code, message)
}
```

### ExecutionContext

| Field | Type | Description |
|-------|------|-------------|
| `executionId` | String | Unique per use case execution |
| `correlationId` | String | Distributed tracing |
| `causationId` | String | Causal chain |
| `principalId` | String | Who triggered this |

### Key Use Cases

**Principal Operations**:
- `CreateUserUseCase`: Create user (INTERNAL or OIDC)
- `UpdateUserUseCase`: Update user info
- `AssignRolesUseCase`: Assign roles to principal
- `ActivateUserUseCase`/`DeactivateUserUseCase`: Status changes
- `DeleteUserUseCase`: Delete user
- `GrantClientAccessUseCase`: Grant PARTNER access to client
- `RevokeClientAccessUseCase`: Revoke access

**Application Operations**:
- `CreateApplicationUseCase`: Register app/integration
- `UpdateApplicationUseCase`: Modify app
- `ActivateApplicationUseCase`/`DeactivateApplicationUseCase`
- `DeleteApplicationUseCase`: Remove app
- `ProvisionServiceAccountUseCase`: Create app's service account

**Authorization Operations**:
- `CreateRoleUseCase`: Create new role
- `UpdateRoleUseCase`: Modify role
- `DeleteRoleUseCase`: Remove role
- `SyncRolesUseCase`: Sync roles from IDP

---

## Secret Management

### SecretService

Resolves secrets from multiple backends:

| Format | Backend |
|--------|---------|
| `encrypted:BASE64_CIPHERTEXT` | Locally encrypted |
| `aws-sm://secret-name` | AWS Secrets Manager |
| `aws-ps://parameter-name` | AWS Parameter Store |
| `gcp-sm://projects/PROJECT/secrets/NAME/versions/VERSION` | GCP Secret Manager |
| `vault://path/to/secret#key` | HashiCorp Vault |

### Usage

```
secretService.resolve(reference): String           // Get plaintext
secretService.resolveOptional(reference): Optional<String>
secretService.validate(reference): ValidationResult // Check if resolvable
```

### Key Uses

- OIDC client secrets (`ClientAuthConfig.oidcClientSecretRef`)
- Webhook auth tokens (`WebhookCredentials.authTokenRef`)
- Webhook signing secrets (`WebhookCredentials.signingSecretRef`)
- OAuth client secrets (`OAuthClient.clientSecretRef`)

---

## Audit Logging

### AuditLog Entity

**Entity**: `AuditLog` (MongoDB collection: `audit_logs`)

| Field | Type | Description |
|-------|------|-------------|
| `id` | String | TSID |
| `entityType` | String | e.g., "EventType", "User" |
| `entityId` | String | The entity's ID |
| `operation` | String | e.g., "CreateEventType", "AddSchema" |
| `operationJson` | String | Full operation as JSON |
| `principalId` | String | Who performed it |
| `performedAt` | Instant | When performed |

### AuditContext

Request-scoped service tracking current principal:

```
auditContext.setPrincipalId(principalId)    // Manual
auditContext.setSystemPrincipal()           // System jobs
auditContext.requirePrincipalId()           // Require set
```

**SYSTEM Principal**: Used for background jobs, CLI tools, startup tasks.

---

## BFF Layer

RESTful endpoints optimized for web clients, returning string IDs for JavaScript safety.

### Resources

| Resource | Path | Purpose |
|----------|------|---------|
| `RoleBffResource` | `/bff/roles` | List, get, filter roles |
| `EventBffResource` | `/bff/events` | Event queries |
| `RawEventBffResource` | `/bff/raw-events` | Raw event access |
| `DispatchJobBffResource` | `/bff/dispatch-jobs` | Job queries |
| `RawDispatchJobBffResource` | `/bff/raw-dispatch-jobs` | Raw job access |
| `EventTypeBffResource` | `/bff/event-types` | Event type management |

---

## Data Structures

### RoleAssignment

```
RoleAssignment {
    roleName: String           // e.g., "platform:iam-admin"
    assignmentSource: String   // "MANUAL", "IDP_SYNC", etc.
    assignedAt: Instant
}
```

### DispatchAttempt

```
DispatchAttempt {
    attemptNumber: Integer
    attemptedAt: Instant
    statusCode: Integer
    responseBody: String
    errorMessage: String
    durationMs: Long
}
```

### DispatchJobMetadata

```
DispatchJobMetadata {
    key: String
    value: String
}
```

---

## Configuration

### Key Properties

```properties
# Auth Mode
flowcatalyst.auth.mode=embedded|remote

# JWT Configuration
flowcatalyst.auth.jwt.issuer=flowcatalyst
flowcatalyst.auth.jwt.private-key-path=/keys/private.pem
flowcatalyst.auth.jwt.public-key-path=/keys/public.pem
flowcatalyst.auth.jwt.access-token-expiry=PT1H
flowcatalyst.auth.jwt.refresh-token-expiry=P30D

# External Base URL (for OAuth callbacks)
flowcatalyst.auth.external-base-url=http://localhost:4200

# MongoDB
quarkus.mongodb.connection-string=mongodb://...

# Secret Management
flowcatalyst.secrets.default-provider=encrypted|aws-sm|gcp-sm|vault
```

---

## Key Architectural Patterns

### 1. Denormalization for Query Performance

- **Principal.roles**: Embedded list instead of separate collection
- **AuthRole.applicationCode**: Denormalized from ApplicationId
- Enables fast single-document reads

### 2. TSID for Entity IDs

- 13-character Crockford Base32 strings
- Sortable by timestamp (newer IDs > older IDs)
- JavaScript-safe (no precision loss)
- URL-safe and case-insensitive

### 3. Event Sourcing with Domain Events

- All state changes produce events
- Events are immutable records
- Full audit trail and causation chain
- Enables rebuilding state or deriving read models

### 4. Use Case Command Pattern

- Explicit commands encapsulate intent
- Result type for success/failure handling
- Atomic commit of entity + event
- ExecutionContext for tracing across microservices

### 5. Code-First Authorization

- Permissions/roles defined in annotated classes
- Registry scanned at startup
- Synced to database for querying
- Keeps source of truth in code

### 6. Reference-Based Secrets

- Secrets never stored plaintext in database
- Reference strings only (encrypted:xxx, aws-sm://xxx)
- SecretService resolves at runtime
- Supports multiple backends

---

## Implementation Checklist

To reimplement this module:

1. **Data Models**: All entities with proper relationships
2. **Repositories**: CRUD + custom queries for all entities
3. **Services**: Authentication, Authorization, User, Token, Secret, Role
4. **Use Cases**: Command/Result pattern for all operations
5. **Events**: DomainEvent interface with CloudEvents-like schema
6. **Authentication**: OAuth2 flows, OIDC, JWT
7. **Audit**: Request-scoped context, AuditLog entity
8. **Multi-Tenancy**: Client isolation, UserScope-based access
9. **Authorization (RBAC)**: Code-first permissions, PermissionRegistry
10. **Secret Management**: Multi-backend secret resolution
11. **Dispatch System**: DispatchJob with webhook signing
12. **REST API**: BFF layer with string IDs
