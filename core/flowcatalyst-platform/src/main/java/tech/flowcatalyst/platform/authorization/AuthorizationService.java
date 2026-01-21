package tech.flowcatalyst.platform.authorization;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.ws.rs.ForbiddenException;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;

import java.util.Set;
import java.util.stream.Collectors;

/**
 * Service for authorization operations using code-first permissions and roles.
 * Provides RBAC permission checks against PermissionRegistry.
 *
 * IMPORTANT: This service ONLY validates RBAC permissions.
 * Tenant isolation and business rules MUST be enforced in application logic.
 *
 * Permission format: {subdomain}:{context}:{aggregate}:{action}
 * Example: "platform:tenant:user:create"
 *
 * Role format: {subdomain}:{role-name}
 * Example: "platform:tenant-admin"
 */
@ApplicationScoped
public class AuthorizationService {

    @Inject
    PrincipalRepository principalRepo;

    @Inject
    PermissionRegistry permissionRegistry;

    /**
     * Check if a principal has a specific permission.
     *
     * Permission string format: {subdomain}:{context}:{aggregate}:{action}
     * Example: "platform:tenant:user:create"
     *
     * @param principalId The principal ID
     * @param permissionString The permission string to check
     * @return true if principal has the permission
     */
    public boolean hasPermission(String principalId, String permissionString) {
        // Get all role names for this principal
        Set<String> roleNames = getRoleNames(principalId);

        // Get all permissions granted by these roles
        Set<String> grantedPermissions = permissionRegistry.getPermissionsForRoles(roleNames);

        return grantedPermissions.contains(permissionString);
    }

    /**
     * Check if a principal has a specific permission.
     * Convenience method that works with Principal object.
     *
     * @param principal The principal
     * @param permissionString The permission string
     * @return true if principal has the permission
     */
    public boolean hasPermission(Principal principal, String permissionString) {
        return hasPermission(principal.id, permissionString);
    }

    /**
     * Check if a principal has a specific permission using semantic parts.
     * Builds the permission string from parts and checks it.
     *
     * @param principalId The principal ID
     * @param subdomain Business domain (e.g., "platform", "logistics")
     * @param context Bounded context (e.g., "tenant", "dispatch")
     * @param aggregate Resource/entity (e.g., "user", "order")
     * @param action Operation (e.g., "create", "view", "update", "delete")
     * @return true if principal has the permission
     */
    public boolean hasPermission(String principalId, String subdomain, String context,
                                 String aggregate, String action) {
        String permissionString = String.format("%s:%s:%s:%s", subdomain, context, aggregate, action);
        return hasPermission(principalId, permissionString);
    }

    /**
     * Require that a principal has a specific permission.
     * Throws ForbiddenException if permission is not granted.
     *
     * @param principalId The principal ID
     * @param permissionString The permission string
     * @throws ForbiddenException if permission denied
     */
    public void requirePermission(String principalId, String permissionString) {
        if (!hasPermission(principalId, permissionString)) {
            throw new ForbiddenException(
                String.format("Missing permission: %s", permissionString));
        }
    }

    /**
     * Require that a principal has a specific permission.
     * Throws ForbiddenException if permission is not granted.
     *
     * @param principalId The principal ID
     * @param permission The permission definition
     * @throws ForbiddenException if permission denied
     */
    public void requirePermission(String principalId, PermissionDefinition permission) {
        requirePermission(principalId, permission.toPermissionString());
    }

    /**
     * Require that a principal has a specific permission.
     * Convenience method that works with Principal object.
     *
     * @param principal The principal
     * @param permissionString The permission string
     * @throws ForbiddenException if permission denied
     */
    public void requirePermission(Principal principal, String permissionString) {
        requirePermission(principal.id, permissionString);
    }

    /**
     * Require that a principal has a specific permission using semantic parts.
     *
     * @param principalId The principal ID
     * @param subdomain Business domain
     * @param context Bounded context
     * @param aggregate Resource/entity
     * @param action Operation
     * @throws ForbiddenException if permission denied
     */
    public void requirePermission(String principalId, String subdomain, String context,
                                  String aggregate, String action) {
        String permissionString = String.format("%s:%s:%s:%s", subdomain, context, aggregate, action);
        requirePermission(principalId, permissionString);
    }

    /**
     * Get all role names assigned to a principal.
     * Reads from the embedded roles array on Principal.
     *
     * Role string format: {subdomain}:{role-name}
     * Example: "platform:tenant-admin"
     *
     * @param principalId The principal ID
     * @return Set of role name strings
     */
    public Set<String> getRoleNames(String principalId) {
        return principalRepo.findByIdOptional(principalId)
            .map(Principal::getRoleNames)
            .orElse(Set.of());
    }

    /**
     * Get all permission strings granted to a principal (from all their roles).
     *
     * Permission string format: {subdomain}:{context}:{aggregate}:{action}
     * Example: "platform:tenant:user:create"
     *
     * @param principalId The principal ID
     * @return Set of permission strings
     */
    public Set<String> getPermissions(String principalId) {
        Set<String> roleNames = getRoleNames(principalId);
        return permissionRegistry.getPermissionsForRoles(roleNames);
    }

    /**
     * Get all role definitions assigned to a principal.
     * Includes full role metadata (permissions, descriptions).
     *
     * @param principalId The principal ID
     * @return Set of role definitions
     */
    public Set<RoleDefinition> getRoleDefinitions(String principalId) {
        return getRoleNames(principalId).stream()
            .map(roleName -> permissionRegistry.getRole(roleName))
            .filter(opt -> opt.isPresent())
            .map(opt -> opt.get())
            .collect(Collectors.toSet());
    }

    /**
     * Get all permission definitions granted to a principal.
     * Includes full permission metadata (descriptions, parts).
     *
     * @param principalId The principal ID
     * @return Set of permission definitions
     */
    public Set<PermissionDefinition> getPermissionDefinitions(String principalId) {
        return getPermissions(principalId).stream()
            .map(permString -> permissionRegistry.getPermission(permString))
            .filter(opt -> opt.isPresent())
            .map(opt -> opt.get())
            .collect(Collectors.toSet());
    }

    /**
     * Check if a principal has a specific role.
     *
     * @param principalId The principal ID
     * @param roleName The role name to check
     * @return true if principal has the role
     */
    public boolean hasRole(String principalId, String roleName) {
        return getRoleNames(principalId).contains(roleName);
    }

    /**
     * Check if a principal has ANY of the specified roles.
     *
     * @param principalId The principal ID
     * @param roleNames The role names to check
     * @return true if principal has at least one of the roles
     */
    public boolean hasAnyRole(String principalId, String... roleNames) {
        Set<String> principalRoles = getRoleNames(principalId);
        for (String roleName : roleNames) {
            if (principalRoles.contains(roleName)) {
                return true;
            }
        }
        return false;
    }

    /**
     * Check if a principal has ALL of the specified roles.
     *
     * @param principalId The principal ID
     * @param roleNames The role names to check
     * @return true if principal has all of the roles
     */
    public boolean hasAllRoles(String principalId, String... roleNames) {
        Set<String> principalRoles = getRoleNames(principalId);
        for (String roleName : roleNames) {
            if (!principalRoles.contains(roleName)) {
                return false;
            }
        }
        return true;
    }
}
