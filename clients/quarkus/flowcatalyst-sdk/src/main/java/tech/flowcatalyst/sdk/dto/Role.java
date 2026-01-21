package tech.flowcatalyst.sdk.dto;

import java.util.Set;

/**
 * A role definition.
 */
public record Role(
    String id,
    String name,
    String displayName,
    String description,
    String applicationId,
    String applicationCode,
    Set<String> permissions,
    String source,
    boolean clientManaged
) {}
