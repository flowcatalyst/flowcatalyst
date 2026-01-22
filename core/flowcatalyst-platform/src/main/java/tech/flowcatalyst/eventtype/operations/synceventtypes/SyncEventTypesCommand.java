package tech.flowcatalyst.eventtype.operations.synceventtypes;

import java.util.List;

/**
 * Command to bulk sync event types from an external application (SDK).
 *
 * @param applicationCode The application code (prefix for event type codes)
 * @param eventTypes      List of event type definitions to sync
 * @param removeUnlisted  If true, removes SDK event types not in the list
 */
public record SyncEventTypesCommand(
    String applicationCode,
    List<SyncEventTypeItem> eventTypes,
    boolean removeUnlisted
) {
    /**
     * Individual event type item in a sync operation.
     *
     * @param code        Event type code suffix (will be prefixed with app code)
     * @param name        Human-friendly name
     * @param description Optional description
     */
    public record SyncEventTypeItem(
        String code,
        String name,
        String description
    ) {}
}
