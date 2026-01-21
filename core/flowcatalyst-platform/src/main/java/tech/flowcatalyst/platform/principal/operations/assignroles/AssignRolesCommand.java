package tech.flowcatalyst.platform.principal.operations.assignroles;

import java.util.List;

/**
 * Command to assign roles to a user.
 *
 * <p>This is a declarative command - the provided list represents the complete
 * set of roles the user should have after the operation. The use case will
 * compute which roles to add and which to remove.
 *
 * @param userId User's ID (TSID)
 * @param roles  Complete list of role names to assign
 */
public record AssignRolesCommand(
    String userId,
    List<String> roles
) {}
