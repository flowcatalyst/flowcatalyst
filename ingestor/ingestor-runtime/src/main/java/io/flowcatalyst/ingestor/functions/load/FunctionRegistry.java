package io.flowcatalyst.ingestor.functions.load;

import io.flowcatalyst.function.FunctionAddress;
import java.time.Clock;
import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.locks.ReentrantLock;
import org.jspecify.annotations.Nullable;

/// One live [LoadedFunction] per [FunctionAddress]. Bounded by `maxLoaded`: a
/// *warm* entry (kept resident by the platform's own `warm` flag) is never
/// evicted; a *lazy* one is, least-recently-accessed first, to make room for
/// a new address.
///
/// Reads ([#get], [#peek], [#snapshot]) take no lock: the invoke path calls
/// [#get] for every request, and nothing serialises calls of one function (R3).
/// Only [#put] and [#remove] — reconcile-time, never per call — take the
/// mutation lock, which also makes capacity checks and evictions consistent.
public final class FunctionRegistry {

    private final int maxLoaded;
    private final Clock clock;
    private final Map<FunctionAddress, Entry> live = new ConcurrentHashMap<>();

    /// Serialises writers only.
    private final ReentrantLock mutation = new ReentrantLock();

    public FunctionRegistry(int maxLoaded) {
        this(maxLoaded, Clock.systemUTC());
    }

    /// @param clock injectable so a reconciler test can move "now" for the
    ///              idle-eviction clause without sleeping
    public FunctionRegistry(int maxLoaded, Clock clock) {
        if (maxLoaded < 1) {
            throw new IllegalArgumentException("maxLoaded must be at least 1, was " + maxLoaded);
        }
        this.maxLoaded = maxLoaded;
        this.clock = Objects.requireNonNull(clock, "clock");
    }

    /// The live [LoadedFunction] for `address`, or `null`. Counts as an
    /// access for LRU purposes AND stamps the entry's last-accessed time —
    /// this is the method the invoke path uses. A caller that is only
    /// INSPECTING the registry — the reconciler's own bookkeeping — must use
    /// [#peek] instead, or every reconcile cycle would itself count as an
    /// access and no lazy entry could ever go idle.
    public @Nullable LoadedFunction get(FunctionAddress address) {
        Entry entry = live.get(address);
        if (entry == null) {
            return null;
        }
        entry.lastNanos = System.nanoTime();
        entry.lastAccessed = clock.instant();
        return entry.function;
    }

    /// [#get] without the access stamp — for a caller that is inspecting the
    /// registry, not invoking through it.
    public @Nullable LoadedFunction peek(FunctionAddress address) {
        Entry entry = live.get(address);
        return entry == null ? null : entry.function;
    }

    /// Whether `address`'s live entry is warm (exempt from eviction); `false` if there is none.
    public boolean isWarm(FunctionAddress address) {
        Entry entry = live.get(address);
        return entry != null && entry.warm;
    }

    /// One entry, as of the moment [#snapshot] was taken.
    public record Snapshot(FunctionAddress address, LoadedFunction function, boolean warm, Instant lastAccessed) {
    }

    /// A point-in-time copy of every live entry — never stamps an access.
    public List<Snapshot> snapshot() {
        List<Snapshot> out = new ArrayList<>(live.size());
        for (Map.Entry<FunctionAddress, Entry> e : live.entrySet()) {
            out.add(new Snapshot(e.getKey(), e.getValue().function, e.getValue().warm, e.getValue().lastAccessed));
        }
        return out;
    }

    /// Removes and returns `address`'s live entry, or `null` if none — the
    /// caller closes the returned [LoadedFunction] (this never closes
    /// anything itself, same division of labour as [#put]'s displaced return).
    public @Nullable LoadedFunction remove(FunctionAddress address) {
        mutation.lock();
        try {
            Entry entry = live.remove(address);
            return entry == null ? null : entry.function;
        } finally {
            mutation.unlock();
        }
    }

    /// [#put(LoadedFunction, boolean)] with `warm = false`.
    public @Nullable LoadedFunction put(LoadedFunction function) {
        return put(function, false);
    }

    /// Registers `function` as the live version for its address. Returns
    /// the version it displaced for that same address, if any — the caller
    /// closes it once its in-flight invocations finish
    /// ([LoadedFunction#close] drains them).
    ///
    /// @param warm whether this entry is exempt from LRU eviction
    /// @throws IllegalStateException if registering a new address would
    ///                                exceed `maxLoaded` and every existing
    ///                                entry is warm
    public @Nullable LoadedFunction put(LoadedFunction function, boolean warm) {
        Objects.requireNonNull(function, "function");
        FunctionAddress address = function.address();
        @Nullable LoadedFunction evicted = null;
        @Nullable LoadedFunction displaced;
        mutation.lock();
        try {
            if (!live.containsKey(address) && live.size() >= maxLoaded) {
                evicted = evictOneLazyEntryOrThrow();
            }
            Entry previous = live.put(address, new Entry(function, warm, clock.instant()));
            displaced = previous == null ? null : previous.function;
        } finally {
            mutation.unlock();
        }
        if (evicted != null) {
            // Nothing else holds it any more: closed (after its in-flight calls drain) on a thread of its
            // own so that the writer does not wait for them.
            LoadedFunction toClose = evicted;
            Thread.ofVirtual().name("fn-evict-" + toClose.address().render()).start(toClose::close);
        }
        return displaced;
    }

    private LoadedFunction evictOneLazyEntryOrThrow() {
        Map.@Nullable Entry<FunctionAddress, Entry> victim = null;
        for (Map.Entry<FunctionAddress, Entry> candidate : live.entrySet()) {
            if (candidate.getValue().warm) {
                continue;
            }
            if (victim == null || candidate.getValue().lastNanos - victim.getValue().lastNanos < 0) {
                victim = candidate;
            }
        }
        if (victim == null) {
            throw new IllegalStateException("function registry is at capacity (" + maxLoaded
                    + ") and every loaded entry is warm; the platform's warm-capacity check "
                    + "should have prevented this from being reached");
        }
        live.remove(victim.getKey());
        return victim.getValue().function;
    }

    /// The access stamps are written by invoking threads without a lock: each is a single
    /// volatile write of a value that only has to be roughly current.
    private static final class Entry {
        private final LoadedFunction function;
        private final boolean warm;
        private volatile Instant lastAccessed;
        private volatile long lastNanos = System.nanoTime();

        Entry(LoadedFunction function, boolean warm, Instant lastAccessed) {
            this.function = Objects.requireNonNull(function, "function");
            this.warm = warm;
            this.lastAccessed = Objects.requireNonNull(lastAccessed, "lastAccessed");
        }
    }
}
