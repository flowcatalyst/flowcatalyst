package tech.flowcatalyst.platform.test;

import tech.flowcatalyst.platform.authorization.Role;
import tech.flowcatalyst.platform.authorization.RoleDefinition;
import java.util.Set;

@Role
public class PlatformTestClientAdminRole {
    public static final RoleDefinition INSTANCE = RoleDefinition.make(
        "platform",
        "test-client-admin",
        Set.of(
            PlatformClientUserCreateTestPermission.INSTANCE,
            PlatformClientUserViewTestPermission.INSTANCE,
            PlatformClientManageTestPermission.INSTANCE
        ),
        "Test client admin role"
    );
    private PlatformTestClientAdminRole() {}
}
