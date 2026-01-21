package tech.flowcatalyst.sdk.dto;

import tech.flowcatalyst.sdk.enums.DispatchMode;
import tech.flowcatalyst.sdk.enums.SubscriptionSource;
import tech.flowcatalyst.sdk.enums.SubscriptionStatus;

import java.time.Instant;
import java.util.List;

/**
 * A subscription that routes events to a target endpoint.
 */
public record Subscription(
    String id,
    String code,
    String name,
    String description,
    String clientId,
    String clientIdentifier,
    List<EventTypeBinding> eventTypes,
    String target,
    String queue,
    SubscriptionSource source,
    SubscriptionStatus status,
    int maxAgeSeconds,
    String dispatchPoolId,
    String dispatchPoolCode,
    int delaySeconds,
    int sequence,
    DispatchMode mode,
    int timeoutSeconds,
    int maxRetries,
    String serviceAccountId,
    boolean dataOnly,
    Instant createdAt,
    Instant updatedAt
) {}
