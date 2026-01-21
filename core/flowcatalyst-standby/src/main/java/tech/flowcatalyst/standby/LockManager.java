package tech.flowcatalyst.standby;

import io.quarkus.redis.datasource.RedisDataSource;
import io.quarkus.redis.datasource.keys.KeyCommands;
import io.quarkus.redis.datasource.value.ValueCommands;
import io.vertx.mutiny.redis.client.Response;
import jakarta.annotation.PostConstruct;
import jakarta.annotation.PreDestroy;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.inject.Instance;
import jakarta.inject.Inject;

import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.ScheduledFuture;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.logging.Level;
import java.util.logging.Logger;

/**
 * Manages Redis-based distributed lock for primary/standby election using Quarkus Redis client.
 *
 * <h2>Features</h2>
 * <ul>
 *   <li>Uses SET NX EX pattern for atomic lock acquisition (same as Go implementation)</li>
 *   <li>Lua scripts for atomic refresh and release operations</li>
 *   <li>Automatic lock renewal (watchdog) while lock is held</li>
 *   <li>Native image compatible (unlike Redisson)</li>
 * </ul>
 *
 * <h2>Lock Renewal (Watchdog)</h2>
 * When a lock is acquired, an automatic renewal task is started that refreshes the lock
 * TTL at regular intervals (default: every 10 seconds, 1/3 of TTL). This prevents the lock
 * from expiring while the primary is still healthy, matching Redisson's watchdog behavior.
 *
 * <p>Only active when standby mode is enabled.</p>
 */
@ApplicationScoped
public class LockManager {

    private static final Logger LOG = Logger.getLogger(LockManager.class.getName());

    /**
     * Lua script: Atomically refresh lock TTL only if we own it.
     * Returns 1 if refreshed, 0 if we don't own the lock.
     *
     * KEYS[1] = lock key
     * ARGV[1] = our instance ID
     * ARGV[2] = TTL in seconds
     */
    private static final String REFRESH_SCRIPT = """
        if redis.call("get", KEYS[1]) == ARGV[1] then
            return redis.call("expire", KEYS[1], ARGV[2])
        else
            return 0
        end
        """;

    /**
     * Lua script: Atomically release lock only if we own it.
     * Returns 1 if deleted, 0 if we don't own the lock.
     *
     * KEYS[1] = lock key
     * ARGV[1] = our instance ID
     */
    private static final String RELEASE_SCRIPT = """
        if redis.call("get", KEYS[1]) == ARGV[1] then
            return redis.call("del", KEYS[1])
        else
            return 0
        end
        """;

    /** Refresh interval as fraction of TTL (1/3 = refresh 3x before expiry) */
    private static final int REFRESH_DIVISOR = 3;

    @Inject
    StandbyConfig standbyConfig;

    @Inject
    Instance<RedisDataSource> redisDataSourceInstance;

    private ValueCommands<String, String> valueCommands;
    private KeyCommands<String> keyCommands;
    private RedisDataSource redis;

    /** Tracks if THIS instance currently holds the lock */
    private final AtomicBoolean lockHeldByThisInstance = new AtomicBoolean(false);

    /** Scheduler for automatic lock renewal (watchdog) */
    private ScheduledExecutorService watchdogExecutor;

    /** Current watchdog task (null if not holding lock) */
    private volatile ScheduledFuture<?> watchdogTask;

    /** Flag to track if we've been initialized */
    private volatile boolean initialized = false;

    @PostConstruct
    void init() {
        if (standbyConfig.enabled() && redisDataSourceInstance.isResolvable()) {
            this.redis = redisDataSourceInstance.get();
            this.valueCommands = redis.value(String.class, String.class);
            this.keyCommands = redis.key(String.class);

            // Create watchdog executor with virtual threads for efficient scheduling
            this.watchdogExecutor = Executors.newSingleThreadScheduledExecutor(r -> {
                Thread t = Thread.ofVirtual().name("lock-watchdog").unstarted(r);
                return t;
            });

            this.initialized = true;
            LOG.info("LockManager initialized with Quarkus Redis client");
        } else if (standbyConfig.enabled()) {
            LOG.warning("Standby mode enabled but Redis client not available");
        }
    }

