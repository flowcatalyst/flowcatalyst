package tech.flowcatalyst.sdk.dto;

import tech.flowcatalyst.sdk.enums.EventTypeStatus;

import java.time.Instant;
import java.util.List;

/**
 * An event type definition.
 */
public record EventType(
    String id,
    String code,
    String name,
    String description,
    List<SpecVersion> specVersions,
    EventTypeStatus status,
    Instant createdAt,
    Instant updatedAt
) {}
