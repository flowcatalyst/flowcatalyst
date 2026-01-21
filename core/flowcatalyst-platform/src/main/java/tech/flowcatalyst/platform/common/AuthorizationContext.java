package tech.flowcatalyst.platform.common;

import tech.flowcatalyst.platform.principal.ManagedApplicationScope;
import tech.flowcatalyst.platform.principal.PrincipalType;

import java.util.Set;

/**
 * Authorization context for a use case execution.
 *
 * <p>Carries authorization information about the executing principal,
 * including what applications they can manage and what clients they can access.
 *
 * <p>This context enables resource-level authorization checks in use cases:
 * <ul>
 *   <li>Application management scope - which applications can the principal manage?</li>
 *   <li>Client access scope - which clients can the principal access?</li>
 *   <li>Role and permission information for action-level checks</li>
 * </ul>
 *
 * @param principalId             The authenticated principal's ID
 * @param principalType           Type of principal (USER or SERVICE)
 * @param roles                   Roles assigned to the principal
 * @param permissions             Permissions derived from roles
 * @param managedApplicationScope Scope for application management (ALL, SPECIFIC, or NONE)
 * @param managedApplicationIds   Application IDs this principal can manage (when scope is SPECIFIC)
 * @param managedApplicationCodes Application codes this principal can manage (resolved from IDs)
 * @param accessibleClientIds     Client IDs this principal can access
 * @param canAccessAllClients     Whether this principal can access all clients (ANCHOR scope)
 */
public record AuthorizationContext(
    String principalId,
    PrincipalType principalType,
    Set<String> roles,
    Set<String> permissions,
    ManagedApplicationScope managedApplicationScope,
    Set<String> managedApplicationIds,
    Set<String> managedApplicationCodes,
    Set<String> accessibleClientIds,
    boolean canAccessAllClients
) {

    /**
     * Check if this principal can manage all applications.
     *
     * @return true if scope is ALL
     */
    public boolean canManageAllApplications() {
        return managedApplicationScope == ManagedApplicationScope.ALL;
    }

    /**
     * Check if this principal can manage a specific application by ID.
     *
     * @param applicationId the application ID to check
     * @return true if the principal can manage this application
     */
    public boolean canManageApplication(String applicationId) {
        return switch (managedApplicationScope) {
            case ALL -> true;
            case SPECIFIC -> managedApplicationIds != null && managedApplicationIds.contains(applicationId);
            case NONE -> false;
        };
    }

    /**
     * Check if this principal can manage a resource with the given code prefix.
     *
     * <p>Resource codes follow the pattern "{applicationCode}:{resourceName}".
     * For example, "tms:shipment-event" belongs to the "tms" application.
     *
     * @param resourceCode the resource code to check (e.g., "tms:manager" for a role)
     * @return true if the principal can manage resources with this prefix
     */
    public boolean canManageResourceWithPrefix(String resourceCode) {
        if (resourceCode == null || resourceCode.isBlank()) {
            return false;
        }
        return switch (managedApplicationScope) {
            case ALL -> true;
            case SPECIFIC -> managedApplicationCodes != null &&
                managedApplicationCodes.stream()
                    .anyMatch(code -> resourceCode.startsWith(code + ":"));
            case NONE -> false;
        };
    }

    /**
     * Check if this principal is a platform administrator.
     *
     * @return true if the principal has a platform admin role
     */
    public boolean isPlatformAdmin() {
        return roles != null && roles.stream().anyMatch(r ->
            r.equals("platform:super-admin") ||
            r.equals("platform:platform-admin")
        );
    }

    /**
     * Check if this principal is a service account.
     *
     * @return true if principal type is SERVICE
     */
    public boolean isServiceAccount() {
        return principalType == PrincipalType.SERVICE;
    }

    /**
     * Check if this principal can access a specific client.
     *
     * @param clientId the client ID to check
     * @return true if the principal can access this client
     */
    public boolean canAccessClient(String clientId) {
        if (canAccessAllClients) {
            return true;
        }
        return accessibleClientIds != null && accessibleClientIds.contains(clientId);
    }

    /**
     * Check if this principal has a specific role.
     *
     * @param roleName the role name to check
     * @return true if the principal has this role
     */
    public boolean hasRole(String roleName) {
        return roles != null && roles.contains(roleName);
    }

    /**
     * Check if this principal has a specific permission.
     *
     * @param permissionName the permission name to check
     * @return true if the principal has this permission
     */
    public boolean hasPermission(String permissionName) {
        return permissions != null && permissions.contains(permissionName);
    }
}
