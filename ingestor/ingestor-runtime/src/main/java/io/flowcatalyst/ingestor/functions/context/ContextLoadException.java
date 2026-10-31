package io.flowcatalyst.ingestor.functions.context;

import java.io.Serial;

/// A [ContextFactory#build] failure — a load failure like any other: the reconciler records `code` against
/// this (address, version) in its heartbeat, closes whatever was loaded so far, and leaves the
/// previously-loaded version (if any) serving. `code` never carries the raw DSN or any secret value —
/// this exception's message is safe to log as-is. [#isTransient()] says whether the condition can pass
/// (`DB_POOL_LIMIT`: another function's pool may be released) or is permanent (`DB_UNSUPPORTED`).
public final class ContextLoadException extends RuntimeException {

    @Serial
    private static final long serialVersionUID = 1L;

    private final String code;
    private final boolean transientFailure;

    public ContextLoadException(String code, String message) {
        this(code, message, false);
    }

    public ContextLoadException(String code, String message, boolean transientFailure) {
        super(message);
        this.code = code;
        this.transientFailure = transientFailure;
    }

    public boolean isTransient() {
        return transientFailure;
    }

    public String code() {
        return code;
    }
}
