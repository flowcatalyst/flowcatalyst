package tech.flowcatalyst.serviceaccount.entity;

/**
 * Authentication type for webhook credentials.
 * Must match Rust's WebhookAuthType enum serialization (SCREAMING_SNAKE_CASE).
 */
public enum WebhookAuthType {
    /**
     * No authentication.
     */
    NONE,

    /**
     * Bearer token authentication.
     * Authorization header: Bearer {token}
     */
    BEARER_TOKEN,

    /**
     * Basic authentication.
     * Authorization header: Basic {base64-encoded-string}
     */
    BASIC_AUTH,

    /**
     * API key in header.
     */
    API_KEY,

    /**
     * HMAC signature.
     */
    HMAC_SIGNATURE
}
