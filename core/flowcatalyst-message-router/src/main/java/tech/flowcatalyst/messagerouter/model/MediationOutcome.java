package tech.flowcatalyst.messagerouter.model;

import tech.flowcatalyst.messagerouter.mediator.MediationError;

import java.util.Optional;

/**
 * Outcome of a mediation attempt, containing the result, optional delay, and optional typed error.
 *
 * <p>This record wraps {@link MediationResult} with additional context that affects
 * how the message should be handled after mediation:</p>
 * <ul>
 *   <li>{@code result} - The mediation result (SUCCESS, ERROR_PROCESS, ERROR_CONFIG, etc.)</li>
 *   <li>{@code delaySeconds} - Optional delay before message becomes visible again (for retries)</li>
 *   <li>{@code error} - Optional typed error with detailed failure information</li>
 * </ul>
 *
 * <h2>Delay Behavior</h2>
 * <p>When {@code result} is {@code ERROR_PROCESS} (transient error requiring retry):</p>
 * <ul>
 *   <li>If {@code delaySeconds > 0}, the message visibility is set to that value</li>
 *   <li>If {@code delaySeconds == null || delaySeconds <= 0}, default visibility (30s) is used</li>
 * </ul>
 *
 * <h2>Error Field</h2>
 * <p>The {@code error} field provides typed error information for detailed error handling:</p>
 * <ul>
 *   <li>{@link MediationError.Timeout} - Request timed out</li>
 *   <li>{@link MediationError.CircuitOpen} - Circuit breaker prevented request</li>
 *   <li>{@link MediationError.HttpError} - HTTP error response from target</li>
 *   <li>{@link MediationError.NetworkError} - Network-level failure</li>
 *   <li>{@link MediationError.RateLimited} - Rate limited by target</li>
 * </ul>
 *
 * @param result The mediation result
 * @param delaySeconds Optional delay in seconds for retry (1-43200), null for default
 * @param error Optional typed error details
 */
public record MediationOutcome(
    MediationResult result,
    Integer delaySeconds,
    MediationError error
) {
    /** Default delay when none specified */
    public static final int DEFAULT_DELAY_SECONDS = 30;

    /** Maximum delay allowed (12 hours = 43200 seconds, SQS limit) */
    public static final int MAX_DELAY_SECONDS = 43200;

    /**
     * Create an outcome with just a result (no custom delay, no error).
     */
    public static MediationOutcome of(MediationResult result) {
        return new MediationOutcome(result, null, null);
    }

    /**
     * Create an outcome with a result and custom delay (no error).
     */
    public static MediationOutcome of(MediationResult result, Integer delaySeconds) {
        return new MediationOutcome(result, delaySeconds, null);
    }

    /**
     * Create an outcome with a result, delay, and typed error.
     */
    public static MediationOutcome of(MediationResult result, Integer delaySeconds, MediationError error) {
        return new MediationOutcome(result, delaySeconds, error);
    }

    /**
     * Create a SUCCESS outcome.
     */
    public static MediationOutcome success() {
        return new MediationOutcome(MediationResult.SUCCESS, null, null);
    }

    /**
     * Create an ERROR_PROCESS outcome with optional delay.
     */
    public static MediationOutcome errorProcess(Integer delaySeconds) {
        return new MediationOutcome(MediationResult.ERROR_PROCESS, delaySeconds, null);
    }

    /**
     * Create an ERROR_PROCESS outcome with typed error.
     */
    public static MediationOutcome errorProcess(MediationError error) {
        return new MediationOutcome(MediationResult.ERROR_PROCESS, null, error);
    }

    /**
     * Create an ERROR_PROCESS outcome with delay and typed error.
     */
    public static MediationOutcome errorProcess(Integer delaySeconds, MediationError error) {
        return new MediationOutcome(MediationResult.ERROR_PROCESS, delaySeconds, error);
    }

    /**
     * Create an ERROR_CONFIG outcome (no delay needed - won't be retried).
     */
    public static MediationOutcome errorConfig() {
        return new MediationOutcome(MediationResult.ERROR_CONFIG, null, null);
    }

    /**
     * Create an ERROR_CONFIG outcome with typed error.
     */
    public static MediationOutcome errorConfig(MediationError error) {
        return new MediationOutcome(MediationResult.ERROR_CONFIG, null, error);
    }

    /**
     * Create an ERROR_CONNECTION outcome.
     */
    public static MediationOutcome errorConnection() {
        return new MediationOutcome(MediationResult.ERROR_CONNECTION, null, null);
    }

    /**
     * Create an ERROR_CONNECTION outcome with typed error.
     */
    public static MediationOutcome errorConnection(MediationError error) {
        return new MediationOutcome(MediationResult.ERROR_CONNECTION, null, error);
    }

    /**
     * Get the effective delay in seconds, clamped to valid range.
     * Returns DEFAULT_DELAY_SECONDS if delaySeconds is null or <= 0.
     *
     * @return delay in seconds (1-43200)
     */
    public int getEffectiveDelaySeconds() {
        if (delaySeconds == null || delaySeconds <= 0) {
            return DEFAULT_DELAY_SECONDS;
        }
        return Math.min(delaySeconds, MAX_DELAY_SECONDS);
    }

    /**
     * Check if a custom delay was specified.
     */
    public boolean hasCustomDelay() {
        return delaySeconds != null && delaySeconds > 0;
    }

    /**
     * Get the typed error as an Optional.
     *
     * @return the error if present, empty otherwise
     */
    public Optional<MediationError> getError() {
        return Optional.ofNullable(error);
    }

    /**
     * Check if this outcome has a typed error.
     */
    public boolean hasError() {
        return error != null;
    }
}
