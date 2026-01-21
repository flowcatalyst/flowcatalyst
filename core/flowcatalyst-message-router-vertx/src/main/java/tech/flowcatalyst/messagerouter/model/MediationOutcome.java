package tech.flowcatalyst.messagerouter.model;

/**
 * Outcome of a mediation attempt, containing the result and optional delay for retries.
 *
 * <p>This record wraps {@link MediationResult} with additional context that affects
 * how the message should be handled after mediation:</p>
 * <ul>
 *   <li>{@code result} - The mediation result (SUCCESS, ERROR_PROCESS, ERROR_CONFIG, etc.)</li>
 *   <li>{@code delaySeconds} - Optional delay before message becomes visible again (for retries)</li>
 * </ul>
 *
 * <h2>Delay Behavior</h2>
 * <p>When {@code result} is {@code ERROR_PROCESS} (transient error requiring retry):</p>
 * <ul>
 *   <li>If {@code delaySeconds > 0}, the message visibility is set to that value</li>
 *   <li>If {@code delaySeconds == null || delaySeconds <= 0}, default visibility (30s) is used</li>
 * </ul>
 *
 * @param result The mediation result
 * @param delaySeconds Optional delay in seconds for retry (1-43200), null for default
 */
public record MediationOutcome(
    MediationResult result,
    Integer delaySeconds
) {
    /** Default delay when none specified */
    public static final int DEFAULT_DELAY_SECONDS = 30;

    /** Maximum delay allowed (12 hours = 43200 seconds, SQS limit) */
    public static final int MAX_DELAY_SECONDS = 43200;

    /**
     * Create an outcome with just a result (no custom delay).
     */
    public static MediationOutcome of(MediationResult result) {
        return new MediationOutcome(result, null);
    }

    /**
     * Create an outcome with a result and custom delay.
     */
    public static MediationOutcome of(MediationResult result, Integer delaySeconds) {
        return new MediationOutcome(result, delaySeconds);
    }

    /**
     * Create a SUCCESS outcome.
     */
    public static MediationOutcome success() {
        return new MediationOutcome(MediationResult.SUCCESS, null);
    }

    /**
     * Create an ERROR_PROCESS outcome with optional delay.
     */
    public static MediationOutcome errorProcess(Integer delaySeconds) {
        return new MediationOutcome(MediationResult.ERROR_PROCESS, delaySeconds);
    }

    /**
     * Create an ERROR_CONFIG outcome (no delay needed - won't be retried).
     */
    public static MediationOutcome errorConfig() {
        return new MediationOutcome(MediationResult.ERROR_CONFIG, null);
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
}
