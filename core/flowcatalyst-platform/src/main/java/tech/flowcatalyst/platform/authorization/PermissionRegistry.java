package tech.flowcatalyst.platform.authorization;

import io.quarkus.logging.Log;
import io.quarkus.runtime.Startup;
import io.quarkus.runtime.StartupEvent;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.event.Observes;
import tech.flowcatalyst.platform.authorization.platform.*;

import java.util.*;
import java.util.concurrent.ConcurrentHashMap;

/**
 * In-memory registry of all permission and role definitions.
 *
 * Permissions and roles are registered manually at startup.
 * Each @Permission class must have a public static final field named INSTANCE
 * of type PermissionDefinition.
 *
 * Each @Role class must have a public static final field named INSTANCE
 * of type RoleDefinition.
 *
 * The registry provides fast lookup of permissions and roles by their
 * string representation, and validates that all role permissions reference
 * valid permission definitions.
 *
 * This is the source of truth for all permissions and roles in the system.
 */
@ApplicationScoped
@Startup
public class PermissionRegistry {

    // Permission string -> PermissionDefinition
    private final Map<String, PermissionDefinition> permissions = new ConcurrentHashMap<>();

    // Role string -> RoleDefinition
    private final Map<String, RoleDefinition> roles = new ConcurrentHashMap<>();

    /**
     * Initialize the registry at startup by manually registering permissions and roles.
     */
    void onStart(@Observes StartupEvent event) {
        Log.info("Initializing PermissionRegistry...");

        // Register all permissions and roles
        registerAll();

        Log.info("PermissionRegistry initialization complete. Registered " +
            permissions.size() + " permissions and " + roles.size() + " roles");
    }

