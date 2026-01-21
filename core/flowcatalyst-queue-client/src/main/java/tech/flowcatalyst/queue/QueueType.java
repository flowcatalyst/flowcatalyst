package tech.flowcatalyst.queue;

/**
 * Supported queue implementation types.
 */
public enum QueueType {
    /**
     * AWS Simple Queue Service (SQS).
     * Supports both standard and FIFO queues.
     */
    SQS,

    /**
     * Apache ActiveMQ Artemis (JMS).
     * Supports message groups for ordered delivery.
     */
    ACTIVEMQ,

    /**
     * NATS JetStream with durable storage.
     * Supports message groups via subject-based routing.
     */
    NATS,

    /**
     * Embedded SQLite-based queue.
     * Useful for development and single-node deployments.
     */
    EMBEDDED
}
