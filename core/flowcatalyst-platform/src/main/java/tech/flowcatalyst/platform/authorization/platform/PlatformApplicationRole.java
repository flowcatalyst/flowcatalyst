package tech.flowcatalyst.platform.authorization.platform;

import tech.flowcatalyst.platform.authorization.Role;
import tech.flowcatalyst.platform.authorization.RoleDefinition;

import java.util.Set;

/**
 * Application role for internal applications.
 *
 * This role is designed for internal applications that need to:
 * - Create and view dispatch jobs (all of IntegrationRole)
 * - Retry failed dispatch jobs
 * - Manage event types (create, view, update)
 * - Manage subscriptions (create, view, update)
 *
 * More permissions than IntegrationRole, but no delete operations.
 * For service principals (not human users).
 */
@Role
public class PlatformApplicationRole {
    public static final String ROLE_NAME = "platform:application";

    public static final RoleDefinition INSTANCE = RoleDefinition.make(
        "platform",
        "application",
        Set.of(
            // Dispatch job permissions
            PlatformMessagingPermissions.DISPATCH_JOB_CREATE,
            PlatformMessagingPermissions.DISPATCH_JOB_VIEW,
            PlatformMessagingPermissions.DISPATCH_JOB_RETRY,

            // Event type permissions (no delete)
            PlatformMessagingPermissions.EVENT_TYPE_CREATE,
            PlatformMessagingPermissions.EVENT_TYPE_VIEW,
            PlatformMessagingPermissions.EVENT_TYPE_UPDATE,

            // Subscription permissions (no delete)
            PlatformMessagingPermissions.SUBSCRIPTION_CREATE,
            PlatformMessagingPermissions.SUBSCRIPTION_VIEW,
            PlatformMessagingPermissions.SUBSCRIPTION_UPDATE
        ),
        "Application role - manage messaging resources (no delete)"
    );

    private PlatformApplicationRole() {}
}
