package tech.flowcatalyst.platform.authorization.operations.deleterole;

/**
 * Command to delete a Role.
 *
 * @param roleName Full role name (e.g., "tms:admin")
 */
public record DeleteRoleCommand(String roleName) {}
