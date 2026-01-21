package tech.flowcatalyst.platform.test;

import tech.flowcatalyst.platform.authorization.Permission;
import tech.flowcatalyst.platform.authorization.PermissionDefinition;

@Permission
public class TestContextResourceDeletePermission {
    public static final PermissionDefinition INSTANCE = PermissionDefinition.make(
        "test", "context", "resource", "delete", "Test delete permission"
    );
    private TestContextResourceDeletePermission() {}
}
