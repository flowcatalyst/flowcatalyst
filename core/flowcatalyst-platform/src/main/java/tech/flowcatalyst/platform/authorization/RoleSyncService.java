package tech.flowcatalyst.platform.authorization;

import io.quarkus.logging.Log;
import io.quarkus.runtime.Startup;
import io.quarkus.runtime.StartupEvent;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.event.Observes;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.ApplicationRepository;

import java.util.HashSet;
import java.util.List;
import java.util.Optional;
import java.util.Set;

/**
 * Service responsible for synchronizing roles between code definitions
 * and the database at startup.
 *
 * On startup:
 * 1. Code-defined roles (@Role classes) are synced to the auth_roles table
 * 2. Database roles are loaded and registered into PermissionRegistry
 *
 * NOTE: This service only handles startup synchronization. For user-triggered
 * CRUD operations with audit logging, use RoleAdminService.
 */
@ApplicationScoped
@Startup
public class RoleSyncService {

    @Inject
    PermissionRegistry permissionRegistry;

    @Inject
    AuthRoleRepository authRoleRepository;

    @Inject
    ApplicationRepository applicationRepository;

    /**
     * Sync roles after PermissionRegistry has been initialized.
     * Uses a priority of 100 to ensure it runs after PermissionRegistry's onStart (default priority).
     */
    void onStart(@Observes StartupEvent event) {
        Log.info("RoleSyncService: Starting role synchronization...");

        // Step 1: Sync code-defined roles to database
        syncCodeDefinedRolesToDatabase();

        // Step 2: Load database roles into PermissionRegistry
        loadDatabaseRolesIntoRegistry();

        Log.info("RoleSyncService: Role synchronization complete");
    }

    /**
     * Sync all code-defined roles (@Role classes) to the database.
     * Creates or updates roles with source=CODE.
     *
     * This is a system operation that happens at startup and does not
     * require audit logging (no user action involved).
     */
    public void syncCodeDefinedRolesToDatabase() {
        Log.info("Syncing code-defined roles to database...");

        int created = 0;
        int updated = 0;

        for (RoleDefinition roleDef : permissionRegistry.getAllRoles()) {
            String roleName = roleDef.toRoleString();
            String appCode = PermissionRegistry.extractApplicationCode(roleName);

            if (appCode == null) {
                Log.warn("Skipping role with no application code: " + roleName);
                continue;
            }

            // Find or create the application
            Optional<Application> appOpt = applicationRepository.findByCode(appCode);
            if (appOpt.isEmpty()) {
                Log.warn("Application not found for role " + roleName + ", creating: " + appCode);
                Application app = new Application(appCode, appCode);
                app.id = tech.flowcatalyst.platform.shared.TsidGenerator.generate();
                applicationRepository.persist(app);
                appOpt = Optional.of(app);
            }
            Application app = appOpt.get();

            // Find existing role or create new
            Optional<AuthRole> existingOpt = authRoleRepository.findByName(roleName);

            if (existingOpt.isPresent()) {
                // Update existing CODE role
                AuthRole existing = existingOpt.get();
                if (existing.source == AuthRole.RoleSource.CODE) {
                    existing.description = roleDef.description();
                    existing.permissions = roleDef.permissionStrings();
                    existing.displayName = formatDisplayName(roleDef.roleName());
                    authRoleRepository.update(existing);
                    updated++;
                } else {
                    Log.warn("Role " + roleName + " exists with source " + existing.source +
                             ", not overwriting with CODE definition");
                }
            } else {
                // Create new role
                AuthRole authRole = new AuthRole(
                    app.id,
                    app.code,
                    roleName,
                    roleDef.description(),
                    roleDef.permissionStrings(),
                    AuthRole.RoleSource.CODE
                );
                authRole.id = tech.flowcatalyst.platform.shared.TsidGenerator.generate();
                authRole.displayName = formatDisplayName(roleDef.roleName());
                authRoleRepository.persist(authRole);
                created++;
            }
        }

        // Remove stale CODE roles that no longer exist in code
        int removed = removeStaleCodeRoles();

        Log.info("Code role sync complete: " + created + " created, " + updated + " updated, " + removed + " removed");
    }

    /**
     * Remove CODE-sourced roles from the database that no longer exist in code.
     * This cleans up roles that were renamed or deleted from Java code.
     *
     * @return number of roles removed
     */
    int removeStaleCodeRoles() {
        // Collect all current code-defined role names
        Set<String> codeRoleNames = new HashSet<>();
        for (RoleDefinition roleDef : permissionRegistry.getAllRoles()) {
            codeRoleNames.add(roleDef.toRoleString());
        }

        // Find CODE roles in DB that aren't in the current code definitions
        List<AuthRole> codeRolesInDb = authRoleRepository.findBySource(AuthRole.RoleSource.CODE);
        int removed = 0;

        for (AuthRole dbRole : codeRolesInDb) {
            if (!codeRoleNames.contains(dbRole.name)) {
                Log.info("Removing stale CODE role: " + dbRole.name);
                authRoleRepository.delete(dbRole);
                removed++;
            }
        }

        return removed;
    }

    /**
     * Load all database roles into the PermissionRegistry.
     * This includes SDK and DATABASE sourced roles that aren't defined in code.
     */
    public void loadDatabaseRolesIntoRegistry() {
        Log.info("Loading database roles into PermissionRegistry...");

        int loaded = 0;

        List<AuthRole> dbRoles = authRoleRepository.listAll();
        for (AuthRole authRole : dbRoles) {
            // Skip if already registered (code-defined roles are already there)
            if (permissionRegistry.hasRole(authRole.name)) {
                continue;
            }

            // Register database role into the registry
            permissionRegistry.registerRoleDynamic(
                authRole.name,
                authRole.permissions,
                authRole.description
            );
            loaded++;
        }

        Log.info("Loaded " + loaded + " database roles into PermissionRegistry");
    }

    /**
     * Format a role name into a display name.
     * Converts "tenant-admin" to "Tenant Admin".
     */
    private String formatDisplayName(String roleName) {
        if (roleName == null || roleName.isBlank()) {
            return roleName;
        }
        String[] parts = roleName.split("-");
        StringBuilder sb = new StringBuilder();
        for (String part : parts) {
            if (!part.isEmpty()) {
                if (sb.length() > 0) {
                    sb.append(" ");
                }
                sb.append(Character.toUpperCase(part.charAt(0)));
                if (part.length() > 1) {
                    sb.append(part.substring(1));
                }
            }
        }
        return sb.toString();
    }
}
