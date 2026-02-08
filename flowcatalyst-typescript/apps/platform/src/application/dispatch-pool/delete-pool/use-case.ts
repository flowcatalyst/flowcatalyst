/**
 * Delete Dispatch Pool Use Case
 *
 * Archives a dispatch pool (soft delete via ARCHIVED status).
 */

import type { UseCase } from '@flowcatalyst/application';
import { Result, UseCaseError } from '@flowcatalyst/application';
import type { ExecutionContext, UnitOfWork } from '@flowcatalyst/domain-core';

import type { DispatchPoolRepository } from '../../../infrastructure/persistence/index.js';
import { updateDispatchPool, DispatchPoolDeleted } from '../../../domain/index.js';

import type { DeleteDispatchPoolCommand } from './command.js';

export interface DeleteDispatchPoolUseCaseDeps {
	readonly dispatchPoolRepository: DispatchPoolRepository;
	readonly unitOfWork: UnitOfWork;
}

export function createDeleteDispatchPoolUseCase(
	deps: DeleteDispatchPoolUseCaseDeps,
): UseCase<DeleteDispatchPoolCommand, DispatchPoolDeleted> {
	const { dispatchPoolRepository, unitOfWork } = deps;

	return {
		async execute(
			command: DeleteDispatchPoolCommand,
			context: ExecutionContext,
		): Promise<Result<DispatchPoolDeleted>> {
			const pool = await dispatchPoolRepository.findById(command.poolId);
			if (!pool) {
				return Result.failure(
					UseCaseError.notFound('POOL_NOT_FOUND', 'Dispatch pool not found', {
						poolId: command.poolId,
					}),
				);
			}

			if (pool.status === 'ARCHIVED') {
				return Result.failure(
					UseCaseError.businessRule('POOL_ALREADY_ARCHIVED', 'Dispatch pool is already archived'),
				);
			}

			// Soft delete by setting status to ARCHIVED
			const archived = updateDispatchPool(pool, { status: 'ARCHIVED' });

			const event = new DispatchPoolDeleted(context, {
				poolId: pool.id,
				code: pool.code,
				clientId: pool.clientId,
			});

			return unitOfWork.commit(archived, event, command);
		},
	};
}
