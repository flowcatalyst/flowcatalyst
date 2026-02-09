/**
 * Sync EventTypes Use Case
 *
 * Syncs event types from an application SDK. Creates new ones, updates existing
 * API-sourced ones, and optionally removes unlisted API-sourced ones.
 * UI-sourced event types are never modified.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, UseCaseError } from '@flowcatalyst/application';
import type { ExecutionContext, UnitOfWork } from '@flowcatalyst/domain-core';

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

      // Process each event type item
      for (const item of command.eventTypes) {
        const code = buildCode(command.applicationCode, item.subdomain, item.aggregate, item.event);
        syncedCodes.push(code);

        const existing = await eventTypeRepository.findByCode(code);

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
          await eventTypeRepository.insert(newEventType);
          created++;
        } else if (existing.source === 'API') {
          // Update existing API-sourced
          const updatedEntity = updateEventType(existing, {
            name: item.name,
            description: item.description ?? null,
          });
          await eventTypeRepository.update(updatedEntity);
          updated++;
        }
        // Skip UI-sourced event types
      }

      // Remove unlisted API-sourced event types
      if (command.removeUnlisted) {
        const allForApp = await eventTypeRepository.findByCodePrefix(`${command.applicationCode}:`);
        for (const et of allForApp) {
          if (et.source === 'API' && !syncedCodes.includes(et.code)) {
            await eventTypeRepository.deleteById(et.id);
            deleted++;
          }
        }
      }

      const event = new EventTypesSynced(context, {
        applicationCode: command.applicationCode,
        eventTypesCreated: created,
        eventTypesUpdated: updated,
        eventTypesDeleted: deleted,
        syncedEventTypeCodes: syncedCodes,
      });

      // For sync, we use a synthetic aggregate to commit the event
      const syncResult = {
        id: command.applicationCode,
        createdAt: new Date(),
        updatedAt: new Date(),
      };

      return unitOfWork.commit(syncResult, event, command);
    },
  };
}
