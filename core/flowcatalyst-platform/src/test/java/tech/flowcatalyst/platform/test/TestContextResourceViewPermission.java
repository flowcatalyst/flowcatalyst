package tech.flowcatalyst.platform.test;

import tech.flowcatalyst.platform.authorization.Permission;
import tech.flowcatalyst.platform.authorization.PermissionDefinition;

@Permission
public class TestContextResourceViewPermission {
    public static final PermissionDefinition INSTANCE = PermissionDefinition.make(
        "test", "context", "resource", "view", "Test view permission"
    );
    private TestContextResourceViewPermission() {}
}
