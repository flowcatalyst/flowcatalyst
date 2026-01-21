package tech.flowcatalyst.platform.cors;

import io.quarkus.runtime.Startup;
import io.quarkus.scheduler.Scheduled;
import jakarta.annotation.PostConstruct;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import tech.flowcatalyst.platform.cors.CorsAllowedOriginRepository;

import java.time.Instant;
import java.util.Collections;
import java.util.Set;
import java.util.concurrent.atomic.AtomicReference;
import java.util.stream.Collectors;

/**
 * Service for CORS origin lookups with non-blocking in-memory caching.
 *
 * Origins are cached and refreshed every 4 minutes via scheduled task.
 * This ensures the cache is always warm and no blocking DB calls happen
 * on Vert.x event loop threads.
 */
@ApplicationScoped
@Startup // Ensure this bean is initialized at startup
public class CorsService {

    private static final Logger LOG = Logger.getLogger(CorsService.class);

    @Inject
    CorsAllowedOriginRepository repository;

    // Simple non-blocking cache using AtomicReference
    private final AtomicReference<Set<String>> cache = new AtomicReference<>(Collections.emptySet());

    /**
     * Pre-warm the cache at startup.
     */
    @PostConstruct
    void init() {
        refreshCacheFromDatabase();
        LOG.info("CORS origins cache pre-warmed with " + cache.get().size() + " origins");
    }

    /**
     * Refresh cache every 4 minutes (before any theoretical TTL would expire).
     * This runs on a worker thread, not the event loop.
     */
    @Scheduled(every = "4m")
    void scheduledRefresh() {
        refreshCacheFromDatabase();
        LOG.debug("CORS origins cache refreshed via scheduled task");
    }

    /**
     * Check if an origin is allowed.
     * Uses cached origins - never blocks.
     */
    public boolean isOriginAllowed(String origin) {
        if (origin == null || origin.isBlank()) {
            return false;
        }
        return cache.get().contains(origin);
    }

    /**
     * Get all allowed origins.
     * Returns cached set - never blocks.
     */
    public Set<String> getAllowedOrigins() {
        return cache.get();
    }

    /**
     * Invalidate and refresh the cache.
     * Called after add/delete operations.
     * This should be called from a worker thread (e.g., from a @Transactional method).
     */
    public void invalidateCache() {
        refreshCacheFromDatabase();
        LOG.debug("CORS origins cache invalidated and refreshed");
    }

    /**
     * Refresh cache from database.
     * Must be called from a worker thread, not the event loop.
     */
    private void refreshCacheFromDatabase() {
        try {
            Set<String> origins = repository.listAll().stream()
                .map(o -> o.origin)
                .collect(Collectors.toSet());
            cache.set(Set.copyOf(origins)); // Immutable copy
        } catch (Exception e) {
            LOG.warn("Failed to refresh CORS cache: " + e.getMessage());
            // Keep existing cache on failure
        }
    }
}
