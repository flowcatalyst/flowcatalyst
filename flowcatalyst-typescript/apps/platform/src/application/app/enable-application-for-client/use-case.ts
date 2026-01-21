/**
 * Enable Application For Client Use Case
 *
 * Enables an application for a specific client.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type {
	ApplicationRepository,
	ApplicationClientConfigRepository,
	ClientRepository,
} from '../../../infrastructure/persistence/index.js';
import {
	createApplicationClientConfig,
	setApplicationClientConfigEnabled,
	ApplicationEnabledForClient,
} from '../../../domain/index.js';

import type { EnableApplicationForClientCommand } from './command.js';

/**
 * Dependencies for EnableApplicationForClientUseCase.
 */
export interface EnableApplicationForClientUseCaseDeps {
	readonly applicationRepository: ApplicationRepository;
	readonly clientRepository: ClientRepository;
	readonly applicationClientConfigRepository: ApplicationClientConfigRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the EnableApplicationForClientUseCase.
 */
export function createEnableApplicationForClientUseCase(
	deps: EnableApplicationForClientUseCaseDeps,
): UseCase<EnableApplicationForClientCommand, ApplicationEnabledForClient> {
	const { applicationRepository, clientRepository, applicationClientConfigRepository, unitOfWork } = deps;

	return {
		async execute(
			command: EnableApplicationForClientCommand,
			context: ExecutionContext,
		): Promise<Result<ApplicationEnabledForClient>> {
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

			// Verify application exists
			const applicationExists = await applicationRepository.exists(command.applicationId);
			if (!applicationExists) {
				return Result.failure(UseCaseError.notFound('APPLICATION_NOT_FOUND', 'Application not found'));
			}

			// Verify client exists
			const clientExists = await clientRepository.exists(command.clientId);
			if (!clientExists) {
				return Result.failure(UseCaseError.notFound('CLIENT_NOT_FOUND', 'Client not found'));
			}

			// Check if config already exists
			const existingConfig = await applicationClientConfigRepository.findByApplicationAndClient(
				command.applicationId,
				command.clientId,
			);

			let config;
			if (existingConfig) {
				if (existingConfig.enabled) {
					// Already enabled
					const event = new ApplicationEnabledForClient(context, {
						applicationId: command.applicationId,
						clientId: command.clientId,
						configId: existingConfig.id,
					});
					return unitOfWork.commit(existingConfig, event, command);
				}
				// Enable the existing config
				config = setApplicationClientConfigEnabled(existingConfig, true);
			} else {
				// Create new config
				config = createApplicationClientConfig({
					applicationId: command.applicationId,
					clientId: command.clientId,
					enabled: true,
				});
			}

			// Create domain event
			const event = new ApplicationEnabledForClient(context, {
				applicationId: command.applicationId,
				clientId: command.clientId,
				configId: config.id,
			});

			// Commit atomically
			return unitOfWork.commit(config, event, command);
		},
	};
}
