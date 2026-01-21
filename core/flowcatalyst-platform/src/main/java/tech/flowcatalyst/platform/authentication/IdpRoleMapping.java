package tech.flowcatalyst.platform.authentication;

import java.time.Instant;

/**
 * SECURITY: Explicit authorization of IDP roles to internal roles.
 * Only IDP roles in this table are accepted during login.
 * This prevents partners/customers from injecting unauthorized roles.
 *
 * Maps external IDP role names to internal string-based role names.
 * Internal roles must be defined in PermissionRegistry.
 */

public class IdpRoleMapping {

    public String id;

    /**
     * Role name from the external IDP (e.g., Keycloak role name from partner IDP).
     * This is the role name that appears in the OIDC token.
     */
    public String idpRoleName;

    /**
     * Internal role name this IDP role maps to (e.g., "platform:tenant-admin").
     * Must match a role defined in PermissionRegistry.
     */
    public String internalRoleName;

    public Instant createdAt = Instant.now();

    public IdpRoleMapping() {
    }
}
