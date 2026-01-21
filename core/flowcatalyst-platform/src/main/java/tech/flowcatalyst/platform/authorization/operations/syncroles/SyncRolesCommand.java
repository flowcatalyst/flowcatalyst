package tech.flowcatalyst.platform.authorization.operations.syncroles;

import java.util.List;
import java.util.Set;

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
     */
    public record SyncRoleItem(
        String name,
        String displayName,
        String description,
        Set<String> permissions,
        boolean clientManaged
    ) {}
}
