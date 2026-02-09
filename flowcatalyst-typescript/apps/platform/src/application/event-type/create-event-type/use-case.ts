/**
 * Create EventType Use Case
 */

import type { UseCase } from '@flowcatalyst/application';
import {
  validateRequired,
  Result,
  ExecutionContext,
  UseCaseError,
} from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { EventTypeRepository } from '../../../infrastructure/persistence/index.js';
import { createEventType, buildCode, EventTypeCreated } from '../../../domain/index.js';

import type { CreateEventTypeCommand } from './command.js';

const SEGMENT_PATTERN = /^[a-z][a-z0-9-]*$/;

export interface CreateEventTypeUseCaseDeps {
  readonly eventTypeRepository: EventTypeRepository;
  readonly unitOfWork: UnitOfWork;
}

export function createCreateEventTypeUseCase(
  deps: CreateEventTypeUseCaseDeps,
): UseCase<CreateEventTypeCommand, EventTypeCreated> {
  const { eventTypeRepository, unitOfWork } = deps;

  return {
    async execute(
      command: CreateEventTypeCommand,
      context: ExecutionContext,
    ): Promise<Result<EventTypeCreated>> {
      // Validate segments
      for (const [field, value] of [
        ['application', command.application],
        ['subdomain', command.subdomain],
        ['aggregate', command.aggregate],
        ['event', command.event],
      ] as const) {
        const req = validateRequired(value, field, `${field.toUpperCase()}_REQUIRED`);
        if (Result.isFailure(req)) return req;

        if (!SEGMENT_PATTERN.test(value)) {
          return Result.failure(
            UseCaseError.validation(
              `INVALID_${field.toUpperCase()}`,
              `${field} must be lowercase alphanumeric with hyphens, starting with a letter`,
            ),
          );
        }
      }

      // Validate name
      const nameResult = validateRequired(command.name, 'name', 'NAME_REQUIRED');
      if (Result.isFailure(nameResult)) return nameResult;

      if (command.name.length > 100) {
        return Result.failure(
          UseCaseError.validation('NAME_TOO_LONG', 'Name must be 100 characters or less'),
        );
      }

      if (command.description && command.description.length > 255) {
        return Result.failure(
          UseCaseError.validation(
            'DESCRIPTION_TOO_LONG',
            'Description must be 255 characters or less',
          ),
        );
      }

      // Check code uniqueness
      const code = buildCode(
        command.application,
        command.subdomain,
        command.aggregate,
        command.event,
      );
      const codeExists = await eventTypeRepository.existsByCode(code);
      if (codeExists) {
        return Result.failure(
          UseCaseError.businessRule('CODE_EXISTS', 'Event type code already exists', { code }),
        );
      }

      // Create event type
      const eventType = createEventType({
        application: command.application,
        subdomain: command.subdomain,
        aggregate: command.aggregate,
        event: command.event,
        name: command.name,
        description: command.description ?? null,
        clientScoped: command.clientScoped ?? false,
      });

      // Create domain event
      const event = new EventTypeCreated(context, {
        eventTypeId: eventType.id,
        code: eventType.code,
        name: eventType.name,
        description: eventType.description,
      });

      return unitOfWork.commit(eventType, event, command);
    },
  };
}
