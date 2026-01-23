package tech.flowcatalyst.eventtype.operations.synceventtypes;

import java.util.List;

/**
 * Command to bulk sync event types from an external application (SDK).
 *
 * @param applicationCode The application code (first segment of event type codes)
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
     * <p>The full event type code is composed as:
     * {applicationCode}:{subdomain}:{aggregate}:{event}
     *
     * @param subdomain    Subdomain within the application (e.g., "execution", "orders")
     * @param aggregate    Aggregate name (e.g., "trip", "order")
     * @param event        Event name (e.g., "started", "created")
     * @param name         Human-friendly name
     * @param description  Optional description
     * @param clientScoped Whether events of this type are scoped to clients (optional, defaults to false)
     */
    public record SyncEventTypeItem(
        String subdomain,
        String aggregate,
        String event,
        String name,
        String description,
        Boolean clientScoped
    ) {
        /**
         * Build the full event type code from segments.
         *
         * @param applicationCode The application code prefix
         * @return The code in format {applicationCode}:{subdomain}:{aggregate}:{event}
         */
        public String buildCode(String applicationCode) {
            return String.join(":",
                applicationCode != null ? applicationCode.toLowerCase() : "",
                subdomain != null ? subdomain.toLowerCase() : "",
                aggregate != null ? aggregate.toLowerCase() : "",
                event != null ? event.toLowerCase() : ""
            );
        }
    }
}
