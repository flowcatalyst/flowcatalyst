package tech.flowcatalyst.platform.authentication.idp;

import tech.flowcatalyst.platform.authorization.PermissionDefinition;
import tech.flowcatalyst.platform.authorization.RoleDefinition;

import java.util.Set;

/**
 * Adapter interface for syncing roles and permissions to external IDPs.
 *
 * This is used for the PLATFORM IDP ONLY (the IDP we control).
 * We PUSH our code-first roles to the platform IDP so they can be assigned to users.
 *
 * For EXTERNAL IDPs (partner/customer IDPs), we PULL roles via OIDC tokens
 * and validate them against idp_role_mappings (security whitelist).
 *
 * Implementations:
 * - KeycloakSyncAdapter: Syncs to Keycloak via Admin API
 * - EntraSyncAdapter: Syncs to Microsoft Entra via Graph API
 * - Custom adapters can be added for other OIDC-compliant IDPs
 */
public interface IdpSyncAdapter {

    /**
     * Push all roles to the IDP.
     * Creates or updates roles in the IDP to match our code-first definitions.
     *
     * @param roles Set of role definitions to sync
     * @throws IdpSyncException if sync fails
     */
    void syncRolesToIdp(Set<RoleDefinition> roles) throws IdpSyncException;

    /**
     * Push all permissions to the IDP (if the IDP supports permission-level granularity).
     * Some IDPs only support roles, in which case this method may be a no-op.
     *
     * @param permissions Set of permission definitions to sync
     * @throws IdpSyncException if sync fails
     */
    void syncPermissionsToIdp(Set<PermissionDefinition> permissions) throws IdpSyncException;

    /**
     * Get the IDP type this adapter handles.
     *
     * @return IDP type (e.g., KEYCLOAK, ENTRA, OIDC_GENERIC)
     */
    String getIdpType();

    /**
     * Test connectivity to the IDP.
     * Used for health checks and configuration validation.
     *
     * @return true if IDP is reachable and credentials are valid
     */
    boolean testConnection();

    /**
     * Get a human-readable name for this IDP adapter.
     *
     * @return Adapter name (e.g., "Keycloak Admin API", "Microsoft Entra Graph API")
     */
    String getAdapterName();
}
