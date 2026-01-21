package tech.flowcatalyst.platform.test;

import tech.flowcatalyst.platform.authorization.Permission;
import tech.flowcatalyst.platform.authorization.PermissionDefinition;

@Permission
public class PlatformClientUserViewTestPermission {
    public static final PermissionDefinition INSTANCE = PermissionDefinition.make(
        "platform", "client", "user", "view", "View users in client (test)"
    );
    private PlatformClientUserViewTestPermission() {}
}
