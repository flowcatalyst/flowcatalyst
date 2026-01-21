package tech.flowcatalyst.platform.cache;

import jakarta.enterprise.inject.Typed;
import jakarta.inject.Inject;
import jakarta.inject.Singleton;
import jakarta.persistence.EntityManager;
import jakarta.transaction.Transactional;

import java.time.Duration;
import java.time.Instant;
import java.util.Optional;

/**
 * Database-backed cache implementation using PostgreSQL and native queries.
 *
 * <p>Simple distributed caching without requiring Redis.
 * Good for multi-instance deployments where Redis isn't available.
 * Automatic cleanup of expired entries on read.
 *
 * <p>Note: @Typed excludes CacheStore from bean types so only the
 * CacheStoreProducer can provide the CacheStore interface.
 */
@Singleton
@Typed(DatabaseCacheStore.class)
@Transactional
public class DatabaseCacheStore implements CacheStore {

    @Inject
    EntityManager em;

    @Inject
    CacheConfig config;

    @Override
    public Optional<String> get(String cacheName, String key) {
        Instant now = Instant.now();
        @SuppressWarnings("unchecked")
        var results = em.createNativeQuery(
                "SELECT cache_value FROM cache_entries " +
                "WHERE cache_name = :cacheName AND cache_key = :key AND expires_at > :now")
            .setParameter("cacheName", cacheName)
            .setParameter("key", key)
            .setParameter("now", now)
            .getResultList();

        return results.isEmpty() ? Optional.empty() : Optional.of((String) results.get(0));
    }

    @Override
    public void put(String cacheName, String key, String value) {
        put(cacheName, key, value, config.ttl());
    }

    @Override
    public void put(String cacheName, String key, String value, Duration ttl) {
        Instant now = Instant.now();
        Instant expiresAt = now.plus(ttl);

        em.createNativeQuery(
                "INSERT INTO cache_entries (cache_name, cache_key, cache_value, expires_at, created_at) " +
                "VALUES (:cacheName, :key, :value, :expiresAt, :now) " +
                "ON CONFLICT (cache_name, cache_key) DO UPDATE SET " +
                "cache_value = :value, expires_at = :expiresAt")
            .setParameter("cacheName", cacheName)
            .setParameter("key", key)
            .setParameter("value", value)
            .setParameter("expiresAt", expiresAt)
            .setParameter("now", now)
            .executeUpdate();
    }

    @Override
    public void invalidate(String cacheName, String key) {
        em.createNativeQuery(
                "DELETE FROM cache_entries WHERE cache_name = :cacheName AND cache_key = :key")
            .setParameter("cacheName", cacheName)
            .setParameter("key", key)
            .executeUpdate();
    }

    @Override
    public void invalidateAll(String cacheName) {
        em.createNativeQuery(
                "DELETE FROM cache_entries WHERE cache_name = :cacheName")
            .setParameter("cacheName", cacheName)
            .executeUpdate();
    }

    /**
     * Cleanup expired entries. Can be called periodically via scheduler.
     *
     * @return Number of entries removed
     */
    public int cleanupExpired() {
        Instant now = Instant.now();
        return em.createNativeQuery(
                "DELETE FROM cache_entries WHERE expires_at < :now")
            .setParameter("now", now)
            .executeUpdate();
    }
}
