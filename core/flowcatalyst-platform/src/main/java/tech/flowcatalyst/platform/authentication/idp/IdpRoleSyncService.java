package tech.flowcatalyst.platform.authentication.idp;

import io.quarkus.logging.Log;
import io.quarkus.runtime.StartupEvent;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.event.Observes;
import jakarta.inject.Inject;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import tech.flowcatalyst.platform.authorization.PermissionDefinition;
import tech.flowcatalyst.platform.authorization.PermissionRegistry;
import tech.flowcatalyst.platform.authorization.RoleDefinition;

import java.util.*;
import java.util.stream.Collectors;

/**
 * Service that syncs code-first roles and permissions to the platform IDP.
 *
 * This service:
 * 1. Gets all role/permission definitions from PermissionRegistry (code-first)
 * 2. Gets the active platform IDP configuration
 * 3. Creates the appropriate IDP adapter (Keycloak, Entra, etc.)
 * 4. Pushes all roles/permissions to the IDP
 *
 * Sync is triggered:
 * - At application startup (if flowcatalyst.idp.sync-on-startup=true)
 * - On-demand via syncNow() method (recommended)
 * - Via CLI: ./gradlew quarkusDev -- idp sync
 *
 * NOTE: This only syncs to the PLATFORM IDP (the IDP we control).
 * External IDPs (partner/customer IDPs) are NOT synced - we pull roles from them.
 *
 * Configuration:
 * - flowcatalyst.idp.sync-on-startup: Enable/disable automatic sync at startup (default: false)
 */
@ApplicationScoped
public class IdpRoleSyncService {

    @Inject
    PermissionRegistry permissionRegistry;

    @Inject
    PlatformIdpProperties platformIdpProperties;

    @Inject
    AnchorIdpProperties anchorIdpProperties;

    @ConfigProperty(name = "flowcatalyst.idp.sync-on-startup", defaultValue = "false")
    boolean syncOnStartup;

    private boolean initialSyncComplete = false;

    /**
     * Sync roles to platform IDP at startup (after PermissionRegistry is initialized).
     * Only runs if flowcatalyst.idp.sync-on-startup=true.
     * Runs async to avoid blocking startup if IDP is unavailable.
     */
    void onStart(@Observes StartupEvent event) {
        if (!syncOnStartup) {
            Log.info("IDP sync on startup is disabled. Use CLI command 'idp sync' to sync manually.");
            return;
        }

        // Schedule initial sync after startup (give PermissionRegistry time to initialize)
        new Thread(() -> {
            try {
                Thread.sleep(5000); // Wait 5 seconds for PermissionRegistry to initialize
                Log.info("Running initial IDP role sync...");
                syncNow();
                initialSyncComplete = true;
            } catch (Exception e) {
                Log.error("Initial IDP role sync failed: " + e.getMessage(), e);
            }
        }).start();
    }

    /**
     * Sync all roles and permissions to ALL configured IDPs immediately.
     * Syncs to both platform IDP and anchor tenant IDP (if enabled).
     *
     * @throws IdpSyncException if any sync fails
     */
    public void syncNow() throws IdpSyncException {
        Log.info("Starting IDP role sync to all configured targets...");

        // Get all roles and permissions from registry (same for all IDPs)
        Collection<RoleDefinition> allRoles = permissionRegistry.getAllRoles();
        Collection<PermissionDefinition> allPermissions = permissionRegistry.getAllPermissions();
        Set<RoleDefinition> roles = allRoles.stream().collect(Collectors.toSet());
        Set<PermissionDefinition> permissions = allPermissions.stream().collect(Collectors.toSet());

        Log.info("Syncing " + roles.size() + " roles and " + permissions.size() + " permissions");

        boolean anyFailed = false;
        StringBuilder errorSummary = new StringBuilder();
        int syncCount = 0;

        // Sync to platform IDP if enabled
        if (platformIdpProperties.enabled()) {
            syncCount++;
            try {
                syncToPlatformIdp(roles, permissions);
            } catch (Exception e) {
                anyFailed = true;
                String error = "Platform IDP: " + e.getMessage();
                errorSummary.append(error).append("; ");
                Log.error("Failed to sync to platform IDP: " + e.getMessage(), e);
            }
        }

        // Sync to anchor tenant IDP if enabled
        if (anchorIdpProperties.enabled()) {
            syncCount++;
            try {
                syncToAnchorIdp(roles, permissions);
            } catch (Exception e) {
                anyFailed = true;
                String error = "Anchor IDP: " + e.getMessage();
                errorSummary.append(error).append("; ");
                Log.error("Failed to sync to anchor IDP: " + e.getMessage(), e);
            }
        }

        if (syncCount == 0) {
            Log.warn("No IDPs configured for sync. Enable platform or anchor IDP in application.properties");
            return;
        }

        if (anyFailed) {
            throw new IdpSyncException("Some IDP syncs failed: " + errorSummary.toString());
        }

        Log.info("IDP role sync completed successfully for all " + syncCount + " target(s)");
    }

