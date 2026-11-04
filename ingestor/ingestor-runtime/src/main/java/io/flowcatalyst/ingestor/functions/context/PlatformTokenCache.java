package io.flowcatalyst.ingestor.functions.context;

import io.flowcatalyst.function.PlatformTokenException;
import io.flowcatalyst.ingestor.functions.control.Control;
import io.flowcatalyst.ingestor.functions.control.ControlPlane;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.util.Map;
import java.util.Objects;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.locks.ReentrantLock;
import org.jspecify.annotations.Nullable;

/// The host's cache of platform tokens (docs/function-platform-token.md §5), one entry per function id:
///
/// - a token is served while more than `max(60 s, 10% of its lifetime)` remains, and fetched again otherwise;
/// - concurrent callers for one function share one fetch (a lock per function, held across the fetch, so a
///   function's callers queue behind it and the second finds the first's token);
/// - `refresh` skips the cache and replaces the entry, though callers that all asked to refresh at once
///   share the one new token;
/// - a failed fetch is not cached: the caller gets the error and the next call tries again;
/// - [#retain] drops the entries of functions that left the desired document.
///
/// Expiry is on the host's clock, from the moment the fetch began plus `expiresIn`. The token is a
/// credential: it is held only in memory, never logged, and never in a `toString`.
public final class PlatformTokenCache {

    /// The least time a served token must have left.
    static final Duration MIN_REMAINING = Duration.ofSeconds(60);

    private final ControlPlane controlPlane;
    private final Clock clock;
    private final Map<String, Slot> slots = new ConcurrentHashMap<>();

    public PlatformTokenCache(ControlPlane controlPlane, Clock clock) {
        this.controlPlane = Objects.requireNonNull(controlPlane, "controlPlane");
        this.clock = Objects.requireNonNull(clock, "clock");
    }

    /// One function's entry. `current` is replaced whole; `generation` counts replacements so a refreshing
    /// caller that waited on the lock can tell another caller already replaced the token.
    private static final class Slot {
        final ReentrantLock lock = new ReentrantLock();
        volatile @Nullable Held current;
        volatile long generation;
    }

    /// A token, when it was fetched and when it expires.
    private record Held(String value, Instant fetchedAt, Instant expiresAt) {
        boolean usable(Instant now) {
            Duration lifetime = Duration.between(fetchedAt, expiresAt);
            Duration margin = lifetime.dividedBy(10);
            if (margin.compareTo(MIN_REMAINING) < 0) {
                margin = MIN_REMAINING;
            }
            return Duration.between(now, expiresAt).compareTo(margin) > 0;
        }

        @Override
        public String toString() {
            return "Held[expiresAt=" + expiresAt + "]";
        }
    }

    /// The function's token: cached while valid, else fetched through the control plane as `version`.
    ///
    /// @param refresh fetch a new token even when the cached one is valid
    /// @throws PlatformTokenException the platform refused or could not be reached
    public String token(String functionId, int version, boolean refresh) {
        Slot slot = slots.computeIfAbsent(functionId, k -> new Slot());
        long seen = slot.generation;
        Held held = slot.current;
        if (!refresh && held != null && held.usable(clock.instant())) {
            return held.value();
        }
        slot.lock.lock();
        try {
            held = slot.current;
            if (held != null && held.usable(clock.instant()) && (!refresh || slot.generation != seen)) {
                return held.value();
            }
            Instant started = clock.instant();
            Control.TokenResponse response = controlPlane.platformToken(new Control.TokenRequest(functionId, version));
            String value = response.accessToken();
            if (value == null || value.isBlank()) {
                throw new PlatformTokenException(PlatformTokenException.UNAVAILABLE, "the platform's answer carried no token");
            }
            Held fresh = new Held(value, started, started.plusSeconds(Math.max(0, response.expiresIn())));
            slot.current = fresh;
            slot.generation++;
            return value;
        } finally {
            slot.lock.unlock();
        }
    }

    /// Drops the entries of every function not in `functionIds`.
    public void retain(Set<String> functionIds) {
        slots.keySet().removeIf(id -> !functionIds.contains(id));
    }

    /// How many functions hold an entry.
    public int size() {
        return slots.size();
    }
}
