package tech.flowcatalyst.messagerouter.pool;

import org.jboss.logging.Logger;
import tech.flowcatalyst.messagerouter.callback.MessageCallback;
import tech.flowcatalyst.messagerouter.callback.MessageVisibilityControl;
import tech.flowcatalyst.messagerouter.mediator.MediationError;
import tech.flowcatalyst.messagerouter.metrics.PoolMetricsService;
import tech.flowcatalyst.messagerouter.model.MediationOutcome;
import tech.flowcatalyst.messagerouter.model.MediationResult;
import tech.flowcatalyst.messagerouter.model.MessagePointer;
import tech.flowcatalyst.messagerouter.warning.WarningService;

import java.util.Optional;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicInteger;

/**
 * Handles mediation outcomes and manages batch+group FIFO ordering.
 *
 * <p>This class extracts the duplicate outcome handling logic from ProcessPoolImpl,
 * providing a single place for:
 * <ul>
 *   <li>ACK/NACK decisions based on outcome type</li>
 *   <li>Visibility timeout management</li>
 *   <li>Batch+group failure marking for FIFO enforcement</li>
 *   <li>Metrics recording</li>
 * </ul>
 *
 * <h2>FIFO Enforcement</h2>
 * <p>When a message fails within a batch+group, all subsequent messages in that
 * batch+group must also fail to preserve ordering. This class tracks failed
 * batch+groups and provides methods to check and mark failures.
 */
public class OutcomeHandler {

    private static final Logger LOG = Logger.getLogger(OutcomeHandler.class);
    private static final String DEFAULT_GROUP = "__DEFAULT__";

    private final String poolCode;
    private final MessageCallback messageCallback;
    private final PoolMetricsService poolMetrics;
    private final WarningService warningService;

    // Batch+Group FIFO tracking
    private final ConcurrentHashMap<String, Boolean> failedBatchGroups;
    private final ConcurrentHashMap<String, AtomicInteger> batchGroupMessageCount;

    /**
     * Creates an OutcomeHandler for a specific pool.
     *
     * @param poolCode the pool code for metrics/logging
     * @param messageCallback the callback for ack/nack operations
     * @param poolMetrics the metrics service
     * @param warningService the warning service
     * @param failedBatchGroups map tracking failed batch+groups
     * @param batchGroupMessageCount map tracking message counts per batch+group
     */
    public OutcomeHandler(
            String poolCode,
            MessageCallback messageCallback,
            PoolMetricsService poolMetrics,
            WarningService warningService,
            ConcurrentHashMap<String, Boolean> failedBatchGroups,
            ConcurrentHashMap<String, AtomicInteger> batchGroupMessageCount) {
        this.poolCode = poolCode;
        this.messageCallback = messageCallback;
        this.poolMetrics = poolMetrics;
        this.warningService = warningService;
        this.failedBatchGroups = failedBatchGroups;
        this.batchGroupMessageCount = batchGroupMessageCount;
    }

    /**
     * Handle a mediation outcome and perform appropriate ack/nack.
     *
     * @param message the message that was processed
     * @param outcome the mediation outcome
     * @param durationMs processing duration in milliseconds
     */
    public void handleOutcome(MessagePointer message, MediationOutcome outcome, long durationMs) {
        // Defensive null check
        if (outcome == null || outcome.result() == null) {
            LOG.errorf("CRITICAL: Mediator returned null outcome/result for message [%s], treating as transient error",
                message.id());
            outcome = MediationOutcome.errorProcess((Integer) null);
            warningService.addWarning(
                "MEDIATOR_NULL_RESULT",
                "CRITICAL",
                "Mediator returned null outcome for message " + message.id(),
                "ProcessPool:" + poolCode
            );
        }

        MediationResult result = outcome.result();
        String batchGroupKey = getBatchGroupKey(message);

        switch (result) {
            case SUCCESS -> handleSuccess(message, durationMs, batchGroupKey);
            case ERROR_CONFIG -> handleConfigError(message, durationMs, batchGroupKey);
            case ERROR_PROCESS -> handleProcessError(message, outcome, durationMs, batchGroupKey);
            case ERROR_CONNECTION -> handleConnectionError(message, durationMs, batchGroupKey);
            default -> handleUnknownResult(message, result, durationMs, batchGroupKey);
        }
    }

