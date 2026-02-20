/**
 * Update Email Domain Mapping Use Case
 */

import type { UseCase } from "@flowcatalyst/application";
import { Result, UseCaseError } from "@flowcatalyst/application";
import type { ExecutionContext, UnitOfWork } from "@flowcatalyst/domain-core";

import type {
	IdentityProviderRepository,
	EmailDomainMappingRepository,
} from "../../../infrastructure/persistence/index.js";
import {
	updateEmailDomainMapping,
	EmailDomainMappingUpdated,
} from "../../../domain/index.js";

import type { UpdateEmailDomainMappingCommand } from "./command.js";

export interface UpdateEmailDomainMappingUseCaseDeps {
	readonly emailDomainMappingRepository: EmailDomainMappingRepository;
	readonly identityProviderRepository: IdentityProviderRepository;
	readonly unitOfWork: UnitOfWork;
}

export function createUpdateEmailDomainMappingUseCase(
	deps: UpdateEmailDomainMappingUseCaseDeps,
): UseCase<UpdateEmailDomainMappingCommand, EmailDomainMappingUpdated> {
	const {
		emailDomainMappingRepository,
		identityProviderRepository,
		unitOfWork,
	} = deps;

	return {
		async execute(
			command: UpdateEmailDomainMappingCommand,
			context: ExecutionContext,
		): Promise<Result<EmailDomainMappingUpdated>> {
			const mapping = await emailDomainMappingRepository.findById(
				command.emailDomainMappingId,
			);
			if (!mapping) {
				return Result.failure(
					UseCaseError.notFound(
						"MAPPING_NOT_FOUND",
						"Email domain mapping not found",
						{
							emailDomainMappingId: command.emailDomainMappingId,
						},
					),
				);
			}

			// Resolve the effective IDP (new one if changing, existing one otherwise)
			let idp;
			if (command.identityProviderId !== undefined) {
				idp = await identityProviderRepository.findById(
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
			} else {
				idp = await identityProviderRepository.findById(
					mapping.identityProviderId,
				);
			}

			// Multi-tenant IDPs require a tenant ID
			if (idp?.oidcMultiTenant) {
				const effectiveTenantId =
					command.requiredOidcTenantId !== undefined
						? command.requiredOidcTenantId
						: mapping.requiredOidcTenantId;
				if (!effectiveTenantId?.trim()) {
					return Result.failure(
						UseCaseError.validation(
							"OIDC_TENANT_ID_REQUIRED",
							"Required OIDC Tenant ID must be set for multi-tenant identity providers",
							{ field: "requiredOidcTenantId" },
						),
					);
				}
			}

			const updated = updateEmailDomainMapping(mapping, {
				...(command.identityProviderId !== undefined
					? { identityProviderId: command.identityProviderId }
					: {}),
				...(command.scopeType !== undefined
					? { scopeType: command.scopeType }
					: {}),
				...(command.primaryClientId !== undefined
					? { primaryClientId: command.primaryClientId }
					: {}),
				...(command.additionalClientIds !== undefined
					? { additionalClientIds: command.additionalClientIds }
					: {}),
				...(command.grantedClientIds !== undefined
					? { grantedClientIds: command.grantedClientIds }
					: {}),
				...(command.requiredOidcTenantId !== undefined
					? { requiredOidcTenantId: command.requiredOidcTenantId }
					: {}),
				...(command.allowedRoleIds !== undefined
					? { allowedRoleIds: command.allowedRoleIds }
					: {}),
				...(command.syncRolesFromIdp !== undefined
					? { syncRolesFromIdp: command.syncRolesFromIdp }
					: {}),
			});

			const event = new EmailDomainMappingUpdated(context, {
				emailDomainMappingId: updated.id,
				emailDomain: updated.emailDomain,
				identityProviderId: updated.identityProviderId,
				scopeType: updated.scopeType,
				primaryClientId: updated.primaryClientId,
				additionalClientIds: updated.additionalClientIds,
				grantedClientIds: updated.grantedClientIds,
			});

			return unitOfWork.commit(updated, event, command);
		},
	};
}
