package tech.flowcatalyst.platform.test;

import tech.flowcatalyst.platform.authorization.Role;
import tech.flowcatalyst.platform.authorization.RoleDefinition;
import java.util.Set;

@Role
public class TestEditorRole {
    public static final RoleDefinition INSTANCE = RoleDefinition.make(
        "test",
        "editor",
        Set.of(
            TestContextResourceCreatePermission.INSTANCE,
            TestContextResourceViewPermission.INSTANCE,
            TestContextResourceUpdatePermission.INSTANCE
        ),
        "Test editor role with create and update permissions"
    );
    private TestEditorRole() {}
}
