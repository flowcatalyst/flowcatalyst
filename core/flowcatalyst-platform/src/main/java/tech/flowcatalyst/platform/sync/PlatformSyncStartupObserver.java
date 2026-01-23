package tech.flowcatalyst.platform.sync;

import io.quarkus.arc.profile.IfBuildProfile;
import io.quarkus.runtime.StartupEvent;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.event.Observes;
import jakarta.inject.Inject;
import jakarta.transaction.Transactional;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import org.jboss.logging.Logger;

/**
 * Startup observer that triggers platform sync on application start.
 *
 * <p>Runs after Flyway migrations have completed. Syncs platform event types
 * from code to database if the content hash has changed.
 *
 * <p>Can be disabled via configuration for testing or special deployment scenarios.
 */
@ApplicationScoped
public class PlatformSyncStartupObserver {

    private static final Logger LOG = Logger.getLogger(PlatformSyncStartupObserver.class);

    @Inject
    PlatformEventTypeSyncService eventTypeSyncService;

    @ConfigProperty(name = "flowcatalyst.platform.sync.enabled", defaultValue = "true")
    boolean syncEnabled;

    @ConfigProperty(name = "flowcatalyst.platform.sync.event-types.enabled", defaultValue = "true")
    boolean eventTypesSyncEnabled;

    /**
     * Triggered on application startup.
     * Syncs platform registries if enabled and needed.
     */
    @Transactional
    void onStart(@Observes StartupEvent event) {
        if (!syncEnabled) {
            LOG.info("Platform sync disabled via configuration");
            return;
        }

        LOG.info("=== PLATFORM SYNC ===");

        // Sync platform event types
        if (eventTypesSyncEnabled) {
            try {
                PlatformEventTypeSyncService.SyncResult result = eventTypeSyncService.syncIfNeeded();
                if (result.performed()) {
                    LOG.infof("Platform event types: %s (created=%d, updated=%d, total=%d)",
                        result.message(), result.created(), result.updated(), result.total());
                } else {
                    LOG.debugf("Platform event types: %s", result.message());
                }
            } catch (Exception e) {
                LOG.error("Failed to sync platform event types", e);
            }
        }

        LOG.info("=====================");
    }
}
