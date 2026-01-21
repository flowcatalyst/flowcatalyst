/**
 * Delete Auth Config Use Case
 *
 * Deletes a client auth configuration.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { ClientAuthConfigRepository } from '../../../infrastructure/persistence/index.js';
import { AuthConfigDeleted } from '../../../domain/index.js';

import type { DeleteAuthConfigCommand } from './command.js';

/**
 * Dependencies for DeleteAuthConfigUseCase.
 */
export interface DeleteAuthConfigUseCaseDeps {
	readonly clientAuthConfigRepository: ClientAuthConfigRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the DeleteAuthConfigUseCase.
 */
export function createDeleteAuthConfigUseCase(
	deps: DeleteAuthConfigUseCaseDeps,
): UseCase<DeleteAuthConfigCommand, AuthConfigDeleted> {
	const { clientAuthConfigRepository, unitOfWork } = deps;

	return {
		async execute(
			command: DeleteAuthConfigCommand,
			context: ExecutionContext,
		): Promise<Result<AuthConfigDeleted>> {
			// Validate authConfigId
			const idResult = validateRequired(command.authConfigId, 'authConfigId', 'AUTH_CONFIG_ID_REQUIRED');
			if (Result.isFailure(idResult)) {
				return idResult;
			}

			// Find existing config
			const existing = await clientAuthConfigRepository.findById(command.authConfigId);
			if (!existing) {
				return Result.failure(
					UseCaseError.notFound('AUTH_CONFIG_NOT_FOUND', `Auth config not found: ${command.authConfigId}`),
				);
			}

			// Create domain event
			const event = new AuthConfigDeleted(context, {
				authConfigId: existing.id,
				emailDomain: existing.emailDomain,
			});

			// Delete and commit atomically
			return unitOfWork.commitDelete(existing, event, command);
		},
	};
}
