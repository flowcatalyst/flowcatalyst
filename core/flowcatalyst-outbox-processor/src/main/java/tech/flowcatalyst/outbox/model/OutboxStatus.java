package tech.flowcatalyst.outbox.model;

/**
 * Processing status of an outbox item.
 * Uses integer codes for efficient storage and cross-database compatibility.
 *
 * <p>Status codes:
 * <ul>
 *   <li>0 = PENDING - waiting to be processed</li>
 *   <li>1 = SUCCESS - successfully sent to FlowCatalyst</li>
 *   <li>2 = BAD_REQUEST - API returned 400</li>
 *   <li>3 = INTERNAL_ERROR - API returned 500</li>
 *   <li>4 = UNAUTHORIZED - API returned 401</li>
 *   <li>5 = FORBIDDEN - API returned 403</li>
 *   <li>6 = GATEWAY_ERROR - API returned 502/503/504</li>
 *   <li>9 = IN_PROGRESS - currently being processed</li>
 * </ul>
 */
public enum OutboxStatus {
    /**
     * Item is waiting to be processed.
     */
    PENDING(0),

    /**
     * Item has been successfully sent to FlowCatalyst.
     */
    SUCCESS(1),

    /**
     * API returned 400 Bad Request (permanent failure).
     */
    BAD_REQUEST(2),

    /**
     * API returned 500 Internal Server Error.
     */
    INTERNAL_ERROR(3),

    /**
     * API returned 401 Unauthorized.
     */
    UNAUTHORIZED(4),

    /**
     * API returned 403 Forbidden.
     */
    FORBIDDEN(5),

    /**
     * API returned 502/503/504 Gateway Error.
     */
    GATEWAY_ERROR(6),

    /**
     * Item is currently being processed (locked by a processor).
     * Used to prevent re-polling; reset to 0 on crash recovery.
     */
    IN_PROGRESS(9);

    private final int code;

    OutboxStatus(int code) {
        this.code = code;
    }

    /**
     * Get the integer code for this status.
     */
    public int getCode() {
        return code;
    }

    /**
     * Check if this is a terminal status (no further processing).
     */
    public boolean isTerminal() {
        return this == SUCCESS || this == BAD_REQUEST || this == FORBIDDEN;
    }

    /**
     * Check if this status should be retried.
     */
    public boolean isRetryable() {
        return this == INTERNAL_ERROR || this == GATEWAY_ERROR || this == UNAUTHORIZED;
    }

    /**
     * Get OutboxStatus from integer code.
     */
    public static OutboxStatus fromCode(int code) {
        return switch (code) {
            case 0 -> PENDING;
            case 1 -> SUCCESS;
            case 2 -> BAD_REQUEST;
            case 3 -> INTERNAL_ERROR;
            case 4 -> UNAUTHORIZED;
            case 5 -> FORBIDDEN;
            case 6 -> GATEWAY_ERROR;
            case 9 -> IN_PROGRESS;
            default -> throw new IllegalArgumentException("Unknown status code: " + code);
        };
    }

    /**
     * Get OutboxStatus from HTTP status code.
     */
    public static OutboxStatus fromHttpCode(int httpCode) {
        if (httpCode >= 200 && httpCode < 300) {
            return SUCCESS;
        }
        return switch (httpCode) {
            case 400 -> BAD_REQUEST;
            case 401 -> UNAUTHORIZED;
            case 403 -> FORBIDDEN;
            case 502, 503, 504 -> GATEWAY_ERROR;
            default -> INTERNAL_ERROR;
        };
    }
}
