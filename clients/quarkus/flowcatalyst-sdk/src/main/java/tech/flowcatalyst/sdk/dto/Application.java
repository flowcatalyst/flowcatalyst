package tech.flowcatalyst.sdk.dto;

import java.time.Instant;

/**
 * An application definition.
 */
public record Application(
    String id,
    String code,
    String name,
    String description,
    String defaultBaseUrl,
    String iconUrl,
    boolean active,
    Instant createdAt,
    Instant updatedAt
) {}
