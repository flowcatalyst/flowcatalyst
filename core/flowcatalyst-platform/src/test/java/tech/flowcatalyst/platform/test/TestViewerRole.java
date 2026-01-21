package tech.flowcatalyst.platform.test;

import tech.flowcatalyst.platform.authorization.Role;
import tech.flowcatalyst.platform.authorization.RoleDefinition;
import java.util.Set;

@Role
public class TestViewerRole {
    public static final RoleDefinition INSTANCE = RoleDefinition.make(
        "test",
        "viewer",
        Set.of(TestContextResourceViewPermission.INSTANCE),
        "Test viewer role with read-only access"
    );
    private TestViewerRole() {}
}
