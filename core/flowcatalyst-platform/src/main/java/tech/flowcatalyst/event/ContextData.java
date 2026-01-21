package tech.flowcatalyst.event;

/**
 * Key-value pair for searchable context data attached to events.
 * Used for indexing and querying events in the projection.
 */
public class ContextData {

    public String key;
    public String value;

    public ContextData() {
    }

    public ContextData(String key, String value) {
        this.key = key;
        this.value = value;
    }

    // Accessor methods for compatibility
    public String key() { return key; }
    public String value() { return value; }
}
