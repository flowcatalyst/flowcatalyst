package io.flowcatalyst.ingestor.functions.http;

import java.util.Objects;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.Semaphore;
import java.util.concurrent.atomic.AtomicBoolean;
import org.jspecify.annotations.Nullable;

/// Per-function invocation permits, `tryAcquire` only — never a queue (R3): a call that finds none answers 429
/// at once. The semaphore is sized from the function's `limits.maxConcurrency`, created lazily and replaced
/// when a reconcile changes the size (permits already checked out of a replaced semaphore are released
/// into that same object, which nobody else references any more — harmless). `Semaphore` is a CAS
/// counter: acquiring takes no lock.
///
/// A grant is released by whoever holds it, when the work it guards really ends — for a call past its deadline
/// that is when the handler returns, not when the 504 is sent (javalin H7).
public final class Permits {

    private final ConcurrentHashMap<String, Sized> perFunction = new ConcurrentHashMap<>();

    private record Sized(int size, Semaphore semaphore) {
    }

    /// A permit taken; [#release] gives it back, once.
    public static final class Grant {
        private final Semaphore semaphore;
        private final AtomicBoolean released = new AtomicBoolean();

        private Grant(Semaphore semaphore) {
            this.semaphore = semaphore;
        }

        public void release() {
            if (released.compareAndSet(false, true)) {
                semaphore.release();
            }
        }
    }

    /// A permit for `address`, or `null` when `maxConcurrency` calls are already running.
    public @Nullable Grant tryAcquire(String address, int maxConcurrency) {
        Objects.requireNonNull(address, "address");
        Semaphore semaphore = semaphoreFor(address, maxConcurrency);
        return semaphore.tryAcquire() ? new Grant(semaphore) : null;
    }

    /// Test/diagnostic seam: `address`'s own gauge, `-1` if never sized.
    public int available(String address) {
        Sized sized = perFunction.get(address);
        return sized == null ? -1 : sized.semaphore().availablePermits();
    }

    /// Drops `address`'s semaphore once its function has left desired state.
    public void forget(String address) {
        perFunction.remove(address);
    }

    /// Drops the semaphore of every address `keep` does not hold (the functions that left desired state), so the
    /// map does not grow over the life of the process. Calls still running release into their own object.
    public void retainOnly(java.util.Set<String> keep) {
        perFunction.keySet().removeIf(a -> !keep.contains(a));
    }

    private Semaphore semaphoreFor(String address, int size) {
        Sized current = perFunction.get(address);
        if (current != null && current.size() == size) {
            return current.semaphore();
        }
        return perFunction.compute(address,
                (a, existing) -> existing == null || existing.size() != size ? new Sized(size, new Semaphore(size)) : existing)
                .semaphore();
    }
}
