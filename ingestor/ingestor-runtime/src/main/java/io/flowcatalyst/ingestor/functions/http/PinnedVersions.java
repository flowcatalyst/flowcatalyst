package io.flowcatalyst.ingestor.functions.http;

import io.flowcatalyst.ingestor.functions.load.LoadedFunction;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.locks.ReentrantLock;
import java.util.function.Predicate;
import java.util.function.Supplier;
import org.jspecify.annotations.Nullable;

/// A small LRU of versions loaded for a call that is not to the live version (an alias, an explicit `@v{n}`, a
/// candidate under proof), beside the registry and NEVER the registry's live slot. Capacity 8.
///
/// **Lock-free reads.** A call to a cached version is a map lookup and a stamp; only a load, an eviction
/// and a sweep take a lock, and the load lock is per version, so a slow load of one version never holds up a
/// call to another. A version loaded for a call is the same [LoadedFunction] every concurrent call uses.
public final class PinnedVersions {

    public static final int CAPACITY = 8;

    /// An address and a version number.
    public record Key(String address, int version) {
    }

    private static final class Slot {
        final ReentrantLock lock = new ReentrantLock();
        volatile @Nullable LoadedFunction function;
        volatile @Nullable String fingerprint;
        volatile long lastUsedNanos = System.nanoTime();
    }

    private final Map<Key, Slot> slots = new ConcurrentHashMap<>();

    /// The loaded version for `key`, loading it with `loader` if it is not cached. `null` when `loader`
    /// answers `null` (the version cannot be loaded). A cached version loaded with other settings than
    /// `fingerprint` is closed and loaded again.
    public @Nullable LoadedFunction getOrLoad(Key key, String fingerprint, Supplier<@Nullable LoadedFunction> loader) {
        Objects.requireNonNull(key, "key");
        Slot slot = slots.computeIfAbsent(key, k -> new Slot());
        LoadedFunction cached = slot.function;
        if (cached != null && !cached.isClosing() && fingerprint.equals(slot.fingerprint)) {
            slot.lastUsedNanos = System.nanoTime();
            return cached;
        }
        slot.lock.lock();
        try {
            cached = slot.function;
            if (cached != null && !cached.isClosing() && fingerprint.equals(slot.fingerprint)) {
                slot.lastUsedNanos = System.nanoTime();
                return cached;
            }
            LoadedFunction stale = cached;
            LoadedFunction loaded = loader.get();
            if (loaded == null) {
                slot.function = null;
                slot.fingerprint = null;
            } else {
                slot.function = loaded;
                slot.fingerprint = fingerprint;
                slot.lastUsedNanos = System.nanoTime();
            }
            if (stale != null) {
                closeAsync(stale);
            }
            if (loaded != null) {
                evictOverCapacity(key);
            }
            return loaded;
        } finally {
            slot.lock.unlock();
        }
    }

    /// Adds an already loaded version (one the reconciler loaded to prove it).
    public void put(Key key, String fingerprint, LoadedFunction function) {
        Slot slot = slots.computeIfAbsent(key, k -> new Slot());
        slot.lock.lock();
        try {
            LoadedFunction stale = slot.function;
            slot.function = function;
            slot.fingerprint = fingerprint;
            slot.lastUsedNanos = System.nanoTime();
            if (stale != null && stale != function) {
                closeAsync(stale);
            }
            evictOverCapacity(key);
        } finally {
            slot.lock.unlock();
        }
    }

    /// Takes the cached version out WITHOUT closing it (a candidate promoted to live moves to the registry
    /// as the very instance that was proven).
    public @Nullable LoadedFunction take(Key key, String fingerprint) {
        Slot slot = slots.get(key);
        if (slot == null) {
            return null;
        }
        slot.lock.lock();
        try {
            LoadedFunction fn = slot.function;
            if (fn == null || fn.isClosing() || !fingerprint.equals(slot.fingerprint)) {
                return null;
            }
            slot.function = null;
            slot.fingerprint = null;
            slots.remove(key, slot);
            return fn;
        } finally {
            slot.lock.unlock();
        }
    }

    /// The cached version, without stamping a use.
    public @Nullable LoadedFunction peek(Key key) {
        Slot slot = slots.get(key);
        return slot == null ? null : slot.function;
    }

    /// Closes and drops every cached version `keep` does not name — "closed when its version leaves desired
    /// state" — called after each reconcile. Settings changes are handled by [#getOrLoad]'s fingerprint.
    public void sweep(Predicate<Key> keep) {
        for (Map.Entry<Key, Slot> e : List.copyOf(slots.entrySet())) {
            if (keep.test(e.getKey())) {
                continue;
            }
            Slot slot = e.getValue();
            slot.lock.lock();
            try {
                LoadedFunction fn = slot.function;
                slot.function = null;
                slot.fingerprint = null;
                slots.remove(e.getKey(), slot);
                if (fn != null) {
                    closeAsync(fn);
                }
            } finally {
                slot.lock.unlock();
            }
        }
    }

    /// Number of versions currently cached — test seam.
    public int size() {
        int n = 0;
        for (Slot s : slots.values()) {
            if (s.function != null) {
                n++;
            }
        }
        return n;
    }

    /// Closes everything, waiting for each to drain.
    public void close() {
        List<LoadedFunction> all = new ArrayList<>();
        for (Slot slot : slots.values()) {
            slot.lock.lock();
            try {
                LoadedFunction fn = slot.function;
                slot.function = null;
                if (fn != null) {
                    all.add(fn);
                }
            } finally {
                slot.lock.unlock();
            }
        }
        slots.clear();
        all.forEach(LoadedFunction::close);
    }

    /// The caller holds `keep`'s slot lock; the victim's lock is only tried, never waited for, so two loads
    /// cannot deadlock each other.
    private void evictOverCapacity(Key keep) {
        while (size() > CAPACITY) {
            @Nullable Key victimKey = null;
            @Nullable Slot victim = null;
            for (Map.Entry<Key, Slot> e : slots.entrySet()) {
                if (e.getKey().equals(keep) || e.getValue().function == null) {
                    continue;
                }
                if (victim == null || e.getValue().lastUsedNanos - victim.lastUsedNanos < 0) {
                    victimKey = e.getKey();
                    victim = e.getValue();
                }
            }
            if (victim == null || victimKey == null) {
                return;
            }
            if (!victim.lock.tryLock()) {
                return;
            }
            try {
                LoadedFunction fn = victim.function;
                victim.function = null;
                victim.fingerprint = null;
                slots.remove(victimKey, victim);
                if (fn != null) {
                    closeAsync(fn);
                }
            } finally {
                victim.lock.unlock();
            }
        }
    }

    private static void closeAsync(LoadedFunction fn) {
        Thread.ofVirtual().name("fn-pinned-close").start(fn::close);
    }
}
