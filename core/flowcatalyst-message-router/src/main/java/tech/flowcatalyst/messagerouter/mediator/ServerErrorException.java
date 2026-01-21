package tech.flowcatalyst.messagerouter.mediator;

/**
 * Exception thrown when a server returns a 5xx error.
 * This exception is used to trigger circuit breaker and retry logic.
 */
public class ServerErrorException extends RuntimeException {

    private final int statusCode;

    public ServerErrorException(int statusCode, String message) {
        super(String.format("Server error %d: %s", statusCode, message));
        this.statusCode = statusCode;
    }

    public int getStatusCode() {
        return statusCode;
    }
}
