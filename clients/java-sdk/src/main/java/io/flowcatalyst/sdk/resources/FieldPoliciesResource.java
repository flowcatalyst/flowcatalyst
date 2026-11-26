package io.flowcatalyst.sdk.resources;

import io.flowcatalyst.sdk.generated.model.DeleteClientPhotosResponse;
import io.flowcatalyst.sdk.generated.model.FieldPolicyResponse;
import io.flowcatalyst.sdk.generated.model.PutFieldPolicyRequest;
import io.flowcatalyst.sdk.http.Transport;
import java.util.function.Consumer;

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
     * REPLACES the policy: a member left unset takes the platform default (numbers their defaults,
     * {@code photosEnabled} true, {@code workerMaySetOwnPhoto} false, {@code pinReauthDays} 0,
     * {@code unlockKeysEnabled} false, {@code unlockKeyAttestation} NONE), so read the policy with
     * {@link #get} and send every member back to change one: use {@link #update}, or start from
     * {@link #toRequest}.
     *
     * <p>A request that leaves {@code unlockKeysEnabled} unset switches unlock keys off for the
     * client, revokes all its keys and signs those workers out. This method sends exactly what it
     * is given.
     */
    public FieldPolicyResponse put(String clientId, PutFieldPolicyRequest request) {
        return transport.put(path(clientId), request, FieldPolicyResponse.class);
    }

    /**
     * The PUT request that stores {@code policy} again unchanged: every setting the platform
     * returned is set on it (the unlock-key settings and {@code pinReauthDays} 0 included); the
     * response-only members ({@code clientId}, {@code isDefault}, {@code updatedAt}) are not.
     */
    public static PutFieldPolicyRequest toRequest(FieldPolicyResponse policy) {
        var r = new PutFieldPolicyRequest()
                .pinMinLength(policy.getPinMinLength())
                .softLockThreshold(policy.getSoftLockThreshold())
                .softLockSecs(policy.getSoftLockSecs())
                .hardLockThreshold(policy.getHardLockThreshold())
                .sharedIdleSecs(policy.getSharedIdleSecs())
                .personalIdleSecs(policy.getPersonalIdleSecs())
                .shiftMaxSecs(policy.getShiftMaxSecs())
                .requireAttestation(Boolean.TRUE.equals(policy.getRequireAttestation()))
                .pinReauthDays(policy.getPinReauthDays())
                .requireEnrolledDevice(policy.getRequireEnrolledDevice())
                .unenrolledIdleSecs(policy.getUnenrolledIdleSecs())
                .badgeOnlyLogin(policy.getBadgeOnlyLogin())
                .impersonationMaxSecs(policy.getImpersonationMaxSecs())
                .workerCodeDigits(policy.getWorkerCodeDigits())
                .photosEnabled(policy.getPhotosEnabled())
                .workerMaySetOwnPhoto(policy.getWorkerMaySetOwnPhoto())
                .unlockKeysEnabled(policy.getUnlockKeysEnabled());
        if (policy.getUnlockKeyAttestation() != null) {
            r.unlockKeyAttestation(
                    PutFieldPolicyRequest.UnlockKeyAttestationEnum.fromValue(policy.getUnlockKeyAttestation().getValue()));
        }
        return r;
    }

    /**
     * Reads the policy, lets {@code change} alter the request that would store it unchanged, and
     * writes the result: every member {@code change} does not touch keeps its stored value. The
     * read and the write are two calls: a policy edited by someone else in between is overwritten
     * with what was read.
     */
    public FieldPolicyResponse update(String clientId, Consumer<PutFieldPolicyRequest> change) {
        var request = toRequest(get(clientId));
        change.accept(request);
        return put(clientId, request);
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
