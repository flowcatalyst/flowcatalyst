/**
 * Create OAuth Client Use Case
 *
 * Creates a new OAuth client.
 */

import type { UseCase } from "@flowcatalyst/application";
import {
	validateRequired,
	Result,
	type ExecutionContext,
	UseCaseError,
} from "@flowcatalyst/application";
import type { UnitOfWork } from "@flowcatalyst/domain";

import type { OAuthClientRepository } from "../../../infrastructure/persistence/index.js";
import {
	createOAuthClient,
	validateOAuthClient,
	OAuthClientCreated,
	type OAuthClient,
} from "../../../domain/index.js";

import type { CreateOAuthClientCommand } from "./command.js";

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
	const { oauthClientRepository: _oauthClientRepository, unitOfWork } = deps;

	return {
		async execute(
			command: CreateOAuthClientCommand,
			context: ExecutionContext,
		): Promise<Result<OAuthClientCreated>> {
			// Validate clientName
			const clientNameResult = validateRequired(
				command.clientName,
				"clientName",
				"CLIENT_NAME_REQUIRED",
			);
			if (Result.isFailure(clientNameResult)) {
				return clientNameResult;
			}

			// Validate clientType
			const clientTypeResult = validateRequired(
				command.clientType,
				"clientType",
				"CLIENT_TYPE_REQUIRED",
			);
			if (Result.isFailure(clientTypeResult)) {
				return clientTypeResult;
			}

			// Create OAuth client (clientId is auto-generated as TSID)
			const oauthClient = createOAuthClient({
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
				return Result.failure(
					UseCaseError.validation("INVALID_OAUTH_CLIENT", validationError),
				);
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
