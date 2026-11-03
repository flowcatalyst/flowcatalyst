package io.flowcatalyst.sdk.resources;

import io.flowcatalyst.sdk.generated.model.FieldTokenIntrospectRequest;
import io.flowcatalyst.sdk.generated.model.FieldTokenIntrospectResponse;
import io.flowcatalyst.sdk.http.Transport;

/** Online check of field access tokens ({@code /api/field-tokens}); needs {@code platform:iam:field-token:introspect}. */
public final class FieldTokensResource {

    private final Transport transport;

    public FieldTokensResource(Transport transport) {
        this.transport = transport;
    }

    /**
     * Asks the platform whether the token is still live. Use it when the offline signature check
     * ({@link io.flowcatalyst.sdk.field.FieldTokenVerifier}) is not enough and the session must
     * still be valid. Only {@code active} is set when the token is unusable (bad signature,
     * expired, revoked session, suspended worker or device, or outside the caller's reach).
     */
    public FieldTokenIntrospectResponse introspect(String token) {
        return transport.post(
                "/api/field-tokens/introspect",
                new FieldTokenIntrospectRequest().token(token),
                FieldTokenIntrospectResponse.class);
    }
}
