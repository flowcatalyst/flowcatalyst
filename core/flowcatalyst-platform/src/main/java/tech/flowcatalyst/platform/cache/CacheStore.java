package tech.flowcatalyst.platform.cache;

import java.time.Duration;
import java.util.Optional;

/**
 * Abstraction for caching with pluggable backends.
 *
 * <p>Supported backends:
 * <ul>
 *   <li>MEMORY - In-memory using Caffeine (default, single-node)</li>
 *   <li>DATABASE - PostgreSQL table (simple, no extra infrastructure)</li>
 *   <li>REDIS - Redis (distributed, requires Redis server)</li>
 * </ul>
 *
 * <p>Configure via:
 * <pre>
 * flowcatalyst.cache.type=MEMORY|DATABASE|REDIS
 * flowcatalyst.cache.ttl=5m
 * </pre>
 */
public interface CacheStore {

    /**
     * Get a cached value.
     *
     * @param cacheName The cache namespace (e.g., "principals")
     * @param key The cache key
     * @return The cached value, or empty if not found or expired
     */
    Optional<String> get(String cacheName, String key);

    /**
     * Put a value in the cache with default TTL.
     *
     * @param cacheName The cache namespace
     * @param key The cache key
     * @param value The value to cache (JSON string)
     */
    void put(String cacheName, String key, String value);

    /**
     * Put a value in the cache with custom TTL.
     *
     * @param cacheName The cache namespace
     * @param key The cache key
     * @param value The value to cache (JSON string)
     * @param ttl Time-to-live for this entry
     */
    void put(String cacheName, String key, String value, Duration ttl);

    /**
     * Invalidate a specific cache entry.
     *
     * @param cacheName The cache namespace
     * @param key The cache key to invalidate
     */
    void invalidate(String cacheName, String key);

    /**
     * Invalidate all entries in a cache namespace.
     *
     * @param cacheName The cache namespace to clear
     */
    void invalidateAll(String cacheName);

    /**
     * Cache backend type.
     */
    enum CacheType {
        MEMORY,
        DATABASE,
        REDIS
    }
}
