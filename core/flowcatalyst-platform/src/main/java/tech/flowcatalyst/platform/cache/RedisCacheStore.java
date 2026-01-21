package tech.flowcatalyst.platform.cache;

import io.quarkus.arc.lookup.LookupIfProperty;
import jakarta.enterprise.inject.Typed;
import jakarta.inject.Inject;
import jakarta.inject.Singleton;
import org.jboss.logging.Logger;

import java.time.Duration;
import java.util.Optional;

/**
 * Redis-backed cache implementation.
 *
 * <p>Distributed caching for multi-instance deployments.
 * Requires Redis server and quarkus-redis extension.
 *
 * <p>To enable Redis caching:
 * <ol>
 *   <li>Add dependency: implementation("io.quarkus:quarkus-redis")</li>
 *   <li>Configure: quarkus.redis.hosts=redis://localhost:6379</li>
 *   <li>Set: flowcatalyst.cache.type=REDIS</li>
 * </ol>
 *
 * <p>This implementation is a placeholder. When Redis extension is added,
 * uncomment the Redis-specific code.
 *
 * <p>Note: @Typed excludes CacheStore from bean types so only the
 * CacheStoreProducer can provide the CacheStore interface.
 */
@Singleton
@Typed(RedisCacheStore.class)
@LookupIfProperty(name = "flowcatalyst.cache.type", stringValue = "REDIS")
public class RedisCacheStore implements CacheStore {

    private static final Logger LOG = Logger.getLogger(RedisCacheStore.class);

    // Uncomment when quarkus-redis is added:
    // @Inject
    // RedisDataSource redis;

    @Inject
    CacheConfig config;

    public RedisCacheStore() {
        LOG.warn("Redis cache store selected but quarkus-redis extension not installed. " +
                 "Add implementation(\"io.quarkus:quarkus-redis\") to build.gradle.kts");
    }

    private String buildKey(String cacheName, String key) {
        return config.redis().keyPrefix() + cacheName + ":" + key;
    }

    @Override
    public Optional<String> get(String cacheName, String key) {
        // TODO: Implement with Redis when extension is added
        throw new UnsupportedOperationException(
            "Redis cache not available. Add quarkus-redis extension or use MEMORY/DATABASE cache type.");
    }

    @Override
    public void put(String cacheName, String key, String value) {
        throw new UnsupportedOperationException(
            "Redis cache not available. Add quarkus-redis extension or use MEMORY/DATABASE cache type.");
    }

    @Override
    public void put(String cacheName, String key, String value, Duration ttl) {
        throw new UnsupportedOperationException(
            "Redis cache not available. Add quarkus-redis extension or use MEMORY/DATABASE cache type.");
    }

    @Override
    public void invalidate(String cacheName, String key) {
        throw new UnsupportedOperationException(
            "Redis cache not available. Add quarkus-redis extension or use MEMORY/DATABASE cache type.");
    }

    @Override
    public void invalidateAll(String cacheName) {
        throw new UnsupportedOperationException(
            "Redis cache not available. Add quarkus-redis extension or use MEMORY/DATABASE cache type.");
    }
}
