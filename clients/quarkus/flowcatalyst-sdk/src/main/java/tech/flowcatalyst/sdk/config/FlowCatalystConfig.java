package tech.flowcatalyst.sdk.config;

import io.smallrye.config.ConfigMapping;
import io.smallrye.config.WithDefault;
import io.smallrye.config.WithName;

import java.util.Optional;

/**
 * Configuration for the FlowCatalyst SDK.
 *
 * <p>Configure in application.properties:
 * <pre>
 * flowcatalyst.base-url=https://your-instance.flowcatalyst.io
 * flowcatalyst.client-id=your_client_id
 * flowcatalyst.client-secret=your_client_secret
 * flowcatalyst.signing-secret=your_signing_secret
 * </pre>
 */
@ConfigMapping(prefix = "flowcatalyst")
public interface FlowCatalystConfig {

    /**
     * Base URL for the FlowCatalyst API.
     */
    @WithName("base-url")
    @WithDefault("https://api.flowcatalyst.io")
    String baseUrl();

    /**
     * OAuth2 client ID for authentication.
     */
    @WithName("client-id")
    Optional<String> clientId();

    /**
     * OAuth2 client secret for authentication.
     */
    @WithName("client-secret")
    Optional<String> clientSecret();

    /**
     * OAuth2 token endpoint. Defaults to {base-url}/oauth/token.
     */
    @WithName("token-url")
    Optional<String> tokenUrl();

    /**
     * Webhook signing secret for validating incoming webhooks.
     */
    @WithName("signing-secret")
    Optional<String> signingSecret();

    /**
     * HTTP client configuration.
     */
    HttpConfig http();

    /**
     * Outbox configuration.
     */
    OutboxConfig outbox();

    interface HttpConfig {
        /**
         * Request timeout in seconds.
         */
        @WithDefault("30")
        int timeout();

        /**
         * Number of retry attempts for failed requests.
         */
        @WithName("retry-attempts")
        @WithDefault("3")
        int retryAttempts();

        /**
         * Delay between retries in milliseconds.
         */
        @WithName("retry-delay")
        @WithDefault("100")
        int retryDelay();
    }

    interface OutboxConfig {
        /**
         * Enable or disable the outbox functionality.
         */
        @WithDefault("true")
        boolean enabled();

        /**
         * Tenant ID for outbox messages.
         */
        @WithName("tenant-id")
        Optional<String> tenantId();

        /**
         * Default partition for outbox messages.
         */
        @WithName("default-partition")
        @WithDefault("default")
        String defaultPartition();

        /**
         * MongoDB collection name for outbox messages.
         */
        @WithDefault("outbox_messages")
        String collection();
    }
}
