/**
 * Delete Client Use Case
 *
 * Deletes a client from the system.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { ClientRepository } from '../../../infrastructure/persistence/index.js';
import { ClientDeleted } from '../../../domain/index.js';

import type { DeleteClientCommand } from './command.js';

/**
 * Dependencies for DeleteClientUseCase.
 */
export interface DeleteClientUseCaseDeps {
	readonly clientRepository: ClientRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the DeleteClientUseCase.
 */
export function createDeleteClientUseCase(deps: DeleteClientUseCaseDeps): UseCase<DeleteClientCommand, ClientDeleted> {
	const { clientRepository, unitOfWork } = deps;

	return {
		async execute(command: DeleteClientCommand, context: ExecutionContext): Promise<Result<ClientDeleted>> {
			// Validate client ID
			const clientIdResult = validateRequired(command.clientId, 'clientId', 'CLIENT_ID_REQUIRED');
			if (Result.isFailure(clientIdResult)) {
				return clientIdResult;
			}

			// Find client
			const client = await clientRepository.findById(command.clientId);
			if (!client) {
				return Result.failure(UseCaseError.notFound('CLIENT_NOT_FOUND', 'Client not found'));
			}

			// TODO: Check if client has any users or resources
			// For now, we allow deletion

			// Create domain event
			const event = new ClientDeleted(context, {
				clientId: client.id,
				name: client.name,
				identifier: client.identifier,
			});

			// Delete atomically
			return unitOfWork.commitDelete(client, event, command);
		},
	};
}
