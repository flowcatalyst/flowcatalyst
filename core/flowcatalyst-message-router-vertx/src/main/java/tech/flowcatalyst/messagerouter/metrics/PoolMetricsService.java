package tech.flowcatalyst.messagerouter.metrics;

import tech.flowcatalyst.messagerouter.model.PoolStats;

import java.util.Map;

/**
 * Service for tracking processing pool metrics
 */
public interface PoolMetricsService {

    /**
     * Record that a message was submitted to a pool
     */
    void recordMessageSubmitted(String poolCode);

    /**
     * Record that a message processing started
     */
    void recordProcessingStarted(String poolCode);

    /**
     * Record that message processing finished (must be called in finally block)
     * This decrements the active workers counter to prevent metrics drift
     */
    void recordProcessingFinished(String poolCode);

    /**
     * Record that a message processing completed successfully
     */
    void recordProcessingSuccess(String poolCode, long durationMs);

    /**
     * Record that a message processing failed
     */
    void recordProcessingFailure(String poolCode, long durationMs, String errorType);

    /**
     * Record that a message was rejected due to rate limiting
     */
    void recordRateLimitExceeded(String poolCode);

    /**
     * Record that message processing encountered a transient error (will be retried).
     * These should NOT count against success rate since they may eventually succeed.
     * Examples: 200 with ack=false (not ready yet), 5xx errors (infrastructure issues)
     */
    void recordProcessingTransient(String poolCode, long durationMs);

    /**
     * Initialize pool capacity settings (called once when pool is created)
     */
    void initializePoolCapacity(String poolCode, int maxConcurrency, int maxQueueCapacity);

    /**
     * Update gauge metrics for pool state
     *
     * @param poolCode the pool code
     * @param activeWorkers number of workers currently processing messages
     * @param availablePermits number of available concurrency permits
     * @param queueSize total number of queued messages across all groups
     * @param messageGroupCount number of active message groups (each has its own virtual thread)
     */
    void updatePoolGauges(String poolCode, int activeWorkers, int availablePermits, int queueSize, int messageGroupCount);

    /**
     * Get statistics for a specific pool
     */
    PoolStats getPoolStats(String poolCode);

    /**
     * Get statistics for all pools
     */
    Map<String, PoolStats> getAllPoolStats();

    /**
     * Get the timestamp of the last successful processing for a pool
     * Returns null if no processing has occurred yet
     */
    Long getLastActivityTimestamp(String poolCode);

    /**
     * Remove all metrics for a pool
     * Called when a pool is removed during configuration sync
     *
     * @param poolCode the pool code to remove metrics for
     */
    void removePoolMetrics(String poolCode);
}
