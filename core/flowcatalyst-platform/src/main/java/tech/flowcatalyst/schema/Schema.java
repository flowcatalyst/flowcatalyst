package tech.flowcatalyst.schema;

import lombok.Builder;
import lombok.With;
import tech.flowcatalyst.eventtype.SchemaType;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.time.Instant;

/**
 * A reusable schema definition.
 *
 * <p>Schemas define the structure of event payloads and can be:
 * <ul>
 *   <li>Referenced by EventType SpecVersions (eventTypeId + version set)</li>
 *   <li>Standalone for direct use with DispatchJobs (eventTypeId is null)</li>
 * </ul>
 *
 * <p>Content is stored as a string (JSON Schema, Proto, or XSD).
 *
 * <p>Use {@link #create(SchemaType, String)} for safe construction with defaults.
 */
@Builder(toBuilder = true)
@With
public record Schema(
    String id,

    /** Human-friendly name (optional) */
    String name,

    /** Description of what this schema validates (optional) */
    String description,

    /** MIME type for payloads using this schema (e.g., "application/json") */
    String mimeType,

    /** Type of schema (JSON_SCHEMA, PROTO, XSD) */
    SchemaType schemaType,

    /** The schema definition content */
    String content,

    /** Associated event type ID (null for standalone schemas) */
    String eventTypeId,

    /** Version string when linked to an EventType (null for standalone) */
    String version,

    Instant createdAt,
    Instant updatedAt
) {
    /**
     * Check if this is a standalone schema (not linked to an EventType).
     */
    public boolean isStandalone() {
        return eventTypeId == null;
    }

    /**
     * Check if this schema is linked to an EventType.
     */
    public boolean isLinkedToEventType() {
        return eventTypeId != null;
    }

    // ========================================================================
    // Factory Methods
    // ========================================================================

    /**
     * Create a new schema with required fields and sensible defaults.
     *
     * @param schemaType Type of schema (JSON_SCHEMA, PROTO, XSD)
     * @param content    The schema definition content
     * @return A pre-configured builder with defaults set
     */
    public static SchemaBuilder create(SchemaType schemaType, String content) {
        var now = Instant.now();
        return Schema.builder()
            .id(TsidGenerator.generate())
            .schemaType(schemaType)
            .content(content)
            .mimeType("application/json")
            .createdAt(now)
            .updatedAt(now);
    }
}
