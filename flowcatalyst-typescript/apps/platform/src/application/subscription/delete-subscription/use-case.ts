/**
 * Delete Subscription Use Case
 *
 * Hard deletes a subscription and its related entities.
 */

import type { UseCase } from '@flowcatalyst/application';
import { Result, UseCaseError } from '@flowcatalyst/application';
import type { ExecutionContext, UnitOfWork } from '@flowcatalyst/domain-core';

import type { SubscriptionRepository } from '../../../infrastructure/persistence/index.js';
import { SubscriptionDeleted } from '../../../domain/index.js';

import type { DeleteSubscriptionCommand } from './command.js';

export interface DeleteSubscriptionUseCaseDeps {
	readonly subscriptionRepository: SubscriptionRepository;
	readonly unitOfWork: UnitOfWork;
}

export function createDeleteSubscriptionUseCase(
	deps: DeleteSubscriptionUseCaseDeps,
): UseCase<DeleteSubscriptionCommand, SubscriptionDeleted> {
	const { subscriptionRepository, unitOfWork } = deps;

	return {
		async execute(
			command: DeleteSubscriptionCommand,
			context: ExecutionContext,
		): Promise<Result<SubscriptionDeleted>> {
			const subscription = await subscriptionRepository.findById(command.subscriptionId);
			if (!subscription) {
				return Result.failure(
					UseCaseError.notFound('SUBSCRIPTION_NOT_FOUND', 'Subscription not found', {
						subscriptionId: command.subscriptionId,
					}),
				);
			}

			const event = new SubscriptionDeleted(context, {
				subscriptionId: subscription.id,
				code: subscription.code,
				applicationCode: subscription.applicationCode,
				clientId: subscription.clientId,
			});

			return unitOfWork.commitDelete(subscription, event, command);
		},
	};
}
