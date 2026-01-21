package tech.flowcatalyst.platform.authentication;

import io.quarkus.logging.Log;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.authentication.IdpRoleMapping;
import tech.flowcatalyst.platform.authentication.IdpRoleMappingRepository;
import tech.flowcatalyst.platform.authorization.RoleService;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.UserService;

import java.util.HashSet;
import java.util.List;
import java.util.Set;

/**
 * CRITICAL SECURITY: Service for OIDC user and role synchronization.
 *
 * This service implements a critical security control: IDP role authorization.
 * Only IDP roles that are explicitly authorized in the idp_role_mappings table
 * are accepted during OIDC login. This prevents partners/customers from
 * injecting unauthorized roles via compromised or misconfigured IDPs.
 *
 * Example attack prevented:
 * - Partner IDP is compromised and grants all users "super-admin" role
 * - This service rejects the role because it's not in idp_role_mappings
 * - Attack is logged and prevented
 *
 * See docs/auth-architecture.md for full security requirements.
 */
@ApplicationScoped
public class OidcSyncService {

    @Inject
    IdpRoleMappingRepository idpRoleMappingRepo;

    @Inject
    RoleService roleService;

    @Inject
    UserService userService;

    /**
     * Synchronize user information from OIDC token.
     * Creates or updates the user principal based on OIDC claims.
     *
     * @param email User email from OIDC token
     * @param name User display name from OIDC token
     * @param externalIdpId Subject from OIDC token (IDP's user ID)
     * @param clientId Home tenant ID (nullable for anchor domain users)
     * @return Synchronized principal
     */
    public Principal syncOidcUser(String email, String name, String externalIdpId, String clientId) {
        Principal principal = userService.createOrUpdateOidcUser(email, name, externalIdpId, clientId, null);
        userService.updateLastLogin(principal.id);
        return principal;
    }

    /**
     * CRITICAL SECURITY: Synchronize IDP roles to internal roles.
     *
     * This method implements the IDP role authorization security control.
     * Only IDP roles that are explicitly authorized in the idp_role_mappings
     * table are accepted. Any unauthorized role is rejected and logged.
     *
     * Flow:
     * 1. For each IDP role name from the token:
     *    a. Look up the role in idp_role_mappings
     *    b. If found: Accept and add the mapped internal role name
     *    c. If NOT found: REJECT and log as security warning
     * 2. Remove all existing IDP-sourced roles from the principal
     * 3. Assign all authorized internal role names with "IDP_SYNC" source
     *
     * SECURITY NOTE: This prevents the following attack:
     * - A compromised or misconfigured IDP grants unauthorized roles (e.g., "super-admin")
     * - This service rejects the role because it's not in idp_role_mappings
     * - Platform administrator must explicitly authorize IDP roles before they work
     * - All rejections are logged for security auditing
     *
     * @param principal The user principal
     * @param idpRoleNames List of role names from the OIDC token (e.g., from realm_access.roles)
     * @return Set of accepted internal role names (e.g., "platform:tenant-admin")
     */
    public Set<String> syncIdpRoles(Principal principal, List<String> idpRoleNames) {
        Set<String> authorizedRoleNames = new HashSet<>();

        if (idpRoleNames == null || idpRoleNames.isEmpty()) {
            Log.info("No IDP roles provided for principal " + principal.id);
        } else {
            // SECURITY: Only accept IDP roles that are explicitly authorized in idp_role_mappings
            for (String idpRoleName : idpRoleNames) {
                IdpRoleMapping mapping = idpRoleMappingRepo.findByIdpRoleName(idpRoleName).orElse(null);

                if (mapping != null) {
                    // This IDP role is authorized - map to internal role name
                    authorizedRoleNames.add(mapping.internalRoleName);
                    Log.debug("Accepted IDP role '" + idpRoleName + "' for principal " + principal.id +
                        " (maps to internal role '" + mapping.internalRoleName + "')");
                } else {
                    // SECURITY: Reject unauthorized IDP role
                    // This prevents malicious/misconfigured IDPs from granting unauthorized access
                    Log.warn("SECURITY: REJECTED unauthorized IDP role '" + idpRoleName +
                        "' for principal " + principal.id + " (email: " + principal.userIdentity.email + "). " +
                        "Role not found in idp_role_mappings table. " +
                        "Platform administrator must explicitly authorize this IDP role before it can be used.");
                }
            }
        }

        // Remove all existing IDP-sourced roles
        long removedCount = roleService.removeRolesBySource(principal.id, "IDP_SYNC");
        if (removedCount > 0) {
            Log.debug("Removed " + removedCount + " old IDP-sourced roles for principal " + principal.id);
        }

        // Assign all authorized internal role names
        int assignedCount = 0;
        for (String roleName : authorizedRoleNames) {
            try {
                roleService.assignRole(principal.id, roleName, "IDP_SYNC");
                assignedCount++;
            } catch (Exception e) {
                // Role might already be assigned from a previous source
                Log.debug("Could not assign role '" + roleName + "' to principal " + principal.id + ": " + e.getMessage());
            }
        }

        int providedCount = (idpRoleNames != null) ? idpRoleNames.size() : 0;
        Log.info("IDP role sync complete for principal " + principal.id + " (email: " +
            principal.userIdentity.email + "): " +
            providedCount + " IDP roles provided, " +
            authorizedRoleNames.size() + " authorized, " +
            assignedCount + " assigned");

        return authorizedRoleNames;
    }

    /**
     * Full OIDC sync: sync both user info and roles.
     * This is the main method called during OIDC login callback.
     *
     * @param email User email from OIDC token
     * @param name User display name from OIDC token
     * @param externalIdpId Subject from OIDC token
     * @param clientId Home tenant ID (nullable for anchor users)
     * @param idpRoleNames List of role names from OIDC token
     * @return Synchronized principal
     */
    public Principal syncOidcLogin(String email, String name, String externalIdpId,
                                   String clientId, List<String> idpRoleNames) {
        // Sync user information
        Principal principal = syncOidcUser(email, name, externalIdpId, clientId);

        // CRITICAL SECURITY: Sync IDP roles with authorization check
        syncIdpRoles(principal, idpRoleNames);

        return principal;
    }

    /**
     * Audit log all IDP role mappings for a principal.
     * Used for security auditing and debugging.
     *
     * @param principalId Principal ID
     * @return Audit information as a string
     */
    public String auditIdpRoles(String principalId) {
        var assignments = roleService.findAssignmentsByPrincipal(principalId);
        var idpRoles = assignments.stream()
            .filter(a -> "IDP_SYNC".equals(a.assignmentSource))
            .toList();

        StringBuilder audit = new StringBuilder();
        audit.append("Principal ").append(principalId).append(" has ")
            .append(idpRoles.size()).append(" IDP-sourced roles:\n");

        for (var assignment : idpRoles) {
            audit.append("  - Role: ").append(assignment.roleName);
            audit.append(", assigned at ").append(assignment.assignedAt).append("\n");
        }

        return audit.toString();
    }
}
