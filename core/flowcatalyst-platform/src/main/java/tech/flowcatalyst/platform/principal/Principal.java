package tech.flowcatalyst.platform.principal;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Set;
import java.util.stream.Collectors;

/**
 * Unified identity model for both users and service accounts.
 * Follows the architecture documented in docs/auth-architecture.md
 */
public class Principal {

    public String id;

    public PrincipalType type;

    /**
     * Access scope for user principals.
     * Determines which clients this user can access.
     * - ANCHOR: Can access all clients (platform admin)
     * - PARTNER: Can access explicitly assigned clients
     * - CLIENT: Can only access their home client
     *
     * For SERVICE principals, this is typically null.
     */
    public UserScope scope;

    /**
     * Client this principal belongs to (home client).
     * - For CLIENT scope: Required, determines their access
     * - For PARTNER scope: Optional, may have a home client
     * - For ANCHOR scope: Should be null
     * - For SERVICE type with client scope: The client the service account belongs to
     */
    public String clientId;


    /**
     * Applications this principal has access to.
     * Users get no applications by default - each must be explicitly granted.
     * This applies to all user scopes including ANCHOR users.
     */
    public List<String> accessibleApplicationIds = new ArrayList<>();

    public String name;

    public boolean active = true;

    public Instant createdAt = Instant.now();

    public Instant updatedAt = Instant.now();

    /**
     * Embedded user identity (for USER type).
     * Fields are stored as flat columns in the database but assembled here for convenience.
     */
    public UserIdentity userIdentity;

    /**
     * Foreign key to the ServiceAccount entity (for SERVICE type).
     * This is the primary way to link a Principal to a ServiceAccount.
     * The ServiceAccount entity contains webhook credentials and other metadata.
     */
    public String serviceAccountId;


    /**
     * Embedded role assignments (denormalized for MongoDB).
     * This is the source of truth for principal roles.
     */
    public List<RoleAssignment> roles = new ArrayList<>();

    public Principal() {
    }

    /**
     * Get role names as a set for quick lookup.
     */
    public Set<String> getRoleNames() {
        return roles.stream()
            .map(r -> r.roleName)
            .filter(java.util.Objects::nonNull)
            .collect(Collectors.toSet());
    }

    /**
     * Check if principal has a specific role.
     */
    public boolean hasRole(String roleName) {
        return roles.stream().anyMatch(r -> r.roleName.equals(roleName));
    }

    /**
     * Check if principal has access to a specific application.
     */
    public boolean hasApplicationAccess(String applicationId) {
        return accessibleApplicationIds != null && accessibleApplicationIds.contains(applicationId);
    }

    /**
     * Get accessible application IDs as a set for quick lookup.
     */
    public Set<String> getAccessibleApplicationIds() {
        if (accessibleApplicationIds == null) {
            return Set.of();
        }
        return Set.copyOf(accessibleApplicationIds);
    }

    /**
     * Embedded role assignment.
     */
    public static class RoleAssignment {
        public String roleName;
        public String assignmentSource;
        public Instant assignedAt;

        public RoleAssignment() {
        }

        public RoleAssignment(String roleName, String assignmentSource) {
            this.roleName = roleName;
            this.assignmentSource = assignmentSource;
            this.assignedAt = Instant.now();
        }

        public RoleAssignment(String roleName, String assignmentSource, Instant assignedAt) {
            this.roleName = roleName;
            this.assignmentSource = assignmentSource;
            this.assignedAt = assignedAt;
        }
    }
}
