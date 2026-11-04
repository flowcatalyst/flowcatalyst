package io.flowcatalyst.function;

import java.io.Serial;
import java.util.Objects;

/// Thrown by [FunctionContext#platformToken()] when the host cannot give the function a platform
/// token. `code` is one of the platform's host-call error codes (`fn-ts` `FnError`):
///
/// | `code` | Meaning | Retryable |
/// |---|---|---|
/// | `UNAVAILABLE` | the platform could not be reached, failed, or is throttling | yes |
/// | `NOT_DECLARED` | the function's describe does not declare `platformToken` | no |
/// | `NOT_ALLOWED` | the platform will not mint one (not declared there, the application has no linked service account, the account is inactive); the message carries the platform's code | no |
/// | `DEADLINE` | the invocation's deadline had passed | no |
///
/// Unchecked, like [DispatchException]. The message never contains a token.
public final class PlatformTokenException extends RuntimeException {

    /// The `UNAVAILABLE` code, the only retryable one.
    public static final String UNAVAILABLE = "UNAVAILABLE";

    @Serial
    private static final long serialVersionUID = 1L;

    private final String code;

    public PlatformTokenException(String code, String message) {
        super(message == null || message.isEmpty() ? "fn: " + code : "fn: " + code + ": " + message);
        this.code = Objects.requireNonNull(code, "code");
    }

    /// The error code.
    public String code() {
        return code;
    }

    /// Whether trying again later may succeed: exactly when [#code()] is `UNAVAILABLE`.
    public boolean retryable() {
        return UNAVAILABLE.equals(code);
    }
}
