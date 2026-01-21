package tech.flowcatalyst.outbox.processor;

import org.jboss.logging.Logger;
import tech.flowcatalyst.outbox.api.BatchResult;
import tech.flowcatalyst.outbox.api.FlowCatalystApiClient;
import tech.flowcatalyst.outbox.config.OutboxProcessorConfig;
import tech.flowcatalyst.outbox.model.OutboxItem;
import tech.flowcatalyst.outbox.model.OutboxItemType;
import tech.flowcatalyst.outbox.model.OutboxStatus;
import tech.flowcatalyst.outbox.repository.OutboxRepository;

import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.BlockingQueue;
import java.util.concurrent.LinkedBlockingQueue;
import java.util.concurrent.Semaphore;
import java.util.concurrent.atomic.AtomicBoolean;

/**
 * Processes items for a single message group in FIFO order.
 * Uses virtual threads for efficient async processing.
 * Only one batch is sent at a time per group to maintain ordering.
 *
 * <p>After processing each batch:
 * <ol>
 *   <li>Successful items are marked with status=1 (SUCCESS)</li>
 *   <li>Failed retryable items are reset to status=0 with incremented retry count</li>
 *   <li>Failed terminal items are marked with appropriate error status code</li>
 *   <li>In-flight permits are released back to the poller</li>
 * </ol>
 */
public class MessageGroupProcessor {

    private static final Logger LOG = Logger.getLogger(MessageGroupProcessor.class);

    private final OutboxItemType type;
    private final String messageGroup;
    private final BlockingQueue<OutboxItem> queue = new LinkedBlockingQueue<>();
    private final AtomicBoolean running = new AtomicBoolean(false);
    private final Semaphore globalSemaphore;

    private final OutboxProcessorConfig config;
    private final OutboxRepository repository;
    private final FlowCatalystApiClient apiClient;
    private final OutboxPoller outboxPoller;

    public MessageGroupProcessor(
            OutboxItemType type,
            String messageGroup,
            Semaphore globalSemaphore,
            OutboxProcessorConfig config,
            OutboxRepository repository,
            FlowCatalystApiClient apiClient,
            OutboxPoller outboxPoller) {
        this.type = type;
        this.messageGroup = messageGroup;
        this.globalSemaphore = globalSemaphore;
        this.config = config;
        this.repository = repository;
        this.apiClient = apiClient;
        this.outboxPoller = outboxPoller;
    }

    /**
     * Enqueue an item for processing.
     * Starts the processing loop if not already running.
     */
    public void enqueue(OutboxItem item) {
        queue.offer(item);
        tryStartProcessing();
    }

    /**
     * Get the number of items waiting in this processor's queue.
     */
    public int getQueueSize() {
        return queue.size();
    }

    private void tryStartProcessing() {
        if (running.compareAndSet(false, true)) {
            Thread.startVirtualThread(this::processLoop);
        }
    }

    private void processLoop() {
        try {
            while (!queue.isEmpty()) {
                // Acquire global semaphore to limit concurrent group processing
                globalSemaphore.acquire();
                try {
                    processBatch();
                } finally {
                    globalSemaphore.release();
                }
            }
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            LOG.warnf("MessageGroupProcessor interrupted for group %s:%s", type, messageGroup);
        } finally {
            running.set(false);
            // Check if more items arrived while we were finishing
            if (!queue.isEmpty()) {
                tryStartProcessing();
            }
        }
    }

