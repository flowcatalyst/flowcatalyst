package tech.flowcatalyst.platform.test;

import tech.flowcatalyst.platform.authorization.Permission;
import tech.flowcatalyst.platform.authorization.PermissionDefinition;

@Permission
public class TestContextResourceCreatePermission {
    public static final PermissionDefinition INSTANCE = PermissionDefinition.make(
        "test", "context", "resource", "create", "Test create permission"
    );
    private TestContextResourceCreatePermission() {}
}
