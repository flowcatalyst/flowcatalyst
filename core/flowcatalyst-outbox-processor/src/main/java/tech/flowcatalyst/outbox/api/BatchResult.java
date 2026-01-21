package tech.flowcatalyst.outbox.api;

import tech.flowcatalyst.outbox.model.OutboxStatus;

import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * Result of a batch API call with per-item status tracking.
 */
public class BatchResult {

    private int successCount;
    private final Map<String, OutboxStatus> failedItems;
    private String errorMessage;

    public BatchResult() {
        this.successCount = 0;
        this.failedItems = new HashMap<>();
        this.errorMessage = null;
    }

    /**
     * Create a successful result for all items.
     */
    public static BatchResult allSuccess(int count) {
        BatchResult result = new BatchResult();
        result.successCount = count;
        return result;
    }

    /**
     * Create a result where all items failed with the same status.
     */
    public static BatchResult allFailed(List<String> ids, OutboxStatus status, String errorMessage) {
        BatchResult result = new BatchResult();
        result.errorMessage = errorMessage;
        for (String id : ids) {
            result.failedItems.put(id, status);
        }
        return result;
    }

    /**
     * Mark specific items as failed with a status.
     */
    public void markFailed(String id, OutboxStatus status) {
        failedItems.put(id, status);
    }

    /**
     * Set success count.
     */
    public void setSuccessCount(int count) {
        this.successCount = count;
    }

    /**
     * Set error message.
     */
    public void setErrorMessage(String errorMessage) {
        this.errorMessage = errorMessage;
    }

    /**
     * Get the number of successful items.
     */
    public int getSuccessCount() {
        return successCount;
    }

    /**
     * Get map of failed item IDs to their status codes.
     */
    public Map<String, OutboxStatus> getFailedItems() {
        return failedItems;
    }

    /**
     * Get the error message (if any).
     */
    public String getErrorMessage() {
        return errorMessage;
    }

    /**
     * Check if all items succeeded.
     */
    public boolean isAllSuccess() {
        return failedItems.isEmpty();
    }

    /**
     * Check if all items failed.
     */
    public boolean isAllFailed() {
        return successCount == 0 && !failedItems.isEmpty();
    }
}
