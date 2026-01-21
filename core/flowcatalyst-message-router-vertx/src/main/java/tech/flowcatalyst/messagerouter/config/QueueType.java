package tech.flowcatalyst.messagerouter.config;

public enum QueueType {
    ACTIVEMQ,
    SQS,
    NATS,     // NATS JetStream with durable file storage
    EMBEDDED  // Embedded SQLite queue for developer builds (replaces Chronicle)
}