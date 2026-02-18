/**
 * Sync Subscriptions Use Case
 *
 * Bulk create/update/delete anchor-level API-sourced subscriptions from SDK.
 * Only updates/deletes API-sourced subscriptions (UI-sourced are never touched).
 */

import type { UseCase } from '@flowcatalyst/application';
import { Result, UseCaseError } from '@flowcatalyst/application';
import type { ExecutionContext, UnitOfWork } from '@flowcatalyst/domain-core';
import type { TransactionContext } from '@flowcatalyst/persistence';

import type {
  SubscriptionRepository,
  DispatchPoolRepository,
} from '../../../infrastructure/persistence/index.js';
import {
  createSubscription,
  updateSubscription,
  SubscriptionsSynced,
} from '../../../domain/index.js';

import type { SyncSubscriptionsCommand } from './command.js';

export interface SyncSubscriptionsUseCaseDeps {
  readonly subscriptionRepository: SubscriptionRepository;
  readonly dispatchPoolRepository: DispatchPoolRepository;
  readonly unitOfWork: UnitOfWork;
}

export function createSyncSubscriptionsUseCase(
  deps: SyncSubscriptionsUseCaseDeps,
): UseCase<SyncSubscriptionsCommand, SubscriptionsSynced> {
  const { subscriptionRepository, dispatchPoolRepository, unitOfWork } = deps;

  return {
    async execute(
      command: SyncSubscriptionsCommand,
      context: ExecutionContext,
    ): Promise<Result<SubscriptionsSynced>> {
      if (!command.subscriptions || command.subscriptions.length === 0) {
        return Result.failure(
          UseCaseError.validation(
            'SUBSCRIPTIONS_REQUIRED',
            'At least one subscription must be provided',
          ),
        );
      }

      // Validate all items
      const seenCodes = new Set<string>();
      for (const item of command.subscriptions) {
        if (!item.code || !item.code.trim()) {
          return Result.failure(
            UseCaseError.validation('CODE_REQUIRED', 'Subscription code is required'),
          );
        }
        if (!item.name || !item.name.trim()) {
          return Result.failure(
            UseCaseError.validation('NAME_REQUIRED', 'Subscription name is required', {
              code: item.code,
            }),
          );
        }
        if (!item.target || !item.target.trim()) {
          return Result.failure(
            UseCaseError.validation('TARGET_REQUIRED', 'Subscription target is required', {
              code: item.code,
            }),
          );
        }
        if (!item.eventTypes || item.eventTypes.length === 0) {
          return Result.failure(
            UseCaseError.validation('EVENT_TYPES_REQUIRED', 'At least one event type is required', {
              code: item.code,
            }),
          );
        }
        if (seenCodes.has(item.code)) {
          return Result.failure(
            UseCaseError.validation(
              'DUPLICATE_CODE',
              'Duplicate subscription code in sync request',
              {
                code: item.code,
              },
            ),
          );
        }
        seenCodes.add(item.code);
      }

      let created = 0;
      let updated = 0;
      let deleted = 0;
      const syncedCodes: string[] = [];

      const eventData = {
        applicationCode: command.applicationCode,
        subscriptionsCreated: 0,
        subscriptionsUpdated: 0,
        subscriptionsDeleted: 0,
        syncedSubscriptionCodes: [] as string[],
      };

      const event = new SubscriptionsSynced(context, eventData);

      return unitOfWork.commitOperations(event, command, async (tx) => {
        const txCtx = tx as TransactionContext;

        // Process each subscription (anchor-level: clientId = null, source = API)
        for (const item of command.subscriptions) {
          const existing = await subscriptionRepository.findByCodeAndClient(item.code, null, txCtx);

          // Resolve dispatch pool by code if provided
          let dispatchPoolId: string | null = null;
          const dispatchPoolCode: string | null = item.dispatchPoolCode ?? null;
          if (dispatchPoolCode) {
            const pool = await dispatchPoolRepository.findByCodeAndClientId(
              dispatchPoolCode,
              null,
              txCtx,
            );
            if (pool) {
              dispatchPoolId = pool.id;
            }
          }

          if (existing) {
            // Only update API-sourced subscriptions
            if (existing.source !== 'API') {
              syncedCodes.push(item.code);
              continue;
            }

            const updatedSub = updateSubscription(existing, {
              name: item.name,
              description: item.description ?? null,
              eventTypes: item.eventTypes,
              target: item.target,
              queue: item.queue ?? null,
              customConfig: item.customConfig ?? [],
              maxAgeSeconds: item.maxAgeSeconds ?? 86400,
              dispatchPoolId,
              dispatchPoolCode,
              delaySeconds: item.delaySeconds ?? 0,
              sequence: item.sequence ?? 99,
              mode: item.mode ?? 'IMMEDIATE',
              timeoutSeconds: item.timeoutSeconds ?? 30,
              maxRetries: item.maxRetries ?? 3,
              dataOnly: item.dataOnly ?? true,
              status: 'ACTIVE',
            });

            await subscriptionRepository.update(updatedSub, txCtx);
            updated++;
          } else {
            // Create new
            const sub = createSubscription({
              code: item.code,
              applicationCode: command.applicationCode,
              name: item.name,
              description: item.description ?? null,
              clientScoped: item.clientScoped ?? false,
              eventTypes: item.eventTypes,
              target: item.target,
              queue: item.queue ?? null,
              customConfig: item.customConfig ?? [],
              source: 'API',
              maxAgeSeconds: item.maxAgeSeconds ?? 86400,
              dispatchPoolId,
              dispatchPoolCode,
              delaySeconds: item.delaySeconds ?? 0,
              sequence: item.sequence ?? 99,
              mode: item.mode ?? 'IMMEDIATE',
              timeoutSeconds: item.timeoutSeconds ?? 30,
              maxRetries: item.maxRetries ?? 3,
              dataOnly: item.dataOnly ?? true,
            });

            await subscriptionRepository.insert(sub, txCtx);
            created++;
          }

          syncedCodes.push(item.code);
        }

        // Remove unlisted API-sourced subscriptions if requested
        if (command.removeUnlisted) {
          const anchorSubs = await subscriptionRepository.findAnchorLevel(txCtx);
          for (const sub of anchorSubs) {
            if (sub.source === 'API' && !seenCodes.has(sub.code)) {
              await subscriptionRepository.deleteById(sub.id, txCtx);
              deleted++;
            }
          }
        }

        // Update event data with final counts (mutate the same object reference held by the event)
        eventData.subscriptionsCreated = created;
        eventData.subscriptionsUpdated = updated;
        eventData.subscriptionsDeleted = deleted;
        eventData.syncedSubscriptionCodes = syncedCodes;
      });
    },
  };
}
