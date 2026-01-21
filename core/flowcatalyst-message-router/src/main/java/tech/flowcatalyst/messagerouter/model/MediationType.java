package tech.flowcatalyst.messagerouter.model;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonValue;

/**
 * Enumeration of supported mediation types.
 * Defines how messages are mediated to downstream systems.
 */
public enum MediationType {
    /**
     * HTTP-based mediation to external webhooks
     */
    HTTP;

    /**
     * Serializes enum to JSON string value
     */
    @JsonValue
    public String toValue() {
        return name();
    }

    /**
     * Deserializes JSON string to enum value
     *
     * @param value the string value
     * @return the corresponding MediationType
     * @throws IllegalArgumentException if value is unknown
     */
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
