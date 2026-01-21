package tech.flowcatalyst.platform.authentication;

import jakarta.enterprise.context.ApplicationScoped;

/**
 * Service for syncing roles from IDP to internal roles.
 * SECURITY: Only explicitly authorized IDP roles (in idp_role_mappings table) are accepted.
 */
@ApplicationScoped
public class IdpRoleSyncService {

    // TODO: Implement IDP role sync logic
    // - syncIdpRoles(principalId, idpRoleNames)
    // - authorizeIdpRole(idpRoleName, internalRoleId) - platform admin only
    // - removeIdpRoleAuthorization(idpRoleName)
    // - Reject unauthorized IDP roles with logging
}
