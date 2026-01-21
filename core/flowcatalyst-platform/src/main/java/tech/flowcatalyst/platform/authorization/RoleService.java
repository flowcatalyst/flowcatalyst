package tech.flowcatalyst.platform.authorization;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.ws.rs.BadRequestException;
import jakarta.ws.rs.NotFoundException;
import tech.flowcatalyst.platform.authorization.platform.PlatformSuperAdminRole;
import tech.flowcatalyst.platform.client.ClientAccessService;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;
import tech.flowcatalyst.platform.principal.PrincipalType;

import java.util.List;
import java.util.Set;
import java.util.stream.Collectors;

/**
 * Service for role assignment to principals.
 *
 * Roles are stored embedded in the Principal document (MongoDB denormalized pattern).
 *
 * Roles can come from three sources:
 * - CODE: Defined in Java @Role classes (synced to auth_roles at startup)
 * - DATABASE: Created by administrators through the UI
 * - SDK: Registered by external applications via the SDK API
 *
 * Role validation checks both the auth_roles table (primary) and PermissionRegistry (fallback).
 *
 * Role format: {subdomain}:{role-name}
 * Example: "platform:tenant-admin", "logistics:dispatcher"
 */
@ApplicationScoped
public class RoleService {

    @Inject
    PrincipalRepository principalRepo;

    @Inject
    PermissionRegistry permissionRegistry;

    @Inject
    AuthRoleRepository authRoleRepo;

    @Inject
    ClientAccessService clientAccessService;

    /**
     * Assign a role to a principal.
     *
     * Role must exist in either:
     * - auth_roles table (primary source - includes CODE, DATABASE, and SDK roles)
     * - PermissionRegistry (fallback for backwards compatibility)
     *
     * @param principalId Principal ID
     * @param roleName Role name string (e.g., "platform:tenant-admin")
     * @param assignmentSource How the role was assigned ("MANUAL", "IDP_SYNC", etc.)
     * @return Created principal role assignment (as PrincipalRole for API compatibility)
     * @throws NotFoundException if principal not found
     * @throws BadRequestException if assignment already exists or role not defined
     */
    public PrincipalRole assignRole(String principalId, String roleName, String assignmentSource) {
        // Find principal
        Principal principal = principalRepo.findByIdOptional(principalId)
            .orElseThrow(() -> new NotFoundException("Principal not found: " + principalId));

        // Validate role exists in database or registry
        if (!isValidRole(roleName)) {
            throw new BadRequestException("Role not defined: " + roleName + ". " +
                "Role must exist in auth_roles table or be defined in code.");
        }

        // SECURITY: Super Admin role can only be assigned to anchor domain users
        if (isSuperAdminRole(roleName)) {
            if (principal.type != PrincipalType.USER) {
                throw new BadRequestException(
                    "Super Admin role can only be assigned to users, not service accounts");
            }
            if (!clientAccessService.isAnchorDomainUser(principal)) {
                throw new BadRequestException(
                    "Super Admin role can only be assigned to users from anchor domains");
            }
        }

        // Check if assignment already exists
        if (principal.hasRole(roleName)) {
            throw new BadRequestException("Role already assigned to principal: " + roleName);
        }

        // Add role to principal's embedded list
        Principal.RoleAssignment assignment = new Principal.RoleAssignment(
            roleName,
            assignmentSource != null ? assignmentSource : "MANUAL"
        );
        principal.roles.add(assignment);
        principalRepo.update(principal);

        // Return as PrincipalRole for API compatibility
        PrincipalRole result = new PrincipalRole();
        result.principalId = principalId;
        result.roleName = roleName;
        result.assignmentSource = assignment.assignmentSource;
        result.assignedAt = assignment.assignedAt;
        return result;
    }

    /**
     * Check if a role name is valid (exists in DB or registry).
     *
     * @param roleName Role name to validate
     * @return true if role exists
     */
    public boolean isValidRole(String roleName) {
        // Check database first (primary source after sync)
        if (authRoleRepo.existsByName(roleName)) {
            return true;
        }
        // Fallback to registry (for backwards compatibility during transition)
        return permissionRegistry.hasRole(roleName);
    }

    /**
     * Check if a role is the Super Admin role.
     * Super Admin has special restrictions - can only be assigned to anchor domain users.
     *
     * @param roleName Role name to check
     * @return true if this is the Super Admin role
     */
    public boolean isSuperAdminRole(String roleName) {
        return PlatformSuperAdminRole.ROLE_NAME.equals(roleName);
    }

    /**
     * Check if a principal has the Super Admin role.
     *
     * @param principalId Principal ID
     * @return true if principal has Super Admin role
     */
    public boolean isSuperAdmin(String principalId) {
        return hasRole(principalId, PlatformSuperAdminRole.ROLE_NAME);
    }