    /**
     * Handle successful processing - ACK and cleanup.
     */
    private void handleSuccess(MessagePointer message, long durationMs, String batchGroupKey) {
        LOG.infof("Message [%s] processed successfully - ACKing and removing from queue", message.id());
        poolMetrics.recordProcessingSuccess(poolCode, durationMs);
        messageCallback.ack(message);
        decrementAndCleanupBatchGroup(batchGroupKey);
    }

    /**
     * Handle configuration error - ACK to prevent infinite retries.
     */
    private void handleConfigError(MessagePointer message, long durationMs, String batchGroupKey) {
        LOG.warnf("Message [%s] configuration error - ACKing to prevent retry", message.id());
        poolMetrics.recordProcessingFailure(poolCode, durationMs, "ERROR_CONFIG");
        messageCallback.ack(message);
        decrementAndCleanupBatchGroup(batchGroupKey);
    }

    /**
     * Handle processing error (transient) - NACK for retry.
     */
    private void handleProcessError(MessagePointer message, MediationOutcome outcome,
                                    long durationMs, String batchGroupKey) {
        // Record as transient (may succeed on retry)
        poolMetrics.recordProcessingTransient(poolCode, durationMs);

        // Set visibility timeout based on delay from outcome
        setVisibilityTimeout(message, outcome);

        if (outcome.hasCustomDelay()) {
            LOG.warnf("Message [%s] encountered transient error with custom delay=%ds - NACKing for delayed retry",
                message.id(), outcome.getEffectiveDelaySeconds());
        } else {
            LOG.warnf("Message [%s] encountered transient error - NACKing for retry", message.id());
        }

        nackAndMarkBatchGroupFailed(message, batchGroupKey);
    }

    /**
     * Handle connection error - NACK for retry.
     */
    private void handleConnectionError(MessagePointer message, long durationMs, String batchGroupKey) {
        LOG.warnf("Message [%s] connection error - NACKing for retry", message.id());
        poolMetrics.recordProcessingFailure(poolCode, durationMs, "ERROR_CONNECTION");

        // Reset visibility to default for connection errors
        resetVisibilityToDefault(message);
        nackAndMarkBatchGroupFailed(message, batchGroupKey);
    }

    /**
     * Handle unknown result type - treat as transient error.
     */
    private void handleUnknownResult(MessagePointer message, MediationResult result,
                                     long durationMs, String batchGroupKey) {
        LOG.warnf("Message [%s] unexpected result: %s - NACKing for retry", message.id(), result);
        poolMetrics.recordProcessingFailure(poolCode, durationMs, result.name());

        resetVisibilityToDefault(message);
        nackAndMarkBatchGroupFailed(message, batchGroupKey);
    }

    /**
     * NACK a message and mark its batch+group as failed.
     * This is the common failure handling logic extracted from multiple places.
     */
    private void nackAndMarkBatchGroupFailed(MessagePointer message, String batchGroupKey) {
        messageCallback.nack(message);

        if (batchGroupKey != null) {
            boolean wasAlreadyFailed = failedBatchGroups.putIfAbsent(batchGroupKey, Boolean.TRUE) != null;
            if (!wasAlreadyFailed) {
                LOG.warnf("Batch+group [%s] marked as failed - all remaining messages in this batch+group will be nacked",
                    batchGroupKey);
            }
            decrementAndCleanupBatchGroup(batchGroupKey);
        }
    }

