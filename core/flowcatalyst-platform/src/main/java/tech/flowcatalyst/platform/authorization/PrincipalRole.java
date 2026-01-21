package tech.flowcatalyst.platform.authorization;

import java.time.Instant;

/**
 * DTO for role assignment information.
 * Used for API responses and internal compatibility.
 *
 * Actual role storage is embedded in Principal.roles (MongoDB denormalized pattern).
 */
public class PrincipalRole {

    public String principalId;

    /**
     * String role name (e.g., "platform:tenant-admin", "logistics:dispatcher").
     * Must match a role defined in PermissionRegistry.
     */
    public String roleName;

    /**
     * How this role was assigned (MANUAL, IDP_SYNC)
     */
    public String assignmentSource;

    public Instant assignedAt;

    public PrincipalRole() {
    }

    /**
     * Create from Principal.RoleAssignment for API responses.
     */
    public static PrincipalRole from(String principalId, tech.flowcatalyst.platform.principal.Principal.RoleAssignment assignment) {
        PrincipalRole pr = new PrincipalRole();
        pr.principalId = principalId;
        pr.roleName = assignment.roleName;
        pr.assignmentSource = assignment.assignmentSource;
        pr.assignedAt = assignment.assignedAt;
        return pr;
    }
}