    /**
     * Remove a role from a principal.
     *
     * @param principalId Principal ID
     * @param roleName Role name string (e.g., "platform:tenant-admin")
     * @throws NotFoundException if assignment not found
     */
    public void removeRole(String principalId, String roleName) {
        Principal principal = principalRepo.findByIdOptional(principalId)
            .orElseThrow(() -> new NotFoundException("Principal not found: " + principalId));

        boolean removed = principal.roles.removeIf(r -> r.roleName.equals(roleName));
        if (!removed) {
            throw new NotFoundException("Role assignment not found: " + roleName);
        }

        principalRepo.update(principal);
    }

    /**
     * Remove all roles from a principal that have a specific assignment source.
     * Used for IDP sync to remove old IDP-assigned roles before adding new ones.
     *
     * @param principalId Principal ID
     * @param assignmentSource Assignment source to remove (e.g., "IDP_SYNC")
     * @return Number of roles removed
     */
    public long removeRolesBySource(String principalId, String assignmentSource) {
        Principal principal = principalRepo.findByIdOptional(principalId).orElse(null);
        if (principal == null) {
            return 0;
        }

        int sizeBefore = principal.roles.size();
        principal.roles.removeIf(r -> assignmentSource.equals(r.assignmentSource));
        int removed = sizeBefore - principal.roles.size();

        if (removed > 0) {
            principalRepo.update(principal);
        }

        return removed;
    }

    /**
     * Find all role names assigned to a principal.
     *
     * @param principalId Principal ID
     * @return Set of role name strings (e.g., "platform:tenant-admin")
     */
    public Set<String> findRoleNamesByPrincipal(String principalId) {
        return principalRepo.findByIdOptional(principalId)
            .map(Principal::getRoleNames)
            .orElse(Set.of());
    }

    /**
     * Find all role definitions assigned to a principal.
     * Includes full role metadata from PermissionRegistry.
     *
     * @param principalId Principal ID
     * @return Set of role definitions
     */
    public Set<RoleDefinition> findRoleDefinitionsByPrincipal(String principalId) {
        return findRoleNamesByPrincipal(principalId).stream()
            .map(roleName -> permissionRegistry.getRole(roleName))
            .filter(opt -> opt.isPresent())
            .map(opt -> opt.get())
            .collect(Collectors.toSet());
    }

    /**
     * Find all principal role assignments for a principal.
     * Returns embedded roles converted to PrincipalRole for API compatibility.
     *
     * @param principalId Principal ID
     * @return List of principal role assignments
     */
    public List<PrincipalRole> findAssignmentsByPrincipal(String principalId) {
        return principalRepo.findByIdOptional(principalId)
            .map(principal -> principal.roles.stream()
                .map(r -> {
                    PrincipalRole pr = new PrincipalRole();
                    pr.principalId = principalId;
                    pr.roleName = r.roleName;
                    pr.assignmentSource = r.assignmentSource;
                    pr.assignedAt = r.assignedAt;
                    return pr;
                })
                .toList())
            .orElse(List.of());
    }

    /**
     * Check if a principal has a specific role.
     *
     * @param principalId Principal ID
     * @param roleName Role name string
     * @return true if principal has the role
     */
    public boolean hasRole(String principalId, String roleName) {
        return principalRepo.findByIdOptional(principalId)
            .map(p -> p.hasRole(roleName))
            .orElse(false);
    }

    /**
     * Get all permission strings granted to a principal via their roles.
     *
     * @param principalId Principal ID
     * @return Set of permission strings
     */
    public Set<String> getPermissionsForPrincipal(String principalId) {
        Set<String> roleNames = findRoleNamesByPrincipal(principalId);
        return permissionRegistry.getPermissionsForRoles(roleNames);
    }

    /**
     * Get all available roles from the database.
     *
     * @return List of all AuthRole entities
     */
    public List<AuthRole> getAllRoles() {
        return authRoleRepo.listAll();
    }

    /**
     * Get all roles for a specific application.
     *
     * @param applicationCode Application code (e.g., "platform")
     * @return List of AuthRole entities for that application
     */
    public List<AuthRole> getRolesForApplication(String applicationCode) {
        return authRoleRepo.findByApplicationCode(applicationCode);
    }

    /**
     * Get a role by name from the database.
     *
     * @param roleName Full role name (e.g., "platform:tenant-admin")
     * @return AuthRole if found
     */
    public java.util.Optional<AuthRole> getRoleByName(String roleName) {
        return authRoleRepo.findByName(roleName);
    }
}
