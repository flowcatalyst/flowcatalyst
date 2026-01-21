package tech.flowcatalyst.sdk.dto;

import tech.flowcatalyst.sdk.enums.SchemaType;
import tech.flowcatalyst.sdk.enums.SpecVersionStatus;

import java.time.Instant;

/**
 * A schema spec version for an event type.
 */
public record SpecVersion(
    String version,
    String mimeType,
    SchemaType schemaType,
    String schemaId,
    SpecVersionStatus status,
    Instant createdAt
) {}
