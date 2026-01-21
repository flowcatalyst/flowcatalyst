package tech.flowcatalyst.platform.authentication;

/**
 * Authentication provider types for email domains.
 */
public enum AuthProvider {
    /**
     * Internal authentication using username/password stored in FlowCatalyst.
     * Users authenticate directly with credentials managed by the platform.
     */
    INTERNAL,

    /**
     * External OIDC (OpenID Connect) authentication.
     * Users are redirected to an external Identity Provider (e.g., Keycloak, Okta, Auth0).
     */
    OIDC
}
