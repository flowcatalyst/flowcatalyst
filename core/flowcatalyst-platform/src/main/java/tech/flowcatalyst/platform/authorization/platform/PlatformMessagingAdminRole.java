package tech.flowcatalyst.platform.authorization.platform;

import tech.flowcatalyst.platform.authorization.Role;
import tech.flowcatalyst.platform.authorization.RoleDefinition;

import java.util.Set;

/**
 * Messaging Administrator role - manages event types, subscriptions, and dispatch jobs.
 */
@Role
public class PlatformMessagingAdminRole {
    public static final String ROLE_NAME = "platform:messaging-admin";

    public static final RoleDefinition INSTANCE = RoleDefinition.make(
        "platform",
        "messaging-admin",
        Set.of(
            PlatformMessagingPermissions.EVENT_TYPE_VIEW,
            PlatformMessagingPermissions.EVENT_TYPE_CREATE,
            PlatformMessagingPermissions.EVENT_TYPE_UPDATE,
            PlatformMessagingPermissions.EVENT_TYPE_DELETE,
            PlatformMessagingPermissions.SUBSCRIPTION_VIEW,
            PlatformMessagingPermissions.SUBSCRIPTION_CREATE,
            PlatformMessagingPermissions.SUBSCRIPTION_UPDATE,
            PlatformMessagingPermissions.SUBSCRIPTION_DELETE,
            PlatformMessagingPermissions.DISPATCH_JOB_VIEW,
            PlatformMessagingPermissions.DISPATCH_JOB_CREATE,
            PlatformMessagingPermissions.DISPATCH_JOB_RETRY
        ),
        "Messaging administrator - manages event types and subscriptions"
    );

    private PlatformMessagingAdminRole() {}
}
