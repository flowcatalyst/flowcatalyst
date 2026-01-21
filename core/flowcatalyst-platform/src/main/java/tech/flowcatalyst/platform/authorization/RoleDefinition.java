package tech.flowcatalyst.platform.authorization;

import java.util.Set;

/**
 * Definition of a role in the FlowCatalyst system.
 *
 * Roles follow the structure: {application}:{role-name}
 *
 * Examples:
 * - platform:admin
 * - platform:iam-admin
 * - platform:messaging-admin
 * - tms:dispatcher
 * - tms:warehouse-manager
 *
 * Each role maps to a set of permission strings.
 * All parts must be lowercase alphanumeric with hyphens allowed.
 * Roles are defined in code using the @Role annotation.
 */
public interface RoleDefinition {

    String application();         // Application code (e.g., "platform", "tms")
    String roleName();            // Role name within app (e.g., "admin", "dispatcher")
    Set<PermissionRecord> permissions();    // Permissions this role grants
    String description();         // Human-readable description

    /**
     * Generate the string representation of this role.
     * Format: {application}:{role-name}
     *
     * @return Role string (e.g., "platform:admin")
     */
    default String toRoleString() {
        return String.format("%s:%s", application(), roleName());
    }

    /**
     * Get permission strings for this role.
     * Convenience method that converts PermissionRecord objects to strings.
     *
     * @return Set of permission strings (e.g., "platform:iam:user:create")
     */
    default Set<String> permissionStrings() {
        return permissions().stream()
            .map(PermissionRecord::toPermissionString)
            .collect(java.util.stream.Collectors.toSet());
    }

    /**
     * Static factory method to create a role from PermissionDefinition instances.
     * This is the preferred approach as it provides type safety and full metadata.
     *
     * @param application Application code
     * @param roleName Role name within application
     * @param permissions Permission instances this role grants
     * @param description Human-readable description
     * @return Role instance
     */
    static RoleRecord make(String application, String roleName, Set<PermissionDefinition> permissions, String description) {
        // Convert PermissionDefinitions to concrete PermissionRecord instances
        Set<PermissionRecord> permissionRecords = permissions.stream()
            .map(pd -> pd instanceof PermissionRecord pr ? pr :
                 PermissionDefinition.make(pd.application(), pd.context(), pd.aggregate(), pd.action(), pd.description()))
            .collect(java.util.stream.Collectors.toSet());
        return new RoleRecord(application, roleName, permissionRecords, description);
    }

    /**
     * Static factory method to create a role from permission strings.
     * Use this only when you don't have PermissionDefinition instances available.
     *
     * @param application Application code
     * @param roleName Role name within application
     * @param permissionStrings Permission strings this role grants
     * @param description Human-readable description
     * @return Role instance
     */
    static RoleRecord makeFromStrings(String application, String roleName, Set<String> permissionStrings, String description) {
        // Convert strings to PermissionRecord objects
        Set<PermissionRecord> permissions = permissionStrings.stream()
            .map(RoleDefinition::parsePermissionString)
            .collect(java.util.stream.Collectors.toSet());
        return new RoleRecord(application, roleName, permissions, description);
    }

    /**
     * Parse a permission string into a PermissionRecord.
     * Format: application:context:aggregate:action
     */
    private static PermissionRecord parsePermissionString(String permissionString) {
        String[] parts = permissionString.split(":");
        if (parts.length != 4) {
            throw new IllegalArgumentException("Invalid permission string format: " + permissionString);
        }
        return PermissionDefinition.make(parts[0], parts[1], parts[2], parts[3], "Permission created from string");
    }
}
