/**
 * Create Email Domain Mapping Use Case
 *
 * After creating the mapping, cascades scope changes to all existing
 * USER principals with the matching email domain.
 */

import type { UseCase } from "@flowcatalyst/application";
import {
	validateRequired,
	Result,
	UseCaseError,
} from "@flowcatalyst/application";
import type { ExecutionContext, UnitOfWork } from "@flowcatalyst/domain-core";

import type {
	IdentityProviderRepository,
	EmailDomainMappingRepository,
	PrincipalRepository,
	ClientAccessGrantRepository,
} from "../../../infrastructure/persistence/index.js";
import {
	createEmailDomainMapping,
	EmailDomainMappingCreated,
	updatePrincipal,
	PrincipalScope,
} from "../../../domain/index.js";

import type { CreateEmailDomainMappingCommand } from "./command.js";

export interface CreateEmailDomainMappingUseCaseDeps {
	readonly emailDomainMappingRepository: EmailDomainMappingRepository;
	readonly identityProviderRepository: IdentityProviderRepository;
	readonly principalRepository: PrincipalRepository;
	readonly clientAccessGrantRepository: ClientAccessGrantRepository;
	readonly unitOfWork: UnitOfWork;
}

export function createCreateEmailDomainMappingUseCase(
	deps: CreateEmailDomainMappingUseCaseDeps,
): UseCase<CreateEmailDomainMappingCommand, EmailDomainMappingCreated> {
	const {
		emailDomainMappingRepository,
		identityProviderRepository,
		principalRepository,
		clientAccessGrantRepository,
		unitOfWork,
	} = deps;

	return {
		async execute(
			command: CreateEmailDomainMappingCommand,
			context: ExecutionContext,
		): Promise<Result<EmailDomainMappingCreated>> {
			const domainResult = validateRequired(
				command.emailDomain,
				"emailDomain",
				"EMAIL_DOMAIN_REQUIRED",
			);
			if (Result.isFailure(domainResult)) return domainResult;

			const idpResult = validateRequired(
				command.identityProviderId,
				"identityProviderId",
				"IDENTITY_PROVIDER_REQUIRED",
			);
			if (Result.isFailure(idpResult)) return idpResult;

			// Verify IDP exists
			const idp = await identityProviderRepository.findById(
				command.identityProviderId,
			);
			if (!idp) {
				return Result.failure(
					UseCaseError.notFound(
						"IDP_NOT_FOUND",
						"Identity provider not found",
						{
							identityProviderId: command.identityProviderId,
						},
					),
				);
			}

			// Multi-tenant IDPs require a tenant ID
			if (idp.oidcMultiTenant && !command.requiredOidcTenantId?.trim()) {
				return Result.failure(
					UseCaseError.validation(
						"OIDC_TENANT_ID_REQUIRED",
						"Required OIDC Tenant ID must be set for multi-tenant identity providers",
						{ field: "requiredOidcTenantId" },
					),
				);
			}

			// Check for duplicate email domain
			const domainExists =
				await emailDomainMappingRepository.existsByEmailDomain(
					command.emailDomain,
				);
			if (domainExists) {
				return Result.failure(
					UseCaseError.businessRule(
						"DOMAIN_EXISTS",
						"Email domain mapping already exists",
						{
							emailDomain: command.emailDomain,
						},
					),
				);
			}

			const mapping = createEmailDomainMapping({
				emailDomain: command.emailDomain,
				identityProviderId: command.identityProviderId,
				scopeType: command.scopeType,
				primaryClientId: command.primaryClientId ?? null,
				additionalClientIds: command.additionalClientIds ?? [],
				grantedClientIds: command.grantedClientIds ?? [],
				requiredOidcTenantId: command.requiredOidcTenantId ?? null,
				allowedRoleIds: command.allowedRoleIds ?? [],
				syncRolesFromIdp: command.syncRolesFromIdp ?? false,
			});

			const event = new EmailDomainMappingCreated(context, {
				emailDomainMappingId: mapping.id,
				emailDomain: mapping.emailDomain,
				identityProviderId: mapping.identityProviderId,
				scopeType: mapping.scopeType,
				primaryClientId: mapping.primaryClientId,
				additionalClientIds: mapping.additionalClientIds,
				grantedClientIds: mapping.grantedClientIds,
			});

			const result = await unitOfWork.commit(mapping, event, command);

			// Cascade scope changes to existing users on this domain
			if (Result.isSuccess(result)) {
				await cascadeScopeToUsers({
					emailDomain: mapping.emailDomain,
					newScope: mapping.scopeType as PrincipalScope,
					newClientId:
						mapping.scopeType === "CLIENT"
							? (mapping.primaryClientId ?? null)
							: null,
					principalRepository,
					clientAccessGrantRepository,
				});
			}

			return result;
		},
	};
}

/**
 * Cascade scope changes to all existing USER principals with the given email domain.
 * Updates scope/clientId and cleans up client access grants when downgrading to CLIENT.
 */
async function cascadeScopeToUsers(params: {
	emailDomain: string;
	newScope: PrincipalScope;
	newClientId: string | null;
	principalRepository: PrincipalRepository;
	clientAccessGrantRepository: ClientAccessGrantRepository;
}): Promise<void> {
	const {
		emailDomain,
		newScope,
		newClientId,
		principalRepository,
		clientAccessGrantRepository,
	} = params;

	const users = await principalRepository.findUsersByEmailDomain(emailDomain);

	for (const user of users) {
		if (user.scope === newScope && user.clientId === newClientId) {
			continue; // No change needed
		}

		const updated = updatePrincipal(user, {
			scope: newScope,
			clientId: newClientId,
		});
		await principalRepository.update(updated);

		// If downgrading to CLIENT, remove all client access grants
		if (newScope === PrincipalScope.CLIENT && user.scope === PrincipalScope.PARTNER) {
			await clientAccessGrantRepository.deleteByPrincipalId(user.id);
		}
	}
}
