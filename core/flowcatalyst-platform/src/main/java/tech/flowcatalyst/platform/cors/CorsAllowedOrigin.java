package tech.flowcatalyst.platform.cors;

import java.time.Instant;

/**
 * Stores allowed CORS origins for API requests.
 *
 * Managed by platform administrators under Identity & Access.
 * Origins are cached in memory for performance.
 */
public class CorsAllowedOrigin {

    public String id;

    /**
     * The allowed origin (e.g., "https://app.example.com").
     * Must include protocol. Wildcards not supported for security.
     */
    public String origin;

    /**
     * Human-readable description (e.g., "Production SPA", "Development").
     */
    public String description;

    /**
     * Who created this entry.
     */
    public String createdBy;

    public Instant createdAt = Instant.now();
}
