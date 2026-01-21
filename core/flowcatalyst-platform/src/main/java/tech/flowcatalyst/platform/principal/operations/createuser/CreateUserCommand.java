package tech.flowcatalyst.platform.principal.operations.createuser;

/**
 * Command to create a new user.
 *
 * @param email       User's email address (will determine anchor user status)
 * @param password    Plain text password (will be hashed)
 * @param name        User's display name
 * @param clientId    Home client ID (nullable, will be auto-detected from email domain if not provided)
 */
public record CreateUserCommand(
    String email,
    String password,
    String name,
    String clientId
) {}
