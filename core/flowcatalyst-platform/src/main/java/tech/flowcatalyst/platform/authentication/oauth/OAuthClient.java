package tech.flowcatalyst.platform.authentication.oauth;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;

/**
 * OAuth2/OIDC client registration for SPAs, mobile apps, and service clients.
 *
 * <p>Supports two client types:
 * <ul>
 *   <li>PUBLIC: For SPAs and mobile apps (no secret, PKCE required)</li>
 *   <li>CONFIDENTIAL: For server-side apps (has encrypted secret)</li>
 * </ul>
 *
 * <p>OAuth clients can be associated with one or more applications. During authorization,
 * the system verifies that the authenticating user has access to at least one of the
 * associated applications. This enables:
 * <ul>
 *   <li>Single SSO client for multiple applications in one deployment</li>
 *   <li>Separate clients per environment (dev, staging, prod)</li>
 *   <li>Per-client domains via multiple redirect URIs</li>
 * </ul>
 *
 * @see tech.flowcatalyst.platform.application.Application
 */

public class OAuthClient {

    public String id;

    /**
     * Unique client identifier used in OAuth flows.
     * Stored as raw TSID (e.g., "0HZXEQ5Y8JY5Z").
     * External API adds "oauth_" prefix for display (e.g., "oauth_0HZXEQ5Y8JY5Z").
     */
    public String clientId;

    /**
     * Human-readable name for this client.
     * Example: "Production SSO", "Development SPA"
     */
    public String clientName;

    /**
     * Client type determines security requirements.
     */
    public ClientType clientType = ClientType.PUBLIC;

    /**
     * Reference to encrypted client secret (for CONFIDENTIAL clients).
     * Format: "encrypted:BASE64_CIPHERTEXT"
     * Use SecretService to encrypt/decrypt.
     * Null for PUBLIC clients.
     */
    public String clientSecretRef;

    /**
     * Allowed redirect URIs.
     * Must match exactly during authorization.
     * Supports multiple URIs for per-client domains or multiple environments.
     */
    public List<String> redirectUris = new ArrayList<>();

    /**
     * Allowed CORS origins for browser-based clients (SPAs).
     * Used for token endpoint and other OAuth API calls from browsers.
     * Example: ["https://app.example.com", "https://staging.example.com"]
     * If empty, CORS is not enabled for this client.
     */
    public List<String> allowedOrigins = new ArrayList<>();

    /**
     * Allowed grant types.
     * Examples: "authorization_code", "refresh_token", "client_credentials"
     */
    public List<String> grantTypes = new ArrayList<>();

    /**
     * Default scopes for this client.
     * Example: "openid profile email"
     */
    public String defaultScopes;

    /**
     * Whether PKCE is required for authorization code flow.
     * Always enforced for PUBLIC clients regardless of this setting.
     */
    public boolean pkceRequired = true;

    /**
     * Application IDs this OAuth client can authenticate for (user-facing clients only).
     * Users must have access to at least one of these applications
     * to authenticate through this client.
     * Empty list means no application restriction (legacy behavior).
     * Must be empty if serviceAccountPrincipalId is set.
     */
    public List<String> applicationIds = new ArrayList<>();

    /**
     * Service account principal ID this OAuth client belongs to.
     * When set, this is a service account client that:
     * - Can only use client_credentials grant
     * - Cannot have applicationIds (mutual exclusion)
     * - Authenticates as the linked service account principal
     * Null for user-facing application OAuth clients.
     */
    public String serviceAccountPrincipalId;

    /**
     * Whether this client is active.
     */
    public boolean active = true;

    public Instant createdAt = Instant.now();

    public Instant updatedAt = Instant.now();

    public OAuthClient() {
    }

    /**
     * Check if a redirect URI is allowed for this client.
     */
    public boolean isRedirectUriAllowed(String uri) {
        if (redirectUris == null || uri == null) {
            return false;
        }
        return redirectUris.contains(uri);
    }

    /**
     * Check if a CORS origin is allowed for this client.
     */
    public boolean isOriginAllowed(String origin) {
        if (allowedOrigins == null || origin == null) {
            return false;
        }
        return allowedOrigins.contains(origin);
    }

    /**
     * Check if a grant type is allowed for this client.
     */
    public boolean isGrantTypeAllowed(String grantType) {
        if (grantTypes == null || grantType == null) {
            return false;
        }
        return grantTypes.contains(grantType);
    }

    /**
     * Check if this client is associated with any applications.
     */
    public boolean hasApplicationRestrictions() {
        return applicationIds != null && !applicationIds.isEmpty();
    }

    /**
     * Check if this client is associated with a specific application.
     */
    public boolean isAssociatedWithApplication(String applicationId) {
        if (applicationIds == null || applicationId == null) {
            return false;
        }
        return applicationIds.contains(applicationId);
    }

    /**
     * Check if this is a public client (no secret).
     */
    public boolean isPublic() {
        return clientType == ClientType.PUBLIC;
    }

    /**
     * Check if this is a confidential client (has secret).
     */
    public boolean isConfidential() {
        return clientType == ClientType.CONFIDENTIAL;
    }

    /**
     * Check if this is a service account client (for machine-to-machine auth).
     */
    public boolean isServiceAccountClient() {
        return serviceAccountPrincipalId != null;
    }

    /**
     * Check if this is a user-facing application client.
     */
    public boolean isApplicationClient() {
        return serviceAccountPrincipalId == null;
    }

    /**
     * OAuth client type.
     */
    public enum ClientType {
        /**
         * Public client (SPA, mobile app).
         * No client secret, PKCE required.
         */
        PUBLIC,

        /**
         * Confidential client (server-side app).
         * Has client secret.
         */
        CONFIDENTIAL
    }
}
