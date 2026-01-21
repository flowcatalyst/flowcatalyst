package tech.flowcatalyst.queue;

import java.util.List;
import java.util.Optional;

/**
 * Result of publishing message(s) to a queue.
 */
public record QueuePublishResult(
    boolean success,
    List<String> publishedMessageIds,
    List<String> failedMessageIds,
    Optional<String> errorMessage
) {
    /**
     * Create a success result for a single message.
     */
    public static QueuePublishResult success(String messageId) {
        return new QueuePublishResult(true, List.of(messageId), List.of(), Optional.empty());
    }

    /**
     * Create a success result for batch messages.
     */
    public static QueuePublishResult success(List<String> messageIds) {
        return new QueuePublishResult(true, messageIds, List.of(), Optional.empty());
    }

    /**
     * Create a partial success result (some messages failed).
     */
    public static QueuePublishResult partial(List<String> published, List<String> failed, String error) {
        return new QueuePublishResult(false, published, failed, Optional.of(error));
    }

    /**
     * Create a deduplicated result (message already exists).
     */
    public static QueuePublishResult deduplicated(String messageId) {
        return new QueuePublishResult(true, List.of(), List.of(messageId), Optional.of("Deduplicated"));
    }

    /**
     * Create a failure result.
     */
    public static QueuePublishResult failure(String messageId, String error) {
        return new QueuePublishResult(false, List.of(), List.of(messageId), Optional.of(error));
    }

    /**
     * Create a failure result for batch.
     */
    public static QueuePublishResult failure(List<String> messageIds, String error) {
        return new QueuePublishResult(false, List.of(), messageIds, Optional.of(error));
    }

    /**
     * Check if all messages were published successfully.
     */
    public boolean allPublished() {
        return success && failedMessageIds.isEmpty();
    }

    /**
     * Check if the message was deduplicated (not published because duplicate).
     */
    public boolean deduplicated() {
        return success && publishedMessageIds.isEmpty() &&
               errorMessage.map(e -> e.equals("Deduplicated")).orElse(false);
    }
}
