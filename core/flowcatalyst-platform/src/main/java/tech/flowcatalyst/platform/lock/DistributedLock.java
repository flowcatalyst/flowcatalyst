package tech.flowcatalyst.platform.lock;

import java.time.Duration;
import java.util.Optional;
import java.util.function.Supplier;

/**
 * Abstraction for distributed locking across application instances.
 *
 * <p>Used to coordinate exclusive access to shared resources like
 * platform sync operations that should only run on one instance at a time.
 */
public interface DistributedLock {

    /**
     * Try to acquire a lock with the given name.
     *
     * @param lockName Unique name for the lock
     * @param timeout Maximum time to wait for the lock
     * @return A lock handle if acquired, empty if timeout or already held
     */
    Optional<LockHandle> tryAcquire(String lockName, Duration timeout);

    /**
     * Execute a task while holding a lock.
     *
     * <p>Acquires the lock, executes the task, and releases the lock.
     * If the lock cannot be acquired within the timeout, returns empty.
     *
     * @param lockName Unique name for the lock
     * @param timeout Maximum time to wait for the lock
     * @param task The task to execute while holding the lock
     * @return The task result if lock was acquired, empty otherwise
     */
    default <T> Optional<T> withLock(String lockName, Duration timeout, Supplier<T> task) {
        return tryAcquire(lockName, timeout).map(handle -> {
            try {
                return task.get();
            } finally {
                handle.release();
            }
        });
    }

    /**
     * Execute a task while holding a lock (void version).
     *
     * @param lockName Unique name for the lock
     * @param timeout Maximum time to wait for the lock
     * @param task The task to execute while holding the lock
     * @return true if lock was acquired and task executed, false otherwise
     */
    default boolean withLock(String lockName, Duration timeout, Runnable task) {
        return withLock(lockName, timeout, () -> {
            task.run();
            return true;
        }).orElse(false);
    }

    /**
     * Handle to a held lock that can be released.
     */
    interface LockHandle extends AutoCloseable {
        /**
         * Release the lock.
         */
        void release();

        @Override
        default void close() {
            release();
        }
    }
}
