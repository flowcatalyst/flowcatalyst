/**
 * Delete User Use Case
 *
 * Deletes an existing user.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { PrincipalRepository } from '../../../infrastructure/persistence/index.js';
import { UserDeleted, PrincipalType } from '../../../domain/index.js';

import type { DeleteUserCommand } from './command.js';

/**
 * Dependencies for DeleteUserUseCase.
 */
export interface DeleteUserUseCaseDeps {
	readonly principalRepository: PrincipalRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the DeleteUserUseCase.
 */
export function createDeleteUserUseCase(deps: DeleteUserUseCaseDeps): UseCase<DeleteUserCommand, UserDeleted> {
	const { principalRepository, unitOfWork } = deps;

	return {
		async execute(command: DeleteUserCommand, context: ExecutionContext): Promise<Result<UserDeleted>> {
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

			// Create domain event
			const event = new UserDeleted(context, {
				userId: principal.id,
				email: principal.userIdentity?.email ?? '',
			});

			// Delete atomically
			return unitOfWork.commitDelete(principal, event, command);
		},
	};
}
