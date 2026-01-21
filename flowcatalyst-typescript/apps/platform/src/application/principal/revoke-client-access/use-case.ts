/**
 * Revoke Client Access Use Case
 *
 * Revokes a user's access to a specific client.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type {
	PrincipalRepository,
	ClientAccessGrantRepository,
} from '../../../infrastructure/persistence/index.js';
import { PrincipalType, ClientAccessRevoked } from '../../../domain/index.js';

import type { RevokeClientAccessCommand } from './command.js';

/**
 * Dependencies for RevokeClientAccessUseCase.
 */
export interface RevokeClientAccessUseCaseDeps {
	readonly principalRepository: PrincipalRepository;
	readonly clientAccessGrantRepository: ClientAccessGrantRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the RevokeClientAccessUseCase.
 */
export function createRevokeClientAccessUseCase(
	deps: RevokeClientAccessUseCaseDeps,
): UseCase<RevokeClientAccessCommand, ClientAccessRevoked> {
	const { principalRepository, clientAccessGrantRepository, unitOfWork } = deps;

	return {
		async execute(
			command: RevokeClientAccessCommand,
			context: ExecutionContext,
		): Promise<Result<ClientAccessRevoked>> {
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

			// Find the grant
			const grant = await clientAccessGrantRepository.findByPrincipalAndClient(
				command.userId,
				command.clientId,
			);
			if (!grant) {
				return Result.failure(
					UseCaseError.notFound('GRANT_NOT_FOUND', 'Client access grant not found', {
						userId: command.userId,
						clientId: command.clientId,
					}),
				);
			}

			// Create domain event
			const event = new ClientAccessRevoked(context, {
				userId: principal.id,
				email: principal.userIdentity?.email ?? '',
				clientId: command.clientId,
			});

			// Delete and commit atomically
			return unitOfWork.commitDelete(grant, event, command);
		},
	};
}
