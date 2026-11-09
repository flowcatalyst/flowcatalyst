package io.flowcatalyst.sdk.outbox;

/**
 * A push the writer refuses (docs/push-gateway-spec.md section 6.1). {@link #code()} is one of
 * the codes of the framework's {@code conformance/push/topics.json}, which the platform's ingest
 * answers with as well. It is an {@link IllegalArgumentException}, as for a refused dispatch job,
 * so nothing is written.
 */
public final class PushException extends IllegalArgumentException {

    /** The codes of a refused push. */
    public enum Code {
        TOPIC_INVALID,
        TYPE_INVALID,
        TYPE_RESERVED,
        DATA_INVALID,
        DATA_NOT_OBJECT,
        DATA_TOO_LARGE,
        PUSH_ID_INVALID,
        CLIENT_INVALID
    }

    private final Code code;

    public PushException(Code code, String message) {
        super("invalid push: " + code + ": " + message);
        this.code = code;
    }

    public Code code() {
        return code;
    }
}
