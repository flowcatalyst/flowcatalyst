package tech.flowcatalyst.platform.authentication.ratelimit;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import tech.flowcatalyst.platform.authentication.AuthConfig;
import tech.flowcatalyst.platform.cache.CacheStore;

import java.time.Duration;
import java.time.Instant;
import java.util.Optional;

/**
 * Service for rate limiting authentication attempts.
 *
 * <p>Uses CacheStore to track failed attempts with a sliding window.
 * When the number of failed attempts exceeds the configured threshold,
 * subsequent attempts are blocked until the lockout period expires.
 *
 * <p>Configuration via flowcatalyst.auth.rate-limit:
 * <ul>
 *   <li>enabled: Whether rate limiting is active (default: true)</li>
 *   <li>max-failed-attempts: Threshold before lockout (default: 5)</li>
 *   <li>window-duration: Time window for counting failures (default: 15m)</li>
 *   <li>lockout-duration: How long to block after threshold exceeded (default: 15m)</li>
 * </ul>
 */
@ApplicationScoped
public class RateLimitService {

    private static final Logger LOG = Logger.getLogger(RateLimitService.class);
    private static final String CACHE_NAME = "rate-limits";

    @Inject
    AuthConfig authConfig;

    @Inject
    CacheStore cacheStore;

    /**
     * Check if an identifier is currently rate limited.
     *
     * @param identifier The identifier to check (e.g., client_id, username, IP)
     * @param attemptType The type of attempt (e.g., PASSWORD_GRANT, CLIENT_CREDENTIALS)
     * @return RateLimitResult indicating whether the attempt is allowed
     */
    public RateLimitResult check(String identifier, AttemptType attemptType) {
        if (!authConfig.rateLimit().enabled()) {
            return RateLimitResult.allow();
        }

        String cacheKey = buildCacheKey(identifier, attemptType);

        Optional<String> cached = cacheStore.get(CACHE_NAME, cacheKey);
        if (cached.isEmpty()) {
            return RateLimitResult.allow();
        }

        try {
            RateLimitEntry entry = RateLimitEntry.parse(cached.get());

            // Check if still in lockout period
            if (entry.lockedUntil != null && Instant.now().isBefore(entry.lockedUntil)) {
                Duration retryAfter = Duration.between(Instant.now(), entry.lockedUntil);
                LOG.infof("Rate limit exceeded for %s:%s, locked until %s",
                    attemptType, identifier, entry.lockedUntil);
                return RateLimitResult.block(entry.failureCount, retryAfter);
            }

            // Check if failure count exceeds threshold
            int maxAttempts = authConfig.rateLimit().maxFailedAttempts();
            if (entry.failureCount >= maxAttempts) {
                // Apply lockout
                Duration lockoutDuration = authConfig.rateLimit().lockoutDuration();
                Instant lockedUntil = Instant.now().plus(lockoutDuration);
                entry.lockedUntil = lockedUntil;
                cacheStore.put(CACHE_NAME, cacheKey, entry.serialize(), lockoutDuration);

                LOG.warnf("Rate limit threshold exceeded for %s:%s, locking for %s",
                    attemptType, identifier, lockoutDuration);
                return RateLimitResult.block(entry.failureCount, lockoutDuration);
            }

            return RateLimitResult.allow(maxAttempts - entry.failureCount);

        } catch (Exception e) {
            LOG.warnf("Failed to parse rate limit entry for %s:%s, allowing attempt", attemptType, identifier);
            return RateLimitResult.allow();
        }
    }

    /**
     * Record an authentication attempt.
     *
     * @param identifier The identifier (e.g., client_id, username, IP)
     * @param attemptType The type of attempt
     * @param success Whether the attempt was successful
     */
    public void recordAttempt(String identifier, AttemptType attemptType, boolean success) {
        if (!authConfig.rateLimit().enabled()) {
            return;
        }

        String cacheKey = buildCacheKey(identifier, attemptType);

        if (success) {
            // Successful attempt clears the rate limit
            cacheStore.invalidate(CACHE_NAME, cacheKey);
            LOG.debugf("Cleared rate limit for %s:%s after successful attempt", attemptType, identifier);
            return;
        }

        // Failed attempt - increment counter
        Duration windowDuration = authConfig.rateLimit().windowDuration();

        Optional<String> cached = cacheStore.get(CACHE_NAME, cacheKey);
        RateLimitEntry entry;

        if (cached.isPresent()) {
            try {
                entry = RateLimitEntry.parse(cached.get());
                entry.failureCount++;
                entry.lastAttempt = Instant.now();
            } catch (Exception e) {
                entry = new RateLimitEntry(1, Instant.now(), null);
            }
        } else {
            entry = new RateLimitEntry(1, Instant.now(), null);
        }

        cacheStore.put(CACHE_NAME, cacheKey, entry.serialize(), windowDuration);
        LOG.debugf("Recorded failed attempt for %s:%s, count=%d", attemptType, identifier, entry.failureCount);
    }

    private String buildCacheKey(String identifier, AttemptType attemptType) {
        return attemptType.name() + ":" + identifier;
    }

    /**
     * Types of authentication attempts for rate limiting.
     */
    public enum AttemptType {
        PASSWORD_GRANT,
        CLIENT_CREDENTIALS,
        AUTHORIZATION_CODE,
        REFRESH_TOKEN
    }

    /**
     * Result of a rate limit check.
     */
    public record RateLimitResult(
        boolean permitted,
        int remainingAttempts,
        Duration retryAfter
    ) {
        public static RateLimitResult allow() {
            return new RateLimitResult(true, -1, null);
        }

        public static RateLimitResult allow(int remaining) {
            return new RateLimitResult(true, remaining, null);
        }

        public static RateLimitResult block(int failureCount, Duration retryAfter) {
            return new RateLimitResult(false, 0, retryAfter);
        }
    }

    /**
     * Internal entry stored in cache.
     */
    private static class RateLimitEntry {
        int failureCount;
        Instant lastAttempt;
        Instant lockedUntil;

        RateLimitEntry(int failureCount, Instant lastAttempt, Instant lockedUntil) {
            this.failureCount = failureCount;
            this.lastAttempt = lastAttempt;
            this.lockedUntil = lockedUntil;
        }

        String serialize() {
            StringBuilder sb = new StringBuilder();
            sb.append(failureCount);
            sb.append("|");
            sb.append(lastAttempt != null ? lastAttempt.toEpochMilli() : "");
            sb.append("|");
            sb.append(lockedUntil != null ? lockedUntil.toEpochMilli() : "");
            return sb.toString();
        }

        static RateLimitEntry parse(String data) {
            String[] parts = data.split("\\|", -1);
            int count = Integer.parseInt(parts[0]);
            Instant lastAttempt = parts[1].isEmpty() ? null : Instant.ofEpochMilli(Long.parseLong(parts[1]));
            Instant lockedUntil = parts.length > 2 && !parts[2].isEmpty()
                ? Instant.ofEpochMilli(Long.parseLong(parts[2]))
                : null;
            return new RateLimitEntry(count, lastAttempt, lockedUntil);
        }
    }
}
