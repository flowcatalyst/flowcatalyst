/**
 * Delete Application Use Case
 *
 * Deletes an application from the system.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { ApplicationRepository } from '../../../infrastructure/persistence/index.js';
import { ApplicationDeleted } from '../../../domain/index.js';

import type { DeleteApplicationCommand } from './command.js';

/**
 * Dependencies for DeleteApplicationUseCase.
 */
export interface DeleteApplicationUseCaseDeps {
	readonly applicationRepository: ApplicationRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the DeleteApplicationUseCase.
 */
export function createDeleteApplicationUseCase(
	deps: DeleteApplicationUseCaseDeps,
): UseCase<DeleteApplicationCommand, ApplicationDeleted> {
	const { applicationRepository, unitOfWork } = deps;

	return {
		async execute(
			command: DeleteApplicationCommand,
			context: ExecutionContext,
		): Promise<Result<ApplicationDeleted>> {
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

			// Create domain event
			const event = new ApplicationDeleted(context, {
				applicationId: application.id,
				code: application.code,
				name: application.name,
			});

			// Delete atomically
			return unitOfWork.commitDelete(application, event, command);
		},
	};
}
