/**
 * Create Application Use Case
 *
 * Creates a new application in the system.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { ApplicationRepository } from '../../../infrastructure/persistence/index.js';
import { createApplication, ApplicationCreated, ApplicationTypeEnum } from '../../../domain/index.js';

import type { CreateApplicationCommand } from './command.js';

/**
 * Dependencies for CreateApplicationUseCase.
 */
export interface CreateApplicationUseCaseDeps {
	readonly applicationRepository: ApplicationRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the CreateApplicationUseCase.
 */
export function createCreateApplicationUseCase(
	deps: CreateApplicationUseCaseDeps,
): UseCase<CreateApplicationCommand, ApplicationCreated> {
	const { applicationRepository, unitOfWork } = deps;

	return {
		async execute(
			command: CreateApplicationCommand,
			context: ExecutionContext,
		): Promise<Result<ApplicationCreated>> {
			// Validate code
			const codeResult = validateRequired(command.code, 'code', 'CODE_REQUIRED');
			if (Result.isFailure(codeResult)) {
				return codeResult;
			}

			// Validate code format (lowercase, alphanumeric, hyphens)
			const codePattern = /^[a-z0-9][a-z0-9-]{0,48}[a-z0-9]$|^[a-z0-9]$/;
			if (!codePattern.test(command.code.toLowerCase())) {
				return Result.failure(
					UseCaseError.validation(
						'INVALID_CODE',
						'Code must be lowercase alphanumeric with hyphens, 1-50 characters',
					),
				);
			}

			// Validate name
			const nameResult = validateRequired(command.name, 'name', 'NAME_REQUIRED');
			if (Result.isFailure(nameResult)) {
				return nameResult;
			}

			// Check if code already exists
			const codeExists = await applicationRepository.existsByCode(command.code);
			if (codeExists) {
				return Result.failure(
					UseCaseError.businessRule('CODE_EXISTS', 'Application code already exists', {
						code: command.code,
					}),
				);
			}

			// Create application
			const application = createApplication({
				code: command.code,
				name: command.name,
				type: command.type ?? ApplicationTypeEnum.APPLICATION,
				description: command.description ?? null,
				iconUrl: command.iconUrl ?? null,
				website: command.website ?? null,
				logo: command.logo ?? null,
				logoMimeType: command.logoMimeType ?? null,
				defaultBaseUrl: command.defaultBaseUrl ?? null,
			});

			// Create domain event
			const event = new ApplicationCreated(context, {
				applicationId: application.id,
				type: application.type,
				code: application.code,
				name: application.name,
			});

			// Commit atomically
			return unitOfWork.commit(application, event, command);
		},
	};
}
