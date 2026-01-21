package tech.flowcatalyst.queue;

/**
 * A message to be published to a queue.
 *
 * @param messageId Unique identifier for the message
 * @param messageGroupId Message group ID for FIFO ordering (required for FIFO queues)
 * @param deduplicationId Deduplication ID to prevent duplicate messages
 * @param body Message body content (typically JSON)
 */
public record QueueMessage(
    String messageId,
    String messageGroupId,
    String deduplicationId,
    String body
) {
    /**
     * Create a message with auto-generated deduplication ID (same as messageId).
     */
    public static QueueMessage of(String messageId, String messageGroupId, String body) {
        return new QueueMessage(messageId, messageGroupId, messageId, body);
    }

    /**
     * Create a message with no message group (for standard queues).
     */
    public static QueueMessage standard(String messageId, String body) {
        return new QueueMessage(messageId, null, messageId, body);
    }
}
