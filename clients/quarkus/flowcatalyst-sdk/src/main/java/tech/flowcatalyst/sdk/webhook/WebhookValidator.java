package tech.flowcatalyst.sdk.webhook;

import tech.flowcatalyst.sdk.exception.WebhookValidationException;

import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.time.Instant;
import java.util.HexFormat;

/**
 * Validates incoming webhook signatures from FlowCatalyst using HMAC-SHA256.
 *
 * <p>Example usage:
 * <pre>{@code
 * WebhookValidator validator = new WebhookValidator(signingSecret);
 *
 * // Validate manually
 * validator.validate(requestBody, signature, timestamp);
 *
 * // Or with JAX-RS context
 * validator.validateRequest(httpHeaders, requestBody);
 * }</pre>
 */
public class WebhookValidator {

    private static final String SIGNATURE_HEADER = "X-FlowCatalyst-Signature";
    private static final String TIMESTAMP_HEADER = "X-FlowCatalyst-Timestamp";
    private static final int DEFAULT_TOLERANCE_SECONDS = 300; // 5 minutes
    private static final int FUTURE_GRACE_SECONDS = 60;

    private final String signingSecret;

    public WebhookValidator(String signingSecret) {
        if (signingSecret == null || signingSecret.isEmpty()) {
            throw WebhookValidationException.missingSigningSecret();
        }
        this.signingSecret = signingSecret;
    }

    /**
     * Validate a webhook signature.
     *
     * @param payload   Raw request body
     * @param signature Value of X-FlowCatalyst-Signature header
     * @param timestamp Value of X-FlowCatalyst-Timestamp header
     * @return true if validation succeeds
     * @throws WebhookValidationException if validation fails
     */
    public boolean validate(String payload, String signature, String timestamp) {
        return validate(payload, signature, timestamp, DEFAULT_TOLERANCE_SECONDS);
    }

    /**
     * Validate a webhook signature with custom tolerance.
     *
     * @param payload   Raw request body
     * @param signature Value of X-FlowCatalyst-Signature header
     * @param timestamp Value of X-FlowCatalyst-Timestamp header
     * @param tolerance Max age in seconds
     * @return true if validation succeeds
     * @throws WebhookValidationException if validation fails
     */
    public boolean validate(String payload, String signature, String timestamp, int tolerance) {
        // Validate timestamp
        validateTimestamp(timestamp, tolerance);

        // Compute expected signature
        String message = timestamp + payload;
        String expectedSignature = computeHmacSha256(message, signingSecret);

        // Constant-time comparison to prevent timing attacks
        if (!MessageDigest.isEqual(
            expectedSignature.getBytes(StandardCharsets.UTF_8),
            signature.getBytes(StandardCharsets.UTF_8)
        )) {
            throw WebhookValidationException.invalidSignature();
        }

        return true;
    }

    /**
     * Validate a webhook using JAX-RS HttpHeaders.
     *
     * @param headers     The HTTP headers (jakarta.ws.rs.core.HttpHeaders)
     * @param requestBody The raw request body
     * @return true if validation succeeds
     * @throws WebhookValidationException if validation fails
     */
    public boolean validateRequest(jakarta.ws.rs.core.HttpHeaders headers, String requestBody) {
        return validateRequest(headers, requestBody, DEFAULT_TOLERANCE_SECONDS);
    }

    /**
     * Validate a webhook using JAX-RS HttpHeaders with custom tolerance.
     *
     * @param headers     The HTTP headers (jakarta.ws.rs.core.HttpHeaders)
     * @param requestBody The raw request body
     * @param tolerance   Max age in seconds
     * @return true if validation succeeds
     * @throws WebhookValidationException if validation fails
     */
    public boolean validateRequest(jakarta.ws.rs.core.HttpHeaders headers, String requestBody, int tolerance) {
        String signature = headers.getHeaderString(SIGNATURE_HEADER);
        String timestamp = headers.getHeaderString(TIMESTAMP_HEADER);

        if (signature == null || signature.isEmpty()) {
            throw WebhookValidationException.missingSignature();
        }

        if (timestamp == null || timestamp.isEmpty()) {
            throw WebhookValidationException.missingTimestamp();
        }

        return validate(requestBody, signature, timestamp, tolerance);
    }

    /**
     * Validate the timestamp is within tolerance.
     */
    private void validateTimestamp(String timestamp, int tolerance) {
        long webhookTime;
        try {
            webhookTime = Long.parseLong(timestamp);
        } catch (NumberFormatException e) {
            throw WebhookValidationException.invalidSignature();
        }

        long currentTime = Instant.now().getEpochSecond();

        // Check if timestamp is too old
        if (webhookTime < (currentTime - tolerance)) {
            throw WebhookValidationException.timestampExpired(tolerance);
        }

        // Check if timestamp is in the future (with grace period)
        if (webhookTime > (currentTime + FUTURE_GRACE_SECONDS)) {
            throw WebhookValidationException.timestampInFuture();
        }
    }

    /**
     * Compute HMAC-SHA256 signature.
     */
    private String computeHmacSha256(String message, String secret) {
        try {
            Mac mac = Mac.getInstance("HmacSHA256");
            SecretKeySpec secretKeySpec = new SecretKeySpec(
                secret.getBytes(StandardCharsets.UTF_8),
                "HmacSHA256"
            );
            mac.init(secretKeySpec);
            byte[] hmacBytes = mac.doFinal(message.getBytes(StandardCharsets.UTF_8));
            return HexFormat.of().formatHex(hmacBytes);
        } catch (Exception e) {
            throw new RuntimeException("Failed to compute HMAC-SHA256", e);
        }
    }

    /**
     * Get the signature header name.
     */
    public static String signatureHeader() {
        return SIGNATURE_HEADER;
    }

    /**
     * Get the timestamp header name.
     */
    public static String timestampHeader() {
        return TIMESTAMP_HEADER;
    }
}
