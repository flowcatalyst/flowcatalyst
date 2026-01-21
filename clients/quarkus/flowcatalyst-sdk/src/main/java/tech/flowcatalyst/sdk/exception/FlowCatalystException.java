package tech.flowcatalyst.sdk.exception;

import java.util.Map;

/**
 * Base exception for FlowCatalyst SDK errors.
 */
public class FlowCatalystException extends RuntimeException {

    private final int statusCode;
    private final Map<String, Object> context;

    public FlowCatalystException(String message) {
        this(message, 0, null, Map.of());
    }

    public FlowCatalystException(String message, int statusCode) {
        this(message, statusCode, null, Map.of());
    }

    public FlowCatalystException(String message, Throwable cause) {
        this(message, 0, cause, Map.of());
    }

    public FlowCatalystException(String message, int statusCode, Throwable cause, Map<String, Object> context) {
        super(message, cause);
        this.statusCode = statusCode;
        this.context = context != null ? context : Map.of();
    }

    public int getStatusCode() {
        return statusCode;
    }

    public Map<String, Object> getContext() {
        return context;
    }
}
