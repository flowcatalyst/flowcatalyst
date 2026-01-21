package tech.flowcatalyst.platform.authorization.platform;

import tech.flowcatalyst.platform.authorization.Permission;
import tech.flowcatalyst.platform.authorization.PermissionDefinition;

/**
 * Platform administration permissions.
 * Controls access to clients, applications, and platform configuration.
 */
@Permission
public class PlatformAdminPermissions {

    // ========================================================================
    // Client Management
    // ========================================================================

    public static final PermissionDefinition CLIENT_VIEW = PermissionDefinition.make(
        "platform", "admin", "client", "view",
        "View client details and list clients"
    );

    public static final PermissionDefinition CLIENT_CREATE = PermissionDefinition.make(
        "platform", "admin", "client", "create",
        "Create new clients"
    );

    public static final PermissionDefinition CLIENT_UPDATE = PermissionDefinition.make(
        "platform", "admin", "client", "update",
        "Update client details and settings"
    );

    public static final PermissionDefinition CLIENT_DELETE = PermissionDefinition.make(
        "platform", "admin", "client", "delete",
        "Delete or suspend clients"
    );

    // ========================================================================
    // Application Management
    // ========================================================================

    public static final PermissionDefinition APPLICATION_VIEW = PermissionDefinition.make(
        "platform", "admin", "application", "view",
        "View application details and list applications"
    );

    public static final PermissionDefinition APPLICATION_CREATE = PermissionDefinition.make(
        "platform", "admin", "application", "create",
        "Create new applications"
    );

    public static final PermissionDefinition APPLICATION_UPDATE = PermissionDefinition.make(
        "platform", "admin", "application", "update",
        "Update application details and settings"
    );

    public static final PermissionDefinition APPLICATION_DELETE = PermissionDefinition.make(
        "platform", "admin", "application", "delete",
        "Delete or deactivate applications"
    );

    // ========================================================================
    // Platform Configuration (future)
    // ========================================================================

    public static final PermissionDefinition CONFIG_VIEW = PermissionDefinition.make(
        "platform", "admin", "config", "view",
        "View platform configuration"
    );

    public static final PermissionDefinition CONFIG_UPDATE = PermissionDefinition.make(
        "platform", "admin", "config", "update",
        "Update platform configuration"
    );

    private PlatformAdminPermissions() {}
}
