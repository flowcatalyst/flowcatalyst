package tech.flowcatalyst.platform.cache;

import com.github.benmanes.caffeine.cache.Cache;
import com.github.benmanes.caffeine.cache.Caffeine;
import jakarta.enterprise.inject.Typed;
import jakarta.inject.Inject;
import jakarta.inject.Singleton;

import java.time.Duration;
import java.util.Optional;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ConcurrentMap;

/**
 * In-memory cache implementation using Caffeine.
 *
 * <p>Fast, single-node caching. Not shared across multiple instances.
 * Good for development and single-server deployments.
 *
 * <p>Note: @Typed excludes CacheStore from bean types so only the
 * CacheStoreProducer can provide the CacheStore interface.
 */
@Singleton
@Typed(InMemoryCacheStore.class)
public class InMemoryCacheStore implements CacheStore {

    @Inject
    CacheConfig config;

    private final ConcurrentMap<String, Cache<String, String>> caches = new ConcurrentHashMap<>();

    private Cache<String, String> getCache(String cacheName) {
        return caches.computeIfAbsent(cacheName, name ->
            Caffeine.newBuilder()
                .expireAfterWrite(config.ttl())
                .maximumSize(config.maxSize())
                .build()
        );
    }

    @Override
    public Optional<String> get(String cacheName, String key) {
        return Optional.ofNullable(getCache(cacheName).getIfPresent(key));
    }

    @Override
    public void put(String cacheName, String key, String value) {
        getCache(cacheName).put(key, value);
    }

    @Override
    public void put(String cacheName, String key, String value, Duration ttl) {
        // Caffeine doesn't support per-entry TTL easily, use default cache
        // For custom TTL, create a separate cache or use Database/Redis
        getCache(cacheName).put(key, value);
    }

    @Override
    public void invalidate(String cacheName, String key) {
        getCache(cacheName).invalidate(key);
    }

    @Override
    public void invalidateAll(String cacheName) {
        Cache<String, String> cache = caches.get(cacheName);
        if (cache != null) {
            cache.invalidateAll();
        }
    }
}
