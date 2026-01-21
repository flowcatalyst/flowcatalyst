package tech.flowcatalyst.platform.authentication.oidc;

import java.time.Instant;

/**
 * Stores OIDC login state for the authorization code flow.
 * Used to correlate the callback with the original login request
 * and prevent CSRF attacks.
 */

public class OidcLoginState {

    public String state; // Random state parameter (also used as ID)

    /**
     * The email domain that initiated this login
     */
    public String emailDomain;

    /**
     * The client auth config ID used for this login
     */
    public String authConfigId;

    /**
     * Nonce for ID token validation (prevents replay attacks)
     */
    public String nonce;

    /**
     * PKCE code verifier (we generate and store it, send challenge to IDP)
     */
    public String codeVerifier;

    /**
     * Where to redirect after successful login
     */
    public String returnUrl;

    /**
     * Original OAuth parameters if this login is part of an OAuth flow
     * (i.e., a client app initiated login via /oauth/authorize)
     */
    public String oauthClientId;
    public String oauthRedirectUri;
    public String oauthScope;
    public String oauthState;
    public String oauthCodeChallenge;
    public String oauthCodeChallengeMethod;
    public String oauthNonce;

    public Instant createdAt = Instant.now();

    /**
     * State expires after 10 minutes
     */
    public Instant expiresAt = Instant.now().plusSeconds(600);

    public boolean isExpired() {
        return Instant.now().isAfter(expiresAt);
    }
}
