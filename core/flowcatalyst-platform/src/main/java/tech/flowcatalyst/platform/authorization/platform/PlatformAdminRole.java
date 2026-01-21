package tech.flowcatalyst.platform.authorization.platform;

import tech.flowcatalyst.platform.authorization.Role;
import tech.flowcatalyst.platform.authorization.RoleDefinition;

import java.util.Set;

/**
 * Platform Administrator role - manages clients, applications, and platform configuration.
 */
@Role
public class PlatformAdminRole {
    public static final String ROLE_NAME = "platform:platform-admin";

    public static final RoleDefinition INSTANCE = RoleDefinition.make(
        "platform",
        "platform-admin",
        Set.of(
            PlatformAdminPermissions.CLIENT_VIEW,
            PlatformAdminPermissions.CLIENT_CREATE,
            PlatformAdminPermissions.CLIENT_UPDATE,
            PlatformAdminPermissions.CLIENT_DELETE,
            PlatformAdminPermissions.APPLICATION_VIEW,
            PlatformAdminPermissions.APPLICATION_CREATE,
            PlatformAdminPermissions.APPLICATION_UPDATE,
            PlatformAdminPermissions.APPLICATION_DELETE,
            PlatformAdminPermissions.CONFIG_VIEW,
            PlatformAdminPermissions.CONFIG_UPDATE,
            // IDP management - configure authentication for domains
            PlatformIamPermissions.IDP_MANAGE
        ),
        "Platform administrator - manages clients, applications, and identity providers"
    );

    private PlatformAdminRole() {}
}
