package tech.flowcatalyst.platform.authorization.operations.createrole;

import tech.flowcatalyst.platform.authorization.AuthRole;
import tech.flowcatalyst.platform.authorization.PermissionInput;

import java.util.List;
import java.util.Set;
import java.util.stream.Collectors;

/**
 * Command to create a new Role.
 *
 * <p>Permissions are structured with explicit segments to enforce format.
 *
 * @param applicationId  ID of the application this role belongs to
 * @param name           Role name without app prefix (e.g., "admin"). Will be auto-prefixed.
 * @param displayName    Human-readable display name (e.g., "Administrator")
 * @param description    Description of what this role grants access to
 * @param permissions    List of structured permission definitions
 * @param source         Source of this role (DATABASE or SDK)
 * @param clientManaged  Whether this role syncs to client-managed IDPs
 */
public record CreateRoleCommand(
    String applicationId,
    String name,
    String displayName,
    String description,
    List<PermissionInput> permissions,
    AuthRole.RoleSource source,
    boolean clientManaged
) {
    /**
     * Build permission strings from structured inputs.
     *
     * @return Set of permission strings in format {app}:{context}:{aggregate}:{action}
     */
    public Set<String> buildPermissionStrings() {
        if (permissions == null || permissions.isEmpty()) {
            return Set.of();
        }
        return permissions.stream()
            .map(PermissionInput::buildPermissionString)
            .collect(Collectors.toSet());
    }
}
