package tech.flowcatalyst.eventtype.operations.finaliseschema;

/**
 * Command to finalise a schema version.
 *
 * <p>Finalising a schema moves it from FINALISING to CURRENT status.
 * Any existing CURRENT schema with the same major version is automatically
 * deprecated.
 *
 * @param eventTypeId The ID of the event type
 * @param version     The version to finalise
 */
public record FinaliseSchemaCommand(
    String eventTypeId,
    String version
) {}
