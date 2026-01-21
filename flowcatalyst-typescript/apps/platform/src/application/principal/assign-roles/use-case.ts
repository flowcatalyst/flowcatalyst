/**
 * Assign Roles Use Case
 *
 * Assigns roles to a user, replacing their existing roles.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { PrincipalRepository, RoleRepository } from '../../../infrastructure/persistence/index.js';
import { assignRoles, createRoleAssignment, PrincipalType, RolesAssigned } from '../../../domain/index.js';

import type { AssignRolesCommand } from './command.js';

/**
 * Dependencies for AssignRolesUseCase.
 */
export interface AssignRolesUseCaseDeps {
	readonly principalRepository: PrincipalRepository;
	readonly roleRepository: RoleRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the AssignRolesUseCase.
 */
export function createAssignRolesUseCase(
	deps: AssignRolesUseCaseDeps,
): UseCase<AssignRolesCommand, RolesAssigned> {
	const { principalRepository, roleRepository, unitOfWork } = deps;

	return {
		async execute(command: AssignRolesCommand, context: ExecutionContext): Promise<Result<RolesAssigned>> {
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

			// Validate that all roles exist
			for (const roleName of command.roles) {
				const roleExists = await roleRepository.existsByName(roleName);
				if (!roleExists) {
					return Result.failure(
						UseCaseError.validation('ROLE_NOT_FOUND', `Role not found: ${roleName}`, { role: roleName }),
					);
				}
			}

			// Get previous role names
			const previousRoles = principal.roles.map((r) => r.roleName);

			// Create new role assignments
			const roleAssignments = command.roles.map((roleName) =>
				createRoleAssignment(roleName, 'ADMIN_ASSIGNED'),
			);

			// Update principal with new roles
			const updatedPrincipal = assignRoles(principal, roleAssignments);

			// Create domain event
			const event = new RolesAssigned(context, {
				userId: principal.id,
				email: principal.userIdentity?.email ?? '',
				roles: command.roles,
				previousRoles,
			});

			// Commit atomically
			return unitOfWork.commit(updatedPrincipal, event, command);
		},
	};
}
