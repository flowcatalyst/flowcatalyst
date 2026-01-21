package tech.flowcatalyst.eventtype.operations.addschema;

import tech.flowcatalyst.eventtype.SchemaType;

/**
 * Command to add a new schema version to an EventType.
 *
 * <p>The new schema is created in FINALISING status and must be
 * finalised before it can be used for event validation.
 *
 * @param eventTypeId The ID of the event type
 * @param version     Version string in MAJOR.MINOR format (e.g., "1.0", "2.1")
 * @param mimeType    MIME type of the schema (e.g., "application/json")
 * @param schema      The schema definition
 * @param schemaType  Type of schema (JSON_SCHEMA, PROTO, XSD)
 */
public record AddSchemaCommand(
    String eventTypeId,
    String version,
    String mimeType,
    String schema,
    SchemaType schemaType
) {}
