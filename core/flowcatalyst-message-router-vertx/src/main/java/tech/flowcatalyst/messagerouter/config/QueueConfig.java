package tech.flowcatalyst.messagerouter.config;

/**
 * Configuration for a single queue
 *
 * @param queueUri    the unique queue identifier (ActiveMQ: queue name, SQS: ARN/URL, Embedded: queue name)
 * @param queueName   optional human-readable name for display/logging purposes
 * @param connections number of consumer connections for this queue (optional, defaults to global config)
 */
public record QueueConfig(
        String queueUri,
        String queueName,
        Integer connections
) {
}
