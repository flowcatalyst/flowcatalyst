# Entity Overview

FlowCatalyst uses MongoDB as its primary data store. This document provides an overview of all entity collections, their relationships, and key design patterns.

## Entity Relationship Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              AUTHENTICATION                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌──────────────┐     ┌─────────────────┐     ┌─────────────────────────┐  │
│  │   Principal  │────►│   AuthRole      │     │    OAuthClient          │  │
│  │  (users &    │     │  (permissions)  │     │   (OAuth apps)          │  │
│  │  services)   │     └─────────────────┘     └──────────┬──────────────┘  │
│  └──────┬───────┘                                        │                  │
│         │                                                │                  │
│         ▼                                                ▼                  │
│  ┌──────────────┐     ┌─────────────────┐     ┌─────────────────────────┐  │
│  │ClientAccess  │     │ ClientAuthConfig│     │  AuthorizationCode      │  │
│  │   Grant      │     │ (per-domain)    │     │  RefreshToken           │  │
│  └──────────────┘     └─────────────────┘     │  OidcLoginState         │  │
│                                               └─────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│                              MULTI-TENANCY                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌──────────────┐     ┌─────────────────┐     ┌─────────────────────────┐  │
│  │    Client    │◄────│  Application    │     │   ApplicationClient     │  │
│  │   (tenant)   │     │ (integration)   │     │      Config             │  │
│  └──────┬───────┘     └────────┬────────┘     └─────────────────────────┘  │
│         │                      │                                            │
│         ▼                      ▼                                            │
│  ┌──────────────┐     ┌─────────────────┐                                  │
│  │ AnchorDomain │     │ ServiceAccount  │                                  │
│  │              │     │ (webhooks)      │                                  │
│  └──────────────┘     └─────────────────┘                                  │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│                              EVENT SYSTEM                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌──────────────┐     ┌─────────────────┐     ┌─────────────────────────┐  │
│  │    Event     │────►│  Subscription   │────►│    DispatchJob          │  │
│  │              │     │                 │     │                         │  │
│  └──────────────┘     └────────┬────────┘     └──────────┬──────────────┘  │
│         │                      │                         │                  │
│         ▼                      ▼                         ▼                  │
│  ┌──────────────┐     ┌─────────────────┐     ┌─────────────────────────┐  │
│  │  EventType   │     │  DispatchPool   │     │   DispatchAttempt       │  │
│  │   Schema     │     │  (rate limits)  │     │   (embedded)            │  │
│  └──────────────┘     └─────────────────┘     └─────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Collection Summary

| Collection | Entity | Description |
|------------|--------|-------------|
| `auth_principals` | Principal | Users and service accounts |
| `auth_roles` | AuthRole | Role definitions with permissions |
| `auth_permissions` | AuthPermission | Permission definitions |
| `auth_clients` | Client | Tenant entities |
| `auth_applications` | Application | Applications and integrations |
| `anchor_domains` | AnchorDomain | Domains granting ANCHOR scope |
| `client_access_grants` | ClientAccessGrant | Partner access grants |
| `client_auth_config` | ClientAuthConfig | Per-domain auth settings |
| `application_client_config` | ApplicationClientConfig | Per-client app config |
| `service_accounts` | ServiceAccount | Machine credentials |
| `oauth_clients` | OAuthClient | OAuth client registrations |
| `authorization_codes` | AuthorizationCode | OAuth auth codes |
| `refresh_tokens` | RefreshToken | OAuth refresh tokens |
| `oidc_login_state` | OidcLoginState | OIDC flow state |
| `idp_role_mappings` | IdpRoleMapping | External role mapping |
| `events` | Event | CloudEvents records |
| `event_types` | EventType | Event type definitions |
| `schemas` | Schema | JSON/Proto/XSD schemas |
| `subscriptions` | Subscription | Webhook subscriptions |
| `dispatch_pools` | DispatchPool | Rate limiting pools |
| `dispatch_jobs` | DispatchJob | Webhook delivery jobs |
| `audit_logs` | AuditLog | Audit trail |

