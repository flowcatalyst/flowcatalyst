package tech.flowcatalyst.platform.authorization.platform;

import tech.flowcatalyst.platform.authorization.Role;
import tech.flowcatalyst.platform.authorization.RoleDefinition;

import java.util.Set;

/**
 * IAM Administrator role - manages users, roles, permissions, and service accounts.
 */
@Role
public class PlatformIamAdminRole {
    public static final String ROLE_NAME = "platform:iam-admin";

    public static final RoleDefinition INSTANCE = RoleDefinition.make(
        "platform",
        "iam-admin",
        Set.of(
            PlatformIamPermissions.USER_VIEW,
            PlatformIamPermissions.USER_CREATE,
            PlatformIamPermissions.USER_UPDATE,
            PlatformIamPermissions.USER_DELETE,
            PlatformIamPermissions.ROLE_VIEW,
            PlatformIamPermissions.ROLE_CREATE,
            PlatformIamPermissions.ROLE_UPDATE,
            PlatformIamPermissions.ROLE_DELETE,
            PlatformIamPermissions.PERMISSION_VIEW,
            PlatformIamPermissions.SERVICE_ACCOUNT_VIEW,
            PlatformIamPermissions.SERVICE_ACCOUNT_CREATE,
            PlatformIamPermissions.SERVICE_ACCOUNT_UPDATE,
            PlatformIamPermissions.SERVICE_ACCOUNT_DELETE
        ),
        "IAM administrator - manages users, roles, and service accounts"
    );

    private PlatformIamAdminRole() {}
}
