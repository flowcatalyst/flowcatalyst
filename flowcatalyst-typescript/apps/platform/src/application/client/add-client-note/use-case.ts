/**
 * Add Client Note Use Case
 *
 * Adds a note to a client's audit trail.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { ClientRepository } from '../../../infrastructure/persistence/index.js';
import { addClientNote, ClientNoteAdded } from '../../../domain/index.js';

import type { AddClientNoteCommand } from './command.js';

/**
 * Dependencies for AddClientNoteUseCase.
 */
export interface AddClientNoteUseCaseDeps {
	readonly clientRepository: ClientRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the AddClientNoteUseCase.
 */
export function createAddClientNoteUseCase(
	deps: AddClientNoteUseCaseDeps,
): UseCase<AddClientNoteCommand, ClientNoteAdded> {
	const { clientRepository, unitOfWork } = deps;

	return {
		async execute(command: AddClientNoteCommand, context: ExecutionContext): Promise<Result<ClientNoteAdded>> {
			// Validate client ID
			const clientIdResult = validateRequired(command.clientId, 'clientId', 'CLIENT_ID_REQUIRED');
			if (Result.isFailure(clientIdResult)) {
				return clientIdResult;
			}

			// Validate category
			const categoryResult = validateRequired(command.category, 'category', 'CATEGORY_REQUIRED');
			if (Result.isFailure(categoryResult)) {
				return categoryResult;
			}

			// Validate text
			const textResult = validateRequired(command.text, 'text', 'TEXT_REQUIRED');
			if (Result.isFailure(textResult)) {
				return textResult;
			}

			// Find client
			const client = await clientRepository.findById(command.clientId);
			if (!client) {
				return Result.failure(UseCaseError.notFound('CLIENT_NOT_FOUND', 'Client not found'));
			}

			// Get principal ID for the addedBy field
			const addedBy = context.principalId ?? 'SYSTEM';

			// Add note
			const updatedClient = addClientNote(client, command.category, command.text, addedBy);

			// Create domain event
			const event = new ClientNoteAdded(context, {
				clientId: updatedClient.id,
				category: command.category,
				text: command.text,
				addedBy,
			});

			// Commit atomically
			return unitOfWork.commit(updatedClient, event, command);
		},
	};
}
