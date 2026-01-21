package tech.flowcatalyst.platform.test;

import tech.flowcatalyst.platform.authorization.Permission;
import tech.flowcatalyst.platform.authorization.PermissionDefinition;

@Permission
public class PlatformClientUserCreateTestPermission {
    public static final PermissionDefinition INSTANCE = PermissionDefinition.make(
        "platform", "client", "user", "create", "Create users in client (test)"
    );
    private PlatformClientUserCreateTestPermission() {}
}
