/**
 * Grant Client Access Use Case
 *
 * Grants a user access to a specific client.
 * Used for PARTNER scope users who need explicit access to clients.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type {
	PrincipalRepository,
	ClientRepository,
	ClientAccessGrantRepository,
} from '../../../infrastructure/persistence/index.js';
import { createClientAccessGrant, PrincipalType, UserScope, ClientAccessGranted } from '../../../domain/index.js';

import type { GrantClientAccessCommand } from './command.js';

/**
 * Dependencies for GrantClientAccessUseCase.
 */
export interface GrantClientAccessUseCaseDeps {
	readonly principalRepository: PrincipalRepository;
	readonly clientRepository: ClientRepository;
	readonly clientAccessGrantRepository: ClientAccessGrantRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the GrantClientAccessUseCase.
 */
export function createGrantClientAccessUseCase(
	deps: GrantClientAccessUseCaseDeps,
): UseCase<GrantClientAccessCommand, ClientAccessGranted> {
	const { principalRepository, clientRepository, clientAccessGrantRepository, unitOfWork } = deps;

	return {
		async execute(
			command: GrantClientAccessCommand,
			context: ExecutionContext,
		): Promise<Result<ClientAccessGranted>> {
			// Validate userId
			const userIdResult = validateRequired(command.userId, 'userId', 'USER_ID_REQUIRED');
			if (Result.isFailure(userIdResult)) {
				return userIdResult;
			}

			// Validate clientId
			const clientIdResult = validateRequired(command.clientId, 'clientId', 'CLIENT_ID_REQUIRED');
			if (Result.isFailure(clientIdResult)) {
				return clientIdResult;
			}

			// Find the user
			const principal = await principalRepository.findById(command.userId);
			if (!principal) {
				return Result.failure(UseCaseError.notFound('USER_NOT_FOUND', `User not found: ${command.userId}`));
			}

			// Verify it's a USER type
			if (principal.type !== PrincipalType.USER) {
				return Result.failure(
					UseCaseError.businessRule('NOT_A_USER', 'Principal is not a user', { type: principal.type }),
				);
			}

			// Only PARTNER scope users need explicit client access grants
			if (principal.scope !== UserScope.PARTNER) {
				return Result.failure(
					UseCaseError.businessRule(
						'INVALID_SCOPE',
						'Client access grants only apply to PARTNER scope users',
						{ scope: principal.scope },
					),
				);
			}

			// Verify client exists
			const clientExists = await clientRepository.exists(command.clientId);
			if (!clientExists) {
				return Result.failure(
					UseCaseError.notFound('CLIENT_NOT_FOUND', `Client not found: ${command.clientId}`),
				);
			}

			// Check if grant already exists
			const grantExists = await clientAccessGrantRepository.existsByPrincipalAndClient(
				command.userId,
				command.clientId,
			);
			if (grantExists) {
				return Result.failure(
					UseCaseError.businessRule(
						'GRANT_EXISTS',
						'User already has access to this client',
						{ userId: command.userId, clientId: command.clientId },
					),
				);
			}

			// Create client access grant
			const grant = createClientAccessGrant({
				principalId: command.userId,
				clientId: command.clientId,
				grantedBy: context.principalId,
			});

			// Create domain event
			const event = new ClientAccessGranted(context, {
				userId: principal.id,
				email: principal.userIdentity?.email ?? '',
				clientId: command.clientId,
			});

			// Commit atomically
			return unitOfWork.commit(grant, event, command);
		},
	};
}
