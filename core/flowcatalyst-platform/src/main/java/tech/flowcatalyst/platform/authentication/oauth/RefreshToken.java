package tech.flowcatalyst.platform.authentication.oauth;

import java.time.Instant;

/**
 * Stores refresh tokens for long-lived sessions.
 *
 * Features:
 * - Token rotation: Each use issues a new refresh token
 * - Family tracking: All tokens in a family are invalidated on reuse detection
 * - Revocation: Tokens can be explicitly revoked
 *
 * Security: Only the token hash is stored, not the actual token.
 */

public class RefreshToken {

    /**
     * SHA-256 hash of the refresh token.
     * We store the hash, not the plain token, for security.
     */
    public String tokenHash;

    /**
     * The principal this token was issued for.
     */
    public String principalId;

    /**
     * OAuth client that requested this token.
     */
    public String clientId;

    /**
     * Client context for this token.
     */
    public String contextClientId;

    /**
     * Scopes granted with this token.
     */
    public String scope;

    /**
     * Token family for refresh token rotation.
     *
     * All tokens in a family are invalidated if reuse is detected
     * (i.e., an old token is used after a newer one was issued).
     * This protects against token theft.
     */
    public String tokenFamily;

    public Instant createdAt = Instant.now();

    public Instant expiresAt;

    /**
     * Whether this token has been revoked.
     */
    public boolean revoked = false;

    /**
     * When this token was revoked (null if not revoked).
     */
    public Instant revokedAt;

    /**
     * Hash of the token that replaced this one (for rotation tracking).
     */
    public String replacedBy;

    public boolean isExpired() {
        return Instant.now().isAfter(expiresAt);
    }

    public boolean isValid() {
        return !revoked && !isExpired();
    }
}