    /**
     * Manually register all permissions and roles.
     * Add new permission and role classes here.
     *
     * IMPORTANT: Permissions must be registered before roles that depend on them.
     */
    private void registerAll() {
        // ====================================================================
        // IAM Permissions
        // ====================================================================
        registerPermission(PlatformIamPermissions.USER_VIEW);
        registerPermission(PlatformIamPermissions.USER_CREATE);
        registerPermission(PlatformIamPermissions.USER_UPDATE);
        registerPermission(PlatformIamPermissions.USER_DELETE);
        registerPermission(PlatformIamPermissions.ROLE_VIEW);
        registerPermission(PlatformIamPermissions.ROLE_CREATE);
        registerPermission(PlatformIamPermissions.ROLE_UPDATE);
        registerPermission(PlatformIamPermissions.ROLE_DELETE);
        registerPermission(PlatformIamPermissions.PERMISSION_VIEW);
        registerPermission(PlatformIamPermissions.SERVICE_ACCOUNT_VIEW);
        registerPermission(PlatformIamPermissions.SERVICE_ACCOUNT_CREATE);
        registerPermission(PlatformIamPermissions.SERVICE_ACCOUNT_UPDATE);
        registerPermission(PlatformIamPermissions.SERVICE_ACCOUNT_DELETE);
        registerPermission(PlatformIamPermissions.IDP_MANAGE);

        // ====================================================================
        // Platform Admin Permissions
        // ====================================================================
        registerPermission(PlatformAdminPermissions.CLIENT_VIEW);
        registerPermission(PlatformAdminPermissions.CLIENT_CREATE);
        registerPermission(PlatformAdminPermissions.CLIENT_UPDATE);
        registerPermission(PlatformAdminPermissions.CLIENT_DELETE);
        registerPermission(PlatformAdminPermissions.APPLICATION_VIEW);
        registerPermission(PlatformAdminPermissions.APPLICATION_CREATE);
        registerPermission(PlatformAdminPermissions.APPLICATION_UPDATE);
        registerPermission(PlatformAdminPermissions.APPLICATION_DELETE);
        registerPermission(PlatformAdminPermissions.CONFIG_VIEW);
        registerPermission(PlatformAdminPermissions.CONFIG_UPDATE);

        // ====================================================================
        // Messaging Permissions
        // ====================================================================
        registerPermission(PlatformMessagingPermissions.EVENT_VIEW);
        registerPermission(PlatformMessagingPermissions.EVENT_VIEW_RAW);
        registerPermission(PlatformMessagingPermissions.EVENT_TYPE_VIEW);
        registerPermission(PlatformMessagingPermissions.EVENT_TYPE_CREATE);
        registerPermission(PlatformMessagingPermissions.EVENT_TYPE_UPDATE);
        registerPermission(PlatformMessagingPermissions.EVENT_TYPE_DELETE);
        registerPermission(PlatformMessagingPermissions.SUBSCRIPTION_VIEW);
        registerPermission(PlatformMessagingPermissions.SUBSCRIPTION_CREATE);
        registerPermission(PlatformMessagingPermissions.SUBSCRIPTION_UPDATE);
        registerPermission(PlatformMessagingPermissions.SUBSCRIPTION_DELETE);
        registerPermission(PlatformMessagingPermissions.DISPATCH_JOB_VIEW);
        registerPermission(PlatformMessagingPermissions.DISPATCH_JOB_VIEW_RAW);
        registerPermission(PlatformMessagingPermissions.DISPATCH_JOB_CREATE);
        registerPermission(PlatformMessagingPermissions.DISPATCH_JOB_RETRY);
        registerPermission(PlatformMessagingPermissions.DISPATCH_POOL_VIEW);
        registerPermission(PlatformMessagingPermissions.DISPATCH_POOL_CREATE);
        registerPermission(PlatformMessagingPermissions.DISPATCH_POOL_UPDATE);
        registerPermission(PlatformMessagingPermissions.DISPATCH_POOL_DELETE);

        // ====================================================================
        // Application Service Permissions
        // ====================================================================
        registerPermission(PlatformApplicationServicePermissions.APP_EVENT_TYPE_VIEW);
        registerPermission(PlatformApplicationServicePermissions.APP_EVENT_TYPE_CREATE);
        registerPermission(PlatformApplicationServicePermissions.APP_EVENT_TYPE_UPDATE);
        registerPermission(PlatformApplicationServicePermissions.APP_EVENT_TYPE_DELETE);
        registerPermission(PlatformApplicationServicePermissions.APP_SUBSCRIPTION_VIEW);
        registerPermission(PlatformApplicationServicePermissions.APP_SUBSCRIPTION_CREATE);
        registerPermission(PlatformApplicationServicePermissions.APP_SUBSCRIPTION_UPDATE);
        registerPermission(PlatformApplicationServicePermissions.APP_SUBSCRIPTION_DELETE);
        registerPermission(PlatformApplicationServicePermissions.APP_ROLE_VIEW);
        registerPermission(PlatformApplicationServicePermissions.APP_ROLE_CREATE);
        registerPermission(PlatformApplicationServicePermissions.APP_ROLE_UPDATE);
        registerPermission(PlatformApplicationServicePermissions.APP_ROLE_DELETE);
        registerPermission(PlatformApplicationServicePermissions.APP_PERMISSION_VIEW);
        registerPermission(PlatformApplicationServicePermissions.APP_PERMISSION_SYNC);

        // ====================================================================
        // Platform Roles (must be after permissions)
        // ====================================================================
        registerRole(PlatformSuperAdminRole.INSTANCE);
        registerRole(PlatformIamAdminRole.INSTANCE);
        registerRole(PlatformAdminRole.INSTANCE);
        registerRole(PlatformMessagingAdminRole.INSTANCE);
        registerRole(PlatformApplicationServiceRole.INSTANCE);
    }

    /**
     * Register a permission definition.
     * If a permission with the same string already exists, it is silently skipped.
     */
    public void registerPermission(PermissionDefinition permission) {
        String key = permission.toPermissionString();
        if (permissions.containsKey(key)) {
            Log.debug("Permission already registered, skipping: " + key);
            return;
        }
        permissions.put(key, permission);
        Log.debug("Registered permission: " + key);
    }

