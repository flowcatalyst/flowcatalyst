package tech.flowcatalyst.messagerouter.config;

import com.fasterxml.jackson.annotation.JsonProperty;

public record ProcessingPool(
    String code,
    Integer concurrency,
    Integer rateLimitPerMinute
) {
    /**
     * Get effective concurrency - uses provided value or calculates from rate limit.
     *
     * @return concurrency value, calculated as:
     *         - If concurrency provided and > 0: use it
     *         - Else if rateLimitPerMinute provided: max(rateLimitPerMinute / 60, 1)
     *         - Else: default to 1
     */
    public int effectiveConcurrency() {
        if (concurrency != null && concurrency > 0) {
            return concurrency;
        }
        if (rateLimitPerMinute != null && rateLimitPerMinute > 0) {
            // At least 1 worker per second of rate limit capacity
            return Math.max(rateLimitPerMinute / 60, 1);
        }
        return 1; // Fallback default
    }
}
