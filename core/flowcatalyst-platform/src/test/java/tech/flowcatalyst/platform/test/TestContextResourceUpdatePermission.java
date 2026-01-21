package tech.flowcatalyst.platform.test;

import tech.flowcatalyst.platform.authorization.Permission;
import tech.flowcatalyst.platform.authorization.PermissionDefinition;

@Permission
public class TestContextResourceUpdatePermission {
    public static final PermissionDefinition INSTANCE = PermissionDefinition.make(
        "test", "context", "resource", "update", "Test update permission"
    );
    private TestContextResourceUpdatePermission() {}
}
