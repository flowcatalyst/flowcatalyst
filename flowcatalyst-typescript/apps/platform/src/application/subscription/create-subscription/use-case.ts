/**
 * Create Subscription Use Case
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, UseCaseError } from '@flowcatalyst/application';
import type { ExecutionContext, UnitOfWork } from '@flowcatalyst/domain-core';

import type {
  SubscriptionRepository,
  DispatchPoolRepository,
} from '../../../infrastructure/persistence/index.js';
import {
  createSubscription,
  SubscriptionCreated,
  type SubscriptionSource,
} from '../../../domain/index.js';

import type { CreateSubscriptionCommand } from './command.js';

export interface CreateSubscriptionUseCaseDeps {
  readonly subscriptionRepository: SubscriptionRepository;
  readonly dispatchPoolRepository: DispatchPoolRepository;
  readonly unitOfWork: UnitOfWork;
}

const CODE_PATTERN = /^[a-z][a-z0-9-]*$/;

export function createCreateSubscriptionUseCase(
  deps: CreateSubscriptionUseCaseDeps,
): UseCase<CreateSubscriptionCommand, SubscriptionCreated> {
  const { subscriptionRepository, dispatchPoolRepository, unitOfWork } = deps;

  return {
    async execute(
      command: CreateSubscriptionCommand,
      context: ExecutionContext,
    ): Promise<Result<SubscriptionCreated>> {
      // Validate required fields
      const codeResult = validateRequired(command.code, 'code', 'CODE_REQUIRED');
      if (Result.isFailure(codeResult)) return codeResult;

      const nameResult = validateRequired(command.name, 'name', 'NAME_REQUIRED');
      if (Result.isFailure(nameResult)) return nameResult;

      const targetResult = validateRequired(command.target, 'target', 'TARGET_REQUIRED');
      if (Result.isFailure(targetResult)) return targetResult;

      // Validate code format
      if (!CODE_PATTERN.test(command.code)) {
        return Result.failure(
          UseCaseError.validation(
            'INVALID_CODE_FORMAT',
            'Code must start with a lowercase letter and contain only lowercase letters, numbers, and hyphens',
          ),
        );
      }

      // Must have at least one event type
      if (!command.eventTypes || command.eventTypes.length === 0) {
        return Result.failure(
          UseCaseError.validation(
            'EVENT_TYPES_REQUIRED',
            'At least one event type binding is required',
          ),
        );
      }

      // Validate client scoping
      const clientScoped = command.clientScoped ?? false;
      if (!clientScoped && command.clientId) {
        return Result.failure(
          UseCaseError.validation(
            'INVALID_CLIENT_SCOPE',
            'Cannot specify clientId when subscription is not client-scoped',
          ),
        );
      }

      // Validate dispatch pool exists if provided
      if (command.dispatchPoolId) {
        const poolExists = await dispatchPoolRepository.exists(command.dispatchPoolId);
        if (!poolExists) {
          return Result.failure(
            UseCaseError.notFound('DISPATCH_POOL_NOT_FOUND', 'Dispatch pool not found', {
              dispatchPoolId: command.dispatchPoolId,
            }),
          );
        }
      }

      // Check code uniqueness within client scope
      const clientId = command.clientId ?? null;
      const codeExists = await subscriptionRepository.existsByCodeAndClient(command.code, clientId);
      if (codeExists) {
        return Result.failure(
          UseCaseError.businessRule(
            'CODE_EXISTS',
            'Subscription code already exists in this scope',
            {
              code: command.code,
            },
          ),
        );
      }

      const subscription = createSubscription({
        code: command.code,
        applicationCode: command.applicationCode ?? null,
        name: command.name,
        description: command.description ?? null,
        clientId,
        clientScoped,
        eventTypes: command.eventTypes,
        target: command.target,
        queue: command.queue ?? null,
        customConfig: command.customConfig ?? [],
        source: (command.source as SubscriptionSource) ?? 'UI',
        ...(command.maxAgeSeconds !== undefined ? { maxAgeSeconds: command.maxAgeSeconds } : {}),
        dispatchPoolId: command.dispatchPoolId ?? null,
        dispatchPoolCode: command.dispatchPoolCode ?? null,
        ...(command.delaySeconds !== undefined ? { delaySeconds: command.delaySeconds } : {}),
        ...(command.sequence !== undefined ? { sequence: command.sequence } : {}),
        ...(command.mode !== undefined ? { mode: command.mode } : {}),
        ...(command.timeoutSeconds !== undefined ? { timeoutSeconds: command.timeoutSeconds } : {}),
        ...(command.maxRetries !== undefined ? { maxRetries: command.maxRetries } : {}),
        serviceAccountId: command.serviceAccountId ?? null,
        ...(command.dataOnly !== undefined ? { dataOnly: command.dataOnly } : {}),
      });

      const event = new SubscriptionCreated(context, {
        subscriptionId: subscription.id,
        code: subscription.code,
        applicationCode: subscription.applicationCode,
        name: subscription.name,
        clientId: subscription.clientId,
        clientScoped: subscription.clientScoped,
        eventTypes: subscription.eventTypes,
        target: subscription.target,
      });

      return unitOfWork.commit(subscription, event, command);
    },
  };
}
