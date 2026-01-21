# Multi-Tenancy Architecture

FlowCatalyst implements a flexible multi-tenant architecture with three access scopes. This document explains the tenant model, access control, and data isolation.

## Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                    FlowCatalyst Platform                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ANCHOR Scope (Platform Admins)                                │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  • Access to ALL clients                                 │   │
│  │  • Platform-wide operations                              │   │
│  │  • Anchor-level subscriptions & event types             │   │
│  └─────────────────────────────────────────────────────────┘   │
│                           │                                     │
│                           ▼                                     │
│  PARTNER Scope (Partners)                                      │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  • Access to ASSIGNED clients                           │   │
│  │  • Cross-client operations (within grants)              │   │
│  │  • Client management capabilities                        │   │
│  └─────────────────────────────────────────────────────────┘   │
│                           │                                     │
│                           ▼                                     │
│  CLIENT Scope (End Users)                                      │
│  ┌───────────┐  ┌───────────┐  ┌───────────┐                  │
│  │ Client A  │  │ Client B  │  │ Client C  │                  │
│  │ (tenant)  │  │ (tenant)  │  │ (tenant)  │                  │
│  └───────────┘  └───────────┘  └───────────┘                  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## User Scopes

### UserScope Enum

```java
public enum UserScope {
    /**
     * Anchor/platform users - have access to all clients.
     * Typically users from the anchor domain (e.g., flowcatalyst.local).
     */
    ANCHOR,

    /**
     * Partner users - have access to multiple explicitly assigned clients.
     * Their accessible clients are stored in client access grants.
     */
    PARTNER,

    /**
     * Client users - bound to a single client (their home client).
     * Their clientId determines their access scope.
     */
    CLIENT
}
```

### Scope Characteristics

| Scope | Client Access | Use Case |
|-------|---------------|----------|
| **ANCHOR** | All clients (`["*"]`) | Platform administrators, support staff |
| **PARTNER** | Assigned clients (explicit list) | Integration partners, resellers |
| **CLIENT** | Single client (home client) | End users, tenant administrators |

## Tenant Model

### Client Entity

Clients are the primary tenant unit:

```java
@MongoEntity(collection = "auth_clients")
public class Client {
    @BsonId
    public String id;              // TSID

    public String name;            // Display name
    public String identifier;      // URL-safe identifier

    public ClientStatus status;    // ACTIVE, INACTIVE, SUSPENDED
    public String statusReason;    // Reason for status change
    public Instant statusChangedAt;

    public List<ClientNote> notes; // Admin notes/history

    public Instant createdAt;
    public Instant updatedAt;
}
```

### Client Status

| Status | Description |
|--------|-------------|
| `ACTIVE` | Client is operational |
| `INACTIVE` | Client is disabled (soft delete) |
| `SUSPENDED` | Client is temporarily suspended |

### Client Notes

Track administrative history:

```java
public record ClientNote(
    String text,
    Instant timestamp,
    String addedBy,
    String category    // STATUS_CHANGE, ADMIN_NOTE, BILLING
) {}
```

## Access Control

### Principal Entity

Principals represent authenticated identities (users or services):

```java
@MongoEntity(collection = "auth_principals")
public class Principal {
    @BsonId
    public String id;

    public PrincipalType type;     // USER or SERVICE
    public UserScope scope;        // ANCHOR, PARTNER, CLIENT

    public String clientId;        // Home client (for CLIENT scope)
    public String applicationId;   // Optional application association

    public String name;
    public boolean active;

    // Embedded identity (for USER type)
    public UserIdentity userIdentity;

    // Role assignments
    public List<RoleAssignment> roles;

    public Instant createdAt;
    public Instant updatedAt;
}
```

### User Identity

For USER-type principals:

```java
public record UserIdentity(
    String email,
    String emailDomain,
    IdpType idpType,          // INTERNAL or OIDC
    String externalIdpId,     // External IDP user ID
    String passwordHash,      // For INTERNAL auth
    Instant lastLoginAt
) {}
```

### Client Access Grants

For PARTNER scope users, explicit access grants:

```java
@MongoEntity(collection = "client_access_grants")
public class ClientAccessGrant {
    @BsonId
    public String id;

    public String principalId;    // The partner user
    public String clientId;       // Granted access to this client

    public Instant grantedAt;
    public Instant expiresAt;     // Optional expiration
}
```

## Domain-Based Access

### Anchor Domains

Email domains that grant automatic ANCHOR scope:

```java
@MongoEntity(collection = "anchor_domains")
public class AnchorDomain {
    @BsonId
    public String id;

    public String domain;         // e.g., "flowcatalyst.local"
    public Instant createdAt;
}
```

When a user logs in with an email from an anchor domain, they automatically receive ANCHOR scope.

### Client Auth Config

Per-domain authentication configuration:

```java
@MongoEntity(collection = "client_auth_config")
public class ClientAuthConfig {
    @BsonId
    public String id;

    public String emailDomain;           // Domain to match
    public AuthConfigType configType;    // ANCHOR, PARTNER, CLIENT

    // Client bindings
    public String primaryClientId;       // Main client for this domain
    public List<String> additionalClientIds;  // Additional clients
    public List<String> grantedClientIds;     // For PARTNER scope

    // Authentication provider
    public AuthProvider authProvider;    // INTERNAL or OIDC
    public String oidcIssuerUrl;
    public String oidcClientId;
    public boolean oidcMultiTenant;
    public String oidcIssuerPattern;     // Regex for multi-tenant
    public String oidcClientSecretRef;   // Secret reference

    public Instant createdAt;
    public Instant updatedAt;
}
```

