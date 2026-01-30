package tech.flowcatalyst.platform.authentication.domain;

import tech.flowcatalyst.platform.principal.UserScope;

/**
 * Scope type for email domain mappings, determining user access scope.
 *
 * <p>Used in {@link EmailDomainMapping} to define what access scope
 * users authenticating from a given email domain receive.
 */
public enum ScopeType {
    /**
     * Platform-wide/anchor scope.
     * Users authenticating from domains with this scope get ANCHOR access (all clients).
     * Cannot have any client associations.
     */
    ANCHOR,

    /**
     * Partner scope.
     * Users authenticating from domains with this scope get PARTNER access.
     * Has explicitly granted client IDs stored on the mapping.
     */
    PARTNER,

    /**
     * Client-specific scope.
     * Users authenticating from domains with this scope get CLIENT access.
     * Must have a primary client, optionally with additional client exceptions.
     */
    CLIENT;

    /**
     * Convert this scope type to the corresponding user scope.
     */
    public UserScope toUserScope() {
        return switch (this) {
            case ANCHOR -> UserScope.ANCHOR;
            case PARTNER -> UserScope.PARTNER;
            case CLIENT -> UserScope.CLIENT;
        };
    }
}
