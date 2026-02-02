package tech.flowcatalyst.platform.authentication.idp;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;

/**
 * Identity Provider configuration.
 *
 * <p>Defines how users authenticate - either via internal password authentication
 * or through an external OIDC identity provider.
 *
 * <p>Key concepts:
 * <ul>
 *   <li>Each IDP has a unique code for identification</li>
 *   <li>OIDC IDPs require issuer URL, client ID, and client secret</li>
 *   <li>Multi-tenant OIDC (e.g., Entra ID) uses issuer patterns for validation</li>
 *   <li>allowedEmailDomains restricts which domains can authenticate through this IDP</li>
 * </ul>
 *
 * <p>IMPORTANT: The oidcClientSecretRef field stores a reference to the secret,
 * not the secret itself. Use IdentityProviderService to resolve the actual secret.
 */
public class IdentityProvider {

    public String id; // TSID (Crockford Base32)

    /**
     * Unique code identifier for this IDP (e.g., "internal", "okta-acme", "entra-main").
     */
    public String code;

    /**
     * Human-readable name for this IDP.
     */
    public String name;

    /**
     * Type of identity provider: INTERNAL or OIDC.
     */
    public IdentityProviderType type;

    /**
     * OIDC issuer URL (e.g., "https://auth.customer.com/realms/main").
     * For multi-tenant IDPs like Entra, use the generic issuer:
     * - https://login.microsoftonline.com/organizations/v2.0
     */
    public String oidcIssuerUrl;

    /**
     * OIDC client ID.
     */
    public String oidcClientId;

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

    /**
     * Optional explicit JWKS URI for fetching public keys.
     * If not set, derived from oidcIssuerUrl via OpenID Discovery.
     */
    public String oidcJwksUri;

    /**
     * Whether this is a multi-tenant OIDC configuration.
     * When true, the issuer in tokens will vary by tenant (e.g., Entra ID).
     * The actual token issuer will be validated against oidcIssuerPattern.
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
     * Email domains allowed to authenticate through this IDP.
     * If empty, all domains are allowed.
     * This provides a security constraint to prevent unauthorized domains
     * from using this IDP's credentials.
     */
    public List<String> allowedEmailDomains = new ArrayList<>();

    public Instant createdAt = Instant.now();

    public Instant updatedAt = Instant.now();

    /**
     * Validate OIDC configuration if provider is OIDC.
     * @throws IllegalStateException if OIDC is configured but required fields are missing
     */
    public void validateOidcConfig() {
        if (type == IdentityProviderType.OIDC) {
            if (oidcIssuerUrl == null || oidcIssuerUrl.isBlank()) {
                throw new IllegalStateException("OIDC issuer URL is required for OIDC identity provider");
            }
            if (oidcClientId == null || oidcClientId.isBlank()) {
                throw new IllegalStateException("OIDC client ID is required for OIDC identity provider");
            }
        }
    }

    /**
     * Check if this IDP has a client secret configured.
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
     * Validate if a token issuer is valid for this identity provider.
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

    /**
     * Check if an email domain is allowed to authenticate through this IDP.
     * If allowedEmailDomains is empty, all domains are allowed.
     *
     * @param emailDomain The email domain to check
     * @return true if the domain is allowed
     */
    public boolean isEmailDomainAllowed(String emailDomain) {
        if (allowedEmailDomains == null || allowedEmailDomains.isEmpty()) {
            return true; // No restriction
        }
        return allowedEmailDomains.contains(emailDomain.toLowerCase());
    }
}
