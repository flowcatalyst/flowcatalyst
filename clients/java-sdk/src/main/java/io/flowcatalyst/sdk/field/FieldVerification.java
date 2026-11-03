package io.flowcatalyst.sdk.field;

/**
 * The outcome of {@link FieldTokenVerifier#verify}: a verified principal, a refused credential
 * (map to 401), or keys that could not be fetched (map to 503, never to "invalid").
 */
public sealed interface FieldVerification {

    /** The credential is good. */
    record Valid(FieldPrincipal principal) implements FieldVerification {}

    /**
     * The credential is refused. {@code reason} is one of the {@link FieldReason} constants;
     * {@code detail} is for logs, not for the client.
     */
    record Invalid(String reason, String detail) implements FieldVerification {}

    /** The platform's signing keys could not be discovered or fetched. */
    record Unavailable(String detail) implements FieldVerification {}
}
