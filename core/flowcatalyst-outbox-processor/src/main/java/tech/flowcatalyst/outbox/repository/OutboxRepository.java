package tech.flowcatalyst.outbox.repository;

import tech.flowcatalyst.outbox.model.OutboxItem;
import tech.flowcatalyst.outbox.model.OutboxItemType;
import tech.flowcatalyst.outbox.model.OutboxStatus;

import java.util.List;

/**
 * Repository interface for outbox operations.
 * Uses a single-poller, status-based pattern with NO row locking.
 * This works identically across PostgreSQL, MySQL, and MongoDB.
 *
 * <p>The architecture assumes only one poller runs at a time (enforced by leader election),
 * so there's no need for FOR UPDATE SKIP LOCKED or atomic findAndModify operations.
 */
public interface OutboxRepository {

    /**
     * Fetch pending items (status=0) ordered by messageGroup, createdAt.
     * Does NOT lock or modify the items - caller must call markAsInProgress after.
     * This is safe because only one poller runs (enforced by leader election).
     *
     * @param type  The type of items to fetch (EVENT or DISPATCH_JOB)
     * @param limit Maximum number of items to fetch
     * @return List of pending items
     */
    List<OutboxItem> fetchPending(OutboxItemType type, int limit);

    /**
     * Mark items as in-progress (status=9).
     * Must be called immediately after fetchPending, before distributing to queues.
     * This prevents re-polling of items that are being processed.
     *
     * @param type The type of items
     * @param ids  List of item IDs to mark as in-progress
     */
    void markAsInProgress(OutboxItemType type, List<String> ids);

    /**
     * Update items to the specified status code.
     * Used for both success (status=1) and various error types (status=2-6).
     *
     * @param type   The type of items
     * @param ids    List of item IDs to update
     * @param status The new status
     */
    void markWithStatus(OutboxItemType type, List<String> ids, OutboxStatus status);

    /**
     * Update items to the specified status with an error message.
     *
     * @param type         The type of items
     * @param ids          List of item IDs to update
     * @param status       The new status
     * @param errorMessage The error message to store
     */
    void markWithStatusAndError(OutboxItemType type, List<String> ids, OutboxStatus status, String errorMessage);

    /**
     * Fetch items stuck in in-progress status (status=9).
     * Used on startup for crash recovery.
     *
     * @param type The type of items
     * @return List of stuck items
     */
    List<OutboxItem> fetchStuckItems(OutboxItemType type);

    /**
     * Reset stuck items back to pending (status=0).
     * Used on startup for crash recovery.
     *
     * @param type The type of items
     * @param ids  List of item IDs to reset
     */
    void resetStuckItems(OutboxItemType type, List<String> ids);

    /**
     * Increment the retry count for items and reset to pending.
     * Used when an item fails but should be retried.
     *
     * @param type The type of items
     * @param ids  List of item IDs to increment retry count
     */
    void incrementRetryCount(OutboxItemType type, List<String> ids);

    /**
     * Fetch items eligible for periodic recovery.
     * Returns items with error statuses (IN_PROGRESS, BAD_REQUEST, INTERNAL_ERROR,
     * UNAUTHORIZED, FORBIDDEN, GATEWAY_ERROR) that have been in that status
     * longer than the specified timeout.
     *
     * @param type           The type of items to check
     * @param timeoutSeconds Items with updatedAt older than this many seconds ago
     * @param limit          Maximum number of items to fetch
     * @return List of recoverable items
     */
    List<OutboxItem> fetchRecoverableItems(OutboxItemType type, int timeoutSeconds, int limit);

    /**
     * Reset recoverable items back to PENDING status for retry.
     * Does NOT reset retry count - items will be retried with existing count.
     *
     * @param type The type of items
     * @param ids  List of item IDs to reset
     */
    void resetRecoverableItems(OutboxItemType type, List<String> ids);

    /**
     * Count pending items (for metrics).
     *
     * @param type The type of items
     * @return Count of pending items
     */
    long countPending(OutboxItemType type);

    /**
     * Get the table/collection name for the item type.
     *
     * @param type The type of items
     * @return Table/collection name
     */
    String getTableName(OutboxItemType type);

    /**
     * Create the outbox tables/collections if they don't exist.
     */
    void createSchema();
}
