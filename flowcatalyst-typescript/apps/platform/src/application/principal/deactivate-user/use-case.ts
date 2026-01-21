/**
 * Deactivate User Use Case
 *
 * Deactivates an active user.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { PrincipalRepository } from '../../../infrastructure/persistence/index.js';
import { updatePrincipal, UserDeactivated, PrincipalType } from '../../../domain/index.js';

import type { DeactivateUserCommand } from './command.js';

/**
 * Dependencies for DeactivateUserUseCase.
 */
export interface DeactivateUserUseCaseDeps {
	readonly principalRepository: PrincipalRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the DeactivateUserUseCase.
 */
export function createDeactivateUserUseCase(
	deps: DeactivateUserUseCaseDeps,
): UseCase<DeactivateUserCommand, UserDeactivated> {
	const { principalRepository, unitOfWork } = deps;

	return {
		async execute(command: DeactivateUserCommand, context: ExecutionContext): Promise<Result<UserDeactivated>> {
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

			// Check if already inactive
			if (!principal.active) {
				return Result.failure(
					UseCaseError.businessRule('ALREADY_INACTIVE', 'User is already inactive', { userId: principal.id }),
				);
			}

			// Update the principal
			const updatedPrincipal = updatePrincipal(principal, {
				active: false,
			});

			// Create domain event
			const event = new UserDeactivated(context, {
				userId: principal.id,
				email: principal.userIdentity?.email ?? '',
			});

			// Commit atomically
			return unitOfWork.commit(updatedPrincipal, event, command);
		},
	};
}
