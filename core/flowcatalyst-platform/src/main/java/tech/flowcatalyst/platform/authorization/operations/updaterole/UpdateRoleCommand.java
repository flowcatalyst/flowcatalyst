package tech.flowcatalyst.platform.authorization.operations.updaterole;

import java.util.Set;

/**
 * Command to update a Role.
 *
 * @param roleName      Full role name (e.g., "tms:admin")
 * @param displayName   New display name (null to keep existing)
 * @param description   New description (null to keep existing)
 * @param permissions   New permissions (null to keep existing)
 * @param clientManaged New clientManaged value (null to keep existing)
 */
public record UpdateRoleCommand(
    String roleName,
    String displayName,
    String description,
    Set<String> permissions,
    Boolean clientManaged
) {}