## ID Format (TSID)

All entity IDs use TSID (Time-Sorted ID) format:

- **Format**: 13-character Crockford Base32 string
- **Example**: `0HZXEQ5Y8JY5Z`
- **Properties**:
  - Lexicographically sortable (newer IDs sort after older)
  - URL-safe and case-insensitive
  - Safe from JavaScript number precision issues

### Usage

```java
import tech.flowcatalyst.platform.shared.TsidGenerator;

// Generate a new TSID
String id = TsidGenerator.generate();  // e.g., "0HZXEQ5Y8JY5Z"

// Convert between formats (for migration)
Long longId = TsidGenerator.toLong("0HZXEQ5Y8JY5Z");
String strId = TsidGenerator.toString(786259737685263979L);
```

### Entity Pattern

```java
@MongoEntity(collection = "my_entities")
public class MyEntity extends PanacheMongoEntityBase {
    @BsonId
    public String id;  // TSID Crockford Base32

    public String relatedEntityId;  // Foreign key as String
}

// Repository
public class MyEntityRepository
    implements PanacheMongoRepositoryBase<MyEntity, String> {}
```

## Embedded Documents

Several entities use embedded documents for related data:

| Parent Entity | Embedded | Purpose |
|---------------|----------|---------|
| Principal | UserIdentity | User authentication details |
| Principal | RoleAssignment | Role assignments |
| EventType | SpecVersion | Schema versions |
| Subscription | EventTypeBinding | Event type links |
| Subscription | ConfigEntry | Custom configuration |
| DispatchJob | DispatchAttempt | Delivery attempts |
| DispatchJob | DispatchJobMetadata | Key-value metadata |
| ServiceAccount | WebhookCredentials | Auth/signing secrets |
| Client | ClientNote | Admin notes |
| Event | ContextData | Searchable key-values |

## Client Scoping

Many entities support optional client scoping:

```java
public String clientId;  // null = anchor-level, otherwise client-specific
```

**Anchor-level** (clientId = null):
- Visible to all users
- Shared across platform
- Typically for platform-wide resources

**Client-scoped** (clientId = "xxx"):
- Only visible to that client's users
- Isolated per tenant
- Most operational data

## Timestamp Fields

All entities include standard timestamps:

```java
public Instant createdAt = Instant.now();
public Instant updatedAt = Instant.now();
```

## Entity Categories

### Authentication & Authorization

- [Auth Entities](auth-entities.md) - Principal, AuthRole, AuthPermission
- [OAuth Entities](oauth-entities.md) - OAuthClient, tokens, OIDC state

### Multi-Tenancy

- [Client Entities](client-entities.md) - Client, AccessGrant, AnchorDomain, AuthConfig

### Events & Subscriptions

- [Event Entities](event-entities.md) - Event, EventType, Schema
- [Subscription Entities](subscription-entities.md) - Subscription, DispatchPool

### Dispatch

- [Dispatch Entities](dispatch-entities.md) - DispatchJob, ServiceAccount

## Query Patterns

### Find by ID

```java
MyEntity entity = repository.findById("0HZXEQ5Y8JY5Z");
```

### Find by client with anchor fallback

```java
// Client-specific OR anchor-level
repository.find("clientId = ?1 or clientId is null", clientId);
```

### Find active only

```java
repository.find("status", ActiveStatus.ACTIVE);
```

### Find with embedded document criteria

```java
// Find subscriptions for event type
repository.find("eventTypes.eventTypeCode", eventTypeCode);
```

## See Also

- [Auth Entities](auth-entities.md) - Authentication entities
- [Client Entities](client-entities.md) - Multi-tenancy entities
- [Event Entities](event-entities.md) - Event system entities
- [Subscription Entities](subscription-entities.md) - Subscription entities
- [Dispatch Entities](dispatch-entities.md) - Dispatch job entities
- [OAuth Entities](oauth-entities.md) - OAuth/OIDC entities
