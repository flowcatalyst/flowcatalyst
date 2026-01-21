package tech.flowcatalyst.platform.security.secrets;

/**
 * Exception thrown when a secret cannot be resolved.
 */
public class SecretResolutionException extends RuntimeException {

    public SecretResolutionException(String message) {
        super(message);
    }

    public SecretResolutionException(String message, Throwable cause) {
        super(message, cause);
    }
}
