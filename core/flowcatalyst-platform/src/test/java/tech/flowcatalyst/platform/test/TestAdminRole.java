package tech.flowcatalyst.platform.test;

import tech.flowcatalyst.platform.authorization.Role;
import tech.flowcatalyst.platform.authorization.RoleDefinition;

import java.util.Set;

/**
 * Test admin role with all test permissions.
 */
@Role
public class TestAdminRole {
    public static final RoleDefinition INSTANCE = RoleDefinition.make(
        "test",
        "admin",
        Set.of(
            TestContextResourceCreatePermission.INSTANCE,
            TestContextResourceViewPermission.INSTANCE,
            TestContextResourceUpdatePermission.INSTANCE,
            TestContextResourceDeletePermission.INSTANCE
        ),
        "Test admin role with all permissions"
    );

    private TestAdminRole() {}
}
