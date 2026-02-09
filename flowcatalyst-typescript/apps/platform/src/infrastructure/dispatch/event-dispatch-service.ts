/**
 * Event Dispatch Service
 *
 * Builds dispatch jobs for events within the UoW transaction.
 * When a domain event is persisted, this service finds matching active subscriptions
 * and creates dispatch jobs + outbox entries in the same transaction.
 */

import type { DomainEvent } from '@flowcatalyst/domain-core';
import { generateRaw } from '@flowcatalyst/tsid';
import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';
import { dispatchJobs, dispatchJobProjectionFeed } from '@flowcatalyst/persistence';

import type { SubscriptionRepository } from '../persistence/repositories/subscription-repository.js';
import type { Subscription } from '../../domain/index.js';

/**
 * Event dispatch service interface for use in UnitOfWork.
 */
export interface EventDispatchService {
  buildDispatchJobsForEvent(
    event: DomainEvent,
    clientId: string | null,
    db: PostgresJsDatabase,
  ): Promise<void>;
}

/**
 * Dependencies for creating the EventDispatchService.
 */
export interface EventDispatchServiceDeps {
  readonly subscriptionRepository: SubscriptionRepository;
}

/**
 * Create an EventDispatchService.
 */
export function createEventDispatchService(deps: EventDispatchServiceDeps): EventDispatchService {
  const { subscriptionRepository } = deps;

  return {
    async buildDispatchJobsForEvent(
      event: DomainEvent,
      clientId: string | null,
      db: PostgresJsDatabase,
    ): Promise<void> {
      // Find active subscriptions matching this event type code and client scope
      const matchingSubs = await subscriptionRepository.findActiveByEventTypeCode(
        event.eventType,
        clientId,
      );

      if (matchingSubs.length === 0) return;

      const now = new Date();

      for (const sub of matchingSubs) {
        const jobId = generateRaw();
        const idempotencyKey = `${event.eventId}:${sub.id}`;
        const messageGroup = `${sub.code}:${event.messageGroup}`;

        const jobRecord = {
          id: jobId,
          kind: 'EVENT' as const,
          code: event.eventType,
          source: event.source,
          subject: event.subject,
          eventId: event.eventId,
          correlationId: event.correlationId,
          targetUrl: sub.target,
          protocol: 'HTTP_WEBHOOK' as const,
          dataOnly: sub.dataOnly,
          serviceAccountId: sub.serviceAccountId,
          clientId: clientId,
          subscriptionId: sub.id,
          mode: sub.mode,
          dispatchPoolId: sub.dispatchPoolId,
          messageGroup,
          sequence: sub.sequence,
          timeoutSeconds: sub.timeoutSeconds,
          status: 'QUEUED' as const,
          maxRetries: sub.maxRetries,
          attemptCount: 0,
          idempotencyKey,
          createdAt: now,
          updatedAt: now,
        };

        // Insert dispatch job
        await db.insert(dispatchJobs).values(jobRecord);

        // Write to dispatch job projection feed for stream-processor projection
        await db.insert(dispatchJobProjectionFeed).values({
          dispatchJobId: jobId,
          operation: 'INSERT',
          payload: jobRecord,
        });
      }
    },
  };
}
