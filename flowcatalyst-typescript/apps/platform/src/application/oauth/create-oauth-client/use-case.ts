/**
 * Create OAuth Client Use Case
 *
 * Creates a new OAuth client.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { OAuthClientRepository } from '../../../infrastructure/persistence/index.js';
import {
	createOAuthClient,
	validateOAuthClient,
	OAuthClientCreated,
	type OAuthClient,
} from '../../../domain/index.js';

import type { CreateOAuthClientCommand } from './command.js';

/**
 * Dependencies for CreateOAuthClientUseCase.
 */
export interface CreateOAuthClientUseCaseDeps {
	readonly oauthClientRepository: OAuthClientRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the CreateOAuthClientUseCase.
 */
export function createCreateOAuthClientUseCase(
	deps: CreateOAuthClientUseCaseDeps,
): UseCase<CreateOAuthClientCommand, OAuthClientCreated> {
	const { oauthClientRepository, unitOfWork } = deps;

	return {
		async execute(
			command: CreateOAuthClientCommand,
			context: ExecutionContext,
		): Promise<Result<OAuthClientCreated>> {
			// Validate clientId
			const clientIdResult = validateRequired(command.clientId, 'clientId', 'CLIENT_ID_REQUIRED');
			if (Result.isFailure(clientIdResult)) {
				return clientIdResult;
			}

			// Validate clientName
			const clientNameResult = validateRequired(command.clientName, 'clientName', 'CLIENT_NAME_REQUIRED');
			if (Result.isFailure(clientNameResult)) {
				return clientNameResult;
			}

			// Validate clientType
			const clientTypeResult = validateRequired(command.clientType, 'clientType', 'CLIENT_TYPE_REQUIRED');
			if (Result.isFailure(clientTypeResult)) {
				return clientTypeResult;
			}

			// Check if clientId already exists
			const clientIdExists = await oauthClientRepository.existsByClientId(command.clientId);
			if (clientIdExists) {
				return Result.failure(
					UseCaseError.businessRule('CLIENT_ID_EXISTS', 'OAuth client ID already exists', {
						clientId: command.clientId,
					}),
				);
			}

			// Create OAuth client
			const oauthClient = createOAuthClient({
				clientId: command.clientId,
				clientName: command.clientName,
				clientType: command.clientType,
				clientSecretRef: command.clientSecretRef,
				redirectUris: command.redirectUris,
				allowedOrigins: command.allowedOrigins,
				grantTypes: command.grantTypes,
				defaultScopes: command.defaultScopes,
				pkceRequired: command.pkceRequired,
				applicationIds: command.applicationIds,
			});

			// Validate OAuth client configuration
			const validationError = validateOAuthClient(oauthClient as OAuthClient);
			if (validationError) {
				return Result.failure(UseCaseError.validation('INVALID_OAUTH_CLIENT', validationError));
			}

			// Create domain event
			const event = new OAuthClientCreated(context, {
				oauthClientId: oauthClient.id,
				clientId: oauthClient.clientId,
				clientName: oauthClient.clientName,
				clientType: oauthClient.clientType,
			});

			// Commit atomically
			return unitOfWork.commit(oauthClient, event, command);
		},
	};
}
