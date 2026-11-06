package io.flowcatalyst.sdk.field;

import java.time.Instant;
import java.util.List;

/**
 * A verified field worker on a verified key.
 *
 * <p>When {@code actor} is non-null, someone else is signed in as the worker (a supervisor, or an
 * admin through a console sign-in code); every other component is still the WORKER's, so an
 * application's audit trail records the worker as the subject and {@code actor} as the operator.
 *
 * @param workerId the worker's id ({@code sub})
 * @param deviceId the enrolled device's id; empty when the key is not an enrolled device
 *     ({@code deviceMode} is then {@code UNENROLLED})
 * @param deviceMode {@code PERSONAL}, {@code SHARED} or {@code UNENROLLED}
 * @param sessionId the login family ({@code sid})
 * @param jkt the thumbprint of the device key the request was proven with
 * @param tokenId the access token's {@code jti}
 * @param actor who is signed in as the worker (the {@code act} claim), or null
 * @param test true for a test identity
 */
public record FieldPrincipal(
        String workerId,
        String clientId,
        String clientIdentifier,
        String workerCode,
        String name,
        String appCode,
        String deviceId,
        String deviceMode,
        String sessionId,
        List<String> amr,
        Instant authTime,
        Instant expiresAt,
        String jkt,
        String tokenId,
        FieldActor actor,
        boolean test) {

    /** The pre-sign-in-options shape: no actor, not a test identity. */
    public FieldPrincipal(
            String workerId,
            String clientId,
            String clientIdentifier,
            String workerCode,
            String name,
            String appCode,
            String deviceId,
            String deviceMode,
            String sessionId,
            List<String> amr,
            Instant authTime,
            Instant expiresAt,
            String jkt,
            String tokenId) {
        this(workerId, clientId, clientIdentifier, workerCode, name, appCode, deviceId, deviceMode, sessionId,
                amr, authTime, expiresAt, jkt, tokenId, null, false);
    }
}