    private void processBatch() {
        // Drain up to batch size from queue
        List<OutboxItem> batch = new ArrayList<>();
        queue.drainTo(batch, config.apiBatchSize());

        if (batch.isEmpty()) {
            return;
        }

        LOG.debugf("Processing batch of %d %s items for group %s", batch.size(), type, messageGroup);

        try {
            // Call FlowCatalyst API
            BatchResult result;
            if (type == OutboxItemType.EVENT) {
                result = apiClient.createEventsBatch(batch);
            } else {
                result = apiClient.createDispatchJobsBatch(batch);
            }

            // Handle results
            handleBatchResult(batch, result);

        } catch (Exception e) {
            // Unexpected error - treat all items as retriable internal errors
            LOG.errorf(e, "Unexpected error processing batch for group %s:%s", type, messageGroup);
            handleUnexpectedError(batch, e.getMessage());
        } finally {
            // ALWAYS release in-flight permits after processing completes
            outboxPoller.releaseInFlight(batch.size());
        }
    }

    private void handleBatchResult(List<OutboxItem> batch, BatchResult result) {
        if (result.isAllSuccess()) {
            // All items succeeded
            List<String> ids = batch.stream().map(OutboxItem::id).toList();
            repository.markWithStatus(type, ids, OutboxStatus.SUCCESS);
            LOG.debugf("Completed batch of %d %s items for group %s", batch.size(), type, messageGroup);
            return;
        }

        // Some or all items failed - handle based on status codes
        Map<String, OutboxStatus> failedItems = result.getFailedItems();
        String errorMessage = result.getErrorMessage();

        // Separate items by their outcome
        List<String> successIds = new ArrayList<>();
        List<String> retryableIds = new ArrayList<>();
        Map<OutboxStatus, List<String>> terminalByStatus = new HashMap<>();

        for (OutboxItem item : batch) {
            OutboxStatus failedStatus = failedItems.get(item.id());

            if (failedStatus == null) {
                // Item succeeded
                successIds.add(item.id());
            } else if (failedStatus.isRetryable() && item.retryCount() < config.maxRetries()) {
                // Retryable error and not exhausted retries
                retryableIds.add(item.id());
            } else {
                // Terminal error or exhausted retries
                OutboxStatus terminalStatus = failedStatus.isRetryable() ? OutboxStatus.INTERNAL_ERROR : failedStatus;
                terminalByStatus.computeIfAbsent(terminalStatus, k -> new ArrayList<>()).add(item.id());
            }
        }

        // Update database for each category
        if (!successIds.isEmpty()) {
            repository.markWithStatus(type, successIds, OutboxStatus.SUCCESS);
            LOG.debugf("Marked %d items as SUCCESS in group %s:%s", successIds.size(), type, messageGroup);
        }

        if (!retryableIds.isEmpty()) {
            repository.incrementRetryCount(type, retryableIds);
            LOG.infof("Scheduled %d items for retry in group %s:%s", retryableIds.size(), type, messageGroup);
        }

        for (Map.Entry<OutboxStatus, List<String>> entry : terminalByStatus.entrySet()) {
            OutboxStatus status = entry.getKey();
            List<String> ids = entry.getValue();
            repository.markWithStatusAndError(type, ids, status, errorMessage);
            LOG.warnf("Marked %d items with terminal status %s in group %s:%s", ids.size(), status, type, messageGroup);
        }
    }

    private void handleUnexpectedError(List<OutboxItem> batch, String errorMessage) {
        // Separate items that can be retried from those that have exhausted retries
        List<String> retryable = batch.stream()
            .filter(item -> item.retryCount() < config.maxRetries())
            .map(OutboxItem::id)
            .toList();

        List<String> exhausted = batch.stream()
            .filter(item -> item.retryCount() >= config.maxRetries())
            .map(OutboxItem::id)
            .toList();

        if (!retryable.isEmpty()) {
            repository.incrementRetryCount(type, retryable);
            LOG.infof("Scheduled %d items for retry in group %s:%s", retryable.size(), type, messageGroup);
        }

        if (!exhausted.isEmpty()) {
            repository.markWithStatusAndError(type, exhausted, OutboxStatus.INTERNAL_ERROR, errorMessage);
            LOG.warnf("Marked %d items as INTERNAL_ERROR in group %s:%s (max retries exceeded)", exhausted.size(), type, messageGroup);
        }
    }
}
