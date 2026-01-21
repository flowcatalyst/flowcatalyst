/**
 * Activate Application Use Case
 *
 * Activates a previously deactivated application.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { ApplicationRepository } from '../../../infrastructure/persistence/index.js';
import { activateApplication, ApplicationActivated } from '../../../domain/index.js';

import type { ActivateApplicationCommand } from './command.js';

/**
 * Dependencies for ActivateApplicationUseCase.
 */
export interface ActivateApplicationUseCaseDeps {
	readonly applicationRepository: ApplicationRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the ActivateApplicationUseCase.
 */
export function createActivateApplicationUseCase(
	deps: ActivateApplicationUseCaseDeps,
): UseCase<ActivateApplicationCommand, ApplicationActivated> {
	const { applicationRepository, unitOfWork } = deps;

	return {
		async execute(
			command: ActivateApplicationCommand,
			context: ExecutionContext,
		): Promise<Result<ApplicationActivated>> {
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

			// Check if already active
			if (application.active) {
				return Result.failure(
					UseCaseError.businessRule('APPLICATION_ALREADY_ACTIVE', 'Application is already active', {
						applicationId: command.applicationId,
					}),
				);
			}

			// Activate application
			const activatedApplication = activateApplication(application);

			// Create domain event
			const event = new ApplicationActivated(context, {
				applicationId: activatedApplication.id,
				code: activatedApplication.code,
			});

			// Commit atomically
			return unitOfWork.commit(activatedApplication, event, command);
		},
	};
}
