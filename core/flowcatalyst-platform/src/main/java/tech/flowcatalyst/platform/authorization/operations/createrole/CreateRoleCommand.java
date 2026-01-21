package tech.flowcatalyst.platform.authorization.operations.createrole;

import tech.flowcatalyst.platform.authorization.AuthRole;

import java.util.Set;

/**
 * Command to create a new Role.
 *
 * @param applicationId  ID of the application this role belongs to
 * @param name           Role name without app prefix (e.g., "admin"). Will be auto-prefixed.
 * @param displayName    Human-readable display name (e.g., "Administrator")
 * @param description    Description of what this role grants access to
 * @param permissions    Set of permission strings granted by this role
 * @param source         Source of this role (DATABASE or SDK)
 * @param clientManaged  Whether this role syncs to client-managed IDPs
 */
public record CreateRoleCommand(
    String applicationId,
    String name,
    String displayName,
    String description,
    Set<String> permissions,
    AuthRole.RoleSource source,
    boolean clientManaged
) {}
