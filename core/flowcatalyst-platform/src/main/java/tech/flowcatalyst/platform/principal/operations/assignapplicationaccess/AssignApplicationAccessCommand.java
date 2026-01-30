package tech.flowcatalyst.platform.principal.operations.assignapplicationaccess;

import java.util.List;

/**
 * Command for assigning application access to a user.
 *
 * <p>This is a declarative/batch operation - the provided list represents
 * the complete set of applications the user should have access to.
 * Applications not in the list will be removed, new applications will be added.
 *
 * @param userId         The ID of the user to assign application access to
 * @param applicationIds The complete set of application IDs the user should have access to
 */
public record AssignApplicationAccessCommand(
    String userId,
    List<String> applicationIds
) {}