    /**
     * Register a role definition.
     * Validates that all role permissions reference existing permissions.
     * If a role with the same string already exists, it is silently skipped.
     */
    public void registerRole(RoleDefinition role) {
        String key = role.toRoleString();
        if (roles.containsKey(key)) {
            Log.debug("Role already registered, skipping: " + key);
            return;
        }

        // Validate that all role permissions reference existing permissions
        for (PermissionRecord permission : role.permissions()) {
            String permissionString = permission.toPermissionString();
            if (!permissions.containsKey(permissionString)) {
                throw new IllegalStateException(
                    "Role " + key + " references unknown permission: " + permissionString
                );
            }
        }

        roles.put(key, role);
        Log.debug("Registered role: " + key + " with " + role.permissions().size() + " permissions");
    }

    /**
     * Dynamically register a role from the database or SDK.
     * Unlike registerRole(), this does not validate permissions against the registry,
     * as external applications may define their own permission schemes.
     *
     * If a role with the same name already exists, it will be updated.
     *
     * @param roleName Full role name (e.g., "myapp:admin")
     * @param permissionStrings Set of permission strings
     * @param description Human-readable description
     */
    public void registerRoleDynamic(String roleName, Set<String> permissionStrings, String description) {
        if (roleName == null || roleName.isBlank()) {
            Log.warn("Cannot register role with null or blank name");
            return;
        }

        // Parse role name into subdomain and role parts
        String[] parts = roleName.split(":", 2);
        String subdomain = parts[0];
        String roleNamePart = parts.length > 1 ? parts[1] : parts[0];

        // Create a RoleDefinition from the permission strings
        RoleDefinition roleDef = RoleDefinition.makeFromStrings(
            subdomain,
            roleNamePart,
            permissionStrings != null ? permissionStrings : Set.of(),
            description != null ? description : ""
        );

        roles.put(roleName, roleDef);
        Log.debug("Dynamically registered role: " + roleName + " with " +
            (permissionStrings != null ? permissionStrings.size() : 0) + " permissions");
    }

    /**
     * Unregister a role. Used when roles are deleted from the database.
     *
     * @param roleName The role name to unregister
     * @return true if the role was found and removed
     */
    public boolean unregisterRole(String roleName) {
        RoleDefinition removed = roles.remove(roleName);
        if (removed != null) {
            Log.debug("Unregistered role: " + roleName);
            return true;
        }
        return false;
    }

    /**
     * Get a permission definition by its string representation.
     *
     * @param permissionString Permission string (e.g., "logistics:dispatch:order:create")
     * @return Optional containing the permission definition if found
     */
    public Optional<PermissionDefinition> getPermission(String permissionString) {
        return Optional.ofNullable(permissions.get(permissionString));
    }

    /**
     * Get a role definition by its string representation.
     *
     * @param roleString Role string (e.g., "logistics:dispatcher")
     * @return Optional containing the role definition if found
     */
    public Optional<RoleDefinition> getRole(String roleString) {
        return Optional.ofNullable(roles.get(roleString));
    }

    /**
     * Check if a permission exists.
     *
     * @param permissionString Permission string
     * @return true if the permission is registered
     */
    public boolean hasPermission(String permissionString) {
        return permissions.containsKey(permissionString);
    }

    /**
     * Check if a role exists.
     *
     * @param roleString Role string
     * @return true if the role is registered
     */
    public boolean hasRole(String roleString) {
        return roles.containsKey(roleString);
    }

    /**
     * Get all registered permissions.
     *
     * @return Unmodifiable collection of all permission definitions
     */
    public Collection<PermissionDefinition> getAllPermissions() {
        return Collections.unmodifiableCollection(permissions.values());
    }

