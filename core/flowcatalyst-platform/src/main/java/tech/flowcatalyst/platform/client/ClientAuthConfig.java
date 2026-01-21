package tech.flowcatalyst.platform.client;

import tech.flowcatalyst.platform.authentication.AuthProvider;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;

/**
 * Authentication configuration per email domain.
 * Determines whether users from a specific domain authenticate via
 * INTERNAL (password) or OIDC (external IDP).
 *
 * <p>Each config has an {@link AuthConfigType} that determines user access scope:
 * <ul>
 *   <li>ANCHOR: Platform-wide access, users get ANCHOR scope (all clients)</li>
 *   <li>PARTNER: Partner access, users get PARTNER scope (only granted clients)</li>
 *   <li>CLIENT: Client-specific access, users get CLIENT scope (primary + additional clients)</li>
 * </ul>
 *
 * <p>Example:
 * <ul>
 *   <li>flowcatalyst.local (ANCHOR) -> INTERNAL (platform admins)</li>
 *   <li>acmecorp.com (CLIENT) -> INTERNAL (users bound to Acme Corp client)</li>
 *   <li>partner.com (PARTNER) -> OIDC (partner users with multi-client access)</li>
 * </ul>
 *
 * <p>IMPORTANT: The oidcClientSecretRef field stores a reference to the secret,
 * not the secret itself. Use ClientAuthConfigService to resolve the actual secret.
 */
public class ClientAuthConfig {

    public String id; // TSID (Crockford Base32)

    /**
     * The email domain this configuration applies to (e.g., "acmecorp.com")
     */
    public String emailDomain;

    /**
     * The type of this auth configuration, determining user access scope.
     * - ANCHOR: Platform-wide, no client associations allowed
     * - PARTNER: Partner IDP, has granted clients list
     * - CLIENT: Client-specific, has primary client + optional additional clients
     */
    public AuthConfigType configType;

    /**
     * The primary client this auth config belongs to.
     * Required for CLIENT type, must be null for ANCHOR and PARTNER types.
     *
     * @deprecated Use {@link #primaryClientId} instead. This field is kept for
     *             backwards compatibility during migration.
     */
    @Deprecated
    public String clientId;

    /**
     * The primary client this auth config belongs to.
     * Required for CLIENT type, must be null for ANCHOR and PARTNER types.
     */
    public String primaryClientId;

    /**
     * Additional client IDs for CLIENT type configurations.
     * Allows client-bound users to access additional clients as exceptions.
     * Must be empty for ANCHOR and PARTNER types.
     */
    public List<String> additionalClientIds = new ArrayList<>();

    /**
     * Granted client IDs for PARTNER type configurations.
     * Users authenticating through this config can access these clients.
     * Must be empty for ANCHOR and CLIENT types.
     */
    public List<String> grantedClientIds = new ArrayList<>();

    /**
     * Authentication provider type: INTERNAL or OIDC
     */
    public AuthProvider authProvider;

    /**
     * OIDC issuer URL (e.g., "https://auth.customer.com/realms/main")
     * For multi-tenant IDPs like Entra, use the generic issuer:
     * - https://login.microsoftonline.com/organizations/v2.0
     */
    public String oidcIssuerUrl;

    /**
     * OIDC client ID
     */
    public String oidcClientId;

    /**
     * Whether this is a multi-tenant OIDC configuration.
     * When true, the issuer in tokens will vary by tenant (e.g., Entra ID).
     * The actual token issuer will be validated against oidcAllowedIssuers or
     * dynamically constructed using oidcIssuerPattern.
     */
    public boolean oidcMultiTenant = false;

    /**
     * Pattern for validating multi-tenant issuers.
     * Use {tenantId} as placeholder for the tenant ID.
     * Example: "https://login.microsoftonline.com/{tenantId}/v2.0"
     * If not set, defaults to deriving from oidcIssuerUrl.
     */
    public String oidcIssuerPattern;

    /**
     * Reference to the OIDC client secret.
     * This is NOT the plaintext secret - it's a reference for the SecretService.
     * Format depends on configured provider:
     * - encrypted:BASE64_CIPHERTEXT (default)
     * - aws-sm://secret-name
     * - aws-ps://parameter-name
     * - gcp-sm://projects/PROJECT/secrets/NAME
     * - vault://path/to/secret#key
     */
    public String oidcClientSecretRef;

    public Instant createdAt = Instant.now();

    public Instant updatedAt = Instant.now();

    /**
     * Validate OIDC configuration if provider is OIDC.
     * @throws IllegalStateException if OIDC is configured but required fields are missing
     */
    public void validateOidcConfig() {
        if (authProvider == AuthProvider.OIDC) {
            if (oidcIssuerUrl == null || oidcIssuerUrl.isBlank()) {
                throw new IllegalStateException("OIDC issuer URL is required for OIDC auth provider");
            }
            if (oidcClientId == null || oidcClientId.isBlank()) {
                throw new IllegalStateException("OIDC client ID is required for OIDC auth provider");
            }
        }
    }

