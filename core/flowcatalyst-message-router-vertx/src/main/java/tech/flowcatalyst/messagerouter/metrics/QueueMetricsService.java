package tech.flowcatalyst.messagerouter.metrics;

import java.util.Map;

/**
 * Service for tracking queue-level metrics including message throughput,
 * success/failure rates, and queue depth.
 */
public interface QueueMetricsService {

    /**
     * Record that a message was received from a queue
     *
     * @param queueIdentifier the queue name or URI
     */
    void recordMessageReceived(String queueIdentifier);

    /**
     * Record that a message was processed (successfully or failed)
     *
     * @param queueIdentifier the queue name or URI
     * @param success true if processed successfully, false if failed
     */
    void recordMessageProcessed(String queueIdentifier, boolean success);

    /**
     * Record that a message was deferred (rate limiting, capacity, duplicate).
     * Deferred messages are NOT counted as failures - they will be retried later.
     *
     * @param queueIdentifier the queue name or URI
     */
    void recordMessageDeferred(String queueIdentifier);

    /**
     * Record the current queue depth
     *
     * @param queueIdentifier the queue name or URI
     * @param depth current number of messages in queue
     */
    void recordQueueDepth(String queueIdentifier, long depth);

    /**
     * Record pending messages and messages not visible (in-flight)
     *
     * @param queueIdentifier the queue name or URI
     * @param pendingMessages number of pending messages
     * @param messagesNotVisible number of messages currently being processed (not visible)
     */
    void recordQueueMetrics(String queueIdentifier, long pendingMessages, long messagesNotVisible);

    /**
     * Get statistics for a specific queue
     *
     * @param queueIdentifier the queue name or URI
     * @return queue statistics, or null if queue not found
     */
    QueueStats getQueueStats(String queueIdentifier);

    /**
     * Get statistics for all queues
     *
     * @return map of queue identifier to statistics
     */
    Map<String, QueueStats> getAllQueueStats();
}
