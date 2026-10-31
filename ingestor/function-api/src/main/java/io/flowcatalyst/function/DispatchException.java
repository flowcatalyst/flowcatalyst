package io.flowcatalyst.function;

import java.io.Serial;
import java.util.Objects;

/// Thrown by [Dispatcher#dispatch] when a dispatch fails. `code` is one of the
/// platform's host-call error codes (`fn-ts` `FnError`):
///
/// | `code` | Meaning | Retryable |
/// |---|---|---|
/// | `UNAVAILABLE` | the platform could not be reached or failed | yes |
/// | `NOT_DECLARED` | `(target, path)` is not in the function's `dispatches` | no |
/// | `NOT_ALLOWED` | the platform will not deliver there (other application, target not live, no webhook endpoint at the path) | no |
/// | `BAD_REQUEST` | an invalid field | no |
/// | others | `DEADLINE`, `TOO_LARGE`, `CAPABILITY_UNAVAILABLE` | no |
///
/// Unchecked, like [HttpCallRefusedException].
public final class DispatchException extends RuntimeException {

    /// The `UNAVAILABLE` code, the only retryable one.
    public static final String UNAVAILABLE = "UNAVAILABLE";

    @Serial
    private static final long serialVersionUID = 1L;

    private final String code;

    public DispatchException(String code, String message) {
        super(message == null || message.isEmpty() ? "fn: " + code : "fn: " + code + ": " + message);
        this.code = Objects.requireNonNull(code, "code");
    }

    /// The platform's error code.
    public String code() {
        return code;
    }

    /// Whether trying again later may succeed: exactly when [#code()] is `UNAVAILABLE`.
    public boolean retryable() {
        return UNAVAILABLE.equals(code);
    }
}
