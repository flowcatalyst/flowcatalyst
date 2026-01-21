/**
 * Disable Application For Client Use Case
 *
 * Disables an application for a specific client.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { ApplicationClientConfigRepository } from '../../../infrastructure/persistence/index.js';
import { setApplicationClientConfigEnabled, ApplicationDisabledForClient } from '../../../domain/index.js';

import type { DisableApplicationForClientCommand } from './command.js';

/**
 * Dependencies for DisableApplicationForClientUseCase.
 */
export interface DisableApplicationForClientUseCaseDeps {
	readonly applicationClientConfigRepository: ApplicationClientConfigRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the DisableApplicationForClientUseCase.
 */
export function createDisableApplicationForClientUseCase(
	deps: DisableApplicationForClientUseCaseDeps,
): UseCase<DisableApplicationForClientCommand, ApplicationDisabledForClient> {
	const { applicationClientConfigRepository, unitOfWork } = deps;

	return {
		async execute(
			command: DisableApplicationForClientCommand,
			context: ExecutionContext,
		): Promise<Result<ApplicationDisabledForClient>> {
			// Validate application ID
			const appIdResult = validateRequired(command.applicationId, 'applicationId', 'APPLICATION_ID_REQUIRED');
			if (Result.isFailure(appIdResult)) {
				return appIdResult;
			}

			// Validate client ID
			const clientIdResult = validateRequired(command.clientId, 'clientId', 'CLIENT_ID_REQUIRED');
			if (Result.isFailure(clientIdResult)) {
				return clientIdResult;
			}

			// Find existing config
			const existingConfig = await applicationClientConfigRepository.findByApplicationAndClient(
				command.applicationId,
				command.clientId,
			);

			if (!existingConfig) {
				return Result.failure(
					UseCaseError.notFound(
						'CONFIG_NOT_FOUND',
						'Application is not configured for this client',
					),
				);
			}

			if (!existingConfig.enabled) {
				// Already disabled
				const event = new ApplicationDisabledForClient(context, {
					applicationId: command.applicationId,
					clientId: command.clientId,
					configId: existingConfig.id,
				});
				return unitOfWork.commit(existingConfig, event, command);
			}

			// Disable the config
			const config = setApplicationClientConfigEnabled(existingConfig, false);

			// Create domain event
			const event = new ApplicationDisabledForClient(context, {
				applicationId: command.applicationId,
				clientId: command.clientId,
				configId: config.id,
			});

			// Commit atomically
			return unitOfWork.commit(config, event, command);
		},
	};
}
