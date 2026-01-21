/**
 * Delete Role Use Case
 *
 * Deletes an existing role.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { RoleRepository } from '../../../infrastructure/persistence/index.js';
import { RoleDeleted, RoleSource } from '../../../domain/index.js';

import type { DeleteRoleCommand } from './command.js';

/**
 * Dependencies for DeleteRoleUseCase.
 */
export interface DeleteRoleUseCaseDeps {
	readonly roleRepository: RoleRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the DeleteRoleUseCase.
 */
export function createDeleteRoleUseCase(deps: DeleteRoleUseCaseDeps): UseCase<DeleteRoleCommand, RoleDeleted> {
	const { roleRepository, unitOfWork } = deps;

	return {
		async execute(command: DeleteRoleCommand, context: ExecutionContext): Promise<Result<RoleDeleted>> {
			// Validate role ID
			const roleIdResult = validateRequired(command.roleId, 'roleId', 'ROLE_ID_REQUIRED');
			if (Result.isFailure(roleIdResult)) {
				return roleIdResult;
			}

			// Find existing role
			const existingRole = await roleRepository.findById(command.roleId);
			if (!existingRole) {
				return Result.failure(
					UseCaseError.notFound('ROLE_NOT_FOUND', 'Role not found', {
						roleId: command.roleId,
					}),
				);
			}

			// Cannot delete CODE-defined roles
			if (existingRole.source === RoleSource.CODE) {
				return Result.failure(
					UseCaseError.businessRule('CANNOT_DELETE_CODE_ROLE', 'Code-defined roles cannot be deleted', {
						roleId: command.roleId,
						roleName: existingRole.name,
					}),
				);
			}

			// Create domain event
			const event = new RoleDeleted(context, {
				roleId: existingRole.id,
				name: existingRole.name,
			});

			// Delete and commit atomically
			return unitOfWork.commitDelete(existingRole, event, command);
		},
	};
}
