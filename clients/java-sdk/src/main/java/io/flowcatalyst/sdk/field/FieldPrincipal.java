package io.flowcatalyst.sdk.field;

import java.time.Instant;
import java.util.List;

/**
 * A verified field worker on a verified device.
 *
 * @param workerId the worker's id ({@code sub})
 * @param sessionId the login family ({@code sid})
 * @param jkt the thumbprint of the device key the request was proven with
 * @param tokenId the access token's {@code jti}
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
        String tokenId) {}
