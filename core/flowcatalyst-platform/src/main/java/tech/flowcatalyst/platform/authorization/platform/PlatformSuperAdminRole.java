package tech.flowcatalyst.platform.authorization.platform;

import tech.flowcatalyst.platform.authorization.Role;
import tech.flowcatalyst.platform.authorization.RoleDefinition;

import java.util.Set;

/**
 * Platform super administrator role - full access to everything.
 *
 * This role grants all permissions and bypasses most access checks.
 * Should only be assigned to platform operators.
 */
@Role
public class PlatformSuperAdminRole {
    public static final String ROLE_NAME = "platform:super-admin";

    public static final RoleDefinition INSTANCE = RoleDefinition.make(
        "platform",
        "super-admin",
        Set.of(
            // Super admin has all permissions - represented by wildcard in practice
            // Individual permissions listed for reference/documentation
            PlatformIamPermissions.USER_VIEW,
            PlatformIamPermissions.USER_CREATE,
            PlatformIamPermissions.USER_UPDATE,
            PlatformIamPermissions.USER_DELETE,
            PlatformIamPermissions.ROLE_VIEW,
            PlatformIamPermissions.ROLE_CREATE,
            PlatformIamPermissions.ROLE_UPDATE,
            PlatformIamPermissions.ROLE_DELETE,
            PlatformIamPermissions.PERMISSION_VIEW,
            PlatformAdminPermissions.CLIENT_VIEW,
            PlatformAdminPermissions.CLIENT_CREATE,
            PlatformAdminPermissions.CLIENT_UPDATE,
            PlatformAdminPermissions.CLIENT_DELETE,
            PlatformAdminPermissions.APPLICATION_VIEW,
            PlatformAdminPermissions.APPLICATION_CREATE,
            PlatformAdminPermissions.APPLICATION_UPDATE,
            PlatformAdminPermissions.APPLICATION_DELETE,
            PlatformMessagingPermissions.EVENT_VIEW,
            PlatformMessagingPermissions.EVENT_VIEW_RAW,
            PlatformMessagingPermissions.EVENT_TYPE_VIEW,
            PlatformMessagingPermissions.EVENT_TYPE_CREATE,
            PlatformMessagingPermissions.EVENT_TYPE_UPDATE,
            PlatformMessagingPermissions.EVENT_TYPE_DELETE,
            PlatformMessagingPermissions.SUBSCRIPTION_VIEW,
            PlatformMessagingPermissions.SUBSCRIPTION_CREATE,
            PlatformMessagingPermissions.SUBSCRIPTION_UPDATE,
            PlatformMessagingPermissions.SUBSCRIPTION_DELETE,
            PlatformMessagingPermissions.DISPATCH_JOB_VIEW,
            PlatformMessagingPermissions.DISPATCH_JOB_VIEW_RAW,
            PlatformMessagingPermissions.DISPATCH_JOB_CREATE,
            PlatformMessagingPermissions.DISPATCH_JOB_RETRY,
            PlatformMessagingPermissions.DISPATCH_POOL_VIEW,
            PlatformMessagingPermissions.DISPATCH_POOL_CREATE,
            PlatformMessagingPermissions.DISPATCH_POOL_UPDATE,
            PlatformMessagingPermissions.DISPATCH_POOL_DELETE,
            PlatformIamPermissions.IDP_MANAGE,
            // Application service permissions (super admin can manage all apps)
            PlatformApplicationServicePermissions.APP_EVENT_TYPE_VIEW,
            PlatformApplicationServicePermissions.APP_EVENT_TYPE_CREATE,
            PlatformApplicationServicePermissions.APP_EVENT_TYPE_UPDATE,
            PlatformApplicationServicePermissions.APP_EVENT_TYPE_DELETE,
            PlatformApplicationServicePermissions.APP_SUBSCRIPTION_VIEW,
            PlatformApplicationServicePermissions.APP_SUBSCRIPTION_CREATE,
            PlatformApplicationServicePermissions.APP_SUBSCRIPTION_UPDATE,
            PlatformApplicationServicePermissions.APP_SUBSCRIPTION_DELETE,
            PlatformApplicationServicePermissions.APP_ROLE_VIEW,
            PlatformApplicationServicePermissions.APP_ROLE_CREATE,
            PlatformApplicationServicePermissions.APP_ROLE_UPDATE,
            PlatformApplicationServicePermissions.APP_ROLE_DELETE,
            PlatformApplicationServicePermissions.APP_PERMISSION_VIEW,
            PlatformApplicationServicePermissions.APP_PERMISSION_SYNC
        ),
        "Platform super administrator - full access to everything"
    );

    private PlatformSuperAdminRole() {}
}
