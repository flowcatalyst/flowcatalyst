package tech.flowcatalyst.platform.authentication.idp;

/**
 * Type of identity provider.
 */
public enum IdentityProviderType {
    /**
     * Internal authentication using username/password stored in the platform.
     */
    INTERNAL,

    /**
     * External OIDC identity provider (e.g., Okta, Entra ID, Keycloak).
     */
    OIDC
}
