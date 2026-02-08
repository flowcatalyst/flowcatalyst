/**
 * Assign Application Access Use Case
 *
 * Sets which applications a user can access.
 * Computes delta (added/removed) and persists to junction table.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type {
	PrincipalRepository,
	ApplicationRepository,
	ApplicationClientConfigRepository,
	ClientAccessGrantRepository,
} from '../../../infrastructure/persistence/index.js';
import { updatePrincipal, ApplicationAccessAssigned } from '../../../domain/index.js';

import type { AssignApplicationAccessCommand } from './command.js';

/**
 * Dependencies for AssignApplicationAccessUseCase.
 */
export interface AssignApplicationAccessUseCaseDeps {
	readonly principalRepository: PrincipalRepository;
	readonly applicationRepository: ApplicationRepository;
	readonly applicationClientConfigRepository: ApplicationClientConfigRepository;
	readonly clientAccessGrantRepository: ClientAccessGrantRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the AssignApplicationAccessUseCase.
 */
export function createAssignApplicationAccessUseCase(
	deps: AssignApplicationAccessUseCaseDeps,
): UseCase<AssignApplicationAccessCommand, ApplicationAccessAssigned> {
	const { principalRepository, applicationRepository, applicationClientConfigRepository, clientAccessGrantRepository, unitOfWork } = deps;

	return {
		async execute(
			command: AssignApplicationAccessCommand,
			context: ExecutionContext,
		): Promise<Result<ApplicationAccessAssigned>> {
			// Validate userId
			const idResult = validateRequired(command.userId, 'userId', 'USER_ID_REQUIRED');
			if (Result.isFailure(idResult)) {
				return idResult;
			}

			// Find the principal
			const principal = await principalRepository.findById(command.userId);
			if (!principal) {
				return Result.failure(
					UseCaseError.notFound('USER_NOT_FOUND', `User not found: ${command.userId}`),
				);
			}

			// Verify it's a USER type
			if (principal.type !== 'USER') {
				return Result.failure(
					UseCaseError.businessRule('NOT_A_USER', 'Principal is not a user', {
						type: principal.type,
					}),
				);
			}

			// Validate all requested applications exist and are active
			for (const appId of command.applicationIds) {
				const app = await applicationRepository.findById(appId);
				if (!app) {
					return Result.failure(
						UseCaseError.validation('APPLICATION_NOT_FOUND', `Application not found: ${appId}`, {
							applicationId: appId,
						}),
					);
				}
				if (!app.active) {
					return Result.failure(
						UseCaseError.businessRule('APPLICATION_INACTIVE', `Application is not active: ${appId}`, {
							applicationId: appId,
						}),
					);
				}
			}

			// Compute delta
			const currentAppIds = new Set(principal.accessibleApplicationIds);
			const requestedAppIds = new Set(command.applicationIds);

			const added = command.applicationIds.filter((id) => !currentAppIds.has(id));
			const removed = [...currentAppIds].filter((id) => !requestedAppIds.has(id));

			// Update principal domain model
			const updatedPrincipal = updatePrincipal(principal, {
				accessibleApplicationIds: command.applicationIds,
			});

			// Create domain event
			const event = new ApplicationAccessAssigned(context, {
				userId: principal.id,
				applicationIds: [...command.applicationIds],
				added,
				removed,
			});

			// Commit - the UoW will persist the principal, and we also need to update the junction table
			// We persist the junction table rows through the principal repository's setApplicationAccess
			// which the aggregate handler calls as part of persist
			return unitOfWork.commit(updatedPrincipal, event, command);
		},
	};
}
