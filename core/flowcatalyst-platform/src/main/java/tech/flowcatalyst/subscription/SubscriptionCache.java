package tech.flowcatalyst.subscription;

import com.github.benmanes.caffeine.cache.Cache;
import com.github.benmanes.caffeine.cache.Caffeine;
import jakarta.annotation.PostConstruct;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import org.jboss.logging.Logger;
import tech.flowcatalyst.dispatch.DispatchMode;

import java.time.Duration;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

/**
 * In-memory cache for active subscriptions, optimized for event-to-dispatch matching.
 *
 * <p>Uses Caffeine for high-performance caching with configurable TTL.</p>
 *
 * <p>Cache key format: "{eventTypeCode}:{clientId|anchor}"</p>
 *
 * <p>Invalidation is triggered when subscriptions are created, updated, or deleted
 * via the {@link SubscriptionOperations} service.</p>
 */
@ApplicationScoped
public class SubscriptionCache {

    private static final Logger LOG = Logger.getLogger(SubscriptionCache.class);

    @Inject
    SubscriptionRepository subscriptionRepository;

    @ConfigProperty(name = "flowcatalyst.subscription-cache.ttl-minutes", defaultValue = "5")
    int ttlMinutes;

    @ConfigProperty(name = "flowcatalyst.subscription-cache.max-size", defaultValue = "10000")
    int maxSize;

    private Cache<String, List<CachedSubscription>> cache;

    /**
     * Track which event type codes have cache entries for efficient invalidation.
     * Maps eventTypeCode -> set of cache keys that contain subscriptions for this event type.
     */
    private final Map<String, java.util.Set<String>> eventTypeCodeToKeys = new ConcurrentHashMap<>();

    @PostConstruct
    void init() {
        cache = Caffeine.newBuilder()
            .expireAfterWrite(Duration.ofMinutes(ttlMinutes))
            .maximumSize(maxSize)
            .removalListener((key, value, cause) ->
                LOG.debugf("Cache entry removed: key=%s, cause=%s", key, cause))
            .build();
        LOG.infof("SubscriptionCache initialized: TTL=%d minutes, maxSize=%d", ttlMinutes, maxSize);
    }

    /**
     * Get active subscriptions for an event type code and optional client.
     *
     * <p>On cache miss, loads from database.</p>
     *
     * @param eventTypeCode The event type code to match (e.g., "operant:execution:trip:started")
     * @param clientId The client ID (null for anchor-level events)
     * @return List of matching cached subscriptions (never null)
     */
    public List<CachedSubscription> getByEventTypeCode(String eventTypeCode, String clientId) {
        String cacheKey = buildCacheKey(eventTypeCode, clientId);
        return cache.get(cacheKey, k -> loadFromDatabase(eventTypeCode, clientId, cacheKey));
    }

    /**
     * Invalidate cache entries for a specific event type.
     *
     * <p>Called when subscriptions binding to this event type are created, updated, or deleted.</p>
     *
     * @param eventTypeCode The event type code to invalidate
     */
    public void invalidateByEventTypeCode(String eventTypeCode) {
        var keys = eventTypeCodeToKeys.remove(eventTypeCode);
        if (keys != null && !keys.isEmpty()) {
            cache.invalidateAll(keys);
            LOG.debugf("Invalidated %d cache entries for eventTypeCode=%s", keys.size(), eventTypeCode);
        }
    }

    /**
     * Invalidate a specific cache key.
     *
     * @param eventTypeCode The event type code
     * @param clientId The client ID (null for anchor-level)
     */
    public void invalidate(String eventTypeCode, String clientId) {
        String cacheKey = buildCacheKey(eventTypeCode, clientId);
        cache.invalidate(cacheKey);
        LOG.debugf("Invalidated cache for key=%s", cacheKey);
    }

    /**
     * Invalidate all cached entries.
     */
    public void invalidateAll() {
        cache.invalidateAll();
        eventTypeCodeToKeys.clear();
        LOG.info("All subscription cache entries invalidated");
    }

    /**
     * Get cache statistics for monitoring.
     *
     * @return Cache statistics
     */
    public CacheStats getStats() {
        return new CacheStats(
            cache.estimatedSize(),
            eventTypeCodeToKeys.size()
        );
    }

    private String buildCacheKey(String eventTypeCode, String clientId) {
        return eventTypeCode + ":" + (clientId != null ? clientId : "anchor");
    }

    private List<CachedSubscription> loadFromDatabase(String eventTypeCode, String clientId, String cacheKey) {
        LOG.debugf("Cache miss, loading from database: eventTypeCode=%s, clientId=%s", eventTypeCode, clientId);

        // Query active subscriptions matching this event type code and client
        List<Subscription> subscriptions = subscriptionRepository
            .findActiveByEventTypeCodeAndClient(eventTypeCode, clientId);

        // Convert to lightweight cached records
        List<CachedSubscription> cached = subscriptions.stream()
            .map(this::toCachedSubscription)
            .toList();

        // Track for invalidation
        eventTypeCodeToKeys.computeIfAbsent(eventTypeCode, k -> ConcurrentHashMap.newKeySet())
            .add(cacheKey);

        LOG.debugf("Loaded %d subscriptions for eventTypeCode=%s, clientId=%s",
            cached.size(), eventTypeCode, clientId);

        return cached;
    }

    private CachedSubscription toCachedSubscription(Subscription sub) {
        return new CachedSubscription(
            sub.id(),
            sub.code(),
            sub.clientId(),
            sub.target(),
            sub.queue(),
            sub.dispatchPoolId(),
            sub.dispatchPoolCode(),
            sub.serviceAccountId(),
            sub.mode(),
            sub.sequence(),
            sub.delaySeconds(),
            sub.timeoutSeconds(),
            sub.maxRetries(),
            sub.maxAgeSeconds(),
            sub.dataOnly(),
            sub.customConfig()
        );
    }

    /**
     * Lightweight cached subscription data containing only fields needed for dispatch job creation.
     */
    public record CachedSubscription(
        String id,
        String code,
        String clientId,
        String target,
        String queue,
        String dispatchPoolId,
        String dispatchPoolCode,
        String serviceAccountId,
        DispatchMode mode,
        int sequence,
        int delaySeconds,
        int timeoutSeconds,
        int maxRetries,
        int maxAgeSeconds,
        boolean dataOnly,
        List<ConfigEntry> customConfig
    ) {}

    /**
     * Cache statistics for monitoring.
     */
    public record CacheStats(
        long estimatedSize,
        int trackedEventTypes
    ) {}
}
