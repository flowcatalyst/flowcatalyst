/**
 * Change Client Status Use Case
 *
 * Changes a client's status (activate, suspend, deactivate).
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { ClientRepository } from '../../../infrastructure/persistence/index.js';
import { changeClientStatus, ClientStatusChanged } from '../../../domain/index.js';

import type { ChangeClientStatusCommand } from './command.js';

/**
 * Dependencies for ChangeClientStatusUseCase.
 */
export interface ChangeClientStatusUseCaseDeps {
	readonly clientRepository: ClientRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the ChangeClientStatusUseCase.
 */
export function createChangeClientStatusUseCase(
	deps: ChangeClientStatusUseCaseDeps,
): UseCase<ChangeClientStatusCommand, ClientStatusChanged> {
	const { clientRepository, unitOfWork } = deps;

	return {
		async execute(
			command: ChangeClientStatusCommand,
			context: ExecutionContext,
		): Promise<Result<ClientStatusChanged>> {
			// Validate client ID
			const clientIdResult = validateRequired(command.clientId, 'clientId', 'CLIENT_ID_REQUIRED');
			if (Result.isFailure(clientIdResult)) {
				return clientIdResult;
			}

			// Validate new status
			const statusResult = validateRequired(command.newStatus, 'newStatus', 'STATUS_REQUIRED');
			if (Result.isFailure(statusResult)) {
				return statusResult;
			}

			// Find client
			const client = await clientRepository.findById(command.clientId);
			if (!client) {
				return Result.failure(UseCaseError.notFound('CLIENT_NOT_FOUND', 'Client not found'));
			}

			// Check if status is the same
			if (client.status === command.newStatus) {
				return Result.failure(
					UseCaseError.businessRule('STATUS_UNCHANGED', 'Client already has the specified status', {
						currentStatus: client.status,
					}),
				);
			}

			// Get principal ID for the changedBy field
			const changedBy = context.principalId ?? 'SYSTEM';

			// Change status
			const previousStatus = client.status;
			const updatedClient = changeClientStatus(
				client,
				command.newStatus,
				command.reason,
				command.note,
				changedBy,
			);

			// Create domain event
			const event = new ClientStatusChanged(context, {
				clientId: updatedClient.id,
				name: updatedClient.name,
				previousStatus,
				newStatus: command.newStatus,
				reason: command.reason,
			});

			// Commit atomically
			return unitOfWork.commit(updatedClient, event, command);
		},
	};
}
