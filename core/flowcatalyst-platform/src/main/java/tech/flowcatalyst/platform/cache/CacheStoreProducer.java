package tech.flowcatalyst.platform.cache;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.inject.Instance;
import jakarta.enterprise.inject.Produces;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;

/**
 * CDI producer that selects the appropriate CacheStore implementation
 * based on configuration.
 */
@ApplicationScoped
public class CacheStoreProducer {

    private static final Logger LOG = Logger.getLogger(CacheStoreProducer.class);

    @Inject
    CacheConfig config;

    @Inject
    Instance<InMemoryCacheStore> inMemoryCacheStore;

    @Inject
    Instance<DatabaseCacheStore> databaseCacheStore;

    @Inject
    Instance<RedisCacheStore> redisCacheStore;

    @Produces
    @ApplicationScoped
    public CacheStore cacheStore() {
        CacheStore.CacheType type = config.type();
        LOG.infof("Initializing cache store: type=%s, ttl=%s", type, config.ttl());

        return switch (type) {
            case MEMORY -> {
                LOG.info("Using in-memory cache (Caffeine)");
                yield inMemoryCacheStore.get();
            }
            case DATABASE -> {
                LOG.info("Using database cache (PostgreSQL)");
                yield databaseCacheStore.get();
            }
            case REDIS -> {
                LOG.info("Using Redis cache");
                yield redisCacheStore.get();
            }
        };
    }
}