### Scope Resolution Flow

```
1. User logs in with email@domain.com
                │
                ▼
2. Check AnchorDomain for domain
   ├── Found → Scope = ANCHOR
   │
   └── Not Found
                │
                ▼
3. Check ClientAuthConfig for domain
   ├── configType = ANCHOR → Scope = ANCHOR
   │
   ├── configType = PARTNER → Scope = PARTNER
   │                          (with grantedClientIds)
   │
   └── configType = CLIENT → Scope = CLIENT
                              (with primaryClientId)
                │
                ▼
4. No config found → Default to INTERNAL auth
```

## Token Claims

Authentication tokens include client access information:

### ANCHOR Token

```json
{
  "sub": "0HZXEQ5Y8JY5Z",
  "scope": "ANCHOR",
  "clients": ["*"],
  "roles": ["platform-admin"]
}
```

### PARTNER Token

```json
{
  "sub": "0HZXEQ5Y8JY5Z",
  "scope": "PARTNER",
  "clients": ["client-123", "client-456"],
  "roles": ["partner-admin"]
}
```

### CLIENT Token

```json
{
  "sub": "0HZXEQ5Y8JY5Z",
  "scope": "CLIENT",
  "clients": ["client-789"],
  "clientId": "client-789",
  "roles": ["client-admin"]
}
```

## Data Isolation

### Client-Scoped Entities

Most entities have an optional `clientId` field:

```java
public class Subscription {
    // ...
    public String clientId;  // null = anchor-level, otherwise client-specific
}
```

**Query Patterns**:

```java
// Find subscriptions for a specific client
subscriptionRepository.find("clientId", clientId);

// Find anchor-level subscriptions (accessible to all)
subscriptionRepository.find("clientId is null");

// Find subscriptions accessible to a user
// (client-specific + anchor-level)
subscriptionRepository.find("clientId = ?1 or clientId is null", clientId);
```

### Anchor-Level Resources

Resources with `clientId = null` are anchor-level:
- Visible to ANCHOR scope users
- Can be shared across all clients
- Useful for platform-wide event types and schemas

### Client Isolation Rules

| Operation | ANCHOR | PARTNER | CLIENT |
|-----------|--------|---------|--------|
| View own client data | ✓ | ✓ | ✓ |
| View anchor-level data | ✓ | ✓ | ✓ |
| View other client data | ✓ | Only granted | ✗ |
| Create anchor-level | ✓ | ✗ | ✗ |
| Modify anchor-level | ✓ | ✗ | ✗ |
| Create client data | ✓ | In granted | Own only |

## Service Accounts

Machine-to-machine authentication for applications:

```java
@MongoEntity(collection = "service_accounts")
public class ServiceAccount {
    @BsonId
    public String id;

    public String code;           // Unique identifier
    public String name;
    public String description;

    public List<String> clientIds;  // Accessible clients
    public String applicationId;    // Associated application

    public boolean active;

    public WebhookCredentials webhookCredentials;
    public List<RoleAssignment> roles;

    public Instant lastUsedAt;
    public Instant createdAt;
    public Instant updatedAt;
}
```

### Webhook Credentials

```java
public record WebhookCredentials(
    WebhookAuthType authType,      // BEARER or BASIC
    String authTokenRef,           // Secret reference
    String signingSecretRef,       // HMAC signing secret reference
    SignatureAlgorithm signingAlgorithm,  // e.g., HMAC_SHA256
    Instant createdAt,
    Instant regeneratedAt
) {}
```

## Implementation Guidelines

### Enforcing Tenant Isolation

```java
@ApplicationScoped
public class TenantFilter {

    @Inject
    SecurityContext securityContext;

    public Query<T> applyTenantFilter(Query<T> query) {
        UserScope scope = securityContext.getScope();

        return switch (scope) {
            case ANCHOR -> query;  // No filter needed
            case PARTNER -> query.filter("clientId in ?1 or clientId is null",
                                         securityContext.getGrantedClients());
            case CLIENT -> query.filter("clientId = ?1 or clientId is null",
                                        securityContext.getClientId());
        };
    }
}
```

### API Endpoint Pattern

```java
@Path("/api/subscriptions")
public class SubscriptionResource {

    @GET
    public List<Subscription> list(@Context SecurityContext ctx) {
        // Automatically filtered by tenant
        return subscriptionService.findAccessible(ctx);
    }

    @POST
    public Subscription create(CreateSubscriptionCommand cmd,
                               @Context SecurityContext ctx) {
        // Validate user can create for target client
        if (!ctx.canAccessClient(cmd.clientId())) {
            throw new ForbiddenException();
        }
        return subscriptionService.create(cmd);
    }
}
```

## Best Practices

1. **Always filter queries** - Apply tenant filters at repository level
2. **Validate client access** - Check permissions before mutations
3. **Use anchor-level sparingly** - Only for truly shared resources
4. **Audit cross-client access** - Log PARTNER scope operations
5. **Expire access grants** - Set expiration on partner grants
6. **Document client ownership** - Use client notes for history

## See Also

- [Authentication Guide](../guides/authentication.md) - Auth system details
- [Authorization Guide](../guides/authorization.md) - Role-based access
- [Client Entities](../entities/client-entities.md) - Entity reference