    @PreDestroy
    void shutdown() {
        stopWatchdog();
        if (watchdogExecutor != null) {
            watchdogExecutor.shutdown();
            try {
                if (!watchdogExecutor.awaitTermination(5, TimeUnit.SECONDS)) {
                    watchdogExecutor.shutdownNow();
                }
            } catch (InterruptedException e) {
                watchdogExecutor.shutdownNow();
                Thread.currentThread().interrupt();
            }
        }
    }

    /**
     * Attempt to acquire the distributed lock using SET NX EX pattern.
     * If acquired, starts the automatic renewal watchdog.
     *
     * @return true if lock was acquired, false if another instance holds it
     * @throws LockException if Redis is unavailable or connection fails
     */
    public boolean acquireLock() throws LockException {
        // If standby disabled, always return true (single instance mode)
        if (!standbyConfig.enabled() || !initialized) {
            return true;
        }

        try {
            String lockKey = standbyConfig.lockKey();
            String instanceId = standbyConfig.instanceId();
            int ttlSeconds = standbyConfig.lockTtlSeconds();

            // Use SETNX first to check if we can acquire, then set TTL
            // This is atomic enough for our use case - TTL is set immediately after
            boolean acquired = valueCommands.setnx(lockKey, instanceId);

            if (acquired) {
                // Lock acquired! Set the TTL
                keyCommands.expire(lockKey, ttlSeconds);
                lockHeldByThisInstance.set(true);
                startWatchdog();
                LOG.info("Lock acquired by instance: " + instanceId);
                return true;
            }

            // Lock exists - check if we already own it (e.g., after restart with same instanceId)
            String currentOwner = valueCommands.get(lockKey);
            if (instanceId.equals(currentOwner)) {
                // We already own it - refresh the TTL and start watchdog
                keyCommands.expire(lockKey, ttlSeconds);
                lockHeldByThisInstance.set(true);
                startWatchdog();
                LOG.info("Lock already owned by this instance, refreshed TTL");
                return true;
            }

            LOG.fine("Lock held by another instance: " + currentOwner);
            return false;

        } catch (Exception e) {
            LOG.log(Level.SEVERE, "Failed to acquire lock: " + e.getMessage(), e);
            throw new LockException("Redis connection failed - unable to acquire lock", e);
        }
    }

    /**
     * Check if we still hold the lock and refresh it if needed.
     * The watchdog handles automatic refresh, this verifies ownership.
     *
     * @return true if we still hold the lock, false if we lost it
     * @throws LockException if Redis is unavailable or connection fails
     */
    public boolean refreshLock() throws LockException {
        // If standby disabled, always return true (single instance mode)
        if (!standbyConfig.enabled() || !initialized) {
            return true;
        }

        if (!lockHeldByThisInstance.get()) {
            return false;
        }

        try {
            // Use Lua script for atomic check-and-refresh
            boolean refreshed = executeRefreshScript();

            if (!refreshed) {
                LOG.warning("Lost lock - refresh failed (lock expired or stolen)");
                lockHeldByThisInstance.set(false);
                stopWatchdog();
                return false;
            }

            LOG.fine("Lock refreshed for instance: " + standbyConfig.instanceId());
            return true;

        } catch (Exception e) {
            LOG.log(Level.SEVERE, "Failed to refresh lock: " + e.getMessage(), e);
            throw new LockException("Redis connection failed - unable to refresh lock", e);
        }
    }

    /**
     * Release the distributed lock immediately (for graceful shutdown).
     * Uses atomic Lua script to only release if we own it.
     *
     * @return true if lock was released, false if we don't hold it
     */
    public boolean releaseLock() {
        // If standby disabled, nothing to release
        if (!standbyConfig.enabled() || !initialized) {
            return false;
        }

        // Stop watchdog first
        stopWatchdog();

        if (!lockHeldByThisInstance.get()) {
            LOG.fine("Lock not held by this instance, nothing to release");
            return false;
        }

        try {
            String lockKey = standbyConfig.lockKey();
            String instanceId = standbyConfig.instanceId();

            // Use Lua script for atomic check-and-delete
            Response result = redis.execute(
                "EVAL",
                RELEASE_SCRIPT,
                "1",  // number of keys
                lockKey,
                instanceId
            );

            lockHeldByThisInstance.set(false);

            int released = result != null ? result.toInteger() : 0;
            if (released == 1) {
                LOG.info("Lock released by instance: " + instanceId);
                return true;
            } else {
                LOG.warning("Lock was not held by this instance (already expired?)");
                return false;
            }

        } catch (Exception e) {
            LOG.warning("Error releasing lock (will expire automatically): " + e.getMessage());
            lockHeldByThisInstance.set(false);
            return false;
        }
    }