    /**
     * Validate configuration constraints based on config type.
     * @throws IllegalStateException if constraints are violated
     */
    public void validateConfigTypeConstraints() {
        if (configType == null) {
            throw new IllegalStateException("Config type is required");
        }

        // Get effective primary client ID (support both old and new field)
        String effectivePrimaryClientId = getEffectivePrimaryClientId();

        switch (configType) {
            case ANCHOR -> {
                if (effectivePrimaryClientId != null) {
                    throw new IllegalStateException("ANCHOR config cannot have a primary client");
                }
                if (additionalClientIds != null && !additionalClientIds.isEmpty()) {
                    throw new IllegalStateException("ANCHOR config cannot have additional clients");
                }
                if (grantedClientIds != null && !grantedClientIds.isEmpty()) {
                    throw new IllegalStateException("ANCHOR config cannot have granted clients");
                }
            }
            case PARTNER -> {
                if (effectivePrimaryClientId != null) {
                    throw new IllegalStateException("PARTNER config cannot have a primary client");
                }
                if (additionalClientIds != null && !additionalClientIds.isEmpty()) {
                    throw new IllegalStateException("PARTNER config cannot have additional clients");
                }
                // grantedClientIds is allowed (can be empty or have values)
            }
            case CLIENT -> {
                if (effectivePrimaryClientId == null) {
                    throw new IllegalStateException("CLIENT config must have a primary client");
                }
                if (grantedClientIds != null && !grantedClientIds.isEmpty()) {
                    throw new IllegalStateException("CLIENT config cannot have granted clients");
                }
                // additionalClientIds is allowed (can be empty or have values)
            }
        }
    }

    /**
     * Get the effective primary client ID, supporting both old clientId and new primaryClientId fields.
     * Prefers primaryClientId if set, falls back to clientId for backwards compatibility.
     */
    public String getEffectivePrimaryClientId() {
        if (primaryClientId != null) {
            return primaryClientId;
        }
        return clientId; // Backwards compatibility
    }

    /**
     * Get all client IDs this config grants access to.
     * For CLIENT type: primary + additional clients
     * For PARTNER type: granted clients
     * For ANCHOR type: empty (users have access to all via scope)
     */
    public List<String> getAllAccessibleClientIds() {
        if (configType == null) {
            // Backwards compatibility: derive from clientId
            if (clientId != null) {
                return List.of(clientId);
            }
            return List.of();
        }

        return switch (configType) {
            case ANCHOR -> List.of(); // Access determined by scope, not client list
            case PARTNER -> grantedClientIds != null ? List.copyOf(grantedClientIds) : List.of();
            case CLIENT -> {
                List<String> result = new ArrayList<>();
                String primary = getEffectivePrimaryClientId();
                if (primary != null) {
                    result.add(primary);
                }
                if (additionalClientIds != null) {
                    result.addAll(additionalClientIds);
                }
                yield List.copyOf(result);
            }
        };
    }

    /**
     * Get the effective config type, deriving from clientId if not explicitly set.
     * Used for backwards compatibility during migration.
     */
    public AuthConfigType getEffectiveConfigType() {
        if (configType != null) {
            return configType;
        }
        // Backwards compatibility: derive from clientId
        return clientId != null ? AuthConfigType.CLIENT : AuthConfigType.ANCHOR;
    }

    /**
     * Check if this config has a client secret configured.
     */
    public boolean hasClientSecret() {
        return oidcClientSecretRef != null && !oidcClientSecretRef.isBlank();
    }

    /**
     * Get the issuer pattern for multi-tenant validation.
     * Returns the explicit pattern if set, otherwise derives from oidcIssuerUrl.
     * For Entra: replaces /organizations/ or /common/ with /{tenantId}/
     */
    public String getEffectiveIssuerPattern() {
        if (oidcIssuerPattern != null && !oidcIssuerPattern.isBlank()) {
            return oidcIssuerPattern;
        }
        if (oidcIssuerUrl == null) {
            return null;
        }
        // Auto-derive pattern for common multi-tenant IDPs
        return oidcIssuerUrl
            .replace("/organizations/", "/{tenantId}/")
            .replace("/common/", "/{tenantId}/")
            .replace("/consumers/", "/{tenantId}/");
    }

    /**
     * Validate if a token issuer is valid for this configuration.
     * For single-tenant: must match oidcIssuerUrl exactly.
     * For multi-tenant: must match the issuer pattern with any tenant ID.
     *
     * @param tokenIssuer The issuer claim from the token
     * @return true if the issuer is valid
     */
    public boolean isValidIssuer(String tokenIssuer) {
        if (tokenIssuer == null || tokenIssuer.isBlank()) {
            return false;
        }

        if (!oidcMultiTenant) {
            // Single tenant: exact match
            return tokenIssuer.equals(oidcIssuerUrl);
        }

        // Multi-tenant: match against pattern
        String pattern = getEffectiveIssuerPattern();
        if (pattern == null) {
            return false;
        }

        // Convert pattern to regex: {tenantId} -> [a-zA-Z0-9-]+
        String regex = pattern
            .replace(".", "\\.")
            .replace("{tenantId}", "[a-zA-Z0-9-]+");

        return tokenIssuer.matches(regex);
    }
}
