package tech.flowcatalyst.platform.principal.operations.grantclientaccess;

import java.time.Instant;

/**
 * Command to grant a user access to a client.
 *
 * @param userId    User's ID
 * @param clientId  Client ID to grant access to
 * @param expiresAt Optional expiration date for the grant
 */
public record GrantClientAccessCommand(
    String userId,
    String clientId,
    Instant expiresAt
) {}
