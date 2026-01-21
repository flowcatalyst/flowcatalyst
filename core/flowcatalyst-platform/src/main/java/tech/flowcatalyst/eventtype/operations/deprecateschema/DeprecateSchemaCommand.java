package tech.flowcatalyst.eventtype.operations.deprecateschema;

/**
 * Command to deprecate a schema version.
 *
 * <p>Deprecating a schema moves it from CURRENT to DEPRECATED status.
 * Deprecated schemas can still be used for event validation but should
 * be phased out.
 *
 * @param eventTypeId The ID of the event type
 * @param version     The version to deprecate
 */
public record DeprecateSchemaCommand(
    String eventTypeId,
    String version
) {}
