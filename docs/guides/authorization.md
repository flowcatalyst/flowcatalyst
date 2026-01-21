# Authorization Guide

FlowCatalyst implements role-based access control (RBAC) with fine-grained permissions. This guide covers the permission system, role management, and authorization patterns.

## Overview

```
Principal ──► Role(s) ──► Permission(s) ──► Access Decision
```

## Permission Format

Permissions follow a four-part hierarchy:

```
{subdomain}:{context}:{aggregate}:{action}
```

| Part | Description | Examples |
|------|-------------|----------|
| `subdomain` | Application domain | `platform`, `orders`, `crm` |
| `context` | Access context | `admin`, `client`, `public` |
| `aggregate` | Resource type | `clients`, `orders`, `users` |
| `action` | Operation | `read`, `write`, `delete`, `*` |

### Examples

```
platform:admin:clients:read      # Read clients in admin context
platform:admin:clients:write     # Create/update clients
orders:client:orders:create      # Create orders for own client
orders:client:orders:*           # All order operations
*:*:*:*                          # Superuser (all permissions)
```

## Roles

### Role Definition

```java
@MongoEntity(collection = "auth_roles")
public class AuthRole {
    public String id;
    public String applicationId;    // Owning application
    public String name;             // Role name
    public String displayName;
    public String description;
    public Set<String> permissions; // Permission strings
    public RoleSource source;       // CODE, DATABASE, SDK
    public boolean clientManaged;   // Can clients modify?
}
```

### Creating Roles

```http
POST /api/roles
Content-Type: application/json
Authorization: Bearer {admin-token}

{
  "name": "order-manager",
  "displayName": "Order Manager",
  "description": "Can manage orders and view customers",
  "permissions": [
    "orders:client:orders:create",
    "orders:client:orders:read",
    "orders:client:orders:update",
    "customers:client:customers:read"
  ]
}
```

### Built-in Roles

| Role | Permissions | Description |
|------|-------------|-------------|
| `platform-admin` | `platform:admin:*:*` | Full platform access |
| `platform-iam-admin` | IAM permissions | Manage users and roles |
| `client-admin` | Client-level admin | Manage own client |

## Role Assignment

### Assigning Roles to Users

```http
POST /api/principals/{principalId}/roles
Content-Type: application/json

{
  "roleName": "order-manager",
  "assignmentSource": "MANUAL"
}
```

### Assignment Sources

| Source | Description |
|--------|-------------|
| `MANUAL` | Assigned by administrator |
| `IDP` | Synced from identity provider |
| `SDK` | Assigned via SDK integration |

### IDP Role Mapping

Map external IDP roles to internal roles:

```http
POST /api/auth/idp-role-mappings
Content-Type: application/json

{
  "idpRoleName": "Azure-Admins",
  "internalRoleName": "client-admin"
}
```

Only mapped roles grant internal permissions (security allowlist).

## Checking Permissions

### In Code

```java
@Inject
SecurityContext securityContext;

public void someMethod() {
    // Check single permission
    if (securityContext.hasPermission("orders:client:orders:create")) {
        // User can create orders
    }

    // Check any permission
    if (securityContext.hasAnyPermission(
        "orders:client:orders:update",
        "orders:admin:orders:update"
    )) {
        // User can update orders
    }

    // Get all permissions
    Set<String> permissions = securityContext.getPermissions();
}
```

### Using Annotations

```java
@Path("/orders")
public class OrderResource {

    @POST
    @RolesAllowed("order-manager")  // Role-based
    public Order createOrder(CreateOrderCommand cmd) {
        // ...
    }

    @GET
    @PermissionsRequired("orders:client:orders:read")  // Permission-based
    public List<Order> listOrders() {
        // ...
    }
}
```

## Permission Inheritance

Wildcards enable permission inheritance:

```
orders:client:orders:*     # All actions on orders
orders:client:*:read       # Read all client resources
orders:*:*:*               # All orders domain
*:*:*:*                    # All permissions
```

### Matching Logic

```java
// User has: orders:client:orders:*
// Checking: orders:client:orders:create
// Result: ALLOWED (wildcard matches)

// User has: orders:client:orders:read
// Checking: orders:client:orders:write
// Result: DENIED (no match)
```

## Multi-Tenant Authorization

### Scope-Based Access

| Scope | Client Access | Data Access |
|-------|---------------|-------------|
| ANCHOR | All clients | All data |
| PARTNER | Granted clients | Granted client data |
| CLIENT | Own client only | Own client data |

### Client Context

```java
// Check if user can access a specific client
if (!securityContext.canAccessClient(clientId)) {
    throw new ForbiddenException("Access denied to client");
}

// Get user's accessible clients
List<String> clients = securityContext.getAccessibleClients();
// ANCHOR: ["*"]
// PARTNER: ["client-1", "client-2"]
// CLIENT: ["client-1"]
```

## SDK Role Registration

Applications can register roles via SDK:

```java
@RoleDefinition(
    name = "inventory-manager",
    displayName = "Inventory Manager",
    permissions = {
        "inventory:client:products:*",
        "inventory:client:stock:read"
    }
)
public class InventoryRoles {}
```

Roles sync automatically on application startup.

## Best Practices

### Role Design

1. **Principle of least privilege** - Grant minimum required permissions
2. **Role per responsibility** - Don't overload roles
3. **Use wildcards carefully** - Audit wildcard permissions
4. **Document roles** - Include clear descriptions

### Permission Design

1. **Be specific** - Use specific actions over wildcards
2. **Consistent naming** - Follow subdomain:context:aggregate:action
3. **Context separation** - Separate admin vs client permissions
4. **Audit access** - Log permission checks

### Security

1. **Regular audits** - Review role assignments
2. **Expire temporary access** - Use ClientAccessGrant expiration
3. **Monitor privileged access** - Alert on admin actions
4. **IDP mapping security** - Only map trusted IDP roles

## Configuration

```properties
# Enable permission checking
flowcatalyst.auth.permissions.enabled=true

# Cache permission lookups
flowcatalyst.auth.permissions.cache-ttl=300

# Log permission denials
flowcatalyst.auth.permissions.log-denials=true
```

## See Also

- [Authentication Guide](authentication.md) - Auth system guide
- [Auth Entities](../entities/auth-entities.md) - Role/permission entities
- [Multi-Tenancy](../architecture/multi-tenancy.md) - Tenant model
