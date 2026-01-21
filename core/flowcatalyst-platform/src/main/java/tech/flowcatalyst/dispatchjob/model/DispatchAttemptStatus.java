package tech.flowcatalyst.dispatchjob.model;

import com.fasterxml.jackson.annotation.JsonValue;

/**
 * Status of a dispatch attempt.
 */
public enum DispatchAttemptStatus {
    /** Webhook delivered successfully */
    SUCCESS("SUCCESS"),

    /** Webhook delivery failed */
    FAILURE("FAILURE"),

    /** Target did not respond within timeout */
    TIMEOUT("TIMEOUT"),

    /** Circuit breaker prevented attempt */
    CIRCUIT_OPEN("CIRCUIT_OPEN");

    private final String value;

    DispatchAttemptStatus(String value) {
        this.value = value;
    }

    @JsonValue
    public String getValue() {
        return value;
    }

    public static DispatchAttemptStatus fromValue(String value) {
        for (DispatchAttemptStatus status : values()) {
            if (status.value.equals(value)) {
                return status;
            }
        }
        throw new IllegalArgumentException("Unknown DispatchAttemptStatus: " + value);
    }
}
