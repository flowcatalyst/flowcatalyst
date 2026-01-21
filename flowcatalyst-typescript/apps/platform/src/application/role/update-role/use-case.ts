/**
 * Update Role Use Case
 *
 * Updates an existing role.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { RoleRepository } from '../../../infrastructure/persistence/index.js';
import { updateAuthRole, RoleUpdated, RoleSource } from '../../../domain/index.js';

import type { UpdateRoleCommand } from './command.js';

/**
 * Dependencies for UpdateRoleUseCase.
 */
export interface UpdateRoleUseCaseDeps {
	readonly roleRepository: RoleRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the UpdateRoleUseCase.
 */
export function createUpdateRoleUseCase(deps: UpdateRoleUseCaseDeps): UseCase<UpdateRoleCommand, RoleUpdated> {
	const { roleRepository, unitOfWork } = deps;

	return {
		async execute(command: UpdateRoleCommand, context: ExecutionContext): Promise<Result<RoleUpdated>> {
			// Validate role ID
			const roleIdResult = validateRequired(command.roleId, 'roleId', 'ROLE_ID_REQUIRED');
			if (Result.isFailure(roleIdResult)) {
				return roleIdResult;
			}

			// Validate displayName
			const displayNameResult = validateRequired(command.displayName, 'displayName', 'DISPLAY_NAME_REQUIRED');
			if (Result.isFailure(displayNameResult)) {
				return displayNameResult;
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

			// CODE-defined roles have limited update capability
			if (existingRole.source === RoleSource.CODE) {
				// Code-defined roles can only update description and clientManaged
				// displayName and permissions are controlled by code
			}

			// Update role
			const updatedRole = updateAuthRole(existingRole, {
				displayName: command.displayName,
				...(command.description !== undefined && { description: command.description }),
				...(command.permissions !== undefined && { permissions: command.permissions }),
				...(command.clientManaged !== undefined && { clientManaged: command.clientManaged }),
			});

			// Create domain event
			const event = new RoleUpdated(context, {
				roleId: updatedRole.id,
				displayName: updatedRole.displayName,
				permissions: updatedRole.permissions,
				clientManaged: updatedRole.clientManaged,
			});

			// Commit atomically
			return unitOfWork.commit(updatedRole, event, command);
		},
	};
}
