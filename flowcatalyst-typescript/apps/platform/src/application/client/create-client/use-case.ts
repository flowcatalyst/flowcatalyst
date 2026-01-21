/**
 * Create Client Use Case
 *
 * Creates a new client organization in the system.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { ClientRepository } from '../../../infrastructure/persistence/index.js';
import { createClient, ClientCreated } from '../../../domain/index.js';

import type { CreateClientCommand } from './command.js';

/**
 * Dependencies for CreateClientUseCase.
 */
export interface CreateClientUseCaseDeps {
	readonly clientRepository: ClientRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the CreateClientUseCase.
 */
export function createCreateClientUseCase(deps: CreateClientUseCaseDeps): UseCase<CreateClientCommand, ClientCreated> {
	const { clientRepository, unitOfWork } = deps;

	return {
		async execute(command: CreateClientCommand, context: ExecutionContext): Promise<Result<ClientCreated>> {
			// Validate name
			const nameResult = validateRequired(command.name, 'name', 'NAME_REQUIRED');
			if (Result.isFailure(nameResult)) {
				return nameResult;
			}

			// Validate identifier
			const identifierResult = validateRequired(command.identifier, 'identifier', 'IDENTIFIER_REQUIRED');
			if (Result.isFailure(identifierResult)) {
				return identifierResult;
			}

			// Validate identifier format (lowercase, alphanumeric, hyphens, underscores)
			const identifierPattern = /^[a-z0-9][a-z0-9_-]{0,58}[a-z0-9]$|^[a-z0-9]$/;
			if (!identifierPattern.test(command.identifier.toLowerCase())) {
				return Result.failure(
					UseCaseError.validation(
						'INVALID_IDENTIFIER',
						'Identifier must be lowercase alphanumeric with hyphens/underscores, 1-60 characters',
					),
				);
			}

			// Check if identifier already exists
			const identifierExists = await clientRepository.existsByIdentifier(command.identifier);
			if (identifierExists) {
				return Result.failure(
					UseCaseError.businessRule('IDENTIFIER_EXISTS', 'Client identifier already exists', {
						identifier: command.identifier,
					}),
				);
			}

			// Create client
			const client = createClient({
				name: command.name,
				identifier: command.identifier,
			});

			// Create domain event
			const event = new ClientCreated(context, {
				clientId: client.id,
				name: client.name,
				identifier: client.identifier,
			});

			// Commit atomically
			return unitOfWork.commit(client, event, command);
		},
	};
}
