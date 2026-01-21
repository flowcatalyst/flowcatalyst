package tech.flowcatalyst.platform.authorization.platform;

import tech.flowcatalyst.platform.authorization.Role;
import tech.flowcatalyst.platform.authorization.RoleDefinition;

import java.util.Set;

/**
 * Application service account role.
 *
 * This role is automatically assigned to service accounts created for
 * applications and integrations. It grants permissions to manage resources
 * prefixed with the application's code (event types, subscriptions, roles).
 *
 * Resource scoping is enforced at runtime - even with these permissions,
 * service accounts can only access/modify resources belonging to their
 * linked application.
 *
 * @see tech.flowcatalyst.platform.application.Application#serviceAccountPrincipalId
 */
@Role
public class PlatformApplicationServiceRole {
    public static final String ROLE_NAME = "platform:application-service";

    public static final RoleDefinition INSTANCE = RoleDefinition.make(
        "platform",
        "application-service",
        Set.of(
            // Event type management for own application
            PlatformApplicationServicePermissions.APP_EVENT_TYPE_VIEW,
            PlatformApplicationServicePermissions.APP_EVENT_TYPE_CREATE,
            PlatformApplicationServicePermissions.APP_EVENT_TYPE_UPDATE,
            PlatformApplicationServicePermissions.APP_EVENT_TYPE_DELETE,

            // Subscription management for own application
            PlatformApplicationServicePermissions.APP_SUBSCRIPTION_VIEW,
            PlatformApplicationServicePermissions.APP_SUBSCRIPTION_CREATE,
            PlatformApplicationServicePermissions.APP_SUBSCRIPTION_UPDATE,
            PlatformApplicationServicePermissions.APP_SUBSCRIPTION_DELETE,

            // Role management for own application
            PlatformApplicationServicePermissions.APP_ROLE_VIEW,
            PlatformApplicationServicePermissions.APP_ROLE_CREATE,
            PlatformApplicationServicePermissions.APP_ROLE_UPDATE,
            PlatformApplicationServicePermissions.APP_ROLE_DELETE,

            // Permission management for own application
            PlatformApplicationServicePermissions.APP_PERMISSION_VIEW,
            PlatformApplicationServicePermissions.APP_PERMISSION_SYNC
        ),
        "Application service account - manages own application's resources"
    );

    private PlatformApplicationServiceRole() {}
}
