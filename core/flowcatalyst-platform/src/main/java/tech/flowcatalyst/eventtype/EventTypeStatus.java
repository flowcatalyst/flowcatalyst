package tech.flowcatalyst.eventtype;

/**
 * Status of an EventType.
 */
public enum EventTypeStatus {
    /**
     * EventType is active and can have new events created.
     */
    CURRENT,

    /**
     * EventType is archived - no new events can be created.
     * Can only be set when all spec versions are DEPRECATED.
     */
    ARCHIVE
}
