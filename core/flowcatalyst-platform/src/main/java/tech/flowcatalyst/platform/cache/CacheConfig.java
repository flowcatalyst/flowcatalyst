package tech.flowcatalyst.platform.cache;

import io.smallrye.config.ConfigMapping;
import io.smallrye.config.WithDefault;

import java.time.Duration;

/**
 * Configuration for the cache system.
 */
@ConfigMapping(prefix = "flowcatalyst.cache")
public interface CacheConfig {

    /**
     * Cache backend type: MEMORY, DATABASE, or REDIS.
     */
    @WithDefault("MEMORY")
    CacheStore.CacheType type();

    /**
     * Default time-to-live for cache entries.
     */
    @WithDefault("5m")
    Duration ttl();

    /**
     * Maximum number of entries per cache (for in-memory cache).
     */
    @WithDefault("10000")
    long maxSize();

    /**
     * Redis configuration (only used when type=REDIS).
     */
    Redis redis();

    interface Redis {
        /**
         * Redis key prefix for cache entries.
         */
        @WithDefault("fc:cache:")
        String keyPrefix();
    }
}
