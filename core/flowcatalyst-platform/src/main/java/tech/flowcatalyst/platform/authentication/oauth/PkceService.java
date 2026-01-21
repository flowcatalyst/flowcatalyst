package tech.flowcatalyst.platform.authentication.oauth;

import jakarta.enterprise.context.ApplicationScoped;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.security.SecureRandom;
import java.util.Base64;

/**
 * PKCE (Proof Key for Code Exchange) implementation.
 *
 * PKCE protects the authorization code flow against interception attacks,
 * especially important for public clients (SPAs, mobile apps) that cannot
 * securely store a client secret.
 *
 * Flow:
 * 1. Client generates random code_verifier
 * 2. Client computes code_challenge = SHA256(code_verifier)
 * 3. Client sends code_challenge in authorization request
 * 4. Server stores code_challenge with authorization code
 * 5. Client sends code_verifier in token request
 * 6. Server verifies SHA256(code_verifier) == stored code_challenge
 *
 * @see <a href="https://datatracker.ietf.org/doc/html/rfc7636">RFC 7636 - PKCE</a>
 */
@ApplicationScoped
public class PkceService {

    private static final SecureRandom SECURE_RANDOM = new SecureRandom();

    /**
     * Generate a cryptographically random code verifier.
     *
     * Per RFC 7636, the verifier must be:
     * - Between 43 and 128 characters
     * - Composed of unreserved URI characters [A-Za-z0-9-._~]
     *
     * We generate 48 random bytes which encode to 64 base64url characters.
     */
    public String generateCodeVerifier() {
        byte[] bytes = new byte[48];
        SECURE_RANDOM.nextBytes(bytes);
        return Base64.getUrlEncoder().withoutPadding().encodeToString(bytes);
    }

    /**
     * Generate a code challenge from a verifier using the S256 method.
     *
     * code_challenge = BASE64URL(SHA256(code_verifier))
     *
     * @param codeVerifier The code verifier to hash
     * @return The code challenge (base64url encoded SHA-256 hash)
     */
    public String generateCodeChallenge(String codeVerifier) {
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            byte[] hash = digest.digest(codeVerifier.getBytes(StandardCharsets.US_ASCII));
            return Base64.getUrlEncoder().withoutPadding().encodeToString(hash);
        } catch (NoSuchAlgorithmException e) {
            throw new RuntimeException("SHA-256 not available", e);
        }
    }

    /**
     * Verify that a code verifier matches the stored code challenge.
     *
     * @param codeVerifier The verifier provided in the token request
     * @param codeChallenge The challenge stored with the authorization code
     * @param method Challenge method ("plain" or "S256", defaults to S256)
     * @return true if the verifier matches the challenge
     */
    public boolean verifyCodeChallenge(String codeVerifier, String codeChallenge, String method) {
        if (codeVerifier == null || codeChallenge == null) {
            return false;
        }

        // S256 is the default and recommended method
        if (method == null || "S256".equalsIgnoreCase(method)) {
            String computed = generateCodeChallenge(codeVerifier);
            return constantTimeEquals(computed, codeChallenge);
        }

        // Plain method (not recommended, but allowed by spec)
        if ("plain".equalsIgnoreCase(method)) {
            return constantTimeEquals(codeVerifier, codeChallenge);
        }

        // Unknown method
        return false;
    }

    /**
     * Validate code challenge format.
     *
     * @param codeChallenge The challenge to validate
     * @return true if valid format
     */
    public boolean isValidCodeChallenge(String codeChallenge) {
        if (codeChallenge == null) {
            return false;
        }
        // S256 produces 43 characters (32 bytes base64url encoded)
        // Allow some flexibility for plain method
        return codeChallenge.length() >= 43 && codeChallenge.length() <= 128;
    }

    /**
     * Validate code verifier format.
     *
     * @param codeVerifier The verifier to validate
     * @return true if valid format
     */
    public boolean isValidCodeVerifier(String codeVerifier) {
        if (codeVerifier == null) {
            return false;
        }
        // Per RFC 7636: 43-128 characters, unreserved characters only
        if (codeVerifier.length() < 43 || codeVerifier.length() > 128) {
            return false;
        }
        // Check for valid characters (unreserved URI characters)
        return codeVerifier.matches("^[A-Za-z0-9\\-._~]+$");
    }

    /**
     * Constant-time string comparison to prevent timing attacks.
     */
    private boolean constantTimeEquals(String a, String b) {
        if (a == null || b == null) {
            return false;
        }
        if (a.length() != b.length()) {
            return false;
        }
        int result = 0;
        for (int i = 0; i < a.length(); i++) {
            result |= a.charAt(i) ^ b.charAt(i);
        }
        return result == 0;
    }
}
