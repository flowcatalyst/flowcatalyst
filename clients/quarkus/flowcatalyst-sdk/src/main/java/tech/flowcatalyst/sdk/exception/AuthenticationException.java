package tech.flowcatalyst.sdk.exception;

/**
 * Exception thrown when authentication fails.
 */
public class AuthenticationException extends FlowCatalystException {

    public AuthenticationException(String message) {
        super(message, 401);
    }

    public AuthenticationException(String message, Throwable cause) {
        super(message, 401, cause, null);
    }

    public static AuthenticationException tokenExpired() {
        return new AuthenticationException("Access token expired or invalid");
    }

    public static AuthenticationException invalidCredentials() {
        return new AuthenticationException("Invalid client credentials");
    }

    public static AuthenticationException missingCredentials() {
        return new AuthenticationException("Client ID and secret are required");
    }
}
