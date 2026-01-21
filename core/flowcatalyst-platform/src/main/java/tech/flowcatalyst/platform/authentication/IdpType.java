package tech.flowcatalyst.platform.authentication;

/**
 * Type of Identity Provider used for authentication.
 */
public enum IdpType {
    /**
     * Internal username/password authentication
     */
    INTERNAL,

    /**
     * OIDC authentication (e.g., Keycloak)
     */
    OIDC
}
