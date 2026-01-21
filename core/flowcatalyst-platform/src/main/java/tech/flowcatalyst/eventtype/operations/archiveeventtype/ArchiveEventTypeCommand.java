package tech.flowcatalyst.eventtype.operations.archiveeventtype;

/**
 * Command to archive an EventType.
 *
 * <p>Archiving an EventType moves it from CURRENT to ARCHIVE status.
 * All schema versions must be deprecated before an EventType can be archived.
 *
 * @param eventTypeId The ID of the event type to archive
 */
public record ArchiveEventTypeCommand(
    String eventTypeId
) {}
