/**
 * Update Application Use Case
 *
 * Updates an existing application's details.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { ApplicationRepository } from '../../../infrastructure/persistence/index.js';
import { updateApplication, ApplicationUpdated } from '../../../domain/index.js';

import type { UpdateApplicationCommand } from './command.js';

/**
 * Dependencies for UpdateApplicationUseCase.
 */
export interface UpdateApplicationUseCaseDeps {
	readonly applicationRepository: ApplicationRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the UpdateApplicationUseCase.
 */
export function createUpdateApplicationUseCase(
	deps: UpdateApplicationUseCaseDeps,
): UseCase<UpdateApplicationCommand, ApplicationUpdated> {
	const { applicationRepository, unitOfWork } = deps;

	return {
		async execute(
			command: UpdateApplicationCommand,
			context: ExecutionContext,
		): Promise<Result<ApplicationUpdated>> {
			// Validate application ID
			const idResult = validateRequired(command.applicationId, 'applicationId', 'APPLICATION_ID_REQUIRED');
			if (Result.isFailure(idResult)) {
				return idResult;
			}

			// Validate name
			const nameResult = validateRequired(command.name, 'name', 'NAME_REQUIRED');
			if (Result.isFailure(nameResult)) {
				return nameResult;
			}

			// Find application
			const application = await applicationRepository.findById(command.applicationId);
			if (!application) {
				return Result.failure(UseCaseError.notFound('APPLICATION_NOT_FOUND', 'Application not found'));
			}

			// Update application
			const previousName = application.name;
			const updatedApplication = updateApplication(application, {
				name: command.name,
				description: command.description !== undefined ? command.description : application.description,
				iconUrl: command.iconUrl !== undefined ? command.iconUrl : application.iconUrl,
				website: command.website !== undefined ? command.website : application.website,
				logo: command.logo !== undefined ? command.logo : application.logo,
				logoMimeType: command.logoMimeType !== undefined ? command.logoMimeType : application.logoMimeType,
				defaultBaseUrl: command.defaultBaseUrl !== undefined ? command.defaultBaseUrl : application.defaultBaseUrl,
			});

			// Create domain event
			const event = new ApplicationUpdated(context, {
				applicationId: updatedApplication.id,
				code: updatedApplication.code,
				name: updatedApplication.name,
				previousName,
			});

			// Commit atomically
			return unitOfWork.commit(updatedApplication, event, command);
		},
	};
}
