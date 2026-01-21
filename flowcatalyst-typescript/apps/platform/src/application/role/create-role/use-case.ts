/**
 * Create Role Use Case
 *
 * Creates a new role in the system.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { RoleRepository } from '../../../infrastructure/persistence/index.js';
import { createAuthRole, RoleCreated, RoleSource } from '../../../domain/index.js';

import type { CreateRoleCommand } from './command.js';

/**
 * Dependencies for CreateRoleUseCase.
 */
export interface CreateRoleUseCaseDeps {
	readonly roleRepository: RoleRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the CreateRoleUseCase.
 */
export function createCreateRoleUseCase(deps: CreateRoleUseCaseDeps): UseCase<CreateRoleCommand, RoleCreated> {
	const { roleRepository, unitOfWork } = deps;

	return {
		async execute(command: CreateRoleCommand, context: ExecutionContext): Promise<Result<RoleCreated>> {
			// Validate shortName
			const shortNameResult = validateRequired(command.shortName, 'shortName', 'SHORT_NAME_REQUIRED');
			if (Result.isFailure(shortNameResult)) {
				return shortNameResult;
			}

			// Validate shortName format (lowercase, alphanumeric, hyphens)
			const namePattern = /^[a-z][a-z0-9-]{0,98}[a-z0-9]$|^[a-z]$/;
			if (!namePattern.test(command.shortName.toLowerCase())) {
				return Result.failure(
					UseCaseError.validation(
						'INVALID_SHORT_NAME',
						'Short name must be lowercase alphanumeric with hyphens, 1-100 characters',
					),
				);
			}

			// Validate displayName
			const displayNameResult = validateRequired(command.displayName, 'displayName', 'DISPLAY_NAME_REQUIRED');
			if (Result.isFailure(displayNameResult)) {
				return displayNameResult;
			}

			// Build full name with prefix if application code is provided
			const fullName = command.applicationCode
				? `${command.applicationCode}:${command.shortName.toLowerCase()}`
				: command.shortName.toLowerCase();

			// Check if name already exists
			const nameExists = await roleRepository.existsByName(fullName);
			if (nameExists) {
				return Result.failure(
					UseCaseError.businessRule('ROLE_NAME_EXISTS', 'Role name already exists', {
						name: fullName,
					}),
				);
			}

			// Create role
			const role = createAuthRole({
				applicationId: command.applicationId ?? null,
				applicationCode: command.applicationCode ?? null,
				shortName: command.shortName,
				displayName: command.displayName,
				description: command.description ?? null,
				permissions: command.permissions ?? [],
				source: command.source ?? RoleSource.DATABASE,
				clientManaged: command.clientManaged ?? false,
			});

			// Create domain event
			const event = new RoleCreated(context, {
				roleId: role.id,
				name: role.name,
				displayName: role.displayName,
				applicationId: role.applicationId,
				applicationCode: role.applicationCode,
				source: role.source,
				permissions: role.permissions,
			});

			// Commit atomically
			return unitOfWork.commit(role, event, command);
		},
	};
}
