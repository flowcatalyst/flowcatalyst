package tech.flowcatalyst.platform.principal.operations.activateuser;

/**
 * Command to activate a user.
 *
 * @param userId User's ID
 */
public record ActivateUserCommand(
    String userId
) {}
