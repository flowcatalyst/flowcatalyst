package tech.flowcatalyst.messagerouter.mediator;

import java.time.Duration;

/**
 * Sealed interface representing typed errors from mediation attempts.
 *
 * <p>This provides type-safe error handling for HTTP mediation, allowing
 * exhaustive pattern matching on error types and explicit retryability semantics.
 *
 * <h2>Error Types</h2>
 * <ul>
 *   <li>{@link Timeout} - Request timed out (retryable)</li>
 *   <li>{@link CircuitOpen} - Circuit breaker is open (not retryable)</li>
 *   <li>{@link HttpError} - HTTP error response (5xx retryable, 4xx not)</li>
 *   <li>{@link NetworkError} - Network-level failure (retryable)</li>
 *   <li>{@link RateLimited} - Rate limited by target (retryable after delay)</li>
 * </ul>
 *
 * <h2>Usage Example</h2>
 * <pre>{@code
 * MediationError error = ...;
 * switch (error) {
 *     case MediationError.Timeout t -> handleTimeout(t.duration());
 *     case MediationError.CircuitOpen c -> handleCircuitOpen(c.circuitName());
 *     case MediationError.HttpError h -> handleHttpError(h.statusCode(), h.body());
 *     case MediationError.NetworkError n -> handleNetworkError(n.cause());
 *     case MediationError.RateLimited r -> handleRateLimited(r.retryAfter());
 * }
 * }</pre>
 */
public sealed interface MediationError permits
    MediationError.Timeout,
    MediationError.CircuitOpen,
    MediationError.HttpError,
    MediationError.NetworkError,
    MediationError.RateLimited {

    /**
     * Get a human-readable message describing the error.
     * @return error message
     */
    String message();

    /**
     * Check if this error type is retryable.
     * @return true if the message should be retried, false if it should be permanently failed
     */
    boolean isRetryable();

    /**
     * Request timed out before receiving a response.
     *
     * @param duration how long we waited before timing out
     */
    record Timeout(Duration duration) implements MediationError {
        @Override
        public String message() {
            return "Request timed out after " + duration.toMillis() + "ms";
        }

        @Override
        public boolean isRetryable() {
            return true;
        }
    }

    /**
     * Circuit breaker is open, preventing requests to the target.
     *
     * @param circuitName identifier of the circuit breaker
     */
    record CircuitOpen(String circuitName) implements MediationError {
        @Override
        public String message() {
            return "Circuit breaker open: " + circuitName;
        }

        @Override
        public boolean isRetryable() {
            // Circuit breaker being open means the target is unhealthy
            // We should not retry immediately - let the circuit recover first
            return false;
        }
    }

    /**
     * HTTP error response from the target.
     *
     * @param statusCode HTTP status code (4xx, 5xx)
     * @param body response body (may be null)
     */
    record HttpError(int statusCode, String body) implements MediationError {
        @Override
        public String message() {
            return "HTTP " + statusCode + (body != null ? ": " + truncate(body, 100) : "");
        }

        @Override
        public boolean isRetryable() {
            // 5xx errors are transient server issues - retryable
            // 4xx errors are client/config issues - not retryable
            return statusCode >= 500;
        }

        private static String truncate(String s, int maxLen) {
            return s.length() <= maxLen ? s : s.substring(0, maxLen) + "...";
        }
    }

    /**
     * Network-level error (connection refused, DNS failure, etc).
     *
     * @param cause the underlying exception
     */
    record NetworkError(Throwable cause) implements MediationError {
        @Override
        public String message() {
            return "Network error: " + (cause != null ? cause.getMessage() : "unknown");
        }

        @Override
        public boolean isRetryable() {
            return true;
        }
    }

    /**
     * Rate limited by the target service (HTTP 429).
     *
     * @param retryAfter how long to wait before retrying
     */
    record RateLimited(Duration retryAfter) implements MediationError {
        @Override
        public String message() {
            return "Rate limited, retry after " + retryAfter.toSeconds() + "s";
        }

        @Override
        public boolean isRetryable() {
            return true;
        }
    }
}
