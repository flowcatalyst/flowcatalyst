package tech.flowcatalyst.serviceaccount.entity;

import tech.flowcatalyst.dispatchjob.model.SignatureAlgorithm;

import java.time.Instant;

/**
 * Webhook credentials embedded in ServiceAccount.
 *
 * <p>Used for authenticating and signing outbound webhook requests.</p>
 *
 * <h2>Authentication</h2>
 * <ul>
 *   <li><b>BEARER</b>: {@code Authorization: Bearer {authToken}}</li>
 *   <li><b>BASIC</b>: {@code Authorization: Basic {authToken}} where authToken is base64-encoded</li>
 * </ul>
 *
 * <h2>Signing</h2>
 * <p>Webhooks are signed using HMAC-SHA256:</p>
 * <ul>
 *   <li>Signature payload: {@code timestamp + body}</li>
 *   <li>Headers: {@code X-FLOWCATALYST-SIGNATURE}, {@code X-FLOWCATALYST-TIMESTAMP}</li>
 * </ul>
 */
public class WebhookCredentials {

    /**
     * Authentication type (NONE, BEARER_TOKEN, BASIC_AUTH, API_KEY, HMAC_SIGNATURE).
     */
    public WebhookAuthType authType = WebhookAuthType.BEARER_TOKEN;

    /**
     * The auth token used in Authorization header.
     * <ul>
     *   <li>For BEARER: Raw token (e.g., "fc_randomstring24chars")</li>
     *   <li>For BASIC: Base64-encoded "username:password" string</li>
     * </ul>
     *
     * <p>Stored encrypted using {@code encrypted:xxx} format.</p>
     * <p>User can replace with any custom value.</p>
     */
    public String authTokenRef;

    /**
     * Signing secret for HMAC-SHA256 webhook signatures.
     * Used to generate X-FLOWCATALYST-SIGNATURE header.
     *
     * <p>Auto-generated (32 random bytes, hex-encoded = 64 chars).</p>
     * <p>Stored encrypted using {@code encrypted:xxx} format.</p>
     */
    public String signingSecretRef;

    /**
     * Algorithm used for signing webhooks.
     */
    public SignatureAlgorithm signingAlgorithm = SignatureAlgorithm.HMAC_SHA256;

    /**
     * When credentials were first created.
     */
    public Instant createdAt;

    /**
     * When auth token or signing secret was last regenerated.
     */
    public Instant regeneratedAt;

    public WebhookCredentials() {
    }
}
