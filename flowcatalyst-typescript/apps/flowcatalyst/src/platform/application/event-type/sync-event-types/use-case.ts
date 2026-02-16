/**
 * Sync EventTypes Use Case
 *
 * Syncs event types from an application SDK. Creates new ones, updates existing
 * API-sourced ones, and optionally removes unlisted API-sourced ones.
 * UI-sourced event types are never modified.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result } from '@flowcatalyst/application';
import type { ExecutionContext, UnitOfWork } from '@flowcatalyst/domain-core';
import type { TransactionContext } from '@flowcatalyst/persistence';

import type { EventTypeRepository } from '../../../infrastructure/persistence/index.js';
import {
  buildCode,
  createEventTypeFromApi,
  updateEventType,
  EventTypesSynced,
} from '../../../domain/index.js';

import type { SyncEventTypesCommand } from './command.js';

export interface SyncEventTypesUseCaseDeps {
  readonly eventTypeRepository: EventTypeRepository;
  readonly unitOfWork: UnitOfWork;
}

export function createSyncEventTypesUseCase(
  deps: SyncEventTypesUseCaseDeps,
): UseCase<SyncEventTypesCommand, EventTypesSynced> {
  const { eventTypeRepository, unitOfWork } = deps;

  return {
    async execute(
      command: SyncEventTypesCommand,
      context: ExecutionContext,
    ): Promise<Result<EventTypesSynced>> {
      const appResult = validateRequired(
        command.applicationCode,
        'applicationCode',
        'APPLICATION_CODE_REQUIRED',
      );
      if (Result.isFailure(appResult)) return appResult;

      let created = 0;
      let updated = 0;
      let deleted = 0;
      const syncedCodes: string[] = [];

      const eventData = {
        applicationCode: command.applicationCode,
        eventTypesCreated: 0,
        eventTypesUpdated: 0,
        eventTypesDeleted: 0,
        syncedEventTypeCodes: [] as string[],
      };

      const event = new EventTypesSynced(context, eventData);

      return unitOfWork.commitOperations(event, command, async (tx) => {
        const txCtx = tx as TransactionContext;

        // Process each event type item
        for (const item of command.eventTypes) {
          const code = buildCode(
            command.applicationCode,
            item.subdomain,
            item.aggregate,
            item.event,
          );
          syncedCodes.push(code);

          const existing = await eventTypeRepository.findByCode(code, txCtx);

          if (!existing) {
            // Create new
            const newEventType = createEventTypeFromApi({
              application: command.applicationCode,
              subdomain: item.subdomain,
              aggregate: item.aggregate,
              event: item.event,
              name: item.name,
              description: item.description ?? null,
              clientScoped: item.clientScoped ?? false,
            });
            await eventTypeRepository.insert(newEventType, txCtx);
            created++;
          } else if (existing.source === 'API') {
            // Update existing API-sourced
            const updatedEntity = updateEventType(existing, {
              name: item.name,
              description: item.description ?? null,
            });
            await eventTypeRepository.update(updatedEntity, txCtx);
            updated++;
          }
          // Skip UI-sourced event types
        }

        // Remove unlisted API-sourced event types
        if (command.removeUnlisted) {
          const allForApp = await eventTypeRepository.findByCodePrefix(
            `${command.applicationCode}:`,
            txCtx,
          );
          for (const et of allForApp) {
            if (et.source === 'API' && !syncedCodes.includes(et.code)) {
              await eventTypeRepository.deleteById(et.id, txCtx);
              deleted++;
            }
          }
        }

        // Update event data with final counts (mutate the same object reference held by the event)
        eventData.eventTypesCreated = created;
        eventData.eventTypesUpdated = updated;
        eventData.eventTypesDeleted = deleted;
        eventData.syncedEventTypeCodes = syncedCodes;
      });
    },
  };
}
