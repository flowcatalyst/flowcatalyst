package tech.flowcatalyst.dispatchjob.model;

import com.fasterxml.jackson.annotation.JsonValue;

/**
 * Classification of errors for dispatch job processing.
 *
 * Determines retry behavior:
 * <ul>
 *   <li><b>TRANSIENT</b> - Temporary failure, retry is appropriate (network issues, 5xx, timeouts)</li>
 *   <li><b>NOT_TRANSIENT</b> - Permanent failure, do not retry (4xx client errors, invalid config)</li>
 *   <li><b>UNKNOWN</b> - Cannot determine, treat as transient by default</li>
 * </ul>
 */
public enum ErrorType {
    /** Temporary failure - retry is appropriate */
    TRANSIENT("TRANSIENT"),

    /** Permanent failure - do not retry */
    NOT_TRANSIENT("NOT_TRANSIENT"),

    /** Unknown error type - default to retry */
    UNKNOWN("UNKNOWN");

    private final String value;

    ErrorType(String value) {
        this.value = value;
    }

    @JsonValue
    public String getValue() {
        return value;
    }

    public static ErrorType fromValue(String value) {
        for (ErrorType type : values()) {
            if (type.value.equals(value)) {
                return type;
            }
        }
        return UNKNOWN;
    }
}
