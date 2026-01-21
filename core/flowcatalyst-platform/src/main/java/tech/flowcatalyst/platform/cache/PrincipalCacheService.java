package tech.flowcatalyst.platform.cache;

import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;

import java.util.Optional;

/**
 * Caching layer for Principal data.
 *
 * <p>Caches principal lookups to avoid repeated database queries during request processing.
 * Cache is automatically invalidated when principals are updated.
 *
 * <p>Usage:
 * <pre>
 * Principal principal = principalCacheService.getById(principalId).orElseThrow();
 * </pre>
 */
@ApplicationScoped
public class PrincipalCacheService {

    private static final Logger LOG = Logger.getLogger(PrincipalCacheService.class);
    private static final String CACHE_NAME = "principals";

    @Inject
    CacheStore cacheStore;

    @Inject
    PrincipalRepository principalRepo;

    @Inject
    ObjectMapper objectMapper;

    /**
     * Get principal by ID, using cache.
     *
     * @param principalId The principal ID
     * @return The principal, or empty if not found
     */
    public Optional<Principal> getById(String principalId) {
        if (principalId == null) {
            return Optional.empty();
        }

        // Try cache first
        Optional<String> cached = cacheStore.get(CACHE_NAME, principalId);
        if (cached.isPresent()) {
            try {
                Principal principal = objectMapper.readValue(cached.get(), Principal.class);
                LOG.debugf("Cache hit for principal: %s", principalId);
                return Optional.of(principal);
            } catch (Exception e) {
                LOG.warnf("Failed to deserialize cached principal %s, fetching from database", principalId);
                cacheStore.invalidate(CACHE_NAME, principalId);
            }
        }

        // Fetch from database
        Optional<Principal> principal = principalRepo.findByIdOptional(principalId);
        if (principal.isPresent()) {
            try {
                String json = objectMapper.writeValueAsString(principal.get());
                cacheStore.put(CACHE_NAME, principalId, json);
                LOG.debugf("Cached principal: %s", principalId);
            } catch (Exception e) {
                LOG.warnf("Failed to cache principal %s: %s", principalId, e.getMessage());
            }
        }

        return principal;
    }

    /**
     * Invalidate cache for a specific principal.
     * Call this when a principal is updated or deleted.
     *
     * @param principalId The principal ID to invalidate
     */
    public void invalidate(String principalId) {
        if (principalId != null) {
            cacheStore.invalidate(CACHE_NAME, principalId);
            LOG.debugf("Invalidated cache for principal: %s", principalId);
        }
    }

    /**
     * Invalidate all cached principals.
     * Use sparingly, e.g., after bulk role changes.
     */
    public void invalidateAll() {
        cacheStore.invalidateAll(CACHE_NAME);
        LOG.info("Invalidated all cached principals");
    }
}
