/**
 * Update Dispatch Pool Use Case
 */

import type { UseCase } from "@flowcatalyst/application";
import { Result, UseCaseError } from "@flowcatalyst/application";
import type { ExecutionContext, UnitOfWork } from "@flowcatalyst/domain-core";

import type { DispatchPoolRepository } from "../../../infrastructure/persistence/index.js";
import {
	updateDispatchPool,
	DispatchPoolUpdated,
} from "../../../domain/index.js";

import type { UpdateDispatchPoolCommand } from "./command.js";

export interface UpdateDispatchPoolUseCaseDeps {
	readonly dispatchPoolRepository: DispatchPoolRepository;
	readonly unitOfWork: UnitOfWork;
}

export function createUpdateDispatchPoolUseCase(
	deps: UpdateDispatchPoolUseCaseDeps,
): UseCase<UpdateDispatchPoolCommand, DispatchPoolUpdated> {
	const { dispatchPoolRepository, unitOfWork } = deps;

	return {
		async execute(
			command: UpdateDispatchPoolCommand,
			context: ExecutionContext,
		): Promise<Result<DispatchPoolUpdated>> {
			const pool = await dispatchPoolRepository.findById(command.poolId);
			if (!pool) {
				return Result.failure(
					UseCaseError.notFound("POOL_NOT_FOUND", "Dispatch pool not found", {
						poolId: command.poolId,
					}),
				);
			}

			// Cannot update archived pools
			if (pool.status === "ARCHIVED") {
				return Result.failure(
					UseCaseError.businessRule(
						"POOL_ARCHIVED",
						"Cannot update an archived dispatch pool",
					),
				);
			}

			// Validate rate limit if provided
			if (command.rateLimit !== undefined && command.rateLimit < 1) {
				return Result.failure(
					UseCaseError.validation(
						"INVALID_RATE_LIMIT",
						"Rate limit must be at least 1",
					),
				);
			}

			// Validate concurrency if provided
			if (command.concurrency !== undefined && command.concurrency < 1) {
				return Result.failure(
					UseCaseError.validation(
						"INVALID_CONCURRENCY",
						"Concurrency must be at least 1",
					),
				);
			}

			const updated = updateDispatchPool(pool, {
				...(command.name !== undefined ? { name: command.name } : {}),
				...(command.description !== undefined
					? { description: command.description }
					: {}),
				...(command.rateLimit !== undefined
					? { rateLimit: command.rateLimit }
					: {}),
				...(command.concurrency !== undefined
					? { concurrency: command.concurrency }
					: {}),
				...(command.status !== undefined ? { status: command.status } : {}),
			});

			const event = new DispatchPoolUpdated(context, {
				poolId: updated.id,
				code: updated.code,
				name: updated.name,
				description: updated.description,
				rateLimit: updated.rateLimit,
				concurrency: updated.concurrency,
				status: updated.status,
			});

			return unitOfWork.commit(updated, event, command);
		},
	};
}
