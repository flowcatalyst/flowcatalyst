package tech.flowcatalyst.platform.principal.operations.deactivateuser;

/**
 * Command to deactivate a user.
 *
 * @param userId User's ID
 * @param reason Optional reason for deactivation
 */
public record DeactivateUserCommand(
    String userId,
    String reason
) {}
