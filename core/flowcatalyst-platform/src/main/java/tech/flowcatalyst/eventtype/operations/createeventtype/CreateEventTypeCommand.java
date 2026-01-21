package tech.flowcatalyst.eventtype.operations.createeventtype;

/**
 * Command to create a new EventType.
 *
 * <p>EventTypes are global (not tenant-scoped) and have a unique code
 * following the format: {app}:{subdomain}:{aggregate}:{event}
 *
 * @param code        Unique code in format {app}:{subdomain}:{aggregate}:{event}
 * @param name        Human-friendly name (max 100 chars)
 * @param description Optional description (max 255 chars)
 */
public record CreateEventTypeCommand(
    String code,
    String name,
    String description
) {}
