package tech.flowcatalyst.dispatchjob.security;

import jakarta.enterprise.context.ApplicationScoped;

import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;
import java.nio.charset.StandardCharsets;
import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.HexFormat;

/**
 * Service for signing webhook requests using HMAC-SHA256.
 *
 * <p>The signature is generated using the timestamp concatenated with the payload,
 * then signed with the signing secret. The receiver can verify by reproducing this signature.</p>
 */
@ApplicationScoped
public class WebhookSigner {

    public static final String SIGNATURE_HEADER = "X-FLOWCATALYST-SIGNATURE";
    public static final String TIMESTAMP_HEADER = "X-FLOWCATALYST-TIMESTAMP";
    private static final String ALGORITHM = "HmacSHA256";

    /**
     * Sign a webhook payload with the provided credentials.
     *
     * @param payload       The request body to sign
     * @param authToken     The bearer token for Authorization header
     * @param signingSecret The secret key for HMAC-SHA256 signing
     * @return A signed webhook request with signature, timestamp, and bearer token
     */
    public SignedWebhookRequest sign(String payload, String authToken, String signingSecret) {
        // Generate ISO8601 timestamp with millisecond precision
        String timestamp = Instant.now()
            .truncatedTo(ChronoUnit.MILLIS)
            .toString();

        // Create signature payload: timestamp + body
        String signaturePayload = timestamp + payload;

        // Generate HMAC SHA-256 signature
        String signature = generateHmacSha256(signaturePayload, signingSecret);

        return new SignedWebhookRequest(
            payload,
            signature,
            timestamp,
            authToken
        );
    }

    private String generateHmacSha256(String data, String secret) {
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
            throw new RuntimeException("Failed to generate webhook signature", e);
        }
    }

    public record SignedWebhookRequest(
        String payload,
        String signature,
        String timestamp,
        String bearerToken
    ) {
    }
}
