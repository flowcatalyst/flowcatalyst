package tech.flowcatalyst.dispatchjob.model;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonValue;

/**
 * Enumeration of supported mediation types.
 * Defines how messages are mediated to downstream systems.
 *
 * NOTE: This is a copy of tech.flowcatalyst.messagerouter.model.MediationType
 * kept in sync for the backend's dispatch job functionality.
 */
public enum MediationType {
    /**
     * HTTP-based mediation to external webhooks
     */
    HTTP;

    @JsonValue
    public String toValue() {
        return name();
    }

    @JsonCreator
    public static MediationType fromValue(String value) {
        if (value == null) {
            throw new IllegalArgumentException("Mediation type cannot be null");
        }
        try {
            return valueOf(value.toUpperCase());
        } catch (IllegalArgumentException e) {
            throw new IllegalArgumentException("Unknown mediation type: " + value + ". Supported types: HTTP");
        }
    }
}
