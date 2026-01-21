package tech.flowcatalyst.platform.application;

import java.time.Instant;
import java.util.Map;

/**
 * Per-client configuration for an application.
 *
 * Allows clients to have:
 * - Custom base URL (e.g., client1.operant.com instead of operant.com)
 * - Enabled/disabled status per application
 * - Custom configuration settings
 */
public class ApplicationClientConfig {

    public String id;

    public String applicationId;

    public String clientId;

    /**
     * Whether this client has access to this application.
     * Even if a user has roles for an app, the app must be enabled for their client.
     */
    public boolean enabled = true;

    /**
     * Client-specific URL override.
     * If set, this URL is used instead of the application's defaultBaseUrl.
     * Example: "client1.operant.com" instead of "operant.com"
     */
    public String baseUrlOverride;

    /**
     * Client-specific website URL override.
     * If set, this URL is used instead of the application's website.
     * Example: "client1.yardmanagement.com" instead of "www.yardmanagement.com"
     */
    public String websiteOverride;

    /**
     * Additional client-specific configuration as JSON.
     * Can include branding, feature flags, etc.
     */
    public Map<String, Object> configJson;

    public Instant createdAt = Instant.now();

    public Instant updatedAt = Instant.now();

    public ApplicationClientConfig() {
    }
}