    /**
     * Check if we currently hold the lock.
     *
     * @return true if we hold the lock, false otherwise
     * @throws LockException if Redis is unavailable
     */
    public boolean holdingLock() throws LockException {
        // If standby disabled, always return true (single instance mode)
        if (!standbyConfig.enabled() || !initialized) {
            return true;
        }

        return lockHeldByThisInstance.get();
    }

    /**
     * Get current lock holder info (for monitoring/debugging).
     *
     * @return the instance ID holding the lock, "unlocked" if available, null on error
     */
    public String getCurrentLockHolder() {
        // If standby disabled, return single instance indicator
        if (!standbyConfig.enabled() || !initialized) {
            return "single-instance";
        }

        try {
            String owner = valueCommands.get(standbyConfig.lockKey());
            return owner != null ? owner : "unlocked";
        } catch (Exception e) {
            LOG.warning("Failed to get lock status: " + e.getMessage());
            return null;
        }
    }

    /**
     * Start the watchdog task that automatically refreshes the lock TTL.
     * Runs at TTL/3 interval to ensure lock is refreshed well before expiry.
     */
    private void startWatchdog() {
        if (watchdogTask != null) {
            return; // Already running
        }

        int ttlSeconds = standbyConfig.lockTtlSeconds();
        int refreshIntervalSeconds = Math.max(1, ttlSeconds / REFRESH_DIVISOR);

        watchdogTask = watchdogExecutor.scheduleAtFixedRate(
            this::watchdogRefresh,
            refreshIntervalSeconds,
            refreshIntervalSeconds,
            TimeUnit.SECONDS
        );

        LOG.info("Lock watchdog started (refresh every " + refreshIntervalSeconds + "s, TTL=" + ttlSeconds + "s)");
    }

    /**
     * Stop the watchdog task.
     */
    private void stopWatchdog() {
        ScheduledFuture<?> task = watchdogTask;
        if (task != null) {
            task.cancel(false);
            watchdogTask = null;
            LOG.fine("Lock watchdog stopped");
        }
    }

    /**
     * Watchdog refresh task - called periodically to refresh lock TTL.
     */
    private void watchdogRefresh() {
        if (!lockHeldByThisInstance.get()) {
            stopWatchdog();
            return;
        }

        try {
            boolean refreshed = executeRefreshScript();

            if (refreshed) {
                LOG.fine("Watchdog: Lock TTL refreshed");
            } else {
                LOG.severe("Watchdog: Lost lock - TTL refresh failed!");
                lockHeldByThisInstance.set(false);
                stopWatchdog();
            }
        } catch (Exception e) {
            LOG.severe("Watchdog: Error refreshing lock - " + e.getMessage());
            // Don't set lockHeldByThisInstance to false here - might be transient network issue
            // The main StandbyService refresh loop will detect the failure
        }
    }

    /**
     * Execute the refresh Lua script atomically.
     *
     * @return true if refreshed, false if we don't own the lock
     */
    private boolean executeRefreshScript() {
        String lockKey = standbyConfig.lockKey();
        String instanceId = standbyConfig.instanceId();
        int ttlSeconds = standbyConfig.lockTtlSeconds();

        Response result = redis.execute(
            "EVAL",
            REFRESH_SCRIPT,
            "1",  // number of keys
            lockKey,
            instanceId,
            String.valueOf(ttlSeconds)
        );

        return result != null && result.toInteger() == 1;
    }

    /**
     * Exception thrown when Redis operations fail.
     */
    public static class LockException extends Exception {
        public LockException(String message, Throwable cause) {
            super(message, cause);
        }
    }
}
