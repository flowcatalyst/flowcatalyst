/**
 * Update User Use Case
 *
 * Updates an existing user's information.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { PrincipalRepository } from '../../../infrastructure/persistence/index.js';
import { updatePrincipal, UserUpdated, PrincipalType } from '../../../domain/index.js';

import type { UpdateUserCommand } from './command.js';

/**
 * Dependencies for UpdateUserUseCase.
 */
export interface UpdateUserUseCaseDeps {
	readonly principalRepository: PrincipalRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the UpdateUserUseCase.
 */
export function createUpdateUserUseCase(deps: UpdateUserUseCaseDeps): UseCase<UpdateUserCommand, UserUpdated> {
	const { principalRepository, unitOfWork } = deps;

	return {
		async execute(command: UpdateUserCommand, context: ExecutionContext): Promise<Result<UserUpdated>> {
			// Validate userId
			const userIdResult = validateRequired(command.userId, 'userId', 'USER_ID_REQUIRED');
			if (Result.isFailure(userIdResult)) {
				return userIdResult;
			}

			// Validate name
			const nameResult = validateRequired(command.name, 'name', 'NAME_REQUIRED');
			if (Result.isFailure(nameResult)) {
				return nameResult;
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

			// Check if name actually changed
			if (principal.name === command.name) {
				// No change, but still return success
				const event = new UserUpdated(context, {
					userId: principal.id,
					name: command.name,
					previousName: principal.name,
				});
				return unitOfWork.commit(principal, event, command);
			}

			// Update the principal
			const updatedPrincipal = updatePrincipal(principal, {
				name: command.name,
			});

			// Create domain event
			const event = new UserUpdated(context, {
				userId: principal.id,
				name: command.name,
				previousName: principal.name,
			});

			// Commit atomically
			return unitOfWork.commit(updatedPrincipal, event, command);
		},
	};
}
