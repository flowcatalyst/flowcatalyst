package tech.flowcatalyst.platform.client;

import tech.flowcatalyst.platform.principal.UserScope;

/**
 * Type of authentication configuration, determining user scope and access.
 *
 * This is distinct from AuthProvider (INTERNAL vs OIDC) which determines
 * the authentication mechanism. AuthConfigType determines the access scope
 * of users authenticating through this configuration.
 */
public enum AuthConfigType {
    /**
     * Platform-wide/anchor configuration.
     * Users authenticating through this config get ANCHOR scope (access all clients).
     * Cannot have any client associations.
     */
    ANCHOR,

    /**
     * Partner configuration.
     * Users authenticating through this config get PARTNER scope.
     * Has explicitly granted client IDs stored on the config.
     * Users can only access the granted clients.
     */
    PARTNER,

    /**
     * Client-specific configuration.
     * Users authenticating through this config get CLIENT scope.
     * Must have a primary client, optionally with additional client exceptions.
     */
    CLIENT;

    /**
     * Convert this auth config type to the corresponding user scope.
     */
    public UserScope toUserScope() {
        return switch (this) {
            case ANCHOR -> UserScope.ANCHOR;
            case PARTNER -> UserScope.PARTNER;
            case CLIENT -> UserScope.CLIENT;
        };
    }
}
