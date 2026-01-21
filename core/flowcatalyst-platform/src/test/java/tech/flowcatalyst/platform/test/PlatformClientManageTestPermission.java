package tech.flowcatalyst.platform.test;

import tech.flowcatalyst.platform.authorization.Permission;
import tech.flowcatalyst.platform.authorization.PermissionDefinition;

@Permission
public class PlatformClientManageTestPermission {
    public static final PermissionDefinition INSTANCE = PermissionDefinition.make(
        "platform", "client", "client", "manage", "Manage client settings (test)"
    );
    private PlatformClientManageTestPermission() {}
}
