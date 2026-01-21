package tech.flowcatalyst.platform.authorization.platform;

import tech.flowcatalyst.platform.authorization.Role;
import tech.flowcatalyst.platform.authorization.RoleDefinition;

import java.util.Set;

/**
 * Integration role for external systems.
 *
 * This role is designed for external integrations that need to:
 * - Create dispatch jobs (send events)
 * - View their own dispatch jobs
 *
 * Minimal permissions - just what's needed to publish events.
 * For service principals (not human users).
 */
@Role
public class PlatformIntegrationRole {
    public static final String ROLE_NAME = "platform:integration";

    public static final RoleDefinition INSTANCE = RoleDefinition.make(
        "platform",
        "integration",
        Set.of(
            PlatformMessagingPermissions.DISPATCH_JOB_CREATE,
            PlatformMessagingPermissions.DISPATCH_JOB_VIEW
        ),
        "Integration role - create and view dispatch jobs"
    );

    private PlatformIntegrationRole() {}
}
