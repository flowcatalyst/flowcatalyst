package tech.flowcatalyst.eventtype.operations.createeventtype;

/**
 * Command to create a new EventType.
 *
 * <p>EventTypes are global (not tenant-scoped) and have a unique code
 * following the format: {application}:{subdomain}:{aggregate}:{event}
 *
 * <p>Each segment is provided separately to enforce the code structure.
 * Segments must be lowercase alphanumeric with hyphens, starting with a letter.
 *
 * @param application  Application code (e.g., "operant", "platform")
 * @param subdomain    Subdomain within the application (e.g., "execution", "control-plane")
 * @param aggregate    Aggregate name (e.g., "trip", "eventtype")
 * @param event        Event name (e.g., "started", "created")
 * @param name         Human-friendly name (max 100 chars)
 * @param description  Optional description (max 255 chars)
 * @param clientScoped Whether events of this type are scoped to a client context
 */
public record CreateEventTypeCommand(
    String application,
    String subdomain,
    String aggregate,
    String event,
    String name,
    String description,
    boolean clientScoped
) {
    /**
     * Build the full event type code from segments.
     *
     * @return The code in format {application}:{subdomain}:{aggregate}:{event}
     */
    public String buildCode() {
        return String.join(":",
            application != null ? application.toLowerCase() : "",
            subdomain != null ? subdomain.toLowerCase() : "",
            aggregate != null ? aggregate.toLowerCase() : "",
            event != null ? event.toLowerCase() : ""
        );
    }
}
