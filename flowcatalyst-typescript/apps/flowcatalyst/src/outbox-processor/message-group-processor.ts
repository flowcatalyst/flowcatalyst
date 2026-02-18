/**
 * Message Group Processor
 *
 * Processes items for a single message group in FIFO order.
 * Only one batch is processed at a time per group to maintain ordering.
 *
 * After processing each batch:
 * 1. Successful items are marked with status=1 (SUCCESS)
 * 2. Failed retryable items are reset to status=0 with incremented retry count
 * 3. Failed terminal items are marked with appropriate error status code
 * 4. In-flight permits are released back to the poller
 */

import { OutboxStatus, type OutboxItem, type OutboxItemType } from './model.js';
import type { OutboxRepository } from './repository/outbox-repository.js';
import type { ApiClient, BatchResult } from './api-client.js';
import type { OutboxProcessorConfig } from './env.js';
import type { Logger } from 'pino';

export interface MessageGroupProcessor {
  /** Enqueue an item for processing. */
  enqueue(item: OutboxItem): void;
  /** Get the number of items waiting in this processor's queue. */
  getQueueSize(): number;
}

export function createMessageGroupProcessor(
  type: OutboxItemType,
  messageGroup: string,
  config: OutboxProcessorConfig,
  repository: OutboxRepository,
  apiClient: ApiClient,
  releaseInFlight: (count: number) => void,
  acquireConcurrency: () => Promise<void>,
  releaseConcurrency: () => void,
  logger: Logger,
): MessageGroupProcessor {
  const queue: OutboxItem[] = [];
  let processing = false;

  function enqueue(item: OutboxItem): void {
    queue.push(item);
    tryStartProcessing();
  }

  function tryStartProcessing(): void {
    if (processing) return;
    if (queue.length === 0) return;
    processing = true;
    processLoop().catch((err) => {
      logger.error({ err, type, messageGroup }, 'Unexpected error in process loop');
    });
  }

  async function processLoop(): Promise<void> {
    try {
      while (queue.length > 0) {
        await acquireConcurrency();
        try {
          await processBatch();
        } finally {
          releaseConcurrency();
        }
      }
    } finally {
      processing = false;
      // Check if more items arrived while we were finishing
      if (queue.length > 0) {
        tryStartProcessing();
      }
    }
  }

  async function processBatch(): Promise<void> {
    // Drain up to batch size from queue
    const batch = queue.splice(0, config.apiBatchSize);
    if (batch.length === 0) return;

    logger.debug({ count: batch.length, type, messageGroup }, 'Processing batch');

    try {
      // Call FlowCatalyst API
      let result: BatchResult;
      switch (type) {
        case 'EVENT':
          result = await apiClient.createEventsBatch(batch);
          break;
        case 'DISPATCH_JOB':
          result = await apiClient.createDispatchJobsBatch(batch);
          break;
        case 'AUDIT_LOG':
          result = await apiClient.createAuditLogsBatch(batch);
          break;
      }

      await handleBatchResult(batch, result);
    } catch (err) {
      // Unexpected error — treat all items as retriable internal errors
      const message = err instanceof Error ? err.message : String(err);
      logger.error({ err, type, messageGroup }, 'Unexpected error processing batch');
      await handleUnexpectedError(batch, message);
    } finally {
      // ALWAYS release in-flight permits after processing completes
      releaseInFlight(batch.length);
    }
  }

  async function handleBatchResult(batch: OutboxItem[], result: BatchResult): Promise<void> {
    if (result.allSuccess) {
      const ids = batch.map((item) => String(item.id));
      await repository.markWithStatus(type, ids, OutboxStatus.SUCCESS);
      logger.debug({ count: batch.length, type, messageGroup }, 'Completed batch');
      return;
    }

    // Some or all items failed — handle based on status codes
    const successIds: string[] = [];
    const retryableIds: string[] = [];
    const terminalByStatus = new Map<OutboxStatus, string[]>();

    for (const item of batch) {
      const id = String(item.id);
      const failedStatus = result.failedItems.get(id);

      if (failedStatus === undefined) {
        successIds.push(id);
      } else if (
        (failedStatus === OutboxStatus.INTERNAL_ERROR ||
          failedStatus === OutboxStatus.GATEWAY_ERROR ||
          failedStatus === OutboxStatus.UNAUTHORIZED) &&
        item.retryCount < config.maxRetries
      ) {
        retryableIds.push(id);
      } else {
        const terminalStatus =
          failedStatus === OutboxStatus.INTERNAL_ERROR ||
          failedStatus === OutboxStatus.GATEWAY_ERROR
            ? OutboxStatus.INTERNAL_ERROR
            : failedStatus;
        const existing = terminalByStatus.get(terminalStatus) ?? [];
        existing.push(id);
        terminalByStatus.set(terminalStatus, existing);
      }
    }

    if (successIds.length > 0) {
      await repository.markWithStatus(type, successIds, OutboxStatus.SUCCESS);
    }

    if (retryableIds.length > 0) {
      await repository.incrementRetryCount(type, retryableIds);
      logger.info({ count: retryableIds.length, type, messageGroup }, 'Scheduled items for retry');
    }

    for (const [status, ids] of terminalByStatus) {
      await repository.markWithStatusAndError(
        type,
        ids,
        status,
        result.errorMessage ?? 'Unknown error',
      );
      logger.warn(
        { count: ids.length, status, type, messageGroup },
        'Marked items with terminal status',
      );
    }
  }

  async function handleUnexpectedError(batch: OutboxItem[], errorMessage: string): Promise<void> {
    const retryable = batch
      .filter((item) => item.retryCount < config.maxRetries)
      .map((item) => String(item.id));

    const exhausted = batch
      .filter((item) => item.retryCount >= config.maxRetries)
      .map((item) => String(item.id));

    if (retryable.length > 0) {
      await repository.incrementRetryCount(type, retryable);
      logger.info({ count: retryable.length, type, messageGroup }, 'Scheduled items for retry');
    }

    if (exhausted.length > 0) {
      await repository.markWithStatusAndError(
        type,
        exhausted,
        OutboxStatus.INTERNAL_ERROR,
        errorMessage,
      );
      logger.warn(
        { count: exhausted.length, type, messageGroup },
        'Marked items as INTERNAL_ERROR (max retries exceeded)',
      );
    }
  }

  return {
    enqueue,
    getQueueSize: () => queue.length,
  };
}
