package tech.flowcatalyst.platform.authorization.platform;

import tech.flowcatalyst.platform.authorization.Permission;
import tech.flowcatalyst.platform.authorization.PermissionDefinition;

/**
 * IAM (Identity and Access Management) permissions.
 * Controls access to users, roles, permissions, and service accounts.
 */
@Permission
public class PlatformIamPermissions {

    // ========================================================================
    // User Management
    // ========================================================================

    public static final PermissionDefinition USER_VIEW = PermissionDefinition.make(
        "platform", "iam", "user", "view",
        "View user details and list users"
    );

    public static final PermissionDefinition USER_CREATE = PermissionDefinition.make(
        "platform", "iam", "user", "create",
        "Create new users"
    );

    public static final PermissionDefinition USER_UPDATE = PermissionDefinition.make(
        "platform", "iam", "user", "update",
        "Update user details and settings"
    );

    public static final PermissionDefinition USER_DELETE = PermissionDefinition.make(
        "platform", "iam", "user", "delete",
        "Delete or deactivate users"
    );

    // ========================================================================
    // Role Management
    // ========================================================================

    public static final PermissionDefinition ROLE_VIEW = PermissionDefinition.make(
        "platform", "iam", "role", "view",
        "View role definitions and assignments"
    );

    public static final PermissionDefinition ROLE_CREATE = PermissionDefinition.make(
        "platform", "iam", "role", "create",
        "Create new roles"
    );

    public static final PermissionDefinition ROLE_UPDATE = PermissionDefinition.make(
        "platform", "iam", "role", "update",
        "Update role definitions and permissions"
    );

    public static final PermissionDefinition ROLE_DELETE = PermissionDefinition.make(
        "platform", "iam", "role", "delete",
        "Delete roles"
    );

    // ========================================================================
    // Permission Management (read-only for most, code-defined)
    // ========================================================================

    public static final PermissionDefinition PERMISSION_VIEW = PermissionDefinition.make(
        "platform", "iam", "permission", "view",
        "View permission definitions"
    );

    // ========================================================================
    // Service Account Management (future)
    // ========================================================================

    public static final PermissionDefinition SERVICE_ACCOUNT_VIEW = PermissionDefinition.make(
        "platform", "iam", "service-account", "view",
        "View service accounts"
    );

    public static final PermissionDefinition SERVICE_ACCOUNT_CREATE = PermissionDefinition.make(
        "platform", "iam", "service-account", "create",
        "Create service accounts"
    );

    public static final PermissionDefinition SERVICE_ACCOUNT_UPDATE = PermissionDefinition.make(
        "platform", "iam", "service-account", "update",
        "Update service accounts"
    );

    public static final PermissionDefinition SERVICE_ACCOUNT_DELETE = PermissionDefinition.make(
        "platform", "iam", "service-account", "delete",
        "Delete service accounts"
    );

    // ========================================================================
    // Identity Provider (IDP) Management
    // ========================================================================

    public static final PermissionDefinition IDP_MANAGE = PermissionDefinition.make(
        "platform", "iam", "idp", "manage",
        "Manage identity provider configurations (create, update, delete domain IDPs)"
    );

    private PlatformIamPermissions() {}
}
