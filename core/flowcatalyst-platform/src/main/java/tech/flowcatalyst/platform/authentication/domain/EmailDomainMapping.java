package tech.flowcatalyst.platform.authentication.domain;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;

/**
 * Maps an email domain to an identity provider and determines user access scope.
 *
 * <p>This entity connects three concepts:
 * <ul>
 *   <li>Email domain (e.g., "acmecorp.com") - unique identifier</li>
 *   <li>Identity provider - how users authenticate</li>
 *   <li>Scope type - what access level users get</li>
 * </ul>
 *
 * <p>Example configurations:
 * <ul>
 *   <li>flowcatalyst.local (ANCHOR) -> internal IDP (platform admins)</li>
 *   <li>acmecorp.com (CLIENT) -> Okta IDP (users bound to Acme Corp client)</li>
 *   <li>partner.com (PARTNER) -> Entra IDP (partner users with multi-client access)</li>
 * </ul>
 *
 * <p>Client associations depend on scope type:
 * <ul>
 *   <li>ANCHOR: No client associations (access to all clients)</li>
 *   <li>PARTNER: grantedClientIds list (explicit multi-client access)</li>
 *   <li>CLIENT: primaryClientId + additionalClientIds (home client with exceptions)</li>
 * </ul>
 */
public class EmailDomainMapping {

    public String id; // TSID (Crockford Base32)

    /**
     * The email domain this mapping applies to (e.g., "acmecorp.com").
     * Must be unique across all mappings.
     */
    public String emailDomain;

    /**
     * Reference to the identity provider used for authentication.
     */
    public String identityProviderId;

    /**
     * The scope type determining user access level.
     * - ANCHOR: Platform-wide, no client associations
     * - PARTNER: Partner access, uses granted clients list
     * - CLIENT: Client-specific, uses primary + additional clients
     */
    public ScopeType scopeType;

    /**
     * The primary client for CLIENT scope type.
     * Required for CLIENT type, must be null for ANCHOR and PARTNER types.
     */
    public String primaryClientId;

    /**
     * Additional client IDs for CLIENT scope type.
     * Allows client-bound users to access additional clients as exceptions.
     * Must be empty for ANCHOR and PARTNER types.
     */
    public List<String> additionalClientIds = new ArrayList<>();

    /**
     * Granted client IDs for PARTNER scope type.
     * Users authenticating from this domain can access these clients.
     * Must be empty for ANCHOR and CLIENT types.
     */
    public List<String> grantedClientIds = new ArrayList<>();

    /**
     * Required OIDC tenant ID for multi-tenant identity providers.
     * For Azure AD/Entra, this is the tenant GUID (e.g., "2e789bd9-a313-462a-b520-df9b586c00ed").
     * If set, the 'tid' claim in the ID token must match this value.
     * This prevents users from one Entra tenant authenticating for a domain belonging to another tenant.
     * Optional - if null, tenant ID is not validated.
     */
    public String requiredOidcTenantId;

    /**
     * Allowed role IDs for users from this email domain.
     * If non-empty, users authenticating from this domain can only have roles from this list.
     * Any other roles will be filtered out during login/sync.
     * If empty, no role restrictions apply (all roles allowed).
     * Typically used for external IDPs with CLIENT or PARTNER scope to restrict what roles
     * external users can be assigned.
     */
    public List<String> allowedRoleIds = new ArrayList<>();

    /**
     * Whether to sync roles from the external IDP during OIDC login.
     * When true, roles from the IDP token are mapped to internal roles via idp_role_mappings.
     * When false (default), no role synchronization occurs during login.
     * Only relevant for OIDC identity providers.
     */
    public boolean syncRolesFromIdp = false;

    public boolean hasRoleRestrictions() {
        return allowedRoleIds != null && !allowedRoleIds.isEmpty();
    }

    public Instant createdAt = Instant.now();

    public Instant updatedAt = Instant.now();

    /**
     * Validate configuration constraints based on scope type.
     * @throws IllegalStateException if constraints are violated
     */
    public void validateScopeTypeConstraints() {
        if (scopeType == null) {
            throw new IllegalStateException("Scope type is required");
        }

        switch (scopeType) {
            case ANCHOR -> {
                if (primaryClientId != null) {
                    throw new IllegalStateException("ANCHOR scope cannot have a primary client");
                }
                if (additionalClientIds != null && !additionalClientIds.isEmpty()) {
                    throw new IllegalStateException("ANCHOR scope cannot have additional clients");
                }
                if (grantedClientIds != null && !grantedClientIds.isEmpty()) {
                    throw new IllegalStateException("ANCHOR scope cannot have granted clients");
                }
            }
            case PARTNER -> {
                if (primaryClientId != null) {
                    throw new IllegalStateException("PARTNER scope cannot have a primary client");
                }
                if (additionalClientIds != null && !additionalClientIds.isEmpty()) {
                    throw new IllegalStateException("PARTNER scope cannot have additional clients");
                }
                // grantedClientIds is allowed (can be empty or have values)
            }
            case CLIENT -> {
                if (primaryClientId == null) {
                    throw new IllegalStateException("CLIENT scope must have a primary client");
                }
                if (grantedClientIds != null && !grantedClientIds.isEmpty()) {
                    throw new IllegalStateException("CLIENT scope cannot have granted clients");
                }
                // additionalClientIds is allowed (can be empty or have values)
            }
        }
    }

    /**
     * Get all client IDs this mapping grants access to.
     * For CLIENT type: primary + additional clients
     * For PARTNER type: granted clients
     * For ANCHOR type: empty (users have access to all via scope)
     */
    public List<String> getAllAccessibleClientIds() {
        if (scopeType == null) {
            return List.of();
        }

        return switch (scopeType) {
            case ANCHOR -> List.of(); // Access determined by scope, not client list
            case PARTNER -> grantedClientIds != null ? List.copyOf(grantedClientIds) : List.of();
            case CLIENT -> {
                List<String> result = new ArrayList<>();
                if (primaryClientId != null) {
                    result.add(primaryClientId);
                }
                if (additionalClientIds != null) {
                    result.addAll(additionalClientIds);
                }
                yield List.copyOf(result);
            }
        };
    }
}
