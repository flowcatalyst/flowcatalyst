package io.flowcatalyst.sdk.resources;

import io.flowcatalyst.sdk.generated.model.FieldPolicyResponse;
import io.flowcatalyst.sdk.generated.model.PutFieldPolicyRequest;
import io.flowcatalyst.sdk.http.Transport;

/** The per-client field policy: PIN rules, lockouts, session lifetimes ({@code /api/field-policies}). */
public final class FieldPoliciesResource {

    private final Transport transport;

    public FieldPoliciesResource(Transport transport) {
        this.transport = transport;
    }

    /** The effective policy; {@code isDefault} is true when the client has no stored policy. */
    public FieldPolicyResponse get(String clientId) {
        return transport.get(path(clientId), null, FieldPolicyResponse.class);
    }

    /** Replaces the policy; an unset numeric member takes the platform default. */
    public FieldPolicyResponse put(String clientId, PutFieldPolicyRequest request) {
        return transport.put(path(clientId), request, FieldPolicyResponse.class);
    }

    private static String path(String clientId) {
        return "/api/field-policies/" + Transport.enc(clientId);
    }
}
