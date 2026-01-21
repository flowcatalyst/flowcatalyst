package tech.flowcatalyst.platform.principal.operations.revokeclientaccess;

/**
 * Command to revoke a user's access to a client.
 *
 * @param userId   User's ID
 * @param clientId Client ID to revoke access from
 */
public record RevokeClientAccessCommand(
    String userId,
    String clientId
) {}
