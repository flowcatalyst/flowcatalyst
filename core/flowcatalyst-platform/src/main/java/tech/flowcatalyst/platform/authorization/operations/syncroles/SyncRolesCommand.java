package tech.flowcatalyst.platform.authorization.operations.syncroles;

import tech.flowcatalyst.platform.authorization.PermissionInput;

import java.util.List;
import java.util.Set;
import java.util.stream.Collectors;

/**
 * Command to bulk sync roles from an external application (SDK).
 *
 * @param applicationId   ID of the application
 * @param roles           List of role definitions to sync
 * @param removeUnlisted  If true, removes SDK roles not in the list
 */
public record SyncRolesCommand(
    String applicationId,
    List<SyncRoleItem> roles,
    boolean removeUnlisted
) {
    /**
     * Individual role item in a sync operation.
     *
     * <p>Permissions are structured with explicit segments to enforce format.
     *
     * @param name          Role name without app prefix (e.g., "admin")
     * @param displayName   Human-readable display name
     * @param description   Description of what this role grants access to
     * @param permissions   List of structured permission definitions
     * @param clientManaged Whether this role syncs to client-managed IDPs
     */
    public record SyncRoleItem(
        String name,
        String displayName,
        String description,
        List<PermissionInput> permissions,
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
}
