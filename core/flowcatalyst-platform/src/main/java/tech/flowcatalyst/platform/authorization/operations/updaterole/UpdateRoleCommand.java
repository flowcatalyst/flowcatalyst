package tech.flowcatalyst.platform.authorization.operations.updaterole;

import tech.flowcatalyst.platform.authorization.PermissionInput;

import java.util.List;
import java.util.Set;
import java.util.stream.Collectors;

/**
 * Command to update a Role.
 *
 * <p>Permissions are structured with explicit segments to enforce format.
 *
 * @param roleName      Full role name (e.g., "tms:admin")
 * @param displayName   New display name (null to keep existing)
 * @param description   New description (null to keep existing)
 * @param permissions   New structured permissions (null to keep existing)
 * @param clientManaged New clientManaged value (null to keep existing)
 */
public record UpdateRoleCommand(
    String roleName,
    String displayName,
    String description,
    List<PermissionInput> permissions,
    Boolean clientManaged
) {
    /**
     * Build permission strings from structured inputs.
     *
     * @return Set of permission strings, or null if permissions is null
     */
    public Set<String> buildPermissionStrings() {
        if (permissions == null) {
            return null;
        }
        if (permissions.isEmpty()) {
            return Set.of();
        }
        return permissions.stream()
            .map(PermissionInput::buildPermissionString)
            .collect(Collectors.toSet());
    }
}
