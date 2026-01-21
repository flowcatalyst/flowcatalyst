/**
 * Delete OAuth Client Use Case
 *
 * Deletes an OAuth client.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { OAuthClientRepository } from '../../../infrastructure/persistence/index.js';
import { OAuthClientDeleted } from '../../../domain/index.js';

import type { DeleteOAuthClientCommand } from './command.js';

/**
 * Dependencies for DeleteOAuthClientUseCase.
 */
export interface DeleteOAuthClientUseCaseDeps {
	readonly oauthClientRepository: OAuthClientRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the DeleteOAuthClientUseCase.
 */
export function createDeleteOAuthClientUseCase(
	deps: DeleteOAuthClientUseCaseDeps,
): UseCase<DeleteOAuthClientCommand, OAuthClientDeleted> {
	const { oauthClientRepository, unitOfWork } = deps;

	return {
		async execute(
			command: DeleteOAuthClientCommand,
			context: ExecutionContext,
		): Promise<Result<OAuthClientDeleted>> {
			// Validate oauthClientId
			const idResult = validateRequired(command.oauthClientId, 'oauthClientId', 'OAUTH_CLIENT_ID_REQUIRED');
			if (Result.isFailure(idResult)) {
				return idResult;
			}

			// Find existing client
			const existing = await oauthClientRepository.findById(command.oauthClientId);
			if (!existing) {
				return Result.failure(
					UseCaseError.notFound('OAUTH_CLIENT_NOT_FOUND', `OAuth client not found: ${command.oauthClientId}`),
				);
			}

			// Create domain event
			const event = new OAuthClientDeleted(context, {
				oauthClientId: existing.id,
				clientId: existing.clientId,
			});

			// Delete and commit atomically
			return unitOfWork.commitDelete(existing, event, command);
		},
	};
}
