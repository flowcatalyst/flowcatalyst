package tech.flowcatalyst.eventtype;

/**
 * Source of an event type - how it was created.
 */
public enum EventTypeSource {
    /**
     * Created or synced via SDK/API.
     */
    API,

    /**
     * Created via the user interface.
     */
    UI
}
