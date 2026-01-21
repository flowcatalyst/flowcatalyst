package tech.flowcatalyst.eventtype.operations.deleteeventtype;

/**
 * Command to delete an EventType.
 *
 * <p>An EventType can only be deleted if:
 * <ul>
 *   <li>It is in ARCHIVE status, OR</li>
 *   <li>It is in CURRENT status with all schemas still in FINALISING status
 *       (i.e., no schema was ever finalised)</li>
 * </ul>
 *
 * @param eventTypeId The ID of the event type to delete
 */
public record DeleteEventTypeCommand(
    String eventTypeId
) {}
