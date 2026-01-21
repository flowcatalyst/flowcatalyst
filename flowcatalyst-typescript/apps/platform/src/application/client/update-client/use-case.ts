/**
 * Update Client Use Case
 *
 * Updates an existing client's details.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { ClientRepository } from '../../../infrastructure/persistence/index.js';
import { ClientUpdated } from '../../../domain/index.js';

import type { UpdateClientCommand } from './command.js';

/**
 * Dependencies for UpdateClientUseCase.
 */
export interface UpdateClientUseCaseDeps {
	readonly clientRepository: ClientRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the UpdateClientUseCase.
 */
export function createUpdateClientUseCase(deps: UpdateClientUseCaseDeps): UseCase<UpdateClientCommand, ClientUpdated> {
	const { clientRepository, unitOfWork } = deps;

	return {
		async execute(command: UpdateClientCommand, context: ExecutionContext): Promise<Result<ClientUpdated>> {
			// Validate client ID
			const clientIdResult = validateRequired(command.clientId, 'clientId', 'CLIENT_ID_REQUIRED');
			if (Result.isFailure(clientIdResult)) {
				return clientIdResult;
			}

			// Validate name
			const nameResult = validateRequired(command.name, 'name', 'NAME_REQUIRED');
			if (Result.isFailure(nameResult)) {
				return nameResult;
			}

			// Find client
			const client = await clientRepository.findById(command.clientId);
			if (!client) {
				return Result.failure(UseCaseError.notFound('CLIENT_NOT_FOUND', 'Client not found'));
			}

			// Check if name changed
			if (client.name === command.name) {
				// No changes, but still return success
				const event = new ClientUpdated(context, {
					clientId: client.id,
					name: client.name,
					previousName: client.name,
				});
				return unitOfWork.commit(client, event, command);
			}

			// Update client
			const previousName = client.name;
			const updatedClient = {
				...client,
				name: command.name,
				updatedAt: new Date(),
			};

			// Create domain event
			const event = new ClientUpdated(context, {
				clientId: updatedClient.id,
				name: updatedClient.name,
				previousName,
			});

			// Commit atomically
			return unitOfWork.commit(updatedClient, event, command);
		},
	};
}
