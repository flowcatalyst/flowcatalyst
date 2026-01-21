package tech.flowcatalyst.event.read;

import java.time.Instant;
import java.util.List;

/**
 * Read-optimized projection of events from the events_read collection.
 *
 * This collection is populated by the stream processor and has rich indexes
 * for efficient querying. The raw events collection is write-optimized.
 */

public class EventRead {

    public String id;

    /** CloudEvents spec version */
    public String specVersion;

    /** Event type identifier (full code: app:subdomain:aggregate:event) */
    public String type;

    /**
     * Application code - first segment of type.
     * Parsed from type for efficient filtering.
     */
    public String application;

    /**
     * Subdomain - second segment of type.
     * Parsed from type for efficient filtering.
     */
    public String subdomain;

    /**
     * Aggregate - third segment of type.
     * Parsed from type for efficient filtering.
     */
    public String aggregate;

    /** Event source URI */
    public String source;

    /** Subject/aggregate identifier */
    public String subject;

    /** When the event occurred */
    public Instant time;

    /** Event payload data (JSON string) */
    public String data;

    /** Message group for ordering */
    public String messageGroup;

    /** Correlation ID for distributed tracing */
    public String correlationId;

    /** ID of the event that caused this event */
    public String causationId;

    /** Deduplication/idempotency key */
    public String deduplicationId;

    /** Context data for filtering */
    public List<ContextDataRead> contextData;

    /** Client ID for multi-tenant scoping */
    public String clientId;

    /** When this projection was created */
    public Instant projectedAt;

    public EventRead() {
    }

    // Record for context data
    public static class ContextDataRead {
        public String key;
        public String value;

        public ContextDataRead() {
        }

        public ContextDataRead(String key, String value) {
            this.key = key;
            this.value = value;
        }
    }
}
