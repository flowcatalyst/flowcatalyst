package tech.flowcatalyst.platform.authentication.oauth;

import java.time.Instant;

/**
 * Stores authorization codes for the OAuth2 authorization code flow.
 *
 * Authorization codes are:
 * - Short-lived (default: 10 minutes)
 * - Single-use (marked as used after exchange)
 * - Bound to client, redirect URI, and PKCE challenge
 */

public class AuthorizationCode {

    /**
     * MongoDB document ID.
     */
    public String id;

    /**
     * The authorization code value (64 char random string).
     * Has a unique index for lookups.
     */
    public String code;

    /**
     * OAuth client that initiated this authorization.
     */
    public String clientId;

    /**
     * The authenticated principal.
     */
    public String principalId;

    /**
     * Redirect URI used in the authorization request.
     * Must match exactly during token exchange.
     */
    public String redirectUri;

    /**
     * Requested scopes.
     */
    public String scope;

    /**
     * PKCE code challenge (required for public clients).
     */
    public String codeChallenge;

    /**
     * PKCE challenge method (S256 or plain).
     */
    public String codeChallengeMethod;

    /**
     * OIDC nonce for replay protection.
     */
    public String nonce;

    /**
     * Client-provided state for CSRF protection.
     */
    public String state;

    /**
     * Client context for the authorization.
     */
    public String contextClientId;

    public Instant createdAt = Instant.now();

    public Instant expiresAt;

    /**
     * Whether this code has been used (single-use enforcement).
     */
    public boolean used = false;

    public boolean isExpired() {
        return Instant.now().isAfter(expiresAt);
    }

    public boolean isValid() {
        return !used && !isExpired();
    }
}
