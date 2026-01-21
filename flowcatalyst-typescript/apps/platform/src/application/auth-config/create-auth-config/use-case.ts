/**
 * Create Auth Config Use Case
 *
 * Creates a new client auth configuration.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { ClientAuthConfigRepository } from '../../../infrastructure/persistence/index.js';
import {
	createInternalAuthConfig,
	createOidcAuthConfig,
	validateOidcConfig,
	validateConfigTypeConstraints,
	AuthConfigCreated,
	type ClientAuthConfig,
	type NewClientAuthConfig,
} from '../../../domain/index.js';

import type { CreateInternalAuthConfigCommand, CreateOidcAuthConfigCommand } from './command.js';

/**
 * Dependencies for CreateAuthConfigUseCase.
 */
export interface CreateAuthConfigUseCaseDeps {
	readonly clientAuthConfigRepository: ClientAuthConfigRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the CreateInternalAuthConfigUseCase.
 */
export function createCreateInternalAuthConfigUseCase(
	deps: CreateAuthConfigUseCaseDeps,
): UseCase<CreateInternalAuthConfigCommand, AuthConfigCreated> {
	const { clientAuthConfigRepository, unitOfWork } = deps;

	return {
		async execute(
			command: CreateInternalAuthConfigCommand,
			context: ExecutionContext,
		): Promise<Result<AuthConfigCreated>> {
			// Validate emailDomain
			const domainResult = validateRequired(command.emailDomain, 'emailDomain', 'EMAIL_DOMAIN_REQUIRED');
			if (Result.isFailure(domainResult)) {
				return domainResult;
			}

			// Validate configType
			const typeResult = validateRequired(command.configType, 'configType', 'CONFIG_TYPE_REQUIRED');
			if (Result.isFailure(typeResult)) {
				return typeResult;
			}

			// Check if domain already exists
			const domainExists = await clientAuthConfigRepository.existsByEmailDomain(command.emailDomain);
			if (domainExists) {
				return Result.failure(
					UseCaseError.businessRule('DOMAIN_EXISTS', 'Auth config already exists for domain', {
						emailDomain: command.emailDomain,
					}),
				);
			}

			// Create auth config
			const authConfig = createInternalAuthConfig({
				emailDomain: command.emailDomain,
				configType: command.configType,
				primaryClientId: command.primaryClientId,
				additionalClientIds: command.additionalClientIds,
				grantedClientIds: command.grantedClientIds,
			});

			// Validate config type constraints
			const constraintError = validateConfigTypeConstraints(authConfig as ClientAuthConfig);
			if (constraintError) {
				return Result.failure(UseCaseError.businessRule('CONSTRAINT_VIOLATION', constraintError));
			}

			// Create domain event
			const event = new AuthConfigCreated(context, {
				authConfigId: authConfig.id,
				emailDomain: authConfig.emailDomain,
				configType: authConfig.configType,
				authProvider: authConfig.authProvider,
				primaryClientId: authConfig.primaryClientId,
			});

			// Commit atomically
			return unitOfWork.commit(authConfig, event, command);
		},
	};
}

/**
 * Create the CreateOidcAuthConfigUseCase.
 */
export function createCreateOidcAuthConfigUseCase(
	deps: CreateAuthConfigUseCaseDeps,
): UseCase<CreateOidcAuthConfigCommand, AuthConfigCreated> {
	const { clientAuthConfigRepository, unitOfWork } = deps;

	return {
		async execute(
			command: CreateOidcAuthConfigCommand,
			context: ExecutionContext,
		): Promise<Result<AuthConfigCreated>> {
			// Validate emailDomain
			const domainResult = validateRequired(command.emailDomain, 'emailDomain', 'EMAIL_DOMAIN_REQUIRED');
			if (Result.isFailure(domainResult)) {
				return domainResult;
			}

			// Validate configType
			const typeResult = validateRequired(command.configType, 'configType', 'CONFIG_TYPE_REQUIRED');
			if (Result.isFailure(typeResult)) {
				return typeResult;
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

			// Check if domain already exists
			const domainExists = await clientAuthConfigRepository.existsByEmailDomain(command.emailDomain);
			if (domainExists) {
				return Result.failure(
					UseCaseError.businessRule('DOMAIN_EXISTS', 'Auth config already exists for domain', {
						emailDomain: command.emailDomain,
					}),
				);
			}

			// Create auth config
			const authConfig = createOidcAuthConfig({
				emailDomain: command.emailDomain,
				configType: command.configType,
				primaryClientId: command.primaryClientId,
				additionalClientIds: command.additionalClientIds,
				grantedClientIds: command.grantedClientIds,
				oidcIssuerUrl: command.oidcIssuerUrl,
				oidcClientId: command.oidcClientId,
				oidcClientSecretRef: command.oidcClientSecretRef,
				oidcMultiTenant: command.oidcMultiTenant,
				oidcIssuerPattern: command.oidcIssuerPattern,
			});

			// Validate config type constraints
			const constraintError = validateConfigTypeConstraints(authConfig as ClientAuthConfig);
			if (constraintError) {
				return Result.failure(UseCaseError.businessRule('CONSTRAINT_VIOLATION', constraintError));
			}

			// Validate OIDC config
			const oidcError = validateOidcConfig(authConfig as ClientAuthConfig);
			if (oidcError) {
				return Result.failure(UseCaseError.validation('INVALID_OIDC_CONFIG', oidcError));
			}

			// Create domain event
			const event = new AuthConfigCreated(context, {
				authConfigId: authConfig.id,
				emailDomain: authConfig.emailDomain,
				configType: authConfig.configType,
				authProvider: authConfig.authProvider,
				primaryClientId: authConfig.primaryClientId,
			});

			// Commit atomically
			return unitOfWork.commit(authConfig, event, command);
		},
	};
}