    /**
     * Sync to platform IDP.
     */
    private void syncToPlatformIdp(Set<RoleDefinition> roles, Set<PermissionDefinition> permissions)
            throws IdpSyncException {
        String name = platformIdpProperties.name().orElse("Platform IDP");
        Log.info("Syncing to Platform IDP: " + name + " (type: " + platformIdpProperties.type() + ")");

        IdpSyncAdapter adapter = createPlatformAdapter();
        adapter.syncRolesToIdp(roles);
        adapter.syncPermissionsToIdp(permissions);

        Log.info("Successfully synced to Platform IDP");
    }

    /**
     * Sync to anchor tenant IDP.
     */
    private void syncToAnchorIdp(Set<RoleDefinition> roles, Set<PermissionDefinition> permissions)
            throws IdpSyncException {
        String name = anchorIdpProperties.name().orElse("Anchor Tenant IDP");
        Log.info("Syncing to Anchor Tenant IDP: " + name + " (type: " + anchorIdpProperties.type() + ")");

        IdpSyncAdapter adapter = createAnchorAdapter();
        adapter.syncRolesToIdp(roles);
        adapter.syncPermissionsToIdp(permissions);

        Log.info("Successfully synced to Anchor Tenant IDP");
    }

    /**
     * Create platform IDP adapter (public for CLI usage).
     */
    public IdpSyncAdapter createPlatformAdapter() throws IdpSyncException {
        String type = platformIdpProperties.type();

        if ("KEYCLOAK".equals(type)) {
            PlatformIdpProperties.KeycloakConfig cfg = platformIdpProperties.keycloak()
                .orElseThrow(() -> new IdpSyncException("Platform IDP Keycloak config is missing"));
            return new KeycloakSyncAdapter(cfg.adminUrl(), cfg.realm(), cfg.clientId(), cfg.clientSecret());
        } else if ("ENTRA".equals(type)) {
            PlatformIdpProperties.EntraConfig cfg = platformIdpProperties.entra()
                .orElseThrow(() -> new IdpSyncException("Platform IDP Entra config is missing"));
            return new EntraSyncAdapter(cfg.tenantId(), cfg.clientId(), cfg.clientSecret(), cfg.applicationObjectId());
        } else {
            throw new IdpSyncException("Unknown platform IDP type: " + type);
        }
    }

    /**
     * Create anchor IDP adapter (public for CLI usage).
     */
    public IdpSyncAdapter createAnchorAdapter() throws IdpSyncException {
        String type = anchorIdpProperties.type();

        if ("KEYCLOAK".equals(type)) {
            AnchorIdpProperties.KeycloakConfig cfg = anchorIdpProperties.keycloak()
                .orElseThrow(() -> new IdpSyncException("Anchor IDP Keycloak config is missing"));
            return new KeycloakSyncAdapter(cfg.adminUrl(), cfg.realm(), cfg.clientId(), cfg.clientSecret());
        } else if ("ENTRA".equals(type)) {
            AnchorIdpProperties.EntraConfig cfg = anchorIdpProperties.entra()
                .orElseThrow(() -> new IdpSyncException("Anchor IDP Entra config is missing"));
            return new EntraSyncAdapter(cfg.tenantId(), cfg.clientId(), cfg.clientSecret(), cfg.applicationObjectId());
        } else {
            throw new IdpSyncException("Unknown anchor IDP type: " + type);
        }
    }

    /**
     * Check if platform IDP is enabled.
     */
    public boolean isPlatformIdpEnabled() {
        return platformIdpProperties.enabled();
    }

    /**
     * Check if anchor IDP is enabled.
     */
    public boolean isAnchorIdpEnabled() {
        return anchorIdpProperties.enabled();
    }
}
