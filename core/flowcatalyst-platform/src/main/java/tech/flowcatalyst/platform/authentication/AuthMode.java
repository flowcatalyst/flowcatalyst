package tech.flowcatalyst.platform.authentication;

/**
 * Auth deployment mode for the platform.
 *
 * This determines whether the application runs as a full Identity Provider (IdP)
 * or delegates authentication to an external IdP.
 */
public enum AuthMode {
    /**
     * Full IdP mode - the application handles:
     * - Token issuance (JWT signing)
     * - User authentication (login endpoints)
     * - OAuth2/OIDC endpoints
     * - Tenant/user management
     * - Federation with external IDPs
     *
     * Use this mode when:
     * - Running as a standalone auth service
     * - Running an application with embedded auth
     */
    EMBEDDED,

    /**
     * Token validation only mode - the application:
     * - Validates tokens using remote JWKS
     * - Redirects auth requests to external IdP
     * - Does NOT issue tokens
     * - Does NOT expose auth management endpoints
     *
     * Use this mode when:
     * - Auth is handled by a separate FlowCatalyst auth service
     * - Using an external IdP (Keycloak, Auth0, etc.)
     */
    REMOTE
}
