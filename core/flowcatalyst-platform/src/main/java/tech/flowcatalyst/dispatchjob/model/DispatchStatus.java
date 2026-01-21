package tech.flowcatalyst.dispatchjob.model;

import com.fasterxml.jackson.annotation.JsonValue;

/**
 * Status of a dispatch job through its lifecycle.
 *
 * Lifecycle:
 * <pre>
 * PENDING → QUEUED → IN_PROGRESS → COMPLETED
 *                  ↓             ↓
 *              (retry)      ERROR (retries exceeded or NOT_TRANSIENT)
 *                  ↓
 *               QUEUED
 *
 * Any state → CANCELLED (manual intervention)
 * </pre>
 */
public enum DispatchStatus {
    /** Created, waiting for message pointer to be queued */
    PENDING("PENDING"),

    /** Message pointer is on the queue (message-router) */
    QUEUED("QUEUED"),

    /** Being processed by the processing endpoint */
    IN_PROGRESS("IN_PROGRESS"),

    /** Successfully delivered */
    COMPLETED("COMPLETED"),

    /** Retries exhausted or non-transient failure */
    ERROR("ERROR"),

    /** Manually cancelled - no longer relevant */
    CANCELLED("CANCELLED");

    private final String value;

    DispatchStatus(String value) {
        this.value = value;
    }

    @JsonValue
    public String getValue() {
        return value;
    }

    public static DispatchStatus fromValue(String value) {
        for (DispatchStatus status : values()) {
            if (status.value.equals(value)) {
                return status;
            }
        }
        throw new IllegalArgumentException("Unknown DispatchStatus: " + value);
    }
}