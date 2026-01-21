package tech.flowcatalyst.platform.principal.operations.deleteuser;

/**
 * Command to delete a user.
 *
 * @param userId User's ID
 */
public record DeleteUserCommand(
    String userId
) {}
