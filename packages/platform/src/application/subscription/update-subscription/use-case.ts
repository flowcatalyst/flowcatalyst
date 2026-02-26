/**
 * Update Subscription Use Case
 */

import type { UseCase } from "@flowcatalyst/application";
import { Result, UseCaseError } from "@flowcatalyst/application";
import type { ExecutionContext, UnitOfWork } from "@flowcatalyst/domain";

import type {
	SubscriptionRepository,
	DispatchPoolRepository,
} from "../../../infrastructure/persistence/index.js";
import {
	updateSubscription,
	SubscriptionUpdated,
} from "../../../domain/index.js";

import type { UpdateSubscriptionCommand } from "./command.js";

export interface UpdateSubscriptionUseCaseDeps {
	readonly subscriptionRepository: SubscriptionRepository;
	readonly dispatchPoolRepository: DispatchPoolRepository;
	readonly unitOfWork: UnitOfWork;
}

export function createUpdateSubscriptionUseCase(
	deps: UpdateSubscriptionUseCaseDeps,
): UseCase<UpdateSubscriptionCommand, SubscriptionUpdated> {
	const { subscriptionRepository, dispatchPoolRepository, unitOfWork } = deps;

	return {
		async execute(
			command: UpdateSubscriptionCommand,
			context: ExecutionContext,
		): Promise<Result<SubscriptionUpdated>> {
			const subscription = await subscriptionRepository.findById(
				command.subscriptionId,
			);
			if (!subscription) {
				return Result.failure(
					UseCaseError.notFound(
						"SUBSCRIPTION_NOT_FOUND",
						"Subscription not found",
						{
							subscriptionId: command.subscriptionId,
						},
					),
				);
			}

			// Validate event types if provided
			if (command.eventTypes !== undefined && command.eventTypes.length === 0) {
				return Result.failure(
					UseCaseError.validation(
						"EVENT_TYPES_REQUIRED",
						"At least one event type binding is required",
					),
				);
			}

			// Validate dispatch pool if changing
			if (
				command.dispatchPoolId !== undefined &&
				command.dispatchPoolId !== null
			) {
				const poolExists = await dispatchPoolRepository.exists(
					command.dispatchPoolId,
				);
				if (!poolExists) {
					return Result.failure(
						UseCaseError.notFound(
							"DISPATCH_POOL_NOT_FOUND",
							"Dispatch pool not found",
							{
								dispatchPoolId: command.dispatchPoolId,
							},
						),
					);
				}
			}

			const updated = updateSubscription(subscription, {
				...(command.name !== undefined ? { name: command.name } : {}),
				...(command.description !== undefined
					? { description: command.description }
					: {}),
				...(command.eventTypes !== undefined
					? { eventTypes: command.eventTypes }
					: {}),
				...(command.target !== undefined ? { target: command.target } : {}),
				...(command.queue !== undefined ? { queue: command.queue } : {}),
				...(command.customConfig !== undefined
					? { customConfig: command.customConfig }
					: {}),
				...(command.status !== undefined ? { status: command.status } : {}),
				...(command.maxAgeSeconds !== undefined
					? { maxAgeSeconds: command.maxAgeSeconds }
					: {}),
				...(command.dispatchPoolId !== undefined
					? { dispatchPoolId: command.dispatchPoolId }
					: {}),
				...(command.dispatchPoolCode !== undefined
					? { dispatchPoolCode: command.dispatchPoolCode }
					: {}),
				...(command.delaySeconds !== undefined
					? { delaySeconds: command.delaySeconds }
					: {}),
				...(command.sequence !== undefined
					? { sequence: command.sequence }
					: {}),
				...(command.mode !== undefined ? { mode: command.mode } : {}),
				...(command.timeoutSeconds !== undefined
					? { timeoutSeconds: command.timeoutSeconds }
					: {}),
				...(command.maxRetries !== undefined
					? { maxRetries: command.maxRetries }
					: {}),
				...(command.serviceAccountId !== undefined
					? { serviceAccountId: command.serviceAccountId }
					: {}),
				...(command.dataOnly !== undefined
					? { dataOnly: command.dataOnly }
					: {}),
			});

			const event = new SubscriptionUpdated(context, {
				subscriptionId: updated.id,
				code: updated.code,
				applicationCode: updated.applicationCode,
				name: updated.name,
				clientId: updated.clientId,
				eventTypes: updated.eventTypes,
				target: updated.target,
			});

			return unitOfWork.commit(updated, event, command);
		},
	};
}
