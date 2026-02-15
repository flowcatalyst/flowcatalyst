/**
 * Job Dispatcher
 *
 * Dispatches individual dispatch jobs to the external queue.
 * Builds a MessagePointer, serializes it, publishes via QueuePublisher,
 * and updates the job status to QUEUED on success.
 */

import { eq } from 'drizzle-orm';
import { dispatchJobs, type DispatchJobRecord } from '@flowcatalyst/persistence';
import type { QueuePublisher } from '@flowcatalyst/queue-core';
import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';
import type { DispatchSchedulerConfig, SchedulerLogger } from './config.js';

export interface JobDispatcher {
  /** Dispatch a single job to the queue. Returns true on success. */
  dispatch(job: DispatchJobRecord): Promise<boolean>;
}

export function createJobDispatcher(
  config: DispatchSchedulerConfig,
  db: PostgresJsDatabase,
  publisher: QueuePublisher,
  logger: SchedulerLogger,
): JobDispatcher {
  return {
    async dispatch(job) {
      try {
        // Build MessagePointer
        const pointer = {
          id: job.id,
          poolCode: job.dispatchPoolId ?? config.defaultDispatchPoolCode,
          messageGroupId: job.messageGroup ?? 'default',
          mediationType: 'HTTP',
          mediationTarget: config.processingEndpoint,
        };

        const messageBody = JSON.stringify(pointer);

        const result = await publisher.publish({
          messageId: job.id,
          messageGroupId: job.messageGroup ?? 'default',
          messageDeduplicationId: job.id,
          body: messageBody,
        });

        if (result.success) {
          // Update status to QUEUED
          await db
            .update(dispatchJobs)
            .set({ status: 'QUEUED', updatedAt: new Date() })
            .where(eq(dispatchJobs.id, job.id));

          logger.debug({ jobId: job.id }, 'Dispatched job to queue, status updated to QUEUED');
          return true;
        }

        // Check for deduplication (still mark as QUEUED)
        if (result.error?.includes('Deduplicated') || result.error?.includes('deduplicated')) {
          await db
            .update(dispatchJobs)
            .set({ status: 'QUEUED', updatedAt: new Date() })
            .where(eq(dispatchJobs.id, job.id));

          logger.debug({ jobId: job.id }, 'Job was deduplicated (already dispatched)');
          return true;
        }

        logger.warn({ jobId: job.id, error: result.error }, 'Failed to dispatch job');
        return false;
      } catch (err) {
        logger.error({ err, jobId: job.id }, 'Error dispatching job');
        return false;
      }
    },
  };
}
