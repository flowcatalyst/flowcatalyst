package tech.flowcatalyst.outbox.model;

import java.time.Instant;

/**
 * Represents an item in the outbox table/collection.
 * Items are polled from customer databases and sent to FlowCatalyst APIs.
 */
public record OutboxItem(
    /**
     * Unique identifier for the outbox item (TSID format).
     */
    String id,

    /**
     * Type of item: EVENT or DISPATCH_JOB.
     */
    OutboxItemType type,

    /**
     * Message group for FIFO ordering.
     * Items within the same group are processed in order.
     */
    String messageGroup,

    /**
     * JSON payload to send to FlowCatalyst API.
     */
    String payload,

    /**
     * Current processing status (integer code).
     */
    OutboxStatus status,

    /**
     * Number of times this item has been retried.
     */
    int retryCount,

    /**
     * When the item was created in the outbox.
     */
    Instant createdAt,

    /**
     * When the item was last updated.
     */
    Instant updatedAt,

    /**
     * Error message if the item failed.
     */
    String errorMessage
) {
    /**
     * Get the effective message group (returns "default" if null or empty).
     */
    public String getEffectiveMessageGroup() {
        return (messageGroup == null || messageGroup.isEmpty()) ? "default" : messageGroup;
    }

    /**
     * Check if the item is pending.
     */
    public boolean isPending() {
        return status == OutboxStatus.PENDING;
    }

    /**
     * Check if the item is in progress.
     */
    public boolean isInProgress() {
        return status == OutboxStatus.IN_PROGRESS;
    }

    /**
     * Check if the item was successfully processed.
     */
    public boolean isSuccess() {
        return status == OutboxStatus.SUCCESS;
    }
}
