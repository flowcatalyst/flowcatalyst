package io.flowcatalyst.sdk.resources;

import io.flowcatalyst.sdk.generated.model.DeleteClientPhotosResponse;
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

    /**
     * Replaces the policy: a member left unset takes the platform default (numbers their defaults,
     * {@code photosEnabled} true, {@code workerMaySetOwnPhoto} false), so read the policy with
     * {@link #get} and send every member back to change one.
     */
    public FieldPolicyResponse put(String clientId, PutFieldPolicyRequest request) {
        return transport.put(path(clientId), request, FieldPolicyResponse.class);
    }

    /**
     * Deletes every stored photo of the client's field workers, whatever the policy's
     * {@code photosEnabled} says (switching photos off only hides them). Answers how many were
     * removed; repeating it removes none. Needs {@code platform:iam:field-worker:manage} for the
     * client; an application-confined service account is refused (403 APPLICATION_FORBIDDEN): it
     * removes its own workers' photos one at a time. It cannot be undone.
     */
    public DeleteClientPhotosResponse deletePhotos(String clientId) {
        return transport.delete(path(clientId) + "/photos", DeleteClientPhotosResponse.class);
    }

    private static String path(String clientId) {
        return "/api/field-policies/" + Transport.enc(clientId);
    }
}