    /**
     * Get all registered roles.
     *
     * @return Unmodifiable collection of all role definitions
     */
    public Collection<RoleDefinition> getAllRoles() {
        return Collections.unmodifiableCollection(roles.values());
    }

    /**
     * Get all permissions granted by a role.
     * This provides access to full permission metadata (subdomain, context, aggregate, action, description).
     *
     * @param roleString Role string
     * @return Set of permissions, or empty set if role not found
     */
    public Set<PermissionRecord> getPermissionsForRole(String roleString) {
        RoleDefinition role = roles.get(roleString);
        return role != null ? role.permissions() : Collections.emptySet();
    }

    /**
     * Get all permission strings granted by multiple roles.
     *
     * @param roleStrings Collection of role strings
     * @return Set of unique permission strings from all roles
     */
    public Set<String> getPermissionsForRoles(Collection<String> roleStrings) {
        Set<String> allPermissions = new HashSet<>();
        for (String roleString : roleStrings) {
            RoleDefinition role = roles.get(roleString);
            if (role != null) {
                allPermissions.addAll(role.permissionStrings());
            }
        }
        return allPermissions;
    }

    // ========================================================================
    // Application-aware role methods
    // ========================================================================

    /**
     * Extract the application code from a role string.
     * Role format: {application}:{subdomain}:{role-name} or {application}:{role-name}
     *
     * Examples:
     * - "operant:dispatch:admin" → "operant"
     * - "platform:admin" → "platform"
     *
     * @param roleString The full role string
     * @return The application code (first segment before colon)
     */
    public static String extractApplicationCode(String roleString) {
        if (roleString == null || roleString.isBlank()) {
            return null;
        }
        int idx = roleString.indexOf(':');
        return idx > 0 ? roleString.substring(0, idx) : roleString;
    }

    /**
     * Get the display name for a role (without application prefix).
     * Role format: {application}:{display-name}
     *
     * Examples:
     * - "operant:dispatch:admin" → "dispatch:admin"
     * - "platform:admin" → "admin"
     *
     * @param roleString The full role string
     * @return The display name (everything after first colon)
     */
    public static String getDisplayName(String roleString) {
        if (roleString == null || roleString.isBlank()) {
            return null;
        }
        int idx = roleString.indexOf(':');
        return idx > 0 ? roleString.substring(idx + 1) : roleString;
    }

    /**
     * Get all roles for a specific application.
     *
     * @param applicationCode The application code to filter by
     * @return Collection of role definitions for that application
     */
    public Collection<RoleDefinition> getRolesForApplication(String applicationCode) {
        if (applicationCode == null || applicationCode.isBlank()) {
            return Collections.emptyList();
        }
        String prefix = applicationCode + ":";
        return roles.values().stream()
            .filter(r -> r.toRoleString().startsWith(prefix))
            .toList();
    }

    /**
     * Extract unique application codes from a collection of role strings.
     *
     * @param roleStrings Collection of role strings
     * @return Set of unique application codes
     */
    public static Set<String> extractApplicationCodes(Collection<String> roleStrings) {
        if (roleStrings == null || roleStrings.isEmpty()) {
            return Collections.emptySet();
        }
        Set<String> appCodes = new HashSet<>();
        for (String role : roleStrings) {
            String appCode = extractApplicationCode(role);
            if (appCode != null) {
                appCodes.add(appCode);
            }
        }
        return appCodes;
    }

    /**
     * Filter roles to only those for a specific application.
     *
     * @param roleStrings Collection of role strings
     * @param applicationCode The application code to filter by
     * @return Set of roles belonging to that application
     */
    public static Set<String> filterRolesForApplication(Collection<String> roleStrings, String applicationCode) {
        if (roleStrings == null || roleStrings.isEmpty() || applicationCode == null) {
            return Collections.emptySet();
        }
        String prefix = applicationCode + ":";
        return roleStrings.stream()
            .filter(r -> r != null && r.startsWith(prefix))
            .collect(java.util.stream.Collectors.toSet());
    }
}
