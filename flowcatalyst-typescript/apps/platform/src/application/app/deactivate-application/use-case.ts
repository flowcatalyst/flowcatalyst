/**
 * Deactivate Application Use Case
 *
 * Deactivates an application.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { ApplicationRepository } from '../../../infrastructure/persistence/index.js';
import { deactivateApplication, ApplicationDeactivated } from '../../../domain/index.js';

import type { DeactivateApplicationCommand } from './command.js';

/**
 * Dependencies for DeactivateApplicationUseCase.
 */
export interface DeactivateApplicationUseCaseDeps {
	readonly applicationRepository: ApplicationRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the DeactivateApplicationUseCase.
 */
export function createDeactivateApplicationUseCase(
	deps: DeactivateApplicationUseCaseDeps,
): UseCase<DeactivateApplicationCommand, ApplicationDeactivated> {
	const { applicationRepository, unitOfWork } = deps;

	return {
		async execute(
			command: DeactivateApplicationCommand,
			context: ExecutionContext,
		): Promise<Result<ApplicationDeactivated>> {
			// Validate application ID
			const idResult = validateRequired(command.applicationId, 'applicationId', 'APPLICATION_ID_REQUIRED');
			if (Result.isFailure(idResult)) {
				return idResult;
			}

			// Find application
			const application = await applicationRepository.findById(command.applicationId);
			if (!application) {
				return Result.failure(UseCaseError.notFound('APPLICATION_NOT_FOUND', 'Application not found'));
			}

			// Check if already deactivated
			if (!application.active) {
				return Result.failure(
					UseCaseError.businessRule('APPLICATION_ALREADY_INACTIVE', 'Application is already inactive', {
						applicationId: command.applicationId,
					}),
				);
			}

			// Deactivate application
			const deactivatedApplication = deactivateApplication(application);

			// Create domain event
			const event = new ApplicationDeactivated(context, {
				applicationId: deactivatedApplication.id,
				code: deactivatedApplication.code,
			});

			// Commit atomically
			return unitOfWork.commit(deactivatedApplication, event, command);
		},
	};
}
