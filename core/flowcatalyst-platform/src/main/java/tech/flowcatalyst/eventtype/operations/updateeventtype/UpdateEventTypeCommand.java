package tech.flowcatalyst.eventtype.operations.updateeventtype;

/**
 * Command to update an EventType's metadata.
 *
 * <p>Only name and description can be updated. The code is immutable.
 * Null values mean "don't update this field".
 *
 * @param eventTypeId The ID of the event type to update
 * @param name        New name (null to keep current)
 * @param description New description (null to keep current)
 */
public record UpdateEventTypeCommand(
    String eventTypeId,
    String name,
    String description
) {}
