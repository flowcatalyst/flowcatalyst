package tech.flowcatalyst.sdk.dto;

/**
 * A permission definition.
 */
public record Permission(
    String id,
    String name,
    String description,
    String applicationCode
) {}
