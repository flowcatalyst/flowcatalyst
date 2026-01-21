package tech.flowcatalyst.platform.authorization;

import java.time.Instant;
import java.util.HashSet;
import java.util.Set;

/**
 * Represents a role definition stored in the database.
 *
 * Roles can come from three sources:
 * - CODE: Defined in Java @Role classes, synced to DB at startup
 * - DATABASE: Created by administrators through the UI
 * - SDK: Registered by external applications via the SDK API
 *
 * The role name is prefixed with the application code (e.g., "platform:tenant-admin").
 * SDK roles are auto-prefixed when registered.
 */
public class AuthRole {

    public String id;

    /**
     * The application this role belongs to (stored as ID reference).
     */
    public String applicationId;

    /**
     * The application code (denormalized for queries).
     */
    public String applicationCode;

    /**
     * Full role name with application prefix (e.g., "platform:tenant-admin").
     */
    public String name;

    /**
     * Human-readable display name (e.g., "Tenant Administrator").
     */
    public String displayName;

    /**
     * Description of what this role grants access to.
     */
    public String description;

    /**
     * Set of permission strings granted by this role.
     */
    public Set<String> permissions = new HashSet<>();

    /**
     * Source of this role definition.
     * CODE = from @Role Java classes (synced at startup)
     * DATABASE = created by admin through UI
     * SDK = registered by external application
     */
    public RoleSource source = RoleSource.DATABASE;

    /**
     * If true, this role syncs to IDPs configured for client-managed roles.
     * Used for selective IDP synchronization.
     */
    public boolean clientManaged = false;

    public Instant createdAt = Instant.now();

    public Instant updatedAt = Instant.now();

    public AuthRole() {
    }

    /**
     * Create a role for an application.
     * The name should already include the application prefix.
     */
    public AuthRole(String applicationId, String applicationCode, String name, String description, Set<String> permissions, RoleSource source) {
        this.applicationId = applicationId;
        this.applicationCode = applicationCode;
        this.name = name;
        this.description = description;
        this.permissions = permissions != null ? permissions : new HashSet<>();
        this.source = source;
    }

    /**
     * Extract the role name without the application prefix.
     */
    public String getShortName() {
        if (name != null && name.contains(":")) {
            return name.substring(name.indexOf(':') + 1);
        }
        return name;
    }

    /**
     * Source of a role definition.
     */
    public enum RoleSource {
        /** Defined in Java @Role classes, synced to DB at startup */
        CODE,
        /** Created by administrators through the UI */
        DATABASE,
        /** Registered by external applications via the SDK API */
        SDK
    }
}
