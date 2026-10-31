package io.flowcatalyst.ingestor.functions.control;

import java.io.Serial;
import org.jspecify.annotations.Nullable;

/// A control-plane call that could not be completed: an HTTP status the platform answered (`status`
/// &gt; 0, with its `code` and message) or a transport failure (`status` 0). Messages never carry a
/// secret: they are the platform's own error text or a fixed phrase.
public final class ControlPlaneException extends Exception {

    @Serial
    private static final long serialVersionUID = 1L;

    private final int status;
    private final String code;
    private final boolean permanent;

    public ControlPlaneException(int status, String code, String message, @Nullable Throwable cause) {
        this(status, code, message, cause, false);
    }

    /// @param permanent a failure retrying cannot fix (the bytes arrived but are not the artifact the
    ///                  platform named: wrong digest, too large, a malformed digest)
    public ControlPlaneException(int status, String code, String message, @Nullable Throwable cause, boolean permanent) {
        super("control plane: " + (status > 0 ? "HTTP " + status + " " : "") + (code.isEmpty() ? "" : code + ": ") + message, cause);
        this.status = status;
        this.code = code;
        this.permanent = permanent;
    }

    /// The HTTP status, or 0 when there was no answer.
    public int status() {
        return status;
    }

    /// The platform's error code, or "".
    public String code() {
        return code;
    }

    /// The Go runner's rule (`isTransientLoadError`): control-plane 5xx, 429, 408/425, 401/403 (token expiry, a
    /// presigned URL that lapsed) and every transport failure are transient; the artifact missing
    /// (404/410) or otherwise rejected (other 4xx) and everything marked permanent are not.
    public boolean isTransient() {
        if (permanent) {
            return false;
        }
        if (status == 0) {
            return true;
        }
        return switch (status) {
            case 408, 425, 429, 401, 403 -> true;
            default -> status >= 500;
        };
    }
}
