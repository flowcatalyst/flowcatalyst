package tech.flowcatalyst.dispatchjob.security;

import jakarta.enterprise.context.ApplicationScoped;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import org.jboss.logging.Logger;

import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.HexFormat;
import java.util.Optional;

/**
 * Service for generating and validating HMAC-SHA256 auth tokens for dispatch job processing.
 *
 * <p>This implements the authentication flow between the platform and message router:</p>
 * <ol>
 *   <li>Platform creates a dispatch job and generates an HMAC token using the app key</li>
 *   <li>Platform sends the job to SQS with the token in the MessagePointer</li>
 *   <li>Message router receives the message and calls back to platform with the same token</li>
 *   <li>Platform validates the token by re-computing the HMAC and comparing</li>
 * </ol>
 *
 * <p>The token is computed as: HMAC-SHA256(dispatchJobId, appKey)</p>
 */
@ApplicationScoped
public class DispatchAuthService {

    private static final Logger LOG = Logger.getLogger(DispatchAuthService.class);
    private static final String ALGORITHM = "HmacSHA256";

    @ConfigProperty(name = "flowcatalyst.app-key")
    Optional<String> appKey;

    /**
     * Generate an HMAC-SHA256 auth token for a dispatch job ID.
     *
     * @param dispatchJobId The dispatch job ID to generate a token for
     * @return The hex-encoded HMAC-SHA256 token
     * @throws IllegalStateException if the app key is not configured
     */
    public String generateAuthToken(String dispatchJobId) {
        String key = appKey.orElseThrow(() ->
            new IllegalStateException("flowcatalyst.app-key is not configured. Cannot generate auth token."));

        return hmacSha256Hex(dispatchJobId, key);
    }

    /**
     * Validate an auth token from the message router.
     *
     * @param dispatchJobId The dispatch job ID from the request
     * @param token The token from the Authorization header
     * @return true if the token is valid, false otherwise
     */
    public boolean validateAuthToken(String dispatchJobId, String token) {
        if (token == null || token.isBlank() || dispatchJobId == null || dispatchJobId.isBlank()) {
            return false;
        }

        if (appKey.isEmpty()) {
            LOG.error("flowcatalyst.app-key is not configured. Cannot validate auth token.");
            return false;
        }

        try {
            String expected = generateAuthToken(dispatchJobId);
            // Use constant-time comparison to prevent timing attacks
            return MessageDigest.isEqual(
                expected.getBytes(StandardCharsets.UTF_8),
                token.getBytes(StandardCharsets.UTF_8)
            );
        } catch (Exception e) {
            LOG.errorf(e, "Error validating auth token for dispatch job [%s]", dispatchJobId);
            return false;
        }
    }

    /**
     * Check if the app key is configured.
     */
    public boolean isConfigured() {
        return appKey.isPresent() && !appKey.get().isBlank();
    }

    private String hmacSha256Hex(String data, String secret) {
        try {
            Mac mac = Mac.getInstance(ALGORITHM);
            SecretKeySpec secretKeySpec = new SecretKeySpec(
                secret.getBytes(StandardCharsets.UTF_8),
                ALGORITHM
            );
            mac.init(secretKeySpec);

            byte[] hash = mac.doFinal(data.getBytes(StandardCharsets.UTF_8));

            // Return as hex string (lowercase)
            return HexFormat.of().formatHex(hash);

        } catch (Exception e) {
            throw new RuntimeException("Failed to generate HMAC-SHA256", e);
        }
    }
}
