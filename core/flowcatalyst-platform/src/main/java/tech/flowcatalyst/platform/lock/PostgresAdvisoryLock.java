package tech.flowcatalyst.platform.lock;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import org.jboss.logging.Logger;

import java.time.Duration;
import java.util.Optional;
import java.util.concurrent.atomic.AtomicBoolean;

/**
 * Distributed lock implementation using PostgreSQL advisory locks.
 *
 * <p>Advisory locks are application-level locks that don't affect row-level
 * locking. They're perfect for coordinating work across multiple instances.
 *
 * <p>Lock names are hashed to int64 for PostgreSQL's advisory lock functions.
 */
@ApplicationScoped
public class PostgresAdvisoryLock implements DistributedLock {

    private static final Logger LOG = Logger.getLogger(PostgresAdvisoryLock.class);

    @Inject
    EntityManager em;

    @Override
    public Optional<LockHandle> tryAcquire(String lockName, Duration timeout) {
        long lockId = hashLockName(lockName);
        long startTime = System.currentTimeMillis();
        long timeoutMs = timeout.toMillis();

        while (System.currentTimeMillis() - startTime < timeoutMs) {
            Boolean acquired = (Boolean) em.createNativeQuery(
                    "SELECT pg_try_advisory_lock(:lockId)")
                .setParameter("lockId", lockId)
                .getSingleResult();

            if (Boolean.TRUE.equals(acquired)) {
                LOG.debugf("Acquired advisory lock: %s (id=%d)", lockName, lockId);
                return Optional.of(new AdvisoryLockHandle(lockName, lockId));
            }

            // Wait a bit before retrying
            try {
                Thread.sleep(Math.min(100, timeoutMs / 10));
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                return Optional.empty();
            }
        }

        LOG.debugf("Failed to acquire advisory lock within timeout: %s (id=%d)", lockName, lockId);
        return Optional.empty();
    }

    /**
     * Hash a lock name to a 64-bit integer for PostgreSQL advisory locks.
     * Uses a stable hash algorithm to ensure consistent lock IDs across instances.
     */
    private long hashLockName(String lockName) {
        // Use a simple but stable hash - FNV-1a 64-bit
        long hash = 0xcbf29ce484222325L;
        for (byte b : lockName.getBytes()) {
            hash ^= b;
            hash *= 0x100000001b3L;
        }
        return hash;
    }

    /**
     * Handle for a held advisory lock.
     */
    private class AdvisoryLockHandle implements LockHandle {
        private final String lockName;
        private final long lockId;
        private final AtomicBoolean released = new AtomicBoolean(false);

        AdvisoryLockHandle(String lockName, long lockId) {
            this.lockName = lockName;
            this.lockId = lockId;
        }

        @Override
        public void release() {
            if (released.compareAndSet(false, true)) {
                try {
                    em.createNativeQuery("SELECT pg_advisory_unlock(:lockId)")
                        .setParameter("lockId", lockId)
                        .getSingleResult();
                    LOG.debugf("Released advisory lock: %s (id=%d)", lockName, lockId);
                } catch (Exception e) {
                    LOG.warnf(e, "Failed to release advisory lock: %s (id=%d)", lockName, lockId);
                }
            }
        }
    }
}
