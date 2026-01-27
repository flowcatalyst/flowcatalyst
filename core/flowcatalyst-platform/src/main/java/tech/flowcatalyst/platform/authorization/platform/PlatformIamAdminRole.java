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
            PlatformIamPermissions.SERVICE_ACCOUNT_DELETE,
            // OAuth client management
            PlatformIamPermissions.OAUTH_CLIENT_VIEW,
            PlatformIamPermissions.OAUTH_CLIENT_CREATE,
            PlatformIamPermissions.OAUTH_CLIENT_UPDATE,
            PlatformIamPermissions.OAUTH_CLIENT_DELETE,
            // Anchor domain management
            PlatformIamPermissions.ANCHOR_DOMAIN_VIEW,
            PlatformIamPermissions.ANCHOR_DOMAIN_MANAGE
        ),
        "IAM administrator - manages users, roles, and service accounts"
    );

    private PlatformIamAdminRole() {}
}
