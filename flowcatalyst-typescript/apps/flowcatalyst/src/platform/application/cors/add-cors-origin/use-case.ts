/**
 * Add CORS Origin Use Case
 */

import type { UseCase } from '@flowcatalyst/application';
import {
  validateRequired,
  Result,
  ExecutionContext,
  UseCaseError,
} from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { CorsAllowedOriginRepository } from '../../../infrastructure/persistence/index.js';
import { createCorsAllowedOrigin, CorsOriginAdded } from '../../../domain/index.js';

import type { AddCorsOriginCommand } from './command.js';

export interface AddCorsOriginUseCaseDeps {
  readonly corsAllowedOriginRepository: CorsAllowedOriginRepository;
  readonly unitOfWork: UnitOfWork;
}

export function createAddCorsOriginUseCase(
  deps: AddCorsOriginUseCaseDeps,
): UseCase<AddCorsOriginCommand, CorsOriginAdded> {
  const { corsAllowedOriginRepository, unitOfWork } = deps;

  return {
    async execute(
      command: AddCorsOriginCommand,
      context: ExecutionContext,
    ): Promise<Result<CorsOriginAdded>> {
      // Validate origin
      const originResult = validateRequired(command.origin, 'origin', 'ORIGIN_REQUIRED');
      if (Result.isFailure(originResult)) {
        return originResult;
      }

      // Validate origin format (must be a valid URL origin, wildcards allowed in hostname)
      // Supports: https://example.com, https://*.example.com, https://qa-*.example.com
      const originPattern =
        /^https?:\/\/(\*\.)?[a-zA-Z0-9*]([a-zA-Z0-9\-*]*[a-zA-Z0-9*])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?)*(:\d+)?$/;
      if (!originPattern.test(command.origin)) {
        return Result.failure(
          UseCaseError.validation(
            'INVALID_ORIGIN',
            'Origin must be a valid URL origin (e.g., https://example.com or https://*.example.com)',
          ),
        );
      }

      // Check if origin already exists
      const exists = await corsAllowedOriginRepository.existsByOrigin(command.origin);
      if (exists) {
        return Result.failure(
          UseCaseError.businessRule('ORIGIN_EXISTS', 'CORS origin already exists', {
            origin: command.origin,
          }),
        );
      }

      // Create entity
      const entity = createCorsAllowedOrigin(
        command.origin,
        command.description,
        context.principalId,
      );

      // Create event
      const event = new CorsOriginAdded(context, {
        originId: entity.id,
        origin: entity.origin,
      });

      return unitOfWork.commit(entity, event, command);
    },
  };
}