    /**
     * Check if a message should be auto-nacked due to prior failure in its batch+group.
     *
     * @param message the message to check
     * @return true if this message should be nacked without processing
     */
    public boolean shouldAutoNack(MessagePointer message) {
        String batchGroupKey = getBatchGroupKey(message);
        return batchGroupKey != null && failedBatchGroups.containsKey(batchGroupKey);
    }

    /**
     * Handle auto-nack for a message due to prior batch+group failure.
     *
     * @param message the message to auto-nack
     */
    public void handleAutoNack(MessagePointer message) {
        String batchGroupKey = getBatchGroupKey(message);
        LOG.warnf("Message [%s] from failed batch+group [%s], nacking to preserve FIFO ordering",
            message.id(), batchGroupKey);

        // Set fast-fail visibility for quicker retry
        setFastFailVisibility(message);
        messageCallback.nack(message);

        // Record as processing failure
        poolMetrics.recordProcessingFailure(poolCode, 0, "BATCH_GROUP_FAILED");
        decrementAndCleanupBatchGroup(batchGroupKey);
    }

    /**
     * Track a message for batch+group counting.
     *
     * @param message the message to track
     */
    public void trackBatchGroupMessage(MessagePointer message) {
        String batchGroupKey = getBatchGroupKey(message);
        if (batchGroupKey != null) {
            batchGroupMessageCount.computeIfAbsent(batchGroupKey, k -> new AtomicInteger(0))
                .incrementAndGet();
            LOG.debugf("Tracking message [%s] in batch+group [%s], count incremented",
                message.id(), batchGroupKey);
        }
    }

    /**
     * Decrement batch+group count and cleanup if all messages processed.
     */
    public void decrementAndCleanupBatchGroup(String batchGroupKey) {
        if (batchGroupKey == null) {
            return;
        }

        AtomicInteger counter = batchGroupMessageCount.get(batchGroupKey);
        if (counter != null) {
            int remaining = counter.decrementAndGet();
            LOG.debugf("Batch+group [%s] count decremented, remaining: %d", batchGroupKey, remaining);

            if (remaining <= 0) {
                batchGroupMessageCount.remove(batchGroupKey);
                failedBatchGroups.remove(batchGroupKey);
                LOG.debugf("Batch+group [%s] fully processed, cleaned up tracking maps", batchGroupKey);
            }
        }
    }

    /**
     * Get the batch+group key for a message.
     *
     * @param message the message
     * @return the batch+group key, or null if not in a batch
     */
    public static String getBatchGroupKey(MessagePointer message) {
        String batchId = message.batchId();
        if (batchId == null || batchId.isBlank()) {
            return null;
        }
        String messageGroupId = message.messageGroupId() != null ? message.messageGroupId() : DEFAULT_GROUP;
        return batchId + "|" + messageGroupId;
    }

    /**
     * Set visibility timeout based on outcome delay.
     */
    private void setVisibilityTimeout(MessagePointer message, MediationOutcome outcome) {
        if (messageCallback instanceof MessageVisibilityControl visibilityControl) {
            if (outcome.hasCustomDelay()) {
                visibilityControl.setVisibilityDelay(message, outcome.getEffectiveDelaySeconds());
            } else {
                visibilityControl.resetVisibilityToDefault(message);
            }
        }
    }

    /**
     * Reset visibility to default (for connection errors and unknown results).
     */
    private void resetVisibilityToDefault(MessagePointer message) {
        if (messageCallback instanceof MessageVisibilityControl visibilityControl) {
            visibilityControl.resetVisibilityToDefault(message);
        }
    }

    /**
     * Set fast-fail visibility for batch+group failures (10s for quick retry).
     */
    private void setFastFailVisibility(MessagePointer message) {
        if (messageCallback instanceof MessageVisibilityControl visibilityControl) {
            visibilityControl.setFastFailVisibility(message);
        }
    }

    /**
     * Get an optional error from the outcome for detailed error handling.
     *
     * @param outcome the mediation outcome
     * @return the error if present
     */
    public static Optional<MediationError> getError(MediationOutcome outcome) {
        return outcome.getError();
    }
}
