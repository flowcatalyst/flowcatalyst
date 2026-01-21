/**
 * Update Auth Config Use Cases
 *
 * Use cases for updating client auth configurations.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { ClientAuthConfigRepository } from '../../../infrastructure/persistence/index.js';
import {
	validateOidcConfig,
	validateConfigTypeConstraints,
	AuthConfigUpdated,
	type ClientAuthConfig,
} from '../../../domain/index.js';

import type {
	UpdateOidcSettingsCommand,
	UpdateConfigTypeCommand,
	UpdateAdditionalClientsCommand,
	UpdateGrantedClientsCommand,
} from './command.js';

/**
 * Dependencies for UpdateAuthConfigUseCase.
 */
export interface UpdateAuthConfigUseCaseDeps {
	readonly clientAuthConfigRepository: ClientAuthConfigRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the UpdateOidcSettingsUseCase.
 */
export function createUpdateOidcSettingsUseCase(
	deps: UpdateAuthConfigUseCaseDeps,
): UseCase<UpdateOidcSettingsCommand, AuthConfigUpdated> {
	const { clientAuthConfigRepository, unitOfWork } = deps;

	return {
		async execute(
			command: UpdateOidcSettingsCommand,
			context: ExecutionContext,
		): Promise<Result<AuthConfigUpdated>> {
			// Validate authConfigId
			const idResult = validateRequired(command.authConfigId, 'authConfigId', 'AUTH_CONFIG_ID_REQUIRED');
			if (Result.isFailure(idResult)) {
				return idResult;
			}

			// Validate OIDC fields
			const issuerResult = validateRequired(command.oidcIssuerUrl, 'oidcIssuerUrl', 'OIDC_ISSUER_URL_REQUIRED');
			if (Result.isFailure(issuerResult)) {
				return issuerResult;
			}

			const clientIdResult = validateRequired(command.oidcClientId, 'oidcClientId', 'OIDC_CLIENT_ID_REQUIRED');
			if (Result.isFailure(clientIdResult)) {
				return clientIdResult;
			}

			// Find existing config
			const existing = await clientAuthConfigRepository.findById(command.authConfigId);
			if (!existing) {
				return Result.failure(
					UseCaseError.notFound('AUTH_CONFIG_NOT_FOUND', `Auth config not found: ${command.authConfigId}`),
				);
			}

			// Verify it's an OIDC config
			if (existing.authProvider !== 'OIDC') {
				return Result.failure(
					UseCaseError.businessRule('NOT_OIDC_CONFIG', 'Cannot update OIDC settings on non-OIDC config'),
				);
			}

			// Create updated config
			const updated: ClientAuthConfig = {
				...existing,
				oidcIssuerUrl: command.oidcIssuerUrl,
				oidcClientId: command.oidcClientId,
				...(command.oidcClientSecretRef !== undefined && { oidcClientSecretRef: command.oidcClientSecretRef }),
				...(command.oidcMultiTenant !== undefined && { oidcMultiTenant: command.oidcMultiTenant }),
				...(command.oidcIssuerPattern !== undefined && { oidcIssuerPattern: command.oidcIssuerPattern }),
			};

			// Validate OIDC config
			const oidcError = validateOidcConfig(updated);
			if (oidcError) {
				return Result.failure(UseCaseError.validation('INVALID_OIDC_CONFIG', oidcError));
			}

			// Create domain event
			const event = new AuthConfigUpdated(context, {
				authConfigId: updated.id,
				emailDomain: updated.emailDomain,
				configType: updated.configType,
				authProvider: updated.authProvider,
				changes: {
					oidcIssuerUrl: command.oidcIssuerUrl,
					oidcClientId: command.oidcClientId,
					...(command.oidcMultiTenant !== undefined && { oidcMultiTenant: command.oidcMultiTenant }),
				},
			});

			// Commit atomically
			return unitOfWork.commit(updated, event, command);
		},
	};
}

/**
 * Create the UpdateConfigTypeUseCase.
 */
export function createUpdateConfigTypeUseCase(
	deps: UpdateAuthConfigUseCaseDeps,
): UseCase<UpdateConfigTypeCommand, AuthConfigUpdated> {
	const { clientAuthConfigRepository, unitOfWork } = deps;

	return {
		async execute(
			command: UpdateConfigTypeCommand,
			context: ExecutionContext,
		): Promise<Result<AuthConfigUpdated>> {
			// Validate authConfigId
			const idResult = validateRequired(command.authConfigId, 'authConfigId', 'AUTH_CONFIG_ID_REQUIRED');
			if (Result.isFailure(idResult)) {
				return idResult;
			}

			// Validate configType
			const typeResult = validateRequired(command.configType, 'configType', 'CONFIG_TYPE_REQUIRED');
			if (Result.isFailure(typeResult)) {
				return typeResult;
			}

			// Find existing config
			const existing = await clientAuthConfigRepository.findById(command.authConfigId);
			if (!existing) {
				return Result.failure(
					UseCaseError.notFound('AUTH_CONFIG_NOT_FOUND', `Auth config not found: ${command.authConfigId}`),
				);
			}

			// Create updated config - reset lists based on new type
			const updated: ClientAuthConfig = {
				...existing,
				configType: command.configType,
				primaryClientId: command.primaryClientId ?? null,
				additionalClientIds: command.configType === 'CLIENT' ? existing.additionalClientIds : [],
				grantedClientIds: command.configType === 'PARTNER' ? existing.grantedClientIds : [],
			};

			// Validate config type constraints
			const constraintError = validateConfigTypeConstraints(updated);
			if (constraintError) {
				return Result.failure(UseCaseError.businessRule('CONSTRAINT_VIOLATION', constraintError));
			}

			// Create domain event
			const event = new AuthConfigUpdated(context, {
				authConfigId: updated.id,
				emailDomain: updated.emailDomain,
				configType: updated.configType,
				authProvider: updated.authProvider,
				changes: {
					configType: command.configType,
					primaryClientId: command.primaryClientId,
				},
			});

			// Commit atomically
			return unitOfWork.commit(updated, event, command);
		},
	};
}

/**
 * Create the UpdateAdditionalClientsUseCase.
 */
export function createUpdateAdditionalClientsUseCase(
	deps: UpdateAuthConfigUseCaseDeps,
): UseCase<UpdateAdditionalClientsCommand, AuthConfigUpdated> {
	const { clientAuthConfigRepository, unitOfWork } = deps;

	return {
		async execute(
			command: UpdateAdditionalClientsCommand,
			context: ExecutionContext,
		): Promise<Result<AuthConfigUpdated>> {
			// Validate authConfigId
			const idResult = validateRequired(command.authConfigId, 'authConfigId', 'AUTH_CONFIG_ID_REQUIRED');
			if (Result.isFailure(idResult)) {
				return idResult;
			}

			// Find existing config
			const existing = await clientAuthConfigRepository.findById(command.authConfigId);
			if (!existing) {
				return Result.failure(
					UseCaseError.notFound('AUTH_CONFIG_NOT_FOUND', `Auth config not found: ${command.authConfigId}`),
				);
			}

			// Verify it's a CLIENT type
			if (existing.configType !== 'CLIENT') {
				return Result.failure(
					UseCaseError.businessRule(
						'NOT_CLIENT_CONFIG',
						'Additional clients are only allowed for CLIENT config type',
					),
				);
			}

			// Create updated config
			const updated: ClientAuthConfig = {
				...existing,
				additionalClientIds: command.additionalClientIds,
			};

			// Create domain event
			const event = new AuthConfigUpdated(context, {
				authConfigId: updated.id,
				emailDomain: updated.emailDomain,
				configType: updated.configType,
				authProvider: updated.authProvider,
				changes: {
					additionalClientIds: command.additionalClientIds,
				},
			});

			// Commit atomically
			return unitOfWork.commit(updated, event, command);
		},
	};
}

/**
 * Create the UpdateGrantedClientsUseCase.
 */
export function createUpdateGrantedClientsUseCase(
	deps: UpdateAuthConfigUseCaseDeps,
): UseCase<UpdateGrantedClientsCommand, AuthConfigUpdated> {
	const { clientAuthConfigRepository, unitOfWork } = deps;

	return {
		async execute(
			command: UpdateGrantedClientsCommand,
			context: ExecutionContext,
		): Promise<Result<AuthConfigUpdated>> {
			// Validate authConfigId
			const idResult = validateRequired(command.authConfigId, 'authConfigId', 'AUTH_CONFIG_ID_REQUIRED');
			if (Result.isFailure(idResult)) {
				return idResult;
			}

			// Find existing config
			const existing = await clientAuthConfigRepository.findById(command.authConfigId);
			if (!existing) {
				return Result.failure(
					UseCaseError.notFound('AUTH_CONFIG_NOT_FOUND', `Auth config not found: ${command.authConfigId}`),
				);
			}

			// Verify it's a PARTNER type
			if (existing.configType !== 'PARTNER') {
				return Result.failure(
					UseCaseError.businessRule(
						'NOT_PARTNER_CONFIG',
						'Granted clients are only allowed for PARTNER config type',
					),
				);
			}

			// Create updated config
			const updated: ClientAuthConfig = {
				...existing,
				grantedClientIds: command.grantedClientIds,
			};

			// Create domain event
			const event = new AuthConfigUpdated(context, {
				authConfigId: updated.id,
				emailDomain: updated.emailDomain,
				configType: updated.configType,
				authProvider: updated.authProvider,
				changes: {
					grantedClientIds: command.grantedClientIds,
				},
			});

			// Commit atomically
			return unitOfWork.commit(updated, event, command);
		},
	};
}
