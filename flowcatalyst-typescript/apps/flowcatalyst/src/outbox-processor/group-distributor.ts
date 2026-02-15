/**
 * Group Distributor
 *
 * Routes outbox items to the appropriate MessageGroupProcessor
 * based on type and message group. Maintains a map of active processors
 * and a concurrency limiter for controlling max concurrent groups.
 */

import type { OutboxItem, OutboxItemType } from './model.js';
import { createMessageGroupProcessor, type MessageGroupProcessor } from './message-group-processor.js';
import type { OutboxRepository } from './repository/outbox-repository.js';
import type { ApiClient } from './api-client.js';
import type { OutboxProcessorConfig } from './env.js';
import type { Logger } from 'pino';

export interface GroupDistributor {
  /** Distribute an item to its message group processor. */
  distribute(item: OutboxItem): void;
  /** Get the number of active message group processors. */
  getActiveProcessorCount(): number;
}

export function createGroupDistributor(
  config: OutboxProcessorConfig,
  repository: OutboxRepository,
  apiClient: ApiClient,
  releaseInFlight: (count: number) => void,
  logger: Logger,
): GroupDistributor {
  const processors = new Map<string, MessageGroupProcessor>();

  // Simple semaphore: counter + waiting queue
  let activeConcurrency = 0;
  const waitingQueue: Array<() => void> = [];

  function acquireConcurrency(): Promise<void> {
    if (activeConcurrency < config.maxConcurrentGroups) {
      activeConcurrency++;
      return Promise.resolve();
    }
    return new Promise<void>((resolve) => {
      waitingQueue.push(() => {
        activeConcurrency++;
        resolve();
      });
    });
  }

  function releaseConcurrency(): void {
    activeConcurrency--;
    const next = waitingQueue.shift();
    if (next) {
      next();
    }
  }

  function distribute(item: OutboxItem): void {
    const groupKey = `${item.type}:${item.messageGroup ?? 'default'}`;

    let processor = processors.get(groupKey);
    if (!processor) {
      logger.debug({ groupKey }, 'Creating new processor for group');
      processor = createMessageGroupProcessor(
        item.type,
        item.messageGroup ?? 'default',
        config,
        repository,
        apiClient,
        releaseInFlight,
        acquireConcurrency,
        releaseConcurrency,
        logger,
      );
      processors.set(groupKey, processor);
    }

    processor.enqueue(item);
  }

  return {
    distribute,
    getActiveProcessorCount: () => processors.size,
  };
}
