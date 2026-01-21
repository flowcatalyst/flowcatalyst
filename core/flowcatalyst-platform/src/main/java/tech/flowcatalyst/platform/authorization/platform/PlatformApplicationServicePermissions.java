package tech.flowcatalyst.platform.authorization.platform;

import tech.flowcatalyst.platform.authorization.Permission;
import tech.flowcatalyst.platform.authorization.PermissionDefinition;

/**
 * Permissions for application service accounts.
 *
 * These permissions allow applications/integrations to manage their own
 * resources (event types, subscriptions, roles) programmatically.
 *
 * Note: Resource scoping is enforced at runtime - service accounts can only
 * manage resources prefixed with their application's code.
 */
@Permission
public class PlatformApplicationServicePermissions {

    // ========================================================================
    // Event Type Management (scoped to application's prefix)
    // ========================================================================

    public static final PermissionDefinition APP_EVENT_TYPE_VIEW = PermissionDefinition.make(
        "platform", "application-service", "event-type", "view",
        "View event types for own application"
    );

    public static final PermissionDefinition APP_EVENT_TYPE_CREATE = PermissionDefinition.make(
        "platform", "application-service", "event-type", "create",
        "Create event types for own application"
    );

    public static final PermissionDefinition APP_EVENT_TYPE_UPDATE = PermissionDefinition.make(
        "platform", "application-service", "event-type", "update",
        "Update event types for own application"
    );

    public static final PermissionDefinition APP_EVENT_TYPE_DELETE = PermissionDefinition.make(
        "platform", "application-service", "event-type", "delete",
        "Delete event types for own application"
    );

    // ========================================================================
    // Subscription Management (scoped to application's prefix)
    // ========================================================================

    public static final PermissionDefinition APP_SUBSCRIPTION_VIEW = PermissionDefinition.make(
        "platform", "application-service", "subscription", "view",
        "View subscriptions for own application"
    );

    public static final PermissionDefinition APP_SUBSCRIPTION_CREATE = PermissionDefinition.make(
        "platform", "application-service", "subscription", "create",
        "Create subscriptions for own application"
    );

    public static final PermissionDefinition APP_SUBSCRIPTION_UPDATE = PermissionDefinition.make(
        "platform", "application-service", "subscription", "update",
        "Update subscriptions for own application"
    );

    public static final PermissionDefinition APP_SUBSCRIPTION_DELETE = PermissionDefinition.make(
        "platform", "application-service", "subscription", "delete",
        "Delete subscriptions for own application"
    );

    // ========================================================================
    // Role Management (scoped to application's prefix)
    // ========================================================================

    public static final PermissionDefinition APP_ROLE_VIEW = PermissionDefinition.make(
        "platform", "application-service", "role", "view",
        "View roles for own application"
    );

    public static final PermissionDefinition APP_ROLE_CREATE = PermissionDefinition.make(
        "platform", "application-service", "role", "create",
        "Create roles for own application"
    );

    public static final PermissionDefinition APP_ROLE_UPDATE = PermissionDefinition.make(
        "platform", "application-service", "role", "update",
        "Update roles for own application"
    );

    public static final PermissionDefinition APP_ROLE_DELETE = PermissionDefinition.make(
        "platform", "application-service", "role", "delete",
        "Delete roles for own application"
    );

    // ========================================================================
    // Permission Management (scoped to application's prefix)
    // ========================================================================

    public static final PermissionDefinition APP_PERMISSION_VIEW = PermissionDefinition.make(
        "platform", "application-service", "permission", "view",
        "View permissions for own application"
    );

    public static final PermissionDefinition APP_PERMISSION_SYNC = PermissionDefinition.make(
        "platform", "application-service", "permission", "sync",
        "Sync/register permissions for own application"
    );

    private PlatformApplicationServicePermissions() {}
}
