/**
 * Assign Service Account Roles Use Case
 *
 * Assigns roles to a service account, replacing existing roles.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { PrincipalRepository, RoleRepository } from '../../../infrastructure/persistence/index.js';
import { assignRoles, createRoleAssignment, RolesAssigned } from '../../../domain/index.js';

import type { AssignServiceAccountRolesCommand } from './command.js';

/**
 * Dependencies for AssignServiceAccountRolesUseCase.
 */
export interface AssignServiceAccountRolesUseCaseDeps {
	readonly principalRepository: PrincipalRepository;
	readonly roleRepository: RoleRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the AssignServiceAccountRolesUseCase.
 */
export function createAssignServiceAccountRolesUseCase(
	deps: AssignServiceAccountRolesUseCaseDeps,
): UseCase<AssignServiceAccountRolesCommand, RolesAssigned> {
	const { principalRepository, roleRepository, unitOfWork } = deps;

	return {
		async execute(
			command: AssignServiceAccountRolesCommand,
			context: ExecutionContext,
		): Promise<Result<RolesAssigned>> {
			// Validate serviceAccountId
			const idResult = validateRequired(command.serviceAccountId, 'serviceAccountId', 'SERVICE_ACCOUNT_ID_REQUIRED');
			if (Result.isFailure(idResult)) {
				return idResult;
			}

			// Find the principal
			const principal = await principalRepository.findById(command.serviceAccountId);
			if (!principal) {
				return Result.failure(
					UseCaseError.notFound('SERVICE_ACCOUNT_NOT_FOUND', `Service account not found: ${command.serviceAccountId}`),
				);
			}

			// Verify it's a SERVICE type
			if (principal.type !== 'SERVICE' || !principal.serviceAccount) {
				return Result.failure(
					UseCaseError.businessRule('NOT_A_SERVICE_ACCOUNT', 'Principal is not a service account', {
						type: principal.type,
					}),
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

			// Create domain event (reuse RolesAssigned)
			const event = new RolesAssigned(context, {
				userId: principal.id,
				email: `sa:${principal.serviceAccount.code}`,
				roles: [...command.roles],
				previousRoles,
			});

			// Commit
			return unitOfWork.commit(updatedPrincipal, event, command);
		},
	};
}
