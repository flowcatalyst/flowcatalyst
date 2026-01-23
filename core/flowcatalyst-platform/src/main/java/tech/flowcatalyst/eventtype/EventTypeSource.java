package tech.flowcatalyst.eventtype;

/**
 * Source of an event type - how it was created.
 */
public enum EventTypeSource {
    /**
     * Defined in code (platform event types).
     * These are synced to the database on startup and cannot be modified via UI/API.
     */
    CODE,

    /**
     * Created or synced via SDK/API.
     */
    API,

    /**
     * Created via the user interface.
     */
    UI
}
