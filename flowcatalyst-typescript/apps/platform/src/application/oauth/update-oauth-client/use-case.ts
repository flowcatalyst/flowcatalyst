/**
 * Update OAuth Client Use Cases
 *
 * Use cases for updating OAuth clients.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { OAuthClientRepository } from '../../../infrastructure/persistence/index.js';
import {
	validateOAuthClient,
	OAuthClientUpdated,
	OAuthClientSecretRegenerated,
	type OAuthClient,
} from '../../../domain/index.js';

import type { UpdateOAuthClientCommand, RegenerateOAuthClientSecretCommand } from './command.js';

/**
 * Dependencies for UpdateOAuthClientUseCase.
 */
export interface UpdateOAuthClientUseCaseDeps {
	readonly oauthClientRepository: OAuthClientRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the UpdateOAuthClientUseCase.
 */
export function createUpdateOAuthClientUseCase(
	deps: UpdateOAuthClientUseCaseDeps,
): UseCase<UpdateOAuthClientCommand, OAuthClientUpdated> {
	const { oauthClientRepository, unitOfWork } = deps;

	return {
		async execute(
			command: UpdateOAuthClientCommand,
			context: ExecutionContext,
		): Promise<Result<OAuthClientUpdated>> {
			// Validate oauthClientId
			const idResult = validateRequired(command.oauthClientId, 'oauthClientId', 'OAUTH_CLIENT_ID_REQUIRED');
			if (Result.isFailure(idResult)) {
				return idResult;
			}

			// Find existing client
			const existing = await oauthClientRepository.findById(command.oauthClientId);
			if (!existing) {
				return Result.failure(
					UseCaseError.notFound('OAUTH_CLIENT_NOT_FOUND', `OAuth client not found: ${command.oauthClientId}`),
				);
			}

			// Build changes object for event
			const changes: Record<string, unknown> = {};

			// Create updated client
			const updated: OAuthClient = {
				...existing,
				...(command.clientName !== undefined && { clientName: command.clientName }),
				...(command.redirectUris !== undefined && { redirectUris: command.redirectUris }),
				...(command.allowedOrigins !== undefined && { allowedOrigins: command.allowedOrigins }),
				...(command.grantTypes !== undefined && { grantTypes: command.grantTypes }),
				...(command.defaultScopes !== undefined && { defaultScopes: command.defaultScopes }),
				...(command.pkceRequired !== undefined && { pkceRequired: command.pkceRequired }),
				...(command.applicationIds !== undefined && { applicationIds: command.applicationIds }),
				...(command.active !== undefined && { active: command.active }),
			};

			// Track changes
			if (command.clientName !== undefined) changes['clientName'] = command.clientName;
			if (command.redirectUris !== undefined) changes['redirectUris'] = command.redirectUris;
			if (command.allowedOrigins !== undefined) changes['allowedOrigins'] = command.allowedOrigins;
			if (command.grantTypes !== undefined) changes['grantTypes'] = command.grantTypes;
			if (command.defaultScopes !== undefined) changes['defaultScopes'] = command.defaultScopes;
			if (command.pkceRequired !== undefined) changes['pkceRequired'] = command.pkceRequired;
			if (command.applicationIds !== undefined) changes['applicationIds'] = command.applicationIds;
			if (command.active !== undefined) changes['active'] = command.active;

			// Validate updated client
			const validationError = validateOAuthClient(updated);
			if (validationError) {
				return Result.failure(UseCaseError.validation('INVALID_OAUTH_CLIENT', validationError));
			}

			// Create domain event
			const event = new OAuthClientUpdated(context, {
				oauthClientId: updated.id,
				clientId: updated.clientId,
				changes,
			});

			// Commit atomically
			return unitOfWork.commit(updated, event, command);
		},
	};
}

/**
 * Create the RegenerateOAuthClientSecretUseCase.
 */
export function createRegenerateOAuthClientSecretUseCase(
	deps: UpdateOAuthClientUseCaseDeps,
): UseCase<RegenerateOAuthClientSecretCommand, OAuthClientSecretRegenerated> {
	const { oauthClientRepository, unitOfWork } = deps;

	return {
		async execute(
			command: RegenerateOAuthClientSecretCommand,
			context: ExecutionContext,
		): Promise<Result<OAuthClientSecretRegenerated>> {
			// Validate oauthClientId
			const idResult = validateRequired(command.oauthClientId, 'oauthClientId', 'OAUTH_CLIENT_ID_REQUIRED');
			if (Result.isFailure(idResult)) {
				return idResult;
			}

			// Validate newSecretRef
			const secretResult = validateRequired(command.newSecretRef, 'newSecretRef', 'NEW_SECRET_REF_REQUIRED');
			if (Result.isFailure(secretResult)) {
				return secretResult;
			}

			// Find existing client
			const existing = await oauthClientRepository.findById(command.oauthClientId);
			if (!existing) {
				return Result.failure(
					UseCaseError.notFound('OAUTH_CLIENT_NOT_FOUND', `OAuth client not found: ${command.oauthClientId}`),
				);
			}

			// Only confidential clients can have secrets regenerated
			if (existing.clientType !== 'CONFIDENTIAL') {
				return Result.failure(
					UseCaseError.businessRule(
						'NOT_CONFIDENTIAL_CLIENT',
						'Only confidential clients can have their secrets regenerated',
					),
				);
			}

			// Update secret ref
			const updated: OAuthClient = {
				...existing,
				clientSecretRef: command.newSecretRef,
			};

			// Create domain event
			const event = new OAuthClientSecretRegenerated(context, {
				oauthClientId: updated.id,
				clientId: updated.clientId,
			});

			// Commit atomically
			return unitOfWork.commit(updated, event, command);
		},
	};
}
