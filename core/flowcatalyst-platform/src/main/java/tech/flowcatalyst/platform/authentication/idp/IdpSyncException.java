package tech.flowcatalyst.platform.authentication.idp;

/**
 * Exception thrown when IDP synchronization fails.
 */
public class IdpSyncException extends Exception {

    public IdpSyncException(String message) {
        super(message);
    }

    public IdpSyncException(String message, Throwable cause) {
        super(message, cause);
    }
}
