package tech.flowcatalyst.platform.authentication;

import io.quarkus.runtime.annotations.StaticInitSafe;
import io.smallrye.config.ConfigMapping;
import io.smallrye.config.WithDefault;
import io.smallrye.config.WithName;

import java.time.Duration;
import java.util.Optional;

/**
 * Configuration for the FlowCatalyst authentication module.
 *
 * Example configuration:
 * <pre>
 * # Embedded mode (full IdP)
 * flowcatalyst.auth.mode=embedded
 * flowcatalyst.auth.jwt.issuer=https://auth.example.com
 * flowcatalyst.auth.jwt.private-key-path=/keys/private.pem
 *
 * # Remote mode (validation only)
 * flowcatalyst.auth.mode=remote
 * flowcatalyst.auth.remote.jwks-url=https://auth.example.com/.well-known/jwks.json
 * </pre>
 */
@StaticInitSafe
@ConfigMapping(prefix = "flowcatalyst.auth")
public interface AuthConfig {

    /**
     * Auth deployment mode.
     * - EMBEDDED: Full IdP with token issuance and management endpoints
     * - REMOTE: Token validation only, delegates to external IdP
     */
    @WithDefault("embedded")
    AuthMode mode();

    /**
     * JWT configuration for token issuance and validation.
     */
    JwtConfig jwt();

    /**
     * Session/cookie configuration.
     */
    SessionConfig session();

    /**
     * PKCE configuration for OAuth2 flows.
     */
    PkceConfig pkce();

    /**
     * Remote IdP configuration (used when mode=remote).
     */
    RemoteConfig remote();

    /**
     * External base URL for OAuth/OIDC callbacks.
     * Set this to the public URL where users access the platform.
     * In dev: http://localhost:4200
     * In prod: https://platform.example.com
     */
    @WithName("external-base-url")
    Optional<String> externalBaseUrl();

    /**
     * JWT configuration.
     */
    interface JwtConfig {
        /**
         * Token issuer (iss claim).
         * Should match the public URL of this auth service.
         */
        @WithDefault("flowcatalyst")
        String issuer();

        /**
         * Path to the RSA private key for signing tokens (PEM format).
         * Required in embedded mode.
         */
        @WithName("private-key-path")
        Optional<String> privateKeyPath();

        /**
         * Path to the RSA public key for validating tokens (PEM format).
         * Required in embedded mode.
         */
        @WithName("public-key-path")
        Optional<String> publicKeyPath();

        /**
         * Access token expiry duration.
         * Default: 1 hour
         */
        @WithName("access-token-expiry")
        @WithDefault("PT1H")
        Duration accessTokenExpiry();

        /**
         * Refresh token expiry duration.
         * Default: 30 days
         */
        @WithName("refresh-token-expiry")
        @WithDefault("P30D")
        Duration refreshTokenExpiry();

        /**
         * Session token expiry duration (for cookie-based sessions).
         * Default: 24 hours
         */
        @WithName("session-token-expiry")
        @WithDefault("PT24H")
        Duration sessionTokenExpiry();

        /**
         * Authorization code expiry duration.
         * Default: 10 minutes
         */
        @WithName("authorization-code-expiry")
        @WithDefault("PT10M")
        Duration authorizationCodeExpiry();
    }

    /**
     * Session configuration for cookie-based authentication.
     */
    interface SessionConfig {
        /**
         * Whether session cookies should be secure (HTTPS only).
         * Should be true in production.
         */
        @WithDefault("true")
        boolean secure();

        /**
         * SameSite attribute for session cookies.
         * Options: Strict, Lax, None
         */
        @WithName("same-site")
        @WithDefault("Lax")
        String sameSite();

        /**
         * Cookie name for session token.
         */
        @WithName("cookie-name")
        @WithDefault("fc_session")
        String cookieName();
    }

    /**
     * PKCE (Proof Key for Code Exchange) configuration.
     */
    interface PkceConfig {
        /**
         * Whether PKCE is required for all authorization code flows.
         * Strongly recommended for public clients (SPAs, mobile apps).
         */
        @WithDefault("true")
        boolean required();
    }

    /**
     * Remote IdP configuration for token validation.
     * Used when mode=remote.
     */
    interface RemoteConfig {
        /**
         * Expected issuer (iss claim) of tokens from the remote IdP.
         */
        Optional<String> issuer();

        /**
         * URL to fetch JWKS (JSON Web Key Set) from the remote IdP.
         * Example: https://auth.example.com/.well-known/jwks.json
         */
        @WithName("jwks-url")
        Optional<String> jwksUrl();

        /**
         * How long to cache the remote JWKS.
         * Default: 1 hour
         */
        @WithName("jwks-cache-duration")
        @WithDefault("PT1H")
        Duration jwksCacheDuration();

        /**
         * URL to redirect users to for login (in remote mode).
         */
        @WithName("login-url")
        Optional<String> loginUrl();

        /**
         * URL to redirect users to for logout (in remote mode).
         */
        @WithName("logout-url")
        Optional<String> logoutUrl();

        /**
         * Base URL of the remote platform service.
         * Used for redirecting to /platform in remote mode.
         */
        @WithName("platform-url")
        Optional<String> platformUrl();
    }
}
