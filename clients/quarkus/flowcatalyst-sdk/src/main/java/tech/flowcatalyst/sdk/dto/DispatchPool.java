package tech.flowcatalyst.sdk.dto;

import tech.flowcatalyst.sdk.enums.DispatchPoolStatus;

import java.time.Instant;

/**
 * A dispatch pool for rate limiting webhooks.
 */
public record DispatchPool(
    String id,
    String code,
    String name,
    String description,
    int rateLimit,
    int concurrency,
    String clientId,
    String clientIdentifier,
    DispatchPoolStatus status,
    Instant createdAt,
    Instant updatedAt
) {}
