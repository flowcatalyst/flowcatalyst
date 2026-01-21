/**
 * Activate User Use Case
 *
 * Activates a deactivated user.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { PrincipalRepository } from '../../../infrastructure/persistence/index.js';
import { updatePrincipal, UserActivated, PrincipalType } from '../../../domain/index.js';

import type { ActivateUserCommand } from './command.js';

/**
 * Dependencies for ActivateUserUseCase.
 */
export interface ActivateUserUseCaseDeps {
	readonly principalRepository: PrincipalRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the ActivateUserUseCase.
 */
export function createActivateUserUseCase(deps: ActivateUserUseCaseDeps): UseCase<ActivateUserCommand, UserActivated> {
	const { principalRepository, unitOfWork } = deps;

	return {
		async execute(command: ActivateUserCommand, context: ExecutionContext): Promise<Result<UserActivated>> {
			// Validate userId
			const userIdResult = validateRequired(command.userId, 'userId', 'USER_ID_REQUIRED');
			if (Result.isFailure(userIdResult)) {
				return userIdResult;
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

			// Check if already active
			if (principal.active) {
				return Result.failure(
					UseCaseError.businessRule('ALREADY_ACTIVE', 'User is already active', { userId: principal.id }),
				);
			}

			// Update the principal
			const updatedPrincipal = updatePrincipal(principal, {
				active: true,
			});

			// Create domain event
			const event = new UserActivated(context, {
				userId: principal.id,
				email: principal.userIdentity?.email ?? '',
			});

			// Commit atomically
			return unitOfWork.commit(updatedPrincipal, event, command);
		},
	};
}
