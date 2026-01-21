package tech.flowcatalyst.messagerouter.consumer;

public interface QueueConsumer {

    /**
     * Starts the queue consumer and begins polling/consuming messages
     */
    void start();

    /**
     * Stops the queue consumer from accepting new messages
     */
    void stop();

    /**
     * Returns the queue identifier (name or URI)
     */
    String getQueueIdentifier();

    /**
     * Returns true if the consumer has fully stopped (all threads terminated)
     */
    boolean isFullyStopped();

    /**
     * Returns the timestamp (milliseconds since epoch) of the last successful poll attempt.
     * This is updated on each poll iteration, regardless of whether messages were received.
     * Returns 0 if consumer has never polled.
     */
    long getLastPollTime();

    /**
     * Returns true if the consumer is healthy (running and actively polling).
     * A consumer is considered unhealthy if:
     * - It has stopped running
     * - It hasn't polled in the last 60 seconds (stalled/hung)
     */
    boolean isHealthy();
}
